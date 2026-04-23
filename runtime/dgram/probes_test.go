package dgram

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

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

func TestDgramProbesParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dgram probes in -short mode")
	}
	skipIfUDPUnavailable(t, "udp4", "0.0.0.0:0")
	root := findRepoRoot(t)
	probesDir := filepath.Join(root, "runtime", "dgram", "testdata", "probes")

	cases := []struct {
		name      string
		needsIPv6 bool
	}{
		{name: "probe1_node_prefix_buffer"},
		{name: "probe2_require_string"},
		{name: "probe3_ipv6", needsIPv6: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsIPv6 && runtime.GOOS == "windows" {
				t.Skip("ipv6 probe skipped on windows")
			}
			if tc.needsIPv6 && os.Getenv("GUN_DGRAM_IPV6_PROBES") != "1" {
				t.Skip("ipv6 probe gated by GUN_DGRAM_IPV6_PROBES=1")
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
