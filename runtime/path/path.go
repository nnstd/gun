package path

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// Sep is the platform-specific path separator.
var Sep = jsvalue.NewString(string(filepath.Separator))

// Delimiter is the platform-specific path delimiter.
var Delimiter = jsvalue.NewString(func() string {
	if runtime.GOOS == "windows" {
		return ";"
	}
	return ":"
}())

// Join joins path segments.
func Join(segments ...*jsvalue.JSValue) *jsvalue.JSValue {
	strs := make([]string, len(segments))
	for i, s := range segments {
		if s != nil {
			strs[i] = s.String()
		}
	}
	return jsvalue.NewString(filepath.Join(strs...))
}

// Resolve resolves a sequence of paths to an absolute path.
func Resolve(paths ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(paths) == 0 {
		abs, _ := filepath.Abs(".")
		return jsvalue.NewString(abs)
	}
	strs := make([]string, len(paths))
	for i, p := range paths {
		if p != nil {
			strs[i] = p.String()
		}
	}
	result := ""
	for i := len(strs) - 1; i >= 0; i-- {
		result = filepath.Join(strs[i], result)
		if filepath.IsAbs(strs[i]) {
			return jsvalue.NewString(filepath.Clean(result))
		}
	}
	abs, _ := filepath.Abs(result)
	return jsvalue.NewString(abs)
}

// Basename returns the last portion of a path.
func Basename(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	return jsvalue.NewString(filepath.Base(s))
}

// Dirname returns the directory name of a path.
func Dirname(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	return jsvalue.NewString(filepath.Dir(s))
}

// Extname returns the extension of the path.
func Extname(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	return jsvalue.NewString(filepath.Ext(s))
}

// Relative returns a relative path from 'from' to 'to'.
func Relative(from, to *jsvalue.JSValue) *jsvalue.JSValue {
	f := ""
	if from != nil {
		f = from.String()
	}
	t := ""
	if to != nil {
		t = to.String()
	}
	rel, err := filepath.Rel(f, t)
	if err != nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(rel)
}

// IsAbsolute returns whether the path is absolute.
func IsAbsolute(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	return jsvalue.NewBool(filepath.IsAbs(s))
}

// Normalize normalizes the given path.
func Normalize(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	return jsvalue.NewString(filepath.Clean(s))
}

// MatchesGlob reports whether path matches the glob pattern.
func MatchesGlob(p, pattern *jsvalue.JSValue) *jsvalue.JSValue {
	ps := ""
	if p != nil {
		ps = p.String()
	}
	pa := ""
	if pattern != nil {
		pa = pattern.String()
	}
	matched, _ := filepath.Match(pa, ps)
	return jsvalue.NewBool(matched)
}

// Parse returns the parsed components of a path as a JSValue object.
func Parse(p *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if p != nil {
		s = p.String()
	}
	dir := filepath.Dir(s)
	base := filepath.Base(s)
	ext := filepath.Ext(s)
	name := strings.TrimSuffix(base, ext)
	root := ""
	if filepath.IsAbs(s) {
		root = string(filepath.Separator)
	}
	return jsvalue.ObjectFrom(
		"root", jsvalue.NewString(root),
		"dir", jsvalue.NewString(dir),
		"base", jsvalue.NewString(base),
		"ext", jsvalue.NewString(ext),
		"name", jsvalue.NewString(name),
	)
}
