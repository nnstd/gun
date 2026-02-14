package os

import (
	stdos "os"
	"testing"
)

func TestHomedir(t *testing.T) {
	if h := Homedir(); h == "" {
		t.Error("Homedir should not be empty")
	}
}

func TestTmpdir(t *testing.T) {
	if d := Tmpdir(); d == "" {
		t.Error("Tmpdir should not be empty")
	}
}

func TestHostname(t *testing.T) {
	h, err := Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if h == "" {
		t.Error("Hostname should not be empty")
	}
}

func TestPlatform(t *testing.T) {
	p := Platform()
	valid := map[string]bool{"darwin": true, "linux": true, "win32": true, "freebsd": true}
	if !valid[p] {
		if p == "" {
			t.Error("Platform should not be empty")
		}
	}
}

func TestArch(t *testing.T) {
	a := Arch()
	if a == "" {
		t.Error("Arch should not be empty")
	}
}

func TestCpus(t *testing.T) {
	if n := Cpus(); n <= 0 {
		t.Errorf("Cpus = %d, want > 0", n)
	}
}

func TestCwd(t *testing.T) {
	if d := Cwd(); d == "" {
		t.Error("Cwd should not be empty")
	}
}

func TestGetenv(t *testing.T) {
	stdos.Setenv("GUN_TEST_VAR", "hello")
	defer stdos.Unsetenv("GUN_TEST_VAR")

	if v := Getenv("GUN_TEST_VAR"); v != "hello" {
		t.Errorf("Getenv = %q, want %q", v, "hello")
	}
}

func TestEnviron(t *testing.T) {
	stdos.Setenv("GUN_TEST_VAR", "world")
	defer stdos.Unsetenv("GUN_TEST_VAR")

	env := Environ()
	if env["GUN_TEST_VAR"] != "world" {
		t.Errorf("Environ missing GUN_TEST_VAR")
	}
}

func TestEOL(t *testing.T) {
	if EOL != "\n" && EOL != "\r\n" {
		t.Errorf("EOL = %q, unexpected value", EOL)
	}
}
