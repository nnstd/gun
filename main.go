package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gun/compiler"
)

func main() {
	output := flag.String("o", "", "output file path (default: stdout)")
	pkgName := flag.String("pkg", "main", "Go package name")
	verbose := flag.Bool("v", false, "verbose mode (print tree-sitter AST)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: gun [flags] <input.ts>\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := args[0]

	info, err := os.Stat(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		err = transpileDir(inputPath, *output, *pkgName, *verbose)
	} else {
		err = transpileFile(inputPath, *output, *pkgName, *verbose)
	}

	if err != nil {
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

	result, err := compiler.Compile(source, pkgName)
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
		// Skip .d.ts files
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
