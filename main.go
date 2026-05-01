package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nnstd/gun/compiler"
	"github.com/nnstd/gun/compiler/runner"
	t262runner "github.com/nnstd/gun/test262/runner"

	"github.com/alecthomas/kong"
)

var cli struct {
	Transpile TranspileCmd `cmd:"transpile" default:"withargs" help:"Transpile TypeScript to Go source."`
	Build     BuildCmd     `cmd:"build" help:"Transpile and compile TypeScript to a binary."`
	Run       RunCmd       `cmd:"run" help:"Transpile, build, and run a TypeScript file."`
	Test262   Test262Cmd   `cmd:"" name:"test262" help:"Run test262 ECMAScript conformance tests."`
}

const defaultCPUProfIntervalMicros = 1000

type TranspileCmd struct {
	Input    string `arg:"" help:"Input .ts file or directory."`
	Output   string `short:"o" help:"Output directory."`
	Pkg      string `short:"p" default:"main" help:"Go package name."`
	Verbose  bool   `short:"v" help:"Verbose output."`
	AST      bool   `help:"Print the tree-sitter AST instead of transpiling."`
	OptLevel int    `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
	Otel     bool   `help:"Enable OpenTelemetry instrumentation."`
}

type BuildCmd struct {
	Input    string `arg:"" help:"Input .ts file or directory."`
	Output   string `short:"o" help:"Output binary path." default:""`
	Pkg      string `short:"p" default:"main" help:"Go package name."`
	Verbose  bool   `short:"v" help:"Verbose output."`
	OptLevel int    `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
	Otel     bool   `help:"Enable OpenTelemetry instrumentation."`
}

type RunCmd struct {
	Input           string   `arg:"" help:"Input .ts file."`
	Pkg             string   `short:"p" default:"main" help:"Go package name."`
	Verbose         bool     `short:"v" help:"Verbose output."`
	OptLevel        int      `short:"O" default:"0" help:"Optimization level for pipeline transpilation (0, 1, 2)."`
	CPUProf         bool     `help:"Write a CPU profile for the executed child binary."`
	CPUProfDir      string   `help:"Directory for CPU profile output."`
	CPUProfName     string   `help:"File name for CPU profile output."`
	CPUProfInterval int      `default:"1000" help:"CPU profile sampling interval in microseconds (v1 only supports 1000)."`
	Otel            bool     `help:"Enable OpenTelemetry instrumentation."`
	Args            []string `arg:"" optional:"" passthrough:"" help:"Arguments to pass to the compiled program."`
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
		return runner.TranspileDir(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose, cmd.OptLevel)
	}

	// No -o: just print to stdout
	if cmd.Output == "" {
		return runner.TranspileFile(cmd.Input, "", cmd.Pkg, "", cmd.Verbose, false, cmd.OptLevel, &compiler.CompileOptions{Otel: cmd.Otel})
	}

	return runner.TranspileProject(cmd.Input, cmd.Output, cmd.Pkg, cmd.Verbose, cmd.OptLevel, &compiler.CompileOptions{Otel: cmd.Otel})
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

	if err := runner.TranspileProject(cmd.Input, tmpDir, cmd.Pkg, cmd.Verbose, cmd.OptLevel, &compiler.CompileOptions{Otel: cmd.Otel}); err != nil {
		return err
	}

	binPath := cmd.Output
	if binPath == "" {
		binPath = strings.TrimSuffix(filepath.Base(cmd.Input), ".ts")
	}
	binPath, _ = filepath.Abs(binPath)

	var buildTags []string
	if cmd.Otel {
		buildTags = append(buildTags, "otel")
	}
	return runner.GoBuild(tmpDir, binPath, cmd.Verbose, buildTags...)
}

