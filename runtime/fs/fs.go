package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/stream"
	"github.com/nnstd/gun/runtime/web"
)

var ReadStream *jsvalue.JSValue
var WriteStream *jsvalue.JSValue

func init() {
	ensureFSStreamClasses()
}

func ensureFSStreamClasses() {
	if ReadStream != nil && WriteStream != nil {
		return
	}
	ReadStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initReadStream(this, args...)
		return nil
	}, stream.Readable)
	WriteStream = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initWriteStream(this, args...)
		return nil
	}, stream.Writable)
	initFSStreamPrototypes()
}

func pathString(path *jsvalue.JSValue) string {
	if path == nil {
		return ""
	}
	if jsvalue.InstanceOf(path, buffer.Buffer).Bool() {
		return string(path.Bytes())
	}
	return path.String()
}

func dataBytes(data *jsvalue.JSValue) []byte {
	if data == nil {
		return nil
	}
	if jsvalue.InstanceOf(data, buffer.Buffer).Bool() {
		return append([]byte(nil), data.Bytes()...)
	}
	return []byte(data.String())
}

func bufferFromBytes(data []byte) *jsvalue.JSValue {
	return buffer.Buffer.Get("from").Call(jsvalue.NewString(string(data)))
}

func readEncoding(opts ...*jsvalue.JSValue) (string, bool) {
	if len(opts) == 0 || opts[0] == nil || opts[0].TypeString() == "undefined" || opts[0].TypeString() == "null" {
		return "", false
	}
	opt := opts[0]
	if opt.TypeString() == "string" {
		enc := opt.String()
		return enc, enc != ""
	}
	if opt.TypeString() == "object" {
		enc := opt.Get("encoding")
		if enc != nil && enc.TypeString() == "string" && enc.String() != "" {
			return enc.String(), true
		}
	}
	return "", false
}

func optionSignal(opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(opts) == 0 {
		return nil
	}
	return web.AbortSignalFromOptions(opts[0])
}

func abortErrFromSignal(signal *jsvalue.JSValue) *jsvalue.JSValue {
	if !web.IsAbortSignal(signal) || !signal.Get("aborted").Bool() {
		return nil
	}
	errVal := web.NewAbortError("The operation was aborted")
	errVal.Set("code", jsvalue.NewString("ABORT_ERR"))
	return errVal
}

func nodeFSError(err error, syscallName, path string) *jsvalue.JSValue {
	if err == nil {
		return nil
	}
	code := "EIO"
	errno := 0
	switch {
	case errors.Is(err, os.ErrNotExist):
		code = "ENOENT"
		errno = int(syscall.ENOENT)
	case errors.Is(err, os.ErrPermission):
		code = "EACCES"
		errno = int(syscall.EACCES)
	case errors.Is(err, os.ErrExist):
		code = "EEXIST"
		errno = int(syscall.EEXIST)
	case errors.Is(err, syscall.EISDIR):
		code = "EISDIR"
		errno = int(syscall.EISDIR)
	case errors.Is(err, syscall.ENOTDIR):
		code = "ENOTDIR"
		errno = int(syscall.ENOTDIR)
	}
	msg := fmt.Sprintf("%s: %s, %s '%s'", code, err.Error(), syscallName, path)
	errVal := jserror.Error.Call(jsvalue.NewString(msg))
	errVal.Set("code", jsvalue.NewString(code))
	errVal.Set("errno", jsvalue.NewNumber(float64(-errno)))
	errVal.Set("syscall", jsvalue.NewString(syscallName))
	errVal.Set("path", jsvalue.NewString(path))
	return errVal
}

// ReadFileSync reads the entire contents of a file.
// It returns a Buffer by default and a string when an encoding is requested.
func ReadFileSync(path *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	p := pathString(path)
	data, err := os.ReadFile(p)
	if err != nil {
		panic(nodeFSError(err, "open", p))
	}
	if _, ok := readEncoding(opts...); ok {
		return jsvalue.NewString(string(data))
	}
	return bufferFromBytes(data)
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
	p := pathString(path)
	if err := os.WriteFile(p, dataBytes(data), 0644); err != nil {
		panic(nodeFSError(err, "open", p))
	}
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
	p := pathString(path)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(nodeFSError(err, "open", p))
	}
	defer f.Close()
	if _, err := f.Write(dataBytes(data)); err != nil {
		panic(nodeFSError(err, "write", p))
	}
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

func ReadFile(path *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	if errVal := abortErrFromSignal(optionSignal(opts...)); errVal != nil {
		panic(errVal)
	}
	return ReadFileSync(path, opts...)
}

func WriteFile(path *jsvalue.JSValue, data *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	if errVal := abortErrFromSignal(optionSignal(opts...)); errVal != nil {
		panic(errVal)
	}
	return WriteFileSync(path, data)
}

