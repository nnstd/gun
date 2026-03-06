package path

import (
	"testing"

	"github.com/nnstd/gun/runtime/builtin"
)

func js(s string) *jsvalue.JSValue { return jsvalue.NewString(s) }

func TestJoin(t *testing.T) {
	got := Join(js("a"), js("b"), js("c"))
	if got.String() != "a/b/c" {
		t.Errorf("Join = %q, want %q", got.String(), "a/b/c")
	}
}

func TestBasename(t *testing.T) {
	got := Basename(js("/foo/bar/baz.txt"))
	if got.String() != "baz.txt" {
		t.Errorf("Basename = %q, want %q", got.String(), "baz.txt")
	}
}

func TestDirname(t *testing.T) {
	got := Dirname(js("/foo/bar/baz.txt"))
	if got.String() != "/foo/bar" {
		t.Errorf("Dirname = %q, want %q", got.String(), "/foo/bar")
	}
}

func TestExtname(t *testing.T) {
	got := Extname(js("file.tar.gz"))
	if got.String() != ".gz" {
		t.Errorf("Extname = %q, want %q", got.String(), ".gz")
	}
}

func TestIsAbsolute(t *testing.T) {
	if !IsAbsolute(js("/foo")).Bool() {
		t.Error("expected /foo to be absolute")
	}
	if IsAbsolute(js("foo/bar")).Bool() {
		t.Error("expected foo/bar to be relative")
	}
}

func TestNormalize(t *testing.T) {
	got := Normalize(js("a/b/../c"))
	if got.String() != "a/c" {
		t.Errorf("Normalize = %q, want %q", got.String(), "a/c")
	}
}

func TestResolveAbsolute(t *testing.T) {
	got := Resolve(js("/foo"), js("bar"), js("baz"))
	if got.String() != "/foo/bar/baz" {
		t.Errorf("Resolve = %q, want %q", got.String(), "/foo/bar/baz")
	}
}

func TestRelative(t *testing.T) {
	got := Relative(js("/foo/bar"), js("/foo/baz"))
	if got.String() != "../baz" {
		t.Errorf("Relative = %q, want %q", got.String(), "../baz")
	}
}

func TestParse(t *testing.T) {
	p := Parse(js("/home/user/file.txt"))
	if p.Get("root").String() != "/" {
		t.Errorf("Root = %q, want %q", p.Get("root").String(), "/")
	}
	if p.Get("base").String() != "file.txt" {
		t.Errorf("Base = %q, want %q", p.Get("base").String(), "file.txt")
	}
	if p.Get("ext").String() != ".txt" {
		t.Errorf("Ext = %q, want %q", p.Get("ext").String(), ".txt")
	}
	if p.Get("name").String() != "file" {
		t.Errorf("Name = %q, want %q", p.Get("name").String(), "file")
	}
}

func TestSep(t *testing.T) {
	if Sep.String() == "" {
		t.Error("Sep should not be empty")
	}
}

func TestDelimiter(t *testing.T) {
	if Delimiter.String() == "" {
		t.Error("Delimiter should not be empty")
	}
}
