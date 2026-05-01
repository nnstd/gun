package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler"
	"github.com/nnstd/gun/compiler/jsonmodule"
	"github.com/nnstd/gun/compiler/yamlmodule"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// GunModuleRoot can be set at build time via -ldflags:
//
//	go build -ldflags "-X github.com/nnstd/gun/compiler/runner.GunModuleRoot=$(pwd)" -o gun .
var GunModuleRoot string

// RequireImport represents a require() call found in TS/JS source.
type RequireImport struct {
	Path     string
	Optional bool
}

// TranspileProject transpiles a .ts entry file and all its relative imports
// into outDir, then scaffolds a go.mod so the result is go-buildable.
func TranspileProject(input, outDir, pkg string, verbose bool, optLevel int, opts *compiler.CompileOptions) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	moduleName := "gunrun"
	goFile := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(input), ".ts")+".go")
	if err := TranspileFile(input, goFile, pkg, moduleName, verbose, false, optLevel, opts); err != nil {
		return err
	}

	inputDir := filepath.Dir(input)
	if inputDir == "" {
		inputDir = "."
	}
	if err := TranspileRelativeImports(input, inputDir, outDir, moduleName, verbose, optLevel, opts); err != nil {
		return err
	}

	if err := TranspileNodeModuleImports(input, inputDir, outDir, moduleName, verbose, optLevel, opts); err != nil {
		return err
	}

	return ScaffoldGoMod(outDir, verbose)
}

// GoBuild runs `go mod tidy` then `go build` in dir and writes the binary to binPath.
func GoBuild(dir, binPath string, verbose bool) error {
	// Run go mod tidy to ensure dependencies are consistent
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = dir
	var stderr strings.Builder
	build.Stderr = &stderr
	if verbose {
		fmt.Fprintf(os.Stderr, "building %s\n", dir)
	}
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed:\n%s\ntranspiled source is at %s", stderr.String(), dir)
	}
	return nil
}

// TranspileFile compiles a single .ts file and writes Go output to outputPath.
// If outputPath is empty, the result is written to stdout.
func TranspileFile(inputPath, outputPath, pkgName, moduleName string, verbose, samePackageImports bool, optLevel int, opts *compiler.CompileOptions) error {
	source, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "compiling %s\n", inputPath)
	}

	if moduleName == "" {
		moduleName = DetectModuleName(inputPath)
	}

	result, err := compiler.CompileWithOptLevelAndPathOptions(source, pkgName, moduleName, inputPath, samePackageImports, optLevel, opts)
	if err != nil {
		return fmt.Errorf("compile %s: %w", inputPath, err)
	}

	if outputPath == "" {
		_, err = os.Stdout.Write(result)
		return err
	}

	return os.WriteFile(outputPath, result, 0644)
}

// TranspileDir walks a directory and transpiles all .ts files into Go.
func TranspileDir(dirPath, outputDir, pkgName string, verbose bool, optLevel int) error {
	if outputDir == "" {
		outputDir = dirPath
	}

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".ts") {
			return nil
		}
		if strings.HasSuffix(path, ".d.ts") {
			return nil
		}

		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		outPath := filepath.Join(outputDir, strings.TrimSuffix(rel, ".ts")+".go")

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}

		return TranspileFile(path, outPath, pkgName, "", verbose, false, optLevel, nil)
	})
}

