package bun

import (
	stdjson "encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	jsonpkg "github.com/nnstd/gun/runtime/builtin/json"
	"github.com/nnstd/gun/runtime/promise"
	"github.com/nnstd/gun/runtime/stream"
	urlpkg "github.com/nnstd/gun/runtime/url"
	"github.com/nnstd/gun/runtime/web"
)

type bunFileState struct {
	path     string
	name     string
	typeName string
	loaded   bool
	data     []byte
	info     os.FileInfo
	mu       sync.Mutex
}

var bunFileStates sync.Map // *jsvalue.JSValue -> *bunFileState

var BunFile = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	pathArg := argAt(args, 0)
	path := bunFilePath(pathArg)
	name := path
	if pathArg != nil && pathArg.TypeString() == "number" {
		name = pathArg.String()
	}
	typeName := bunFileType(path, argAt(args, 1))
	state := &bunFileState{path: path, name: name, typeName: typeName}
	bunFileStates.Store(this, state)
	this.Set("name", jsvalue.NewString(name))
	this.Set("type", jsvalue.NewString(typeName))
	this.Set("size", jsvalue.NewNumber(0))
	this.Set("lastModified", jsvalue.NewNumber(0))
	this.Set("_data", jsvalue.NewString(""))
	return nil
}, nil)

func init() {
	proto := BunFile.Get("prototype")
	proto.Set("exists", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this := argAt(args, 0)
		_, err := os.Stat(bunFileStateFrom(this).path)
		return promise.Promise.Get("resolve").Call(jsvalue.NewBool(err == nil))
	}).MarkAsMethod())
	proto.Set("stat", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this := argAt(args, 0)
		st := bunFileStateFrom(this)
		info, err := os.Stat(st.path)
		if err != nil {
			return promise.Promise.Get("reject").Call(bunFileError(err, "stat", st.path))
		}
		st.updateFromInfo(this, info)
		return promise.Promise.Get("resolve").Call(bunFileStatObject(info))
	}).MarkAsMethod())
	proto.Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data, errVal := bunFileRead(argAt(args, 0))
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		return promise.Promise.Get("resolve").Call(jsvalue.NewString(string(data)))
	}).MarkAsMethod())
	proto.Set("json", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data, errVal := bunFileRead(argAt(args, 0))
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		var decoded any
		if err := stdjson.Unmarshal(data, &decoded); err != nil {
			return promise.Promise.Get("reject").Call(jserror.SyntaxError.Call(jsvalue.NewString("Failed to parse JSON")))
		}
		return promise.Promise.Get("resolve").Call(jsonpkg.Parse(jsvalue.NewString(string(data))))
	}).MarkAsMethod())
	proto.Set("bytes", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data, errVal := bunFileRead(argAt(args, 0))
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		return promise.Promise.Get("resolve").Call(bytesToArray(data))
	}).MarkAsMethod())
	proto.Set("arrayBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data, errVal := bunFileRead(argAt(args, 0))
		if errVal != nil {
			return promise.Promise.Get("reject").Call(errVal)
		}
		return promise.Promise.Get("resolve").Call(bytesToArray(data))
	}).MarkAsMethod())
	proto.Set("slice", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this := argAt(args, 0)
		data, errVal := bunFileRead(this)
		if errVal != nil {
			panic(errVal)
		}
		start, end := sliceBounds(len(data), argAt(args, 1), argAt(args, 2))
		typeName := bunFileStateFrom(this).typeName
		if v := argAt(args, 3); v != nil && v.TypeString() != "undefined" {
			typeName = strings.ToLower(v.String())
		}
		return newMemoryBlob(data[start:end], typeName)
	}).MarkAsMethod())
	proto.Set("stream", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data, errVal := bunFileRead(argAt(args, 0))
		if errVal != nil {
			panic(errVal)
		}
		r := stream.Readable.Call()
		r.MethodCall("push", buffer.Buffer.Get("from").Call(jsvalue.NewString(string(data))))
		return r
	}).MarkAsMethod())
	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this := argAt(args, 0)
		st := bunFileStateFrom(this)
		data := bytesFromWritable(argAt(args, 1))
		if err := os.WriteFile(st.path, data, 0644); err != nil {
			return promise.Promise.Get("reject").Call(bunFileError(err, "open", st.path))
		}
		st.loaded = true
		st.data = append(st.data[:0], data...)
		if info, err := os.Stat(st.path); err == nil {
			st.updateFromInfo(this, info)
		}
		this.Set("_data", jsvalue.NewString(string(data)))
		return promise.Promise.Get("resolve").Call(jsvalue.NewNumber(float64(len(data))))
	}).MarkAsMethod())
	proto.Set("unlink", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		st := bunFileStateFrom(argAt(args, 0))
		if err := os.Remove(st.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return promise.Promise.Get("reject").Call(bunFileError(err, "unlink", st.path))
		}
		return promise.Promise.Get("resolve").Call(jsvalue.NewUndefined())
	}).MarkAsMethod())
	proto.Set("delete", proto.Get("unlink"))
	proto.Set("writer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return newFileSink(argAt(args, 0))
	}).MarkAsMethod())
	proto.Set("formData", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Get("resolve").Call(web.FormData.Call())
	}).MarkAsMethod())
	proto.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString("[object Blob]")
	}).MarkAsMethod())
}

