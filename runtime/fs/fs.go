package fs

import "os"

// ReadFileSync reads the entire contents of a file.
func ReadFileSync(path string) ([]byte, error) {
	return os.ReadFile(path)
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