// ScaffoldGoMod creates a go.mod in tmpDir so that github.com/nnstd/gun/runtime/*
// imports resolve during go build.
//
// In development (local source tree found): uses a replace directive for instant builds.
// For distributed binaries (no local source): fetches the module from GitHub.
func ScaffoldGoMod(tmpDir string, verbose bool) error {
	const modPath = "github.com/nnstd/gun"

	gunRoot, localFound := FindGunModuleRoot()

	if localFound {
		if verbose {
			fmt.Fprintf(os.Stderr, "using local gun module at %s\n", gunRoot)
		}
		gomod := fmt.Sprintf("module gunrun\n\ngo 1.24.0\n\nrequire %s v0.0.0\n\nreplace %s => %s\n", modPath, modPath, gunRoot)
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0644); err != nil {
			return err
		}
		// Copy go.sum so dependency hashes are available
		if data, err := os.ReadFile(filepath.Join(gunRoot, "go.sum")); err == nil {
			os.WriteFile(filepath.Join(tmpDir, "go.sum"), data, 0644)
		}
		return nil
	}

	// No local source — fetch from GitHub
	if verbose {
		fmt.Fprintf(os.Stderr, "fetching %s from module proxy\n", modPath)
	}
	gomod := "module gunrun\n\ngo 1.24.0\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0644); err != nil {
		return err
	}

	get := exec.Command("go", "get", modPath+"@latest")
	get.Dir = tmpDir
	get.Stderr = os.Stderr
	if err := get.Run(); err != nil {
		return fmt.Errorf("go get %s: %w", modPath, err)
	}
	return nil
}

// FindGunModuleRoot locates the gun source tree so runtime imports can be
// resolved via a replace directive. Returns ("", false) when no local source
// is available — the caller should fall back to fetching from GitHub.
//
// Tries in order:
//  1. Build-time embedded path (GunModuleRoot, set via ldflags)
//  2. Walk up from the executable (works for `go build . && ./gun`)
//  3. Walk up from CWD (works for `go run . run ...` during development)
func FindGunModuleRoot() (string, bool) {
	if GunModuleRoot != "" {
		return GunModuleRoot, true
	}

	// Strategy 2: walk up from executable
	if exe, err := os.Executable(); err == nil {
		resolved := exe
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			resolved = r
		}
		if dir, ok := FindModuleDir(filepath.Dir(resolved)); ok {
			return dir, true
		}
	}

	// Strategy 3: walk up from CWD
	if wd, err := os.Getwd(); err == nil {
		if dir, ok := FindModuleDir(wd); ok {
			return dir, true
		}
	}

	return "", false
}

// FindModuleDir walks up from start looking for a go.mod containing
// "module github.com/nnstd/gun".
func FindModuleDir(start string) (string, bool) {
	dir, _ := filepath.Abs(start)
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if strings.TrimSpace(line) == "module github.com/nnstd/gun" {
					return dir, true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// FindRelativeImports scans TS source for relative import paths (./foo or ../foo).
func FindRelativeImports(source []byte) []string {
	var imports []string
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		// Match: from "./..." or from '../...'
		prefix, rest, ok := strings.Cut(line, "from ")
		if !ok {
			continue
		}
		// Verify this is an import statement, not the word "from" in a string
		prefix = strings.TrimSpace(prefix)
		if !strings.HasSuffix(prefix, "import") && !strings.Contains(prefix, " import ") && prefix != "import" && !strings.HasPrefix(line, "import ") && !strings.Contains(prefix, "export") && !strings.HasPrefix(line, "export ") {
			continue
		}
		rest = strings.TrimSpace(rest)
		if len(rest) < 3 {
			continue
		}
		quote := rest[0]
		if quote != '\'' && quote != '"' {
			continue
		}
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			continue
		}
		modPath := rest[1 : end+1]
		if strings.HasPrefix(modPath, ".") {
			imports = append(imports, modPath)
		}
	}
	return imports
}