func file(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	return BunFile.Call(args...)
}

func argAt(args []*jsvalue.JSValue, i int) *jsvalue.JSValue {
	if i < 0 || i >= len(args) {
		return nil
	}
	return args[i]
}

func bunFilePath(v *jsvalue.JSValue) string {
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "null" {
		panic(jserror.InvalidArgType("Expected file path string or file descriptor"))
	}
	if v.TypeString() == "number" {
		return "/dev/fd/" + v.String()
	}
	if href := v.Get("href"); href != nil && href.TypeString() == "string" && strings.HasPrefix(href.String(), "file:") {
		return urlpkg.FileURLToPath(v).String()
	}
	s := v.String()
	if strings.HasPrefix(s, "file:") {
		return urlpkg.FileURLToPath(jsvalue.NewString(s)).String()
	}
	if v.TypeString() != "string" {
		panic(jserror.InvalidArgType("Expected file path string or file descriptor"))
	}
	return s
}

func bunFileType(path string, opts *jsvalue.JSValue) string {
	if opts != nil && opts.TypeString() == "object" {
		if typ := opts.Get("type"); typ != nil && typ.TypeString() == "string" {
			return strings.ToLower(typ.String())
		}
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != "" {
		if typ := mime.TypeByExtension(ext); typ != "" {
			return typ
		}
	}
	return ""
}

func bunFileStateFrom(v *jsvalue.JSValue) *bunFileState {
	if v != nil {
		if raw, ok := bunFileStates.Load(v); ok {
			return raw.(*bunFileState)
		}
	}
	return &bunFileState{}
}

func (s *bunFileState) updateFromInfo(this *jsvalue.JSValue, info os.FileInfo) {
	s.info = info
	if this == nil || info == nil {
		return
	}
	this.Set("size", jsvalue.NewNumber(float64(info.Size())))
	this.Set("lastModified", jsvalue.NewNumber(float64(info.ModTime().UnixMilli())))
}

func bunFileRead(this *jsvalue.JSValue) ([]byte, *jsvalue.JSValue) {
	st := bunFileStateFrom(this)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.loaded {
		return append([]byte(nil), st.data...), nil
	}
	data, err := os.ReadFile(st.path)
	if err != nil {
		if info, statErr := os.Stat(st.path); statErr == nil && info.IsDir() {
			return nil, bunFileReadDirectoryError(st.path)
		}
		return nil, bunFileError(err, "open", st.path)
	}
	st.loaded = true
	st.data = append([]byte(nil), data...)
	if info, err := os.Stat(st.path); err == nil {
		st.updateFromInfo(this, info)
	}
	if this != nil {
		this.Set("_data", jsvalue.NewString(string(data)))
	}
	return append([]byte(nil), data...), nil
}

func bunFileStatObject(info os.FileInfo) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("size", jsvalue.NewNumber(float64(info.Size())))
	obj.Set("mtimeMs", jsvalue.NewNumber(float64(info.ModTime().UnixMilli())))
	obj.Set("birthtimeMs", jsvalue.NewNumber(float64(info.ModTime().UnixMilli())))
	obj.Set("isFile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewBool(!info.IsDir())
	}))
	obj.Set("isDirectory", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewBool(info.IsDir())
	}))
	return obj
}

