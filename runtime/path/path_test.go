package path

import (
	"testing"
)

func TestJoin(t *testing.T) {
	got := Join("a", "b", "c")
	if got != "a/b/c" {
		t.Errorf("Join = %q, want %q", got, "a/b/c")
	}
}

func TestBasename(t *testing.T) {
	got := Basename("/foo/bar/baz.txt")
	if got != "baz.txt" {
		t.Errorf("Basename = %q, want %q", got, "baz.txt")
	}
}

func TestDirname(t *testing.T) {
	got := Dirname("/foo/bar/baz.txt")
	if got != "/foo/bar" {
		t.Errorf("Dirname = %q, want %q", got, "/foo/bar")
	}
}

func TestExtname(t *testing.T) {
	got := Extname("file.tar.gz")
	if got != ".gz" {
		t.Errorf("Extname = %q, want %q", got, ".gz")
	}
}

func TestIsAbsolute(t *testing.T) {
	if !IsAbsolute("/foo") {
		t.Error("expected /foo to be absolute")
	}
	if IsAbsolute("foo/bar") {
		t.Error("expected foo/bar to be relative")
	}
}

func TestNormalize(t *testing.T) {
	got := Normalize("a/b/../c")
	if got != "a/c" {
		t.Errorf("Normalize = %q, want %q", got, "a/c")
	}
}

func TestResolveAbsolute(t *testing.T) {
	got := Resolve("/foo", "bar", "baz")
	if got != "/foo/bar/baz" {
		t.Errorf("Resolve = %q, want %q", got, "/foo/bar/baz")
	}
}

func TestRelative(t *testing.T) {
	got, err := Relative("/foo/bar", "/foo/baz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "../baz" {
		t.Errorf("Relative = %q, want %q", got, "../baz")
	}
}

func TestParse(t *testing.T) {
	p := Parse("/home/user/file.txt")
	if p.Root != "/" {
		t.Errorf("Root = %q, want %q", p.Root, "/")
	}
	if p.Base != "file.txt" {
		t.Errorf("Base = %q, want %q", p.Base, "file.txt")
	}
	if p.Ext != ".txt" {
		t.Errorf("Ext = %q, want %q", p.Ext, ".txt")
	}
	if p.Name != "file" {
		t.Errorf("Name = %q, want %q", p.Name, "file")
	}
}

func TestSep(t *testing.T) {
	if Sep == "" {
		t.Error("Sep should not be empty")
	}
}

func TestDelimiter(t *testing.T) {
	if Delimiter == "" {
		t.Error("Delimiter should not be empty")
	}
}