// ResolveImportFile resolves a relative import path like ./foo to an actual .ts/.js/.json/.yaml file.
// Tries extensions in Node/Bun resolution order.
func ResolveImportFile(importPath, fromDir string) (string, error) {
	clean := importPath
	clean = strings.TrimSuffix(clean, ".ts")
	clean = strings.TrimSuffix(clean, ".js")
	clean = strings.TrimSuffix(clean, ".mjs")
	clean = strings.TrimSuffix(clean, ".json")
	clean = strings.TrimSuffix(clean, ".yaml")
	clean = strings.TrimSuffix(clean, ".yml")

	base := filepath.Join(fromDir, clean)

	// Try file extensions in order
	for _, ext := range []string{".ts", ".js", ".mjs", ".json", ".yaml", ".yml"} {
		if _, err := os.Stat(base + ext); err == nil {
			return base + ext, nil
		}
	}

	// Try index files in directory
	for _, idx := range []string{"index.ts", "index.js", "index.json", "index.yaml", "index.yml"} {
		candidate := filepath.Join(base, idx)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("cannot resolve import %q from %s", importPath, fromDir)
}

// TranspileRelativeImports recursively discovers and transpiles relative imports
// from a TS entry file into the correct subdirectories of tmpDir.
func TranspileRelativeImports(entryFile, inputDir, tmpDir, moduleName string, verbose bool, optLevel int, opts *compiler.CompileOptions) error {
	absInputDir, err := filepath.Abs(inputDir)
	if err != nil {
		return err
	}
	visited := map[string]bool{}
	return walkImports(entryFile, absInputDir, absInputDir, tmpDir, moduleName, verbose, visited, optLevel, opts)
}

func walkImports(tsFile, inputDir, baseDir, tmpDir, moduleName string, verbose bool, visited map[string]bool, optLevel int, opts *compiler.CompileOptions) error {
	absFile, _ := filepath.Abs(tsFile)
	if visited[absFile] {
		return nil
	}
	visited[absFile] = true

	source, err := os.ReadFile(tsFile)
	if err != nil {
		return err
	}
	if IsDataModuleFile(tsFile) {
		return nil
	}

	fileDir := filepath.Dir(tsFile)
	imports := FindRelativeImports(source)
	optionalRequires := FindOptionalRequireImports(source)
	for _, imp := range FindRequireImports(source) {
		if strings.HasPrefix(imp, ".") {
			imports = append(imports, imp)
		}
	}

	for _, imp := range imports {
		resolved, err := ResolveImportFile(imp, fileDir)
		if err != nil {
			if optionalRequires[imp] {
				continue
			}
			return err
		}

		// Determine the relative path from baseDir to the resolved file's directory
		absResolved, _ := filepath.Abs(resolved)
		resolvedDir := filepath.Dir(absResolved)
		relDir, err := filepath.Rel(filepath.Join(baseDir), resolvedDir)
		if err != nil {
			return err
		}

		// The Go package name is the last segment of the relative path
		pkgName := filepath.Base(relDir)
		if pkgName == "." {
			// File is in the same directory — use the import name as subdir
			clean := strings.TrimPrefix(imp, "./")
			clean = strings.TrimPrefix(clean, "../")
			clean = strings.TrimSuffix(clean, ".ts")
			clean = TrimModuleExt(clean)
			pkgName = filepath.Base(clean)
			relDir = clean
		}

		outDir := filepath.Join(tmpDir, relDir)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return err
		}

		outFile := filepath.Join(outDir, filepath.Base(TrimModuleExt(resolved))+".go")
		if IsDataModuleFile(resolved) {
			if err := TranspileDataModuleFile(resolved, outFile, pkgName); err != nil {
				return err
			}
		} else if err := TranspileFile(resolved, outFile, pkgName, moduleName, verbose, false, optLevel, opts); err != nil {
			return err
		}

		// Recurse into this file's imports
		if err := walkImports(resolved, inputDir, baseDir, tmpDir, moduleName, verbose, visited, optLevel, opts); err != nil {
			return err
		}
	}
	return nil
}

// FindRequireImports scans TS/JS source for static require("...") calls.
// Returned specifiers may be relative (./..., ../...) or bare; callers are
// responsible for classification.
func FindRequireImports(source []byte) []string {
	specs, ok := scanRequireImports(source)
	if !ok {
		return findRequireImportsText(source)
	}
	var imports []string
	seen := map[string]bool{}
	for _, spec := range specs {
		if !seen[spec.Path] {
			seen[spec.Path] = true
			imports = append(imports, spec.Path)
		}
	}
	return imports
}

