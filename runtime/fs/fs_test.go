package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.txt")

	if err := WriteFileSync(p, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data := ReadFileSync(p)
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", data, "hello")
	}
}

func TestExistsSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exists.txt")

	if ExistsSync(p) {
		t.Error("file should not exist yet")
	}
	os.WriteFile(p, []byte("x"), 0644)
	if !ExistsSync(p) {
		t.Error("file should exist")
	}
}

func TestMkdirSync(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")

	if err := MkdirSync(nested); err != nil {
		t.Fatal(err)
	}
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

	entries, err := ReaddirSync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}

func TestUnlinkSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "del.txt")
	os.WriteFile(p, []byte("x"), 0644)

	if err := UnlinkSync(p); err != nil {
		t.Fatal(err)
	}
	if ExistsSync(p) {
		t.Error("file should be deleted")
	}
}

func TestStatSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stat.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	info, err := StatSync(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 5 {
		t.Errorf("got size %d, want 5", info.Size())
	}
}

func TestAppendFileSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "append.txt")
	os.WriteFile(p, []byte("hello"), 0644)

	if err := AppendFileSync(p, []byte(" world")); err != nil {
		t.Fatal(err)
	}
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

	if err := CopyFileSync(src, dst); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("got %q, want %q", data, "copy me")
	}
}

func TestRenameSync(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	new := filepath.Join(dir, "new.txt")
	os.WriteFile(old, []byte("move me"), 0644)

	if err := RenameSync(old, new); err != nil {
		t.Fatal(err)
	}
	if ExistsSync(old) {
		t.Error("old file should not exist")
	}
	data, _ := os.ReadFile(new)
	if string(data) != "move me" {
		t.Errorf("got %q, want %q", data, "move me")
	}
}