func bytesToArray(data []byte) *jsvalue.JSValue {
	elems := make([]*jsvalue.JSValue, len(data))
	for i, b := range data {
		elems[i] = jsvalue.NewNumber(float64(b))
	}
	out := jsvalue.NewArray(elems...)
	out.Set("byteLength", jsvalue.NewNumber(float64(len(data))))
	out.Set("length", jsvalue.NewNumber(float64(len(data))))
	return out
}

func bytesFromWritable(v *jsvalue.JSValue) []byte {
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "null" {
		return nil
	}
	if jsvalue.InstanceOf(v, buffer.Buffer).Bool() {
		if bs := v.ByteSliceData(); bs != nil {
			return append([]byte{}, v.Bytes()...)
		}
		return []byte(v.Get("_data").String())
	}
	if jsvalue.InstanceOf(v, BunFile).Bool() {
		data, errVal := bunFileRead(v)
		if errVal != nil {
			panic(errVal)
		}
		return data
	}
	if text := v.Get("text"); text != nil && text.TypeString() == "function" {
		return []byte(promise.Await(text.Call()).String())
	}
	if v.IsArray() {
		out := make([]byte, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = byte(v.Index(i).Number())
		}
		return out
	}
	return []byte(v.String())
}

func sliceBounds(length int, rawStart, rawEnd *jsvalue.JSValue) (int, int) {
	start := 0
	end := length
	if rawStart != nil && rawStart.TypeString() != "undefined" {
		start = int(rawStart.Number())
		if start < 0 {
			start = length + start
		}
	}
	if rawEnd != nil && rawEnd.TypeString() != "undefined" {
		end = int(rawEnd.Number())
		if end < 0 {
			end = length + end
		}
	}
	if start < 0 {
		start = 0
	}
	if start > length {
		start = length
	}
	if end < 0 {
		end = 0
	}
	if end > length {
		end = length
	}
	if end < start {
		end = start
	}
	return start, end
}

func newMemoryBlob(data []byte, typeName string) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("size", jsvalue.NewNumber(float64(len(data))))
	obj.Set("type", jsvalue.NewString(typeName))
	obj.Set("_data", jsvalue.NewString(string(data)))
	obj.Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Get("resolve").Call(jsvalue.NewString(string(data)))
	}))
	obj.Set("bytes", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Get("resolve").Call(bytesToArray(data))
	}))
	obj.Set("arrayBuffer", obj.Get("bytes"))
	obj.Set("slice", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		start, end := sliceBounds(len(data), argAt(args, 0), argAt(args, 1))
		return newMemoryBlob(data[start:end], typeName)
	}))
	return obj
}

func newFileSink(file *jsvalue.JSValue) *jsvalue.JSValue {
	chunks := jsvalue.NewArray()
	sink := jsvalue.NewObject()
	sink.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if v := argAt(args, 0); v != nil {
			chunks.MethodCall("push", jsvalue.NewString(string(bytesFromWritable(v))))
		}
		return jsvalue.NewNumber(float64(chunks.Len()))
	}))
	sink.Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if v := argAt(args, 0); v != nil {
			chunks.MethodCall("push", jsvalue.NewString(string(bytesFromWritable(v))))
		}
		var b strings.Builder
		for _, chunk := range chunks.Array() {
			b.WriteString(chunk.String())
		}
		return file.MethodCall("write", jsvalue.NewString(b.String()))
	}))
	sink.Set("flush", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))
	sink.Set("close", sink.Get("end"))
	return sink
}

func bunFileError(err error, syscallName, path string) *jsvalue.JSValue {
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
	}
	msg := code + ": " + err.Error() + ", " + syscallName + " '" + path + "'"
	errVal := jserror.Error.Call(jsvalue.NewString(msg))
	errVal.Set("code", jsvalue.NewString(code))
	errVal.Set("errno", jsvalue.NewNumber(float64(-errno)))
	errVal.Set("syscall", jsvalue.NewString(syscallName))
	errVal.Set("path", jsvalue.NewString(path))
	if runtime.GOOS == "windows" {
		errVal.Set("path", jsvalue.NewString(filepath.ToSlash(path)))
	}
	return errVal
}

func bunFileReadDirectoryError(path string) *jsvalue.JSValue {
	errVal := jserror.Error.Call(jsvalue.NewString("Directories cannot be read like files"))
	errVal.Set("code", jsvalue.NewString("EISDIR"))
	errVal.Set("errno", jsvalue.NewNumber(0))
	errVal.Set("syscall", jsvalue.NewString("read"))
	errVal.Set("path", jsvalue.NewString(path))
	return errVal
}