// FindOptionalRequireImports returns the set of require() paths that appear
// inside try/catch blocks (i.e. optional requires).
func FindOptionalRequireImports(source []byte) map[string]bool {
	optional := map[string]bool{}
	specs, ok := scanRequireImports(source)
	if !ok {
		return optional
	}
	for _, spec := range specs {
		if spec.Optional {
			optional[spec.Path] = true
		}
	}
	return optional
}

func scanRequireImports(source []byte) ([]RequireImport, bool) {
	parser := sitter.NewParser()
	defer parser.Close()

	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, false
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, false
	}

	var imports []RequireImport
	var walk func(*sitter.Node, bool)
	walk = func(node *sitter.Node, inCatchableTry bool) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "function_declaration", "function_expression", "arrow_function", "method_definition", "generator_function_declaration", "generator_function":
			inCatchableTry = false
		}
		if path, ok := requireCallStringArg(node, source); ok {
			imports = append(imports, RequireImport{Path: path, Optional: inCatchableTry})
		}

		childOptional := inCatchableTry
		if node.Kind() == "try_statement" && node.ChildByFieldName("handler") != nil {
			childOptional = true
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			walk(node.NamedChild(i), childOptional)
		}
	}
	walk(root, false)
	return imports, true
}

func requireCallStringArg(callNode *sitter.Node, source []byte) (string, bool) {
	if callNode == nil || callNode.Kind() != "call_expression" {
		return "", false
	}
	fnNode := callNode.ChildByFieldName("function")
	argsNode := callNode.ChildByFieldName("arguments")
	if fnNode == nil || argsNode == nil {
		return "", false
	}
	if fnNode.Kind() != "identifier" || fnNode.Utf8Text(source) != "require" {
		return "", false
	}
	if argsNode.NamedChildCount() != 1 {
		return "", false
	}
	arg := argsNode.NamedChild(0)
	if arg.Kind() != "string" {
		return "", false
	}
	raw := arg.Utf8Text(source)
	if len(raw) < 2 {
		return "", false
	}
	quote := raw[0]
	if quote != '\'' && quote != '"' || raw[len(raw)-1] != quote {
		return "", false
	}
	return strings.TrimPrefix(raw[1:len(raw)-1], "node:"), true
}

func findRequireImportsText(source []byte) []string {
	var imports []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		rest := line
		for {
			idx := strings.Index(rest, "require(")
			if idx < 0 {
				break
			}
			// Ensure require is a standalone identifier (not .require or $require).
			if idx > 0 {
				prev := rest[idx-1]
				if prev == '.' || prev == '_' || prev == '$' || (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
					rest = rest[idx+len("require("):]
					continue
				}
			}
			after := strings.TrimSpace(rest[idx+len("require("):])
			if len(after) < 2 {
				break
			}
			quote := after[0]
			if quote != '\'' && quote != '"' {
				rest = rest[idx+len("require("):]
				continue
			}
			end := strings.IndexByte(after[1:], quote)
			if end < 0 {
				break
			}
			afterLiteral := strings.TrimSpace(after[end+2:])
			if !strings.HasPrefix(afterLiteral, ")") {
				rest = after[end+2:]
				continue
			}
			modPath := after[1 : end+1]
			modPath = strings.TrimPrefix(modPath, "node:")
			if !seen[modPath] {
				seen[modPath] = true
				imports = append(imports, modPath)
			}
			rest = after[end+2:]
		}
	}
	return imports
}

// TrimModuleExt strips TS/JS/data file extensions from a path.
func TrimModuleExt(path string) string {
	path = strings.TrimSuffix(path, ".ts")
	path = strings.TrimSuffix(path, ".js")
	path = strings.TrimSuffix(path, ".mjs")
	path = strings.TrimSuffix(path, ".json")
	path = strings.TrimSuffix(path, ".yaml")
	path = strings.TrimSuffix(path, ".yml")
	return path
}

// IsJSONFile reports whether path has a .json extension.
func IsJSONFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
}

// IsYAMLFile reports whether path has a .yaml or .yml extension.
func IsYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml")
}

