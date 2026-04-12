package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler"

	"github.com/alecthomas/kong"
)

var cli struct {
	Transpile TranspileCmd `cmd:"" default:"withargs" help:"Transpile TypeScript to Go source."`
	Build     BuildCmd     `cmd:"" help:"Transpile and compile TypeScript to a binary."`
	Run       RunCmd       `cmd:"" help:"Transpile, build, and run a TypeScript file."`
}

type TranspileCmd struct {
	Input    string `arg:"" help:"Input .ts file or directory."`
	Output   string `short:"o" help:"Output directory."`
	Pkg      string `short:"p" default:"main" help:"Go package name."`
	Verbose  bool   `short:"v" help:"Verbose output."`
	AST      bool   `help:"Print the tree-sitter AST instead of transpiling."`
	OptLevel int    `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
}

type BuildCmd struct {
	Input    string `arg:"" help:"Input .ts file or directory."`
	Output   string `short:"o" help:"Output binary path." default:""`
	Pkg      string `short:"p" default:"main" help:"Go package name."`
	Verbose  bool   `short:"v" help:"Verbose output."`
	OptLevel int    `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
}

type RunCmd struct {
	Input    string   `arg:"" help:"Input .ts file."`
	Pkg      string   `short:"p" default:"main" help:"Go package name."`
	Verbose  bool     `short:"v" help:"Verbose output."`
	OptLevel int      `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
	Args     []string `arg:"" optional:"" passthrough:"" help:"Arguments to pass to the compiled program."`
}

func (cmd *TranspileCmd) Run() error {
	if cmd.AST {
		source, err := os.ReadFile(cmd.Input)
		if err != nil {
			return err
		}
		out, err := compiler.DumpAST(source)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	}

	info, err := os.Stat(cmd.Input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return transpileDir(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose, cmd.OptLevel)
	}

	// No -o: just print to stdout
	if cmd.Output == "" {
		return transpileFile(cmd.Input, "", cmd.Pkg, "", cmd.Verbose, false, cmd.OptLevel)
	}

	return transpileProject(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose, cmd.OptLevel)
}

func (cmd *BuildCmd) Run() error {
	info, err := os.Stat(cmd.Input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("build command requires a single .ts file, not a directory")
	}

	tmpDir, err := os.MkdirTemp("", "gun-build-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := transpileProject(cmd.Input, tmpDir, cmd.Pkg, cmd.Verbose, cmd.OptLevel); err != nil {
		return err
	}

	binPath := cmd.Output
	if binPath == "" {
		binPath = strings.TrimSuffix(filepath.Base(cmd.Input), ".ts")
	}
	binPath, _ = filepath.Abs(binPath)

	return goBuild(tmpDir, binPath, cmd.Verbose)
}

func (cmd *RunCmd) Run() error {
	info, err := os.Stat(cmd.Input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("run command requires a single .ts file, not a directory")
	}

	tmpDir, err := os.MkdirTemp("", "gun-run-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := transpileProject(cmd.Input, tmpDir, cmd.Pkg, cmd.Verbose, cmd.OptLevel); err != nil {
		return err
	}

	binName := strings.TrimSuffix(filepath.Base(cmd.Input), ".ts")
	binPath := filepath.Join(tmpDir, binName)
	if err := goBuild(tmpDir, binPath, cmd.Verbose); err != nil {
		return err
	}

	absInput, _ := filepath.Abs(cmd.Input)
	args := append([]string(nil), cmd.Args...)
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	run := exec.Command(binPath, args...)
	run.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+absInput)
	run.Stdin = os.Stdin
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err := run.Run(); err != nil {
		// Propagate the child's exit code silently — it already printed its own errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// transpileProject transpiles a .ts entry file and all its relative imports
// into outDir, then scaffolds a go.mod so the result is go-buildable.
func transpileProject(input, outDir, pkg string, verbose bool, optLevel int) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	moduleName := "gunrun"
	goFile := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(input), ".ts")+".go")
	if err := transpileFile(input, goFile, pkg, moduleName, verbose, false, optLevel); err != nil {
		return err
	}

	inputDir := filepath.Dir(input)
	if inputDir == "" {
		inputDir = "."
	}
	if err := transpileRelativeImports(input, inputDir, outDir, moduleName, verbose, optLevel); err != nil {
		return err
	}

	if err := transpileNodeModuleImports(input, inputDir, outDir, moduleName, verbose, optLevel); err != nil {
		return err
	}

	return scaffoldGoMod(outDir, verbose)
}

// goBuild runs `go mod tidy` then `go build` in dir and writes the binary to binPath.
func goBuild(dir, binPath string, verbose bool) error {
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

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("gun"),
		kong.Description("TypeScript to Go transpiler."),
		kong.UsageOnError(),
	)
	if err := ctx.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func transpileFile(inputPath, outputPath, pkgName, moduleName string, verbose, samePackageImports bool, optLevel int) error {
	source, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "compiling %s\n", inputPath)
	}

	if moduleName == "" {
		moduleName = detectModuleName(inputPath)
	}

	result, err := compiler.CompileWithOptLevelAndPath(source, pkgName, moduleName, inputPath, samePackageImports, optLevel)
	if err != nil {
		return fmt.Errorf("compile %s: %w", inputPath, err)
	}

	if outputPath == "" {
		_, err = os.Stdout.Write(result)
		return err
	}

	return os.WriteFile(outputPath, result, 0644)
}

func transpileDir(dirPath, outputDir, pkgName string, verbose bool, optLevel int) error {
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

		return transpileFile(path, outPath, pkgName, "", verbose, false, optLevel)
	})
}

// scaffoldGoMod creates a go.mod in tmpDir so that github.com/nnstd/gun/runtime/*
// imports resolve during go build.
//
// In development (local source tree found): uses a replace directive for instant builds.
// For distributed binaries (no local source): fetches the module from GitHub.
func scaffoldGoMod(tmpDir string, verbose bool) error {
	const modPath = "github.com/nnstd/gun"

	gunRoot, localFound := findGunModuleRoot()

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

// gunModuleRoot can be set at build time via -ldflags:
//
//	go build -ldflags "-X main.gunModuleRoot=$(pwd)" -o gun .
var gunModuleRoot string

// findGunModuleRoot locates the gun source tree so runtime imports can be
// resolved via a replace directive. Returns ("", false) when no local source
// is available — the caller should fall back to fetching from GitHub.
//
// Tries in order:
//  1. Build-time embedded path (gunModuleRoot, set via ldflags)
//  2. Walk up from the executable (works for `go build . && ./gun`)
//  3. Walk up from CWD (works for `go run . run ...` during development)
func findGunModuleRoot() (string, bool) {
	if gunModuleRoot != "" {
		return gunModuleRoot, true
	}

	// Strategy 2: walk up from executable
	if exe, err := os.Executable(); err == nil {
		resolved := exe
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			resolved = r
		}
		if dir, ok := findModuleDir(filepath.Dir(resolved)); ok {
			return dir, true
		}
	}

	// Strategy 3: walk up from CWD
	if wd, err := os.Getwd(); err == nil {
		if dir, ok := findModuleDir(wd); ok {
			return dir, true
		}
	}

	return "", false
}

// findModuleDir walks up from start looking for a go.mod containing
// "module github.com/nnstd/gun".
func findModuleDir(start string) (string, bool) {
	dir, _ := filepath.Abs(start)
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
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

// findRelativeImports scans TS source for relative import paths (./foo or ../foo).
func findRelativeImports(source []byte) []string {
	var imports []string
	lines := strings.Split(string(source), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		// Match: from "./..." or from '../...'
		idx := strings.Index(line, "from ")
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+5:])
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

// resolveImportFile resolves a relative import path like ./foo to an actual .ts/.js file.
// Tries extensions in Node/Bun resolution order.
func resolveImportFile(importPath, fromDir string) (string, error) {
	clean := importPath
	clean = strings.TrimSuffix(clean, ".ts")
	clean = strings.TrimSuffix(clean, ".js")
	clean = strings.TrimSuffix(clean, ".mjs")

	base := filepath.Join(fromDir, clean)

	// Try file extensions in order
	for _, ext := range []string{".ts", ".js", ".mjs"} {
		if _, err := os.Stat(base + ext); err == nil {
			return base + ext, nil
		}
	}

	// Try index files in directory
	for _, idx := range []string{"index.ts", "index.js"} {
		candidate := filepath.Join(base, idx)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("cannot resolve import %q from %s", importPath, fromDir)
}

// transpileRelativeImports recursively discovers and transpiles relative imports
// from a TS entry file into the correct subdirectories of tmpDir.
func transpileRelativeImports(entryFile, inputDir, tmpDir, moduleName string, verbose bool, optLevel int) error {
	absInputDir, err := filepath.Abs(inputDir)
	if err != nil {
		return err
	}
	visited := map[string]bool{}
	return walkImports(entryFile, absInputDir, absInputDir, tmpDir, moduleName, verbose, visited, optLevel)
}

func walkImports(tsFile, inputDir, baseDir, tmpDir, moduleName string, verbose bool, visited map[string]bool, optLevel int) error {
	absFile, _ := filepath.Abs(tsFile)
	if visited[absFile] {
		return nil
	}
	visited[absFile] = true

	source, err := os.ReadFile(tsFile)
	if err != nil {
		return err
	}

	fileDir := filepath.Dir(tsFile)
	imports := findRelativeImports(source)

	for _, imp := range imports {
		resolved, err := resolveImportFile(imp, fileDir)
		if err != nil {
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
			clean = strings.TrimSuffix(clean, ".js")
			pkgName = filepath.Base(clean)
			relDir = clean
		}

		outDir := filepath.Join(tmpDir, relDir)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return err
		}

		outFile := filepath.Join(outDir, filepath.Base(strings.TrimSuffix(resolved, ".ts"))+".go")
		if err := transpileFile(resolved, outFile, pkgName, moduleName, verbose, false, optLevel); err != nil {
			return err
		}

		// Recurse into this file's imports
		if err := walkImports(resolved, inputDir, baseDir, tmpDir, moduleName, verbose, visited, optLevel); err != nil {
			return err
		}
	}
	return nil
}

// findNodeModuleImports scans TS/JS source for non-relative, non-known imports.
func findNodeModuleImports(source []byte) []string {
	var imports []string
	seen := map[string]bool{}
	lines := strings.Split(string(source), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		idx := strings.Index(line, "from ")
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+5:])
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

// resolveNodeModule walks up from fromDir looking for node_modules/<pkgName>/.
func resolveNodeModule(pkgName, fromDir string) (string, error) {
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

// getNodeModuleEntry reads package.json and determines the entry point file.
func getNodeModuleEntry(pkgDir string) (string, error) {
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
			return filepath.Join(pkgDir, entry), nil
		}
	}

	// Try module field
	if pj.Module != "" {
		return filepath.Join(pkgDir, pj.Module), nil
	}

	// Try main field
	if pj.Main != "" {
		return filepath.Join(pkgDir, pj.Main), nil
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

// resolveSubpathEntry handles subpath exports like "yargs/helpers" by looking up
// the subpath in the root package's exports field.
func resolveSubpathEntry(pkgName, fromDir string) (string, error) {
	root, subpath := splitPkgSubpath(pkgName)
	if subpath == "" {
		return "", fmt.Errorf("cannot determine entry point for %s", pkgName)
	}

	rootDir, err := resolveNodeModule(root, fromDir)
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
		return filepath.Join(rootDir, entry), nil
	}

	return "", fmt.Errorf("cannot resolve subpath export %s in %s", key, root)
}

// splitPkgSubpath splits "pkg/sub" into ("pkg", "sub") and "@scope/pkg/sub" into ("@scope/pkg", "sub").
func splitPkgSubpath(pkgName string) (string, string) {
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

// transpileNodeModuleImports discovers and transpiles node_modules dependencies
// from the entry file and all its relative imports.
func transpileNodeModuleImports(entryFile, inputDir, tmpDir, moduleName string, verbose bool, optLevel int) error {
	absInputDir, _ := filepath.Abs(inputDir)

	// Collect all source files (entry + relative imports) to scan
	sourceFiles := collectAllSourceFiles(entryFile)

	visited := map[string]bool{}
	return processNodeModuleImports(sourceFiles, absInputDir, tmpDir, moduleName, verbose, visited, optLevel)
}

// collectAllSourceFiles returns the entry file plus all transitively-imported relative files.
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

		source, err := os.ReadFile(tsFile)
		if err != nil {
			return
		}
		for _, imp := range findRelativeImports(source) {
			resolved, err := resolveImportFile(imp, filepath.Dir(tsFile))
			if err == nil {
				walk(resolved)
			}
		}
	}
	walk(entryFile)
	return files
}

func processNodeModuleImports(sourceFiles []string, inputDir, tmpDir, moduleName string, verbose bool, visited map[string]bool, optLevel int) error {
	var newSourceFiles []string

	for _, srcFile := range sourceFiles {
		source, err := os.ReadFile(srcFile)
		if err != nil {
			continue
		}
		nodeImports := findNodeModuleImports(source)
		for _, pkgName := range nodeImports {
			if visited[pkgName] {
				continue
			}
			visited[pkgName] = true

			pkgDir, err := resolveNodeModule(pkgName, filepath.Dir(srcFile))
			if err != nil {
				return err
			}
			entryPath, err := getNodeModuleEntry(pkgDir)
			if err != nil {
				// Try subpath export resolution (e.g. "yargs/helpers" → yargs exports["./helpers"])
				entryPath, err = resolveSubpathEntry(pkgName, filepath.Dir(srcFile))
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
			allPkgFiles, err := transpileNodeModuleAsPackage(entryPath, outDir, moduleName, sanitized, verbose, optLevel)
			if err != nil {
				return fmt.Errorf("transpile node_module %s: %w", pkgName, err)
			}
			newSourceFiles = append(newSourceFiles, allPkgFiles...)
		}
	}

	// Recursively process any node_module imports found in the newly transpiled packages
	if len(newSourceFiles) > 0 {
		return processNodeModuleImports(newSourceFiles, inputDir, tmpDir, moduleName, verbose, visited, optLevel)
	}
	return nil
}

// transpileNodeModuleAsPackage discovers all files in an npm package and compiles
// them together using CompilePackage, so cross-file exports are shared.
func transpileNodeModuleAsPackage(entryPath, outDir, moduleName, pkgName string, verbose bool, optLevel int) ([]string, error) {
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
		src, err := os.ReadFile(tsFile)
		if err != nil {
			return err
		}
		for _, imp := range findRelativeImports(src) {
			resolved, err := resolveImportFile(imp, filepath.Dir(tsFile))
			if err != nil {
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
	results, err := compiler.CompilePackageWithOptLevel(files, pkgName, moduleName, absEntry, optLevel)
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

// detectModuleName walks up from the input file to find a go.mod and returns
// the module name. Returns empty string if not found.
func detectModuleName(inputPath string) string {
	dir, _ := filepath.Abs(filepath.Dir(inputPath))
	for {
		gomod := filepath.Join(dir, "go.mod")
		if f, err := os.Open(gomod); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module "))
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
