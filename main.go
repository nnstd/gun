package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gun/compiler"

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
	return transpileFile(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose)
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
	defer os.RemoveAll(tmpDir)

	goFile := filepath.Join(tmpDir, "main.go")
	if err := transpileFile(cmd.Input, goFile, cmd.Pkg, cmd.Verbose); err != nil {
		return err
	}

	// Build
	binPath := filepath.Join(tmpDir, "main")
	build := exec.Command("go", "build", "-o", binPath, goFile)
	build.Stderr = os.Stderr
	if cmd.Verbose {
		fmt.Fprintf(os.Stderr, "building %s\n", goFile)
	}
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

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

func transpileFile(inputPath, outputPath, pkgName string, verbose bool) error {
	source, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "compiling %s\n", inputPath)
	}

	moduleName := detectModuleName(inputPath)
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

		return transpileFile(path, outPath, pkgName, verbose)
	})
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