// IsDataModuleFile reports whether path is a JSON or YAML file.
func IsDataModuleFile(path string) bool {
	return IsJSONFile(path) || IsYAMLFile(path)
}

// TranspileJSONModuleFile compiles a JSON file to Go source.
func TranspileJSONModuleFile(inputPath, outputPath, pkgName string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	if _, err := jsonmodule.Parse(data); err != nil {
		return fmt.Errorf("parse json %s: %w", inputPath, err)
	}
	source, err := jsonmodule.Compile(data, pkgName, "Default", nil)
	if err != nil {
		return fmt.Errorf("compile json %s: %w", inputPath, err)
	}
	return os.WriteFile(outputPath, source, 0644)
}

// TranspileYAMLModuleFile compiles a YAML file to Go source.
func TranspileYAMLModuleFile(inputPath, outputPath, pkgName string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	if _, err := yamlmodule.Parse(data); err != nil {
		return fmt.Errorf("parse yaml %s: %w", inputPath, err)
	}
	source, err := yamlmodule.Compile(data, pkgName, "Default", nil)
	if err != nil {
		return fmt.Errorf("compile yaml %s: %w", inputPath, err)
	}
	return os.WriteFile(outputPath, source, 0644)
}

// TranspileDataModuleFile compiles a JSON or YAML file to Go source.
func TranspileDataModuleFile(inputPath, outputPath, pkgName string) error {
	if IsYAMLFile(inputPath) {
		return TranspileYAMLModuleFile(inputPath, outputPath, pkgName)
	}
	return TranspileJSONModuleFile(inputPath, outputPath, pkgName)
}

// FindNodeModuleImports scans TS/JS source for non-relative, non-known imports.
func FindNodeModuleImports(source []byte) []string {
	var imports []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		prefix, rest, ok := strings.Cut(line, "from ")
		if !ok {
			continue
		}
		// Verify this is an import statement, not the word "from" in a string
		prefix = strings.TrimSpace(prefix)
		if !strings.HasSuffix(prefix, "import") && !strings.Contains(prefix, " import ") && prefix != "import" && !strings.HasPrefix(line, "import ") && !strings.Contains(prefix, "export") && !strings.HasPrefix(line, "export ") {
			continue
		}
		rest = strings.TrimSpace(rest)
		if len(rest) < 3 {
			continue
		}
		quote := rest[0]
		if quote != '\'' && quote != '"' {
			continue
		}
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			continue
		}
		modPath := rest[1 : end+1]
		// Skip relative imports
		if strings.HasPrefix(modPath, ".") {
			continue
		}
		// Strip node: prefix
		modPath = strings.TrimPrefix(modPath, "node:")
		// Skip known/polyfilled modules
		if compiler.IsKnownModule(modPath) {
			continue
		}
		if !seen[modPath] {
			seen[modPath] = true
			imports = append(imports, modPath)
		}
	}
	return imports
}

// ResolveNodeModule walks up from fromDir looking for node_modules/<pkgName>/.
func ResolveNodeModule(pkgName, fromDir string) (string, error) {
	dir, _ := filepath.Abs(fromDir)
	for {
		candidate := filepath.Join(dir, "node_modules", pkgName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find node_modules/%s from %s", pkgName, fromDir)
		}
		dir = parent
	}
}

