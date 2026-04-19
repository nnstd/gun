package nodehttp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// findRepoRoot walks up from the package dir until it finds go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", cwd)
		}
		dir = parent
	}
}

func runGunProbe(t *testing.T, repoRoot, tsPath string) []byte {
	t.Helper()
	cmd := exec.Command("go", "run", ".", "run", tsPath)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gun run %s failed: %v\n%s", tsPath, err, out)
	}
	return out
}

func TestHTTPProbesParityWithBun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping probe parity in -short mode")
	}
	root := findRepoRoot(t)
	probesDir := filepath.Join(root, "runtime", "http", "testdata", "probes")

	cases := []struct {
		name      string
		needsUnix bool
		needsNet  bool
	}{
		{name: "probe1_basic"},
		{name: "probe2_headers"},
		{name: "probe3_post"},
		{name: "probe4_request"},
		{name: "probe5_get"},
		{name: "probe6_unix", needsUnix: true},
		{name: "probe7_https_external", needsNet: true},
		{name: "probe8_https_selfsigned"},
		{name: "probe9_eaddrinuse"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsUnix && runtime.GOOS == "windows" {
				t.Skip("unix socket probe skipped on windows")
			}
			if tc.needsNet && os.Getenv("GUN_NETWORK_PROBES") != "1" {
				t.Skip("network probe gated by GUN_NETWORK_PROBES=1")
			}
			tsPath := filepath.Join(probesDir, tc.name+".ts")
			expectedPath := filepath.Join(probesDir, tc.name+".expected")
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			got := runGunProbe(t, root, tsPath)
			if !bytes.Equal(bytes.TrimRight(got, "\n"), bytes.TrimRight(expected, "\n")) {
				t.Errorf("probe output mismatch\n--- expected ---\n%s\n--- got ---\n%s",
					string(expected), string(got))
			}
		})
	}
}
