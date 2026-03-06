package fs

import (
	"os"
	"path/filepath"

	"github.com/nnstd/gun/runtime/builtin"
)

// ReadFileSync reads the entire contents of a file.
// Returns the file content as a JSValue string.
func ReadFileSync(path *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	data, _ := os.ReadFile(p)
	return jsvalue.NewString(string(data))
}

// Realpath resolves a path to its canonical absolute pathname.
func Realpath(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return jsvalue.NewString(abs)
	}
	return jsvalue.NewString(resolved)
}

// WriteFileSync writes data to a file, replacing it if it already exists.
func WriteFileSync(path *jsvalue.JSValue, data *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	d := ""
	if data != nil {
		d = data.String()
	}
	os.WriteFile(p, []byte(d), 0644)
	return jsvalue.NewUndefined()
}

// ExistsSync returns true if the path exists.
func ExistsSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	_, err := os.Stat(p)
	return jsvalue.NewBool(err == nil)
}

// MkdirSync creates a directory and all parent directories.
func MkdirSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	os.MkdirAll(p, 0755)
	return jsvalue.NewUndefined()
}

// ReaddirSync reads the contents of a directory.
func ReaddirSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return jsvalue.NewArray()
	}
	elems := make([]*jsvalue.JSValue, len(entries))
	for i, e := range entries {
		elems[i] = jsvalue.NewString(e.Name())
	}
	return jsvalue.NewArray(elems...)
}

// UnlinkSync removes a file.
func UnlinkSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	os.Remove(p)
	return jsvalue.NewUndefined()
}

// StatSync returns file info for the given path.
func StatSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	info, err := os.Stat(p)
	if err != nil {
		return jsvalue.NewNull()
	}
	obj := jsvalue.NewObject()
	obj.Set("size", jsvalue.NewNumber(float64(info.Size())))
	obj.Set("isDirectory", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewBool(info.IsDir())
	}))
	obj.Set("isFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewBool(!info.IsDir())
	}))
	return obj
}

// RmdirSync removes a directory.
func RmdirSync(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	os.Remove(p)
	return jsvalue.NewUndefined()
}

// AppendFileSync appends data to a file, creating it if it doesn't exist.
func AppendFileSync(path *jsvalue.JSValue, data *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	d := ""
	if data != nil {
		d = data.String()
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return jsvalue.NewUndefined()
	}
	defer f.Close()
	f.Write([]byte(d))
	return jsvalue.NewUndefined()
}

// CopyFileSync copies src to dst.
func CopyFileSync(src, dst *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if src != nil {
		s = src.String()
	}
	d := ""
	if dst != nil {
		d = dst.String()
	}
	data, err := os.ReadFile(s)
	if err != nil {
		return jsvalue.NewUndefined()
	}
	os.WriteFile(d, data, 0644)
	return jsvalue.NewUndefined()
}

// RenameSync renames (moves) a file.
func RenameSync(oldPath, newPath *jsvalue.JSValue) *jsvalue.JSValue {
	o := ""
	if oldPath != nil {
		o = oldPath.String()
	}
	n := ""
	if newPath != nil {
		n = newPath.String()
	}
	os.Rename(o, n)
	return jsvalue.NewUndefined()
}

// --- fs/promises equivalents ---

func ReadFile(path *jsvalue.JSValue) *jsvalue.JSValue {
	return ReadFileSync(path)
}

func WriteFile(path *jsvalue.JSValue, data *jsvalue.JSValue) *jsvalue.JSValue {
	return WriteFileSync(path, data)
}

func AppendFile(path *jsvalue.JSValue, data *jsvalue.JSValue) *jsvalue.JSValue {
	return AppendFileSync(path, data)
}

func CopyFile(src, dst *jsvalue.JSValue) *jsvalue.JSValue {
	return CopyFileSync(src, dst)
}

func Rename(oldPath, newPath *jsvalue.JSValue) *jsvalue.JSValue {
	return RenameSync(oldPath, newPath)
}

func Mkdir(path *jsvalue.JSValue) *jsvalue.JSValue {
	return MkdirSync(path)
}

func Readdir(path *jsvalue.JSValue) *jsvalue.JSValue {
	return ReaddirSync(path)
}

func Unlink(path *jsvalue.JSValue) *jsvalue.JSValue {
	return UnlinkSync(path)
}

func Stat(path *jsvalue.JSValue) *jsvalue.JSValue {
	return StatSync(path)
}

func Lstat(path *jsvalue.JSValue) *jsvalue.JSValue {
	return StatSync(path) // simplified: same as Stat for now
}

func Rm(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	os.RemoveAll(p)
	return jsvalue.NewUndefined()
}

func Access(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	_, err := os.Stat(p)
	if err != nil {
		return jsvalue.NewBool(false)
	}
	return jsvalue.NewBool(true)
}

func Chmod(path *jsvalue.JSValue, mode *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	m := os.FileMode(0644)
	if mode != nil {
		m = os.FileMode(int(mode.Number()))
	}
	os.Chmod(p, m)
	return jsvalue.NewUndefined()
}

func Chown(path *jsvalue.JSValue, uid, gid *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	u := 0
	if uid != nil {
		u = int(uid.Number())
	}
	g := 0
	if gid != nil {
		g = int(gid.Number())
	}
	os.Chown(p, u, g)
	return jsvalue.NewUndefined()
}

func Link(existingPath, newPath *jsvalue.JSValue) *jsvalue.JSValue {
	e := ""
	if existingPath != nil {
		e = existingPath.String()
	}
	n := ""
	if newPath != nil {
		n = newPath.String()
	}
	os.Link(e, n)
	return jsvalue.NewUndefined()
}

func Symlink(target, path *jsvalue.JSValue) *jsvalue.JSValue {
	t := ""
	if target != nil {
		t = target.String()
	}
	p := ""
	if path != nil {
		p = path.String()
	}
	os.Symlink(t, p)
	return jsvalue.NewUndefined()
}

func Readlink(path *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	link, err := os.Readlink(p)
	if err != nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(link)
}

func Truncate(path *jsvalue.JSValue, size *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if path != nil {
		p = path.String()
	}
	s := int64(0)
	if size != nil {
		s = int64(size.Number())
	}
	os.Truncate(p, s)
	return jsvalue.NewUndefined()
}

func Mkdtemp(prefix *jsvalue.JSValue) *jsvalue.JSValue {
	p := ""
	if prefix != nil {
		p = prefix.String()
	}
	dir, err := os.MkdirTemp("", p)
	if err != nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(dir)
}

func Cp(src, dst *jsvalue.JSValue) *jsvalue.JSValue {
	return CopyFileSync(src, dst)
}
