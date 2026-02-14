package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler"

	"github.com/alecthomas/kong"
)

var cli struct {
	Build BuildCmd `cmd:"" default:"withargs" help:"Transpile TypeScript to Go."`
	Run   RunCmd   `cmd:"" help:"Transpile, build, and run a TypeScript file."`
}

type BuildCmd struct {
	Input   string `arg:"" help:"Input .ts file or directory."`
	Output  string `short:"o" help:"Output file or directory."`
	Pkg     string `short:"p" default:"main" help:"Go package name."`
	Verbose bool   `short:"v" help:"Verbose output."`
}

type RunCmd struct {
	Input   string   `arg:"" help:"Input .ts file."`
	Pkg     string   `short:"p" default:"main" help:"Go package name."`
	Verbose bool     `short:"v" help:"Verbose output."`
	Args    []string `arg:"" optional:"" passthrough:"" help:"Arguments to pass to the compiled program."`
}

func (cmd *BuildCmd) Run() error {
	info, err := os.Stat(cmd.Input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return transpileDir(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose)
	}
	return transpileFile(cmd.Input, cmd.Output, cmd.Pkg, "", cmd.Verbose)
}

func (cmd *RunCmd) Run() error {
	info, err := os.Stat(cmd.Input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("run command requires a single .ts file, not a directory")
	}

	// Transpile to temp dir
	tmpDir, err := os.MkdirTemp("", "gun-run-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	goFile := filepath.Join(tmpDir, "main.go")
	if err := transpileFile(cmd.Input, goFile, cmd.Pkg, "gunrun", cmd.Verbose); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}

	// Transpile relative imports recursively
	inputDir := filepath.Dir(cmd.Input)
	if inputDir == "" {
		inputDir = "."
	}
	if err := transpileRelativeImports(cmd.Input, inputDir, tmpDir, "gunrun", cmd.Verbose); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}

	// Set up go.mod so gun/runtime/* imports resolve
	if err := scaffoldGoMod(tmpDir, cmd.Verbose); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}

	// Build
	binPath := filepath.Join(tmpDir, "main")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = tmpDir
	var buildStderr strings.Builder
	build.Stderr = &buildStderr
	if cmd.Verbose {
		fmt.Fprintf(os.Stderr, "building %s\n", goFile)
	}
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed:\n%s\ntranspiled source is at %s", buildStderr.String(), goFile)
	}
	defer os.RemoveAll(tmpDir)

	// Run
	run := exec.Command(binPath, cmd.Args...)
	run.Stdin = os.Stdin
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	return run.Run()
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

func transpileFile(inputPath, outputPath, pkgName, moduleName string, verbose bool) error {
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
	result, err := compiler.Compile(source, pkgName, moduleName)
	if err != nil {
		return fmt.Errorf("compile %s: %w", inputPath, err)
	}

	if outputPath == "" {
		_, err = os.Stdout.Write(result)
		return err
	}

	return os.WriteFile(outputPath, result, 0644)
}

func transpileDir(dirPath, outputDir, pkgName string, verbose bool) error {
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

		return transpileFile(path, outPath, pkgName, "", verbose)
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

// resolveImportFile resolves a relative import path like ./foo to an actual .ts file.
// Tries importPath.ts first, then importPath/index.ts.
func resolveImportFile(importPath, fromDir string) (string, error) {
	// Strip leading ./ or ../
	clean := importPath
	clean = strings.TrimSuffix(clean, ".ts")
	clean = strings.TrimSuffix(clean, ".js")

	candidate := filepath.Join(fromDir, clean+".ts")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	candidate = filepath.Join(fromDir, clean, "index.ts")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	return "", fmt.Errorf("cannot resolve import %q from %s", importPath, fromDir)
}

// transpileRelativeImports recursively discovers and transpiles relative imports
// from a TS entry file into the correct subdirectories of tmpDir.
func transpileRelativeImports(entryFile, inputDir, tmpDir, moduleName string, verbose bool) error {
	absInputDir, err := filepath.Abs(inputDir)
	if err != nil {
		return err
	}
	visited := map[string]bool{}
	return walkImports(entryFile, absInputDir, absInputDir, tmpDir, moduleName, verbose, visited)
}

func walkImports(tsFile, inputDir, baseDir, tmpDir, moduleName string, verbose bool, visited map[string]bool) error {
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
		if err := transpileFile(resolved, outFile, pkgName, moduleName, verbose); err != nil {
			return err
		}

		// Recurse into this file's imports
		if err := walkImports(resolved, inputDir, baseDir, tmpDir, moduleName, verbose, visited); err != nil {
			return err
		}
	}
	return nil
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
