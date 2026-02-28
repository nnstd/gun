package os

import (
	stdos "os"
	"testing"

	"github.com/nnstd/gun/runtime/jsvalue"
)

func TestHomedir(t *testing.T) {
	if h := Homedir(); h.String() == "" {
		t.Error("Homedir should not be empty")
	}
}

func TestTmpdir(t *testing.T) {
	if d := Tmpdir(); d.String() == "" {
		t.Error("Tmpdir should not be empty")
	}
}

func TestHostname(t *testing.T) {
	h := Hostname()
	if h.String() == "" {
		t.Error("Hostname should not be empty")
	}
}

func TestPlatform(t *testing.T) {
	p := Platform()
	valid := map[string]bool{"darwin": true, "linux": true, "win32": true, "freebsd": true}
	if !valid[p.String()] {
		if p.String() == "" {
			t.Error("Platform should not be empty")
		}
	}
}

func TestArch(t *testing.T) {
	a := Arch()
	if a.String() == "" {
		t.Error("Arch should not be empty")
	}
}

func TestCpus(t *testing.T) {
	if n := Cpus(); n.Number() <= 0 {
		t.Errorf("Cpus = %v, want > 0", n.Number())
	}
}

func TestCwd(t *testing.T) {
	if d := Cwd(); d.String() == "" {
		t.Error("Cwd should not be empty")
	}
}

func TestGetenv(t *testing.T) {
	stdos.Setenv("GUN_TEST_VAR", "hello")
	defer stdos.Unsetenv("GUN_TEST_VAR")

	if v := Getenv(jsvalue.NewString("GUN_TEST_VAR")); v.String() != "hello" {
		t.Errorf("Getenv = %q, want %q", v.String(), "hello")
	}
}

func TestEnviron(t *testing.T) {
	stdos.Setenv("GUN_TEST_VAR", "world")
	defer stdos.Unsetenv("GUN_TEST_VAR")

	env := Environ()
	if env.Get("GUN_TEST_VAR").String() != "world" {
		t.Errorf("Environ missing GUN_TEST_VAR")
	}
}

func TestEOL(t *testing.T) {
	if EOL.String() != "\n" && EOL.String() != "\r\n" {
		t.Errorf("EOL = %q, unexpected value", EOL.String())
	}
}
