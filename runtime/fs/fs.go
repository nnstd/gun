package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReadFileSync reads the entire contents of a file.
// Accepts any type for path (string or *jsvalue.JSValue) and optional encoding.
// Returns []byte directly, matching JS single-value semantics.
func ReadFileSync(path any, opts ...any) []byte {
	data, _ := os.ReadFile(fmt.Sprint(path))
	return data
}

// Realpath resolves a path to its canonical absolute pathname.
func Realpath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}

// WriteFileSync writes data to a file, replacing it if it already exists.
func WriteFileSync(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// ExistsSync returns true if the path exists.
func ExistsSync(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MkdirSync creates a directory and all parent directories.
func MkdirSync(path string) error {
	return os.MkdirAll(path, 0755)
}

// ReaddirSync reads the contents of a directory.
func ReaddirSync(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// UnlinkSync removes a file.
func UnlinkSync(path string) error {
	return os.Remove(path)
}

// StatSync returns file info for the given path.
func StatSync(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// RmdirSync removes a directory.
func RmdirSync(path string) error {
	return os.Remove(path)
}

// AppendFileSync appends data to a file, creating it if it doesn't exist.
func AppendFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// CopyFileSync copies src to dst.
func CopyFileSync(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// RenameSync renames (moves) a file.
func RenameSync(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// --- fs/promises equivalents (no "Sync" suffix) ---

// ReadFile reads the entire contents of a file.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to a file, replacing it if it already exists.
func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// AppendFile appends data to a file, creating it if it doesn't exist.
func AppendFile(path string, data []byte) error {
	return AppendFileSync(path, data)
}

// CopyFile copies src to dst.
func CopyFile(src, dst string) error {
	return CopyFileSync(src, dst)
}

// Rename renames (moves) a file.
func Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// Mkdir creates a directory and all parent directories.
func Mkdir(path string) error {
	return os.MkdirAll(path, 0755)
}

// Readdir reads the contents of a directory.
func Readdir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// Unlink removes a file.
func Unlink(path string) error {
	return os.Remove(path)
}

// Rmdir removes a directory.
func Rmdir(path string) error {
	return os.Remove(path)
}

// Stat returns file info for the given path.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// Lstat returns file info without following symbolic links.
func Lstat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

// Rm removes files and directories recursively.
func Rm(path string) error {
	return os.RemoveAll(path)
}

// Access tests whether the path exists and is accessible.
func Access(path string) error {
	_, err := os.Stat(path)
	return err
}

// Chmod changes the file mode bits.
func Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

// Chown changes the numeric uid and gid of the file.
func Chown(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

// Link creates a hard link.
func Link(existingPath, newPath string) error {
	return os.Link(existingPath, newPath)
}

// Symlink creates a symbolic link.
func Symlink(target, path string) error {
	return os.Symlink(target, path)
}

// Readlink reads the destination of a symbolic link.
func Readlink(path string) (string, error) {
	return os.Readlink(path)
}

// Truncate truncates the file to the specified length.
func Truncate(path string, size int64) error {
	return os.Truncate(path, size)
}

// Mkdtemp creates a unique temporary directory.
func Mkdtemp(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

// Cp recursively copies src to dst.
func Cp(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
