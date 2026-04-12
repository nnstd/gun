package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nnstd/gun/runtime/builtin"
)

func js(s string) *jsvalue.JSValue { return jsvalue.NewString(s) }

func TestReadWriteFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.txt")

	WriteFileSync(js(p), js("hello"))
	data := ReadFileSync(js(p))
	if data.String() != "hello" {
		t.Errorf("got %q, want %q", data.String(), "hello")
	}
}

func TestExistsSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exists.txt")

	if ExistsSync(js(p)).Bool() {
		t.Error("file should not exist yet")
	}
	os.WriteFile(p, []byte("x"), 0644)
	if !ExistsSync(js(p)).Bool() {
		t.Error("file should exist")
	}
}

func TestMkdirSync(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")

	MkdirSync(js(nested))
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestReaddirSync(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	entries := ReaddirSync(js(dir))
	if entries.Len() != 2 {
		t.Errorf("got %d entries, want 2", entries.Len())
	}
}

func TestUnlinkSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "del.txt")
	os.WriteFile(p, []byte("x"), 0644)

	UnlinkSync(js(p))
	if ExistsSync(js(p)).Bool() {
		t.Error("file should be deleted")
	}
}

func TestStatSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stat.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	info := StatSync(js(p))
	if int(info.Get("size").Number()) != 5 {
		t.Errorf("got size %v, want 5", info.Get("size").Number())
	}
}

func TestAppendFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "append.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	AppendFileSync(js(p), js(" world"))
	data, _ := os.ReadFile(p)
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", data, "hello world")
	}
}

func TestCopyFileSync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("copy me"), 0644)

	CopyFileSync(js(src), js(dst))
	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("got %q, want %q", data, "copy me")
	}
}

func TestRenameSync(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	new_ := filepath.Join(dir, "new.txt")
	os.WriteFile(old, []byte("move me"), 0644)

	RenameSync(js(old), js(new_))
	if ExistsSync(js(old)).Bool() {
		t.Error("old file should not exist")
	}
	data, _ := os.ReadFile(new_)
	if string(data) != "move me" {
		t.Errorf("got %q, want %q", data, "move me")
	}
}

func TestAsJSValueAliases(t *testing.T) {
	if AsJSValue.Get("readFile").TypeString() != "function" {
		t.Fatal("expected readFile alias on fs.AsJSValue")
	}
	if AsJSValue.Get("writeFile").TypeString() != "function" {
		t.Fatal("expected writeFile alias on fs.AsJSValue")
	}
	if AsJSValue.Get("promises").TypeString() != "object" {
		t.Fatal("expected promises object on fs.AsJSValue")
	}
}