// GetNodeModuleEntry reads package.json and determines the entry point file.
func GetNodeModuleEntry(pkgDir string) (string, error) {
	pjPath := filepath.Join(pkgDir, "package.json")
	data, err := os.ReadFile(pjPath)
	if err != nil {
		return "", fmt.Errorf("read package.json: %w", err)
	}

	var pj struct {
		Exports json.RawMessage `json:"exports"`
		Module  string          `json:"module"`
		Main    string          `json:"main"`
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return "", fmt.Errorf("parse package.json: %w", err)
	}

	// Try exports field
	if len(pj.Exports) > 0 {
		entry := resolveExportsField(pj.Exports)
		if entry != "" {
			return resolvePackageEntryPath(pkgDir, entry)
		}
	}

	// Try module field
	if pj.Module != "" {
		return resolvePackageEntryPath(pkgDir, pj.Module)
	}

	// Try main field
	if pj.Main != "" {
		return resolvePackageEntryPath(pkgDir, pj.Main)
	}

	// Fallback to index files
	for _, name := range []string{"index.ts", "index.js"} {
		candidate := filepath.Join(pkgDir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("cannot determine entry point for %s", pkgDir)
}

func resolvePackageEntryPath(pkgDir, entry string) (string, error) {
	if resolved, err := ResolveImportFile(entry, pkgDir); err == nil {
		return resolved, nil
	}
	return "", fmt.Errorf("cannot resolve package entry %q in %s", entry, pkgDir)
}

// resolveExportsField extracts an entry path from a package.json "exports" field.
func resolveExportsField(raw json.RawMessage) string {
	// Case 1: exports is a string — "exports": "./index.js"
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	// Case 2: exports is an object — check "." key, then "import"/"default"
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}

	// Check "." entry (the main export)
	dot, ok := obj["."]
	if ok {
		return resolveExportValue(dot)
	}

	// No "." key — the object itself might be conditions
	return pickCondition(obj)
}

func pickCondition(conds map[string]json.RawMessage) string {
	for _, key := range []string{"import", "default", "require"} {
		raw, ok := conds[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		// Nested conditions object (e.g. "import": {"types": "...", "default": "..."})
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil {
			if def, ok := nested["default"]; ok {
				if json.Unmarshal(def, &s) == nil {
					return s
				}
			}
		}
	}
	return ""
}

// resolveExportValue resolves an export value that can be a string, conditions object, or array of those.
func resolveExportValue(raw json.RawMessage) string {
	// String
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Conditions object
	var conds map[string]json.RawMessage
	if json.Unmarshal(raw, &conds) == nil {
		if entry := pickCondition(conds); entry != "" {
			return entry
		}
	}
	// Array — try each element in order
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		for _, elem := range arr {
			if entry := resolveExportValue(elem); entry != "" {
				return entry
			}
		}
	}
	return ""
}

// ResolveSubpathEntry handles subpath exports like "yargs/helpers" by looking up
// the subpath in the root package's exports field.
func ResolveSubpathEntry(pkgName, fromDir string) (string, error) {
	root, subpath := SplitPkgSubpath(pkgName)
	if subpath == "" {
		return "", fmt.Errorf("cannot determine entry point for %s", pkgName)
	}

	rootDir, err := ResolveNodeModule(root, fromDir)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(rootDir, "package.json"))
	if err != nil {
		return "", fmt.Errorf("cannot determine entry point for %s", pkgName)
	}

	var pj struct {
		Exports json.RawMessage `json:"exports"`
	}
	if err := json.Unmarshal(data, &pj); err != nil || len(pj.Exports) == 0 {
		return "", fmt.Errorf("cannot determine entry point for %s", pkgName)
	}

	var obj map[string]json.RawMessage
	if json.Unmarshal(pj.Exports, &obj) != nil {
		return "", fmt.Errorf("cannot determine entry point for %s", pkgName)
	}

	key := "./" + subpath
	raw, ok := obj[key]
	if !ok {
		return "", fmt.Errorf("no subpath export %s in %s", key, root)
	}

	if entry := resolveExportValue(raw); entry != "" {
		return resolvePackageEntryPath(rootDir, entry)
	}

	return "", fmt.Errorf("cannot resolve subpath export %s in %s", key, root)
}

// SplitPkgSubpath splits "pkg/sub" into ("pkg", "sub") and "@scope/pkg/sub" into ("@scope/pkg", "sub").
func SplitPkgSubpath(pkgName string) (string, string) {
	if strings.HasPrefix(pkgName, "@") {
		parts := strings.SplitN(pkgName, "/", 3)
		if len(parts) < 3 {
			return pkgName, ""
		}
		return parts[0] + "/" + parts[1], parts[2]
	}
	parts := strings.SplitN(pkgName, "/", 2)
	if len(parts) < 2 {
		return pkgName, ""
	}
	return parts[0], parts[1]
}

