package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gunTestEntry(t *testing.T) string {
	t.Helper()

	candidates := []string{
		filepath.Join("..", "gun-test", "index.ts"),
		"/Users/nikita/Work/gun-test/index.ts",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err != nil {
				t.Fatal(err)
			}
			return abs
		}
	}
	t.Skip("gun-test fixture not available")
	return ""
}

func TestTranspileProject_BuiltGunTestMatchesCLIParity(t *testing.T) {
	entry := gunTestEntry(t)
	outDir := t.TempDir()
	bin := filepath.Join(outDir, "gun-test-bin")

	t.Setenv("GOCACHE", filepath.Join(outDir, "gocache"))

	if err := transpileProject(entry, outDir, "main", false, 0); err != nil {
		t.Fatal(err)
	}
	if err := goBuild(outDir, bin, false); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "greet", args: []string{"greet", "world"}, want: "Hello, world!"},
		{name: "greet_uppercase", args: []string{"greet", "world", "--uppercase"}, want: "HELLO, WORLD!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
			}

			if got := strings.TrimSpace(stdout.String()); got != tc.want {
				t.Fatalf("stdout mismatch: got %q want %q", got, tc.want)
			}
			if strings.Contains(stderr.String(), "You need to specify a command") {
				t.Fatalf("unexpected yargs demandCommand failure:\n%s", stderr.String())
			}
		})
	}
}
