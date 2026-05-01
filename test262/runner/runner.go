package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nnstd/gun/test262/parser"
)

// TestStatus represents the outcome of a single test execution.
type TestStatus string

const (
	StatusPass  TestStatus = "pass"
	StatusFail  TestStatus = "fail"
	StatusSkip  TestStatus = "skip"
	StatusError TestStatus = "error" // compile/transpile error
)

// TestResult holds the outcome of a single test.
type TestResult struct {
	File       string     `json:"file"`
	Status     TestStatus `json:"status"`
	DurationMs int64      `json:"duration_ms"`
	Error      string     `json:"error,omitempty"`
	SkipReason string     `json:"skip_reason,omitempty"`
}

// RunSummary aggregates results from a test run.
type RunSummary struct {
	Total       int           `json:"total"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	Skipped     int           `json:"skipped"`
	Errors      int           `json:"errors"`
	PassRate    float64       `json:"pass_rate"`
	PassPercent float64       `json:"pass_percent"`
	FailPercent float64       `json:"fail_percent"`
	SkipPercent float64       `json:"skip_percent"`
	ErrorPercent float64      `json:"error_percent"`
	Results     []TestResult  `json:"results"`
}

// Runner orchestrates test262 test execution.
type Runner struct {
	Test262Path string        // path to cloned test262 repo
	SkipList    *SkipList
	BatchSize   int
	Timeout     time.Duration
	Verbose     bool
	Format      string // "json" or "text"
	OptLevel    int
}

// NewRunner creates a Runner with the given options.
func NewRunner(test262Path string, skipList *SkipList, batchSize int, timeout time.Duration, verbose bool, format string, optLevel int) *Runner {
	if batchSize <= 0 {
		batchSize = 20
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if format == "" {
		format = "json"
	}
	return &Runner{
		Test262Path: test262Path,
		SkipList:    skipList,
		BatchSize:   batchSize,
		Timeout:     timeout,
		Verbose:     verbose,
		Format:      format,
		OptLevel:    optLevel,
	}
}

// RunSingle runs a single test file and returns the result.
func (r *Runner) RunSingle(testPath string) TestResult {
	start := time.Now()

	// Resolve absolute path
	absPath := testPath
	if !filepath.IsAbs(testPath) {
		absPath = filepath.Join(r.Test262Path, "test", testPath)
	}

	// Read and parse frontmatter
	source, err := os.ReadFile(absPath)
	if err != nil {
		return TestResult{
			File:   testPath,
			Status: StatusError,
			Error:  fmt.Sprintf("read file: %v", err),
		}
	}

	info, err := parser.ParseFrontmatter(source)
	if err != nil {
		return TestResult{
			File:   testPath,
			Status: StatusError,
			Error:  fmt.Sprintf("parse frontmatter: %v", err),
		}
	}

	// Check skip list
	if r.SkipList != nil {
		if skip, reason := r.SkipList.ShouldSkip(info, testPath); skip {
			return TestResult{
				File:       testPath,
				Status:     StatusSkip,
				SkipReason: reason,
			}
		}
	}

	// Run as a single-item batch
	item := batchItem{
		Name:       sanitizeTestName(testPath),
		FilePath:   testPath,
		Source:     source,
		Info:       info,
		IsNegative: info.IsNegative(),
	}

	result := r.runSingleTest(item)
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// RunDir discovers all .js files in a directory and runs them in batches.
func (r *Runner) RunDir(dirPath string) RunSummary {
	absDir := dirPath
	if !filepath.IsAbs(dirPath) {
		absDir = filepath.Join(r.Test262Path, "test", dirPath)
	}

	// Discover all .js test files
	var allFiles []string
	err := filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".js") {
			return nil
		}
		// Skip _FIXTURE files
		if strings.Contains(filepath.Base(path), "_FIXTURE") {
			return nil
		}
		relPath, err := filepath.Rel(filepath.Join(r.Test262Path, "test"), path)
		if err != nil {
			relPath = path
		}
		allFiles = append(allFiles, relPath)
		return nil
	})
	if err != nil {
		return RunSummary{
			Results: []TestResult{{
				File:   dirPath,
				Status: StatusError,
				Error:  fmt.Sprintf("walk directory: %v", err),
			}},
		}
	}

	summary := RunSummary{
		Total:   len(allFiles),
		Results: make([]TestResult, 0, len(allFiles)),
	}

	// Filter and skip
	var runnable []batchItem
	for _, relPath := range allFiles {
		absPath := filepath.Join(r.Test262Path, "test", relPath)
		source, err := os.ReadFile(absPath)
		if err != nil {
			summary.Results = append(summary.Results, TestResult{
				File:   relPath,
				Status: StatusError,
				Error:  fmt.Sprintf("read file: %v", err),
			})
			summary.Errors++
			continue
		}

		info, err := parser.ParseFrontmatter(source)
		if err != nil {
			summary.Results = append(summary.Results, TestResult{
				File:   relPath,
				Status: StatusError,
				Error:  fmt.Sprintf("parse frontmatter: %v", err),
			})
			summary.Errors++
			continue
		}

		if r.SkipList != nil {
			if skip, reason := r.SkipList.ShouldSkip(info, relPath); skip {
				summary.Results = append(summary.Results, TestResult{
					File:       relPath,
					Status:     StatusSkip,
					SkipReason: reason,
				})
				summary.Skipped++
				if r.Verbose {
					fmt.Fprintf(os.Stderr, "SKIP %s: %s\n", relPath, reason)
				}
				continue
			}
		}

		runnable = append(runnable, batchItem{
			Name:       sanitizeTestName(relPath),
			FilePath:   relPath,
			Source:     source,
			Info:       info,
			IsNegative: info.IsNegative(),
		})
	}

	// Run in batches
	for i := 0; i < len(runnable); i += r.BatchSize {
		end := i + r.BatchSize
		if end > len(runnable) {
			end = len(runnable)
		}
		batch := runnable[i:end]
		batchResults := r.runBatch(batch)
		summary.Results = append(summary.Results, batchResults...)
	}

	// Compute totals
	summary.Passed = 0
	summary.Failed = 0
	summary.Errors = 0
	for _, res := range summary.Results {
		switch res.Status {
		case StatusPass:
			summary.Passed++
		case StatusFail:
			summary.Failed++
		case StatusError:
			summary.Errors++
		}
	}

	nonSkipped := summary.Total - summary.Skipped
	if nonSkipped > 0 {
		summary.PassRate = float64(summary.Passed) / float64(nonSkipped) * 100
	}
	if summary.Total > 0 {
		summary.PassPercent = float64(summary.Passed) / float64(summary.Total) * 100
		summary.FailPercent = float64(summary.Failed) / float64(summary.Total) * 100
		summary.SkipPercent = float64(summary.Skipped) / float64(summary.Total) * 100
		summary.ErrorPercent = float64(summary.Errors) / float64(summary.Total) * 100
	}

	return summary
}

// sanitizeTestName converts a test file path to a collision-safe Go identifier.
func sanitizeTestName(path string) string {
	// Use full relative path (with slashes→underscores) to avoid collisions
	// between test/a/foo.js and test/b/foo.js.
	name := strings.TrimSuffix(path, ".js")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	if name == "" {
		name = "test"
	}
	// Ensure it starts with a letter
	if name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	return name
}