// TranspileNodeModuleImports discovers and transpiles node_modules dependencies
// from the entry file and all its relative imports.
func TranspileNodeModuleImports(entryFile, inputDir, tmpDir, moduleName string, verbose bool, optLevel int, opts *compiler.CompileOptions) error {
	absInputDir, _ := filepath.Abs(inputDir)

	// Collect all source files (entry + relative imports) to scan
	sourceFiles := collectAllSourceFiles(entryFile)

	visited := map[string]bool{}
	return ProcessNodeModuleImports(sourceFiles, absInputDir, tmpDir, moduleName, verbose, visited, optLevel, opts)
}

// CollectAllSourceFiles returns the entry file plus all transitively-imported relative files.
func CollectAllSourceFiles(entryFile string) []string {
	return collectAllSourceFiles(entryFile)
}

func collectAllSourceFiles(entryFile string) []string {
	var files []string
	seen := map[string]bool{}
	var walk func(string)
	walk = func(tsFile string) {
		abs, _ := filepath.Abs(tsFile)
		if seen[abs] {
			return
		}
		seen[abs] = true
		files = append(files, abs)
		if IsDataModuleFile(tsFile) {
			return
		}

		source, err := os.ReadFile(tsFile)
		if err != nil {
			return
		}
		optionalRequires := FindOptionalRequireImports(source)
		for _, imp := range FindRelativeImports(source) {
			resolved, err := ResolveImportFile(imp, filepath.Dir(tsFile))
			if err == nil {
				walk(resolved)
			}
		}
		for _, imp := range FindRequireImports(source) {
			if !strings.HasPrefix(imp, ".") {
				continue
			}
			resolved, err := ResolveImportFile(imp, filepath.Dir(tsFile))
			if err == nil {
				walk(resolved)
			} else if !optionalRequires[imp] {
				return
			}
		}
	}
	walk(entryFile)
	return files
}

// ProcessNodeModuleImports discovers and transpiles node_modules dependencies
// from the given source files. It recurses to handle transitive dependencies.
func ProcessNodeModuleImports(sourceFiles []string, inputDir, tmpDir, moduleName string, verbose bool, visited map[string]bool, optLevel int, opts *compiler.CompileOptions) error {
	var newSourceFiles []string

	for _, srcFile := range sourceFiles {
		if IsDataModuleFile(srcFile) {
			continue
		}
		source, err := os.ReadFile(srcFile)
		if err != nil {
			continue
		}
		optionalRequires := FindOptionalRequireImports(source)
		nodeImports := FindNodeModuleImports(source)
		for _, imp := range FindRequireImports(source) {
			if strings.HasPrefix(imp, ".") {
				continue
			}
			if compiler.IsKnownModule(imp) {
				continue
			}
			nodeImports = append(nodeImports, imp)
		}
		for _, pkgName := range nodeImports {
			if visited[pkgName] {
				continue
			}

			pkgDir, err := ResolveNodeModule(pkgName, filepath.Dir(srcFile))
			if err != nil {
				if optionalRequires[pkgName] {
					continue
				}
				return err
			}
			visited[pkgName] = true
			entryPath, err := GetNodeModuleEntry(pkgDir)
			if err != nil {
				// Try subpath export resolution (e.g. "yargs/helpers" → yargs exports["./helpers"])
				entryPath, err = ResolveSubpathEntry(pkgName, filepath.Dir(srcFile))
				if err != nil {
					return err
				}
			}

			sanitized := compiler.SanitizeGoPkgName(pkgName)
			outDir := filepath.Join(tmpDir, sanitized)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return err
			}

			if verbose {
				fmt.Fprintf(os.Stderr, "transpiling node_module %s → %s\n", pkgName, sanitized)
			}

			// Discover all files in this package and compile together
			allPkgFiles, err := TranspileNodeModuleAsPackage(entryPath, outDir, moduleName, sanitized, verbose, optLevel, opts)
			if err != nil {
				return fmt.Errorf("transpile node_module %s: %w", pkgName, err)
			}
			newSourceFiles = append(newSourceFiles, allPkgFiles...)
		}
	}

	// Recursively process any node_module imports found in the newly transpiled packages
	if len(newSourceFiles) > 0 {
		return ProcessNodeModuleImports(newSourceFiles, inputDir, tmpDir, moduleName, verbose, visited, optLevel, opts)
	}
	return nil
}