func AppendFile(path *jsvalue.JSValue, data *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	if errVal := abortErrFromSignal(optionSignal(opts...)); errVal != nil {
		panic(errVal)
	}
	return AppendFileSync(path, data)
}

func initReadStream(this *jsvalue.JSValue, args ...*jsvalue.JSValue) {
	this.Set("_events", jsvalue.NewObject())
	this.Set("_chunks", jsvalue.NewArray())
	if len(args) > 0 {
		this.Set("path", jsvalue.NewString(pathString(args[0])))
	}
	if len(args) > 1 {
		this.Set("_options", args[1])
	}
	this.Set("_started", jsvalue.NewBool(false))
}

func initWriteStream(this *jsvalue.JSValue, args ...*jsvalue.JSValue) {
	this.Set("_events", jsvalue.NewObject())
	this.Set("_written", jsvalue.NewArray())
	this.Set("_fsChunks", jsvalue.NewArray())
	if len(args) > 0 {
		this.Set("path", jsvalue.NewString(pathString(args[0])))
	}
	if len(args) > 1 {
		this.Set("_options", args[1])
	}
}

func initFSStreamPrototypes() {
	ReadStream.Get("prototype").Set("on", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		stream.Readable.Get("prototype").Get("on").Call(args...)
		if len(args) >= 2 && args[0] != nil {
			startReadStream(args[0])
		}
		return args[0]
	}).MarkAsMethod())
	ReadStream.Get("prototype").Set("pipe", stream.Readable.Get("prototype").Get("pipe"))
	ReadStream.Get("prototype").Set("push", stream.Readable.Get("prototype").Get("push"))

	WriteStream.Get("prototype").Set("on", stream.Writable.Get("prototype").Get("on"))
	WriteStream.Get("prototype").Set("emit", stream.Writable.Get("prototype").Get("emit"))
	WriteStream.Get("prototype").Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[0] != nil {
			args[0].Get("_fsChunks").MethodCall("push", args[1])
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())
	WriteStream.Get("prototype").Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[1] != nil {
			args[0].MethodCall("write", args[1])
		}
		finishWriteStream(args[0])
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
}

func startReadStream(rs *jsvalue.JSValue) {
	if rs.Get("_started").Bool() {
		return
	}
	rs.Set("_started", jsvalue.NewBool(true))
	options := rs.Get("_options")
	p := rs.Get("path").String()
	_, hasEnc := readEncoding(options)

	// Check abort signal on event loop before spawning goroutine.
	if errVal := abortErrFromSignal(web.AbortSignalFromOptions(options)); errVal != nil {
		rs.MethodCall("emit", jsvalue.NewString("error"), errVal)
		return
	}

	// Read file in goroutine (non-blocking).
	// Register handle to keep event loop alive while goroutine runs.
	eventloop.Default.RegisterHandle()
	go func() {
		defer eventloop.Default.UnregisterHandle()
		data, err := os.ReadFile(p)
		if err != nil {
			eventloop.Default.ScheduleCallback(func() {
				rs.MethodCall("emit", jsvalue.NewString("error"), nodeFSError(err, "open", p))
			})
			return
		}
		eventloop.Default.ScheduleCallback(func() {
			if hasEnc {
				rs.MethodCall("push", jsvalue.NewString(string(data)))
			} else {
				rs.MethodCall("push", bufferFromBytes(data))
			}
			rs.MethodCall("emit", jsvalue.NewString("end"))
		})
	}()
}

func finishWriteStream(ws *jsvalue.JSValue) {
	if errVal := abortErrFromSignal(web.AbortSignalFromOptions(ws.Get("_options"))); errVal != nil {
		ws.MethodCall("emit", jsvalue.NewString("error"), errVal)
		return
	}
	var data []byte
	for _, chunk := range ws.Get("_fsChunks").Array() {
		data = append(data, dataBytes(chunk)...)
	}
	p := ws.Get("path").String()

	// Write file in goroutine (non-blocking).
	// Register handle to keep event loop alive while goroutine runs.
	eventloop.Default.RegisterHandle()
	go func() {
		defer eventloop.Default.UnregisterHandle()
		if err := os.WriteFile(p, data, 0644); err != nil {
			eventloop.Default.ScheduleCallback(func() {
				ws.MethodCall("emit", jsvalue.NewString("error"), nodeFSError(err, "open", p))
			})
			return
		}
		eventloop.Default.ScheduleCallback(func() {
			ws.MethodCall("emit", jsvalue.NewString("finish"))
		})
	}()
}

func CreateReadStream(path *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	args := []*jsvalue.JSValue{path}
	if len(opts) > 0 {
		args = append(args, opts[0])
	}
	return ReadStream.Call(args...)
}

func CreateWriteStream(path *jsvalue.JSValue, opts ...*jsvalue.JSValue) *jsvalue.JSValue {
	args := []*jsvalue.JSValue{path}
	if len(opts) > 0 {
		args = append(args, opts[0])
	}
	return WriteStream.Call(args...)
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