func (cmd *RunCmd) Run() error {
	if cmd.CPUProfInterval != defaultCPUProfIntervalMicros {
		return fmt.Errorf("run command only supports --cpu-prof-interval=%d in v1", defaultCPUProfIntervalMicros)
	}
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

	if err := runner.TranspileProject(cmd.Input, tmpDir, cmd.Pkg, cmd.Verbose, cmd.OptLevel, cmd.compileOptions()); err != nil {
		return err
	}

	binName := strings.TrimSuffix(filepath.Base(cmd.Input), ".ts")
	binPath := filepath.Join(tmpDir, binName)
	var runBuildTags []string
	if cmd.Otel {
		runBuildTags = append(runBuildTags, "otel")
	}
	if err := runner.GoBuild(tmpDir, binPath, cmd.Verbose, runBuildTags...); err != nil {
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

func (cmd *RunCmd) compileOptions() *compiler.CompileOptions {
	if !cmd.CPUProf && !cmd.Otel {
		return nil
	}
	return &compiler.CompileOptions{
		CPUProfile: func() *compiler.CPUProfileConfig {
			if !cmd.CPUProf {
				return nil
			}
			return &compiler.CPUProfileConfig{
				Dir:  cmd.CPUProfDir,
				Name: cmd.CPUProfName,
			}
		}(),
		Otel: cmd.Otel,
	}
}

type Test262Cmd struct {
	Repo      string `help:"Path to cloned test262 repository."`
	Test      string `help:"Run a single test file (relative to test262/test/)."`
	Dir       string `help:"Run all tests in a directory (relative to test262/test/)." default:"language"`
	BatchSize int    `default:"20" help:"Number of tests per compiled binary."`
	Timeout   int    `default:"10" help:"Per-test timeout in seconds."`
	Verbose   bool   `short:"v" help:"Verbose output with skip reasons."`
	Format    string `default:"text" help:"Output format: json or text."`
	SkipList  string `help:"Path to custom skip list YAML (default: built-in)."`
	OptLevel  int    `short:"O" default:"0" help:"Optimization level (0, 1, 2)."`

	CloneRepo string `default:"/tmp/test262" help:"Where to clone test262 if not found."`
}

func (cmd *Test262Cmd) Run() error {
	test262Path := cmd.Repo
	if test262Path == "" {
		test262Path = cmd.CloneRepo
	}

	// Auto-clone if needed
	if _, err := os.Stat(filepath.Join(test262Path, "test")); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Cloning test262 into %s...\n", test262Path)
		clone := exec.Command("git", "clone", "--depth", "1", "https://github.com/tc39/test262.git", test262Path)
		clone.Stdout = os.Stderr
		clone.Stderr = os.Stderr
		if err := clone.Run(); err != nil {
			return fmt.Errorf("clone test262: %w", err)
		}
	}

	if cmd.Test == "" && cmd.Dir == "" {
		return fmt.Errorf("either --test or --dir is required")
	}

	var skipList *t262runner.SkipList
	if cmd.SkipList != "" {
		var err error
		skipList, err = t262runner.LoadSkipList(cmd.SkipList)
		if err != nil {
			return fmt.Errorf("load skip list: %w", err)
		}
	} else {
		skipList = t262runner.DefaultSkipList()
	}

	timeout := time.Duration(cmd.Timeout) * time.Second
	r := t262runner.NewRunner(test262Path, skipList, cmd.BatchSize, timeout, cmd.Verbose, cmd.Format, cmd.OptLevel)

	if cmd.Test != "" {
		result := r.RunSingle(cmd.Test)
		printTest262Result(result, cmd.Format)
	} else {
		summary := r.RunDir(cmd.Dir)
		printTest262Summary(summary, cmd.Format)
	}
	return nil
}

func printTest262Result(result t262runner.TestResult, format string) {
	switch format {
	case "text":
		fmt.Printf("%s: %s", result.File, result.Status)
		if result.DurationMs > 0 {
			fmt.Printf(" (%dms)", result.DurationMs)
		}
		if result.Error != "" {
			fmt.Printf(" - %s", result.Error)
		}
		if result.SkipReason != "" {
			fmt.Printf(" [%s]", result.SkipReason)
		}
		fmt.Println()
	default:
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	}
}

func printTest262Summary(summary t262runner.RunSummary, format string) {
	switch format {
	case "text":
		fmt.Printf("Results: %d passed (%.1f%%), %d failed (%.1f%%), %d skipped (%.1f%%), %d errors (%.1f%%)\n",
			summary.Passed, summary.PassPercent,
			summary.Failed, summary.FailPercent,
			summary.Skipped, summary.SkipPercent,
			summary.Errors, summary.ErrorPercent)
		fmt.Printf("Total: %d\n", summary.Total)
		nonSkipped := summary.Total - summary.Skipped
		fmt.Printf("Pass rate: %.1f%% (%d/%d of non-skipped tests)\n",
			summary.PassRate, summary.Passed, nonSkipped)
	default:
		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(data))
	}
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