// TranspileNodeModuleAsPackage discovers all files in an npm package and compiles
// them together using CompilePackage, so cross-file exports are shared.
func TranspileNodeModuleAsPackage(entryPath, outDir, moduleName, pkgName string, verbose bool, optLevel int, opts *compiler.CompileOptions) ([]string, error) {
	// Phase 1: Discover all files in the package
	files := map[string][]byte{}
	allPaths := []string{}

	absEntry, _ := filepath.Abs(entryPath)
	source, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", entryPath, err)
	}
	files[absEntry] = source
	allPaths = append(allPaths, absEntry)

	visited := map[string]bool{absEntry: true}
	var discover func(string) error
	discover = func(tsFile string) error {
		if IsDataModuleFile(tsFile) {
			return nil
		}
		src, err := os.ReadFile(tsFile)
		if err != nil {
			return err
		}
		optionalRequires := FindOptionalRequireImports(src)
		relImports := FindRelativeImports(src)
		for _, imp := range FindRequireImports(src) {
			if strings.HasPrefix(imp, ".") {
				relImports = append(relImports, imp)
			}
		}
		for _, imp := range relImports {
			resolved, err := ResolveImportFile(imp, filepath.Dir(tsFile))
			if err != nil {
				if optionalRequires[imp] {
					continue
				}
				return err
			}
			absResolved, _ := filepath.Abs(resolved)
			if visited[absResolved] {
				continue
			}
			visited[absResolved] = true
			data, err := os.ReadFile(resolved)
			if err != nil {
				return err
			}
			files[absResolved] = data
			allPaths = append(allPaths, absResolved)
			if err := discover(resolved); err != nil {
				return err
			}
		}
		return nil
	}
	if err := discover(entryPath); err != nil {
		return nil, err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  package %s: %d files\n", pkgName, len(files))
	}

	// Phase 2: Compile all files together
	results, err := compiler.CompilePackageWithOptLevelOptions(files, pkgName, moduleName, absEntry, optLevel, opts)
	if err != nil {
		return nil, err
	}

	// Phase 3: Write results
	// Use relative path from package dir for unique filenames when basenames collide.
	pkgDir := filepath.Dir(absEntry)
	for absPath, output := range results {
		rel, err := filepath.Rel(pkgDir, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		// Flatten relative path: build/lib/index.js → build-lib-index.go
		// Strip leading ".." components (Go ignores files starting with ".")
		goName := strings.TrimSuffix(rel, filepath.Ext(rel))
		goName = strings.ReplaceAll(goName, string(filepath.Separator), "-")
		goName = strings.TrimLeft(goName, ".-")
		if goName == "" {
			goName = "file"
		}
		goName += ".go"
		outFile := filepath.Join(outDir, goName)
		if verbose {
			fmt.Fprintf(os.Stderr, "  writing %s\n", outFile)
		}
		if err := os.WriteFile(outFile, output, 0644); err != nil {
			return nil, err
		}
	}

	return allPaths, nil
}

// DetectModuleName walks up from the input file to find a go.mod and returns
// the module name. Returns empty string if not found.
func DetectModuleName(inputPath string) string {
	dir, _ := filepath.Abs(filepath.Dir(inputPath))
	for {
		gomod := filepath.Join(dir, "go.mod")
		if f, err := os.Open(gomod); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if modulePath, ok := strings.CutPrefix(line, "module "); ok {
					return strings.TrimSpace(modulePath)
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
