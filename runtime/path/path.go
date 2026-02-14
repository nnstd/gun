package path

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Sep is the platform-specific path separator.
var Sep = string(filepath.Separator)

// Delimiter is the platform-specific path delimiter.
var Delimiter = func() string {
	if runtime.GOOS == "windows" {
		return ";"
	}
	return ":"
}()

// Join joins path segments.
func Join(segments ...string) string {
	return filepath.Join(segments...)
}

// Resolve resolves a sequence of paths to an absolute path.
func Resolve(paths ...string) string {
	if len(paths) == 0 {
		abs, _ := filepath.Abs(".")
		return abs
	}
	result := ""
	for i := len(paths) - 1; i >= 0; i-- {
		result = filepath.Join(paths[i], result)
		if filepath.IsAbs(paths[i]) {
			return filepath.Clean(result)
		}
	}
	abs, _ := filepath.Abs(result)
	return abs
}

// Basename returns the last portion of a path.
func Basename(p string) string {
	return filepath.Base(p)
}

// Dirname returns the directory name of a path.
func Dirname(p string) string {
	return filepath.Dir(p)
}

// Extname returns the extension of the path.
func Extname(p string) string {
	return filepath.Ext(p)
}

// Relative returns a relative path from 'from' to 'to'.
func Relative(from, to string) (string, error) {
	return filepath.Rel(from, to)
}

// IsAbsolute returns whether the path is absolute.
func IsAbsolute(p string) bool {
	return filepath.IsAbs(p)
}

// Normalize normalizes the given path.
func Normalize(p string) string {
	return filepath.Clean(p)
}

// ParsedPath holds the parsed components of a path.
type ParsedPath struct {
	Root string
	Dir  string
	Base string
	Ext  string
	Name string
}

// Parse returns the parsed components of a path.
func Parse(p string) ParsedPath {
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	ext := filepath.Ext(p)
	name := strings.TrimSuffix(base, ext)
	root := ""
	if filepath.IsAbs(p) {
		root = string(filepath.Separator)
	}
	return ParsedPath{Root: root, Dir: dir, Base: base, Ext: ext, Name: name}
}
