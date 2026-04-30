package ffi

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	"github.com/nnstd/gun/runtime/internal/tinycc"
	"github.com/nnstd/gun/runtime/promise"
)

const (
	typeChar = iota
	typeI8
	typeU8
	typeI16
	typeU16
	typeI32
	typeU32
	typeI64
	typeU64
	typeDouble
	typeFloat
	typeBool
	typePtr
	typeVoid
	typeCString
	typeI64Fast
	typeU64Fast
	typeFunction
	typeNapiEnv
	typeNapiValue
	typeBuffer
)

var (
	CStringCtor    *jsvalue.JSValue
	JSCallbackCtor *jsvalue.JSValue
	AsJSValue      *jsvalue.JSValue
	memcpyOnce     sync.Once
	memcpyFn       func(uintptr, uintptr, uintptr) uintptr
)

type libraryState struct {
	handle uintptr
	tinycc *tinycc.Context
	closed bool
}

type ffiFunction struct {
	ptr     uintptr
	args    []int
	returns int
	lib     *libraryState
	closed  bool
}

func init() {
	CStringCtor = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ptr := uintptrArg(argAt(args, 0))
		offset := int(numberArg(argAt(args, 1)))
		lengthVal := argAt(args, 2)
		length := -1
		if lengthVal != nil && lengthVal.TypeString() != "undefined" {
			length = int(lengthVal.Number())
		}
		text := readCString(ptr, offset, length)
		this.Set("ptr", jsvalue.NewNumber(float64(ptr)))
		this.Set("length", jsvalue.NewNumber(float64(len([]rune(text)))))
		this.Set("_value", jsvalue.NewString(text))
		return nil
	}, nil)
	CStringCtor.Get("prototype").Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(cstringValue(argAt(args, 0)))
	}).MarkAsMethod())
	CStringCtor.Get("prototype").Set("valueOf", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(cstringValue(argAt(args, 0)))
	}).MarkAsMethod())
	CStringCtor.Get("prototype").Set("charAt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return charAt(cstringValue(argAt(args, 0)), int(numberArg(argAt(args, 1))), false)
	}).MarkAsMethod())
	CStringCtor.Get("prototype").Set("at", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return charAt(cstringValue(argAt(args, 0)), int(numberArg(argAt(args, 1))), true)
	}).MarkAsMethod())

	JSCallbackCtor = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		threadsafe := false
		if opts := argAt(args, 1); opts != nil && opts.TypeString() == "object" {
			threadsafe = opts.Get("threadsafe").Bool()
		}
		this.Set("ptr", jsvalue.NewNull())
		this.Set("threadsafe", jsvalue.NewBool(threadsafe))
		this.Set("_callback", argAt(args, 0))
		return nil
	}, nil)
	JSCallbackCtor.Get("prototype").Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if this := argAt(args, 0); this != nil {
			this.Set("ptr", jsvalue.NewNull())
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())

	AsJSValue = jsvalue.NewObject()
	AsJSValue.Set("FFIType", ffiTypeObject())
	AsJSValue.Set("suffix", jsvalue.NewString(librarySuffix()))
	AsJSValue.Set("CString", CStringCtor)
	AsJSValue.Set("JSCallback", JSCallbackCtor)
	AsJSValue.Set("CFunction", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		fn := makeFunctionFromDescriptor(argAt(args, 0), nil)
		fn.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if state, ok := fn.Get("_ffiState").String(), true; ok && state != "" {
				_ = state
			}
			return jsvalue.NewUndefined()
		}))
		return fn
	}))
	AsJSValue.Set("cc", jsvalue.NewFunction(cc))
	AsJSValue.Set("dlopen", jsvalue.NewFunction(dlopen))
	AsJSValue.Set("linkSymbols", jsvalue.NewFunction(linkSymbols))
	AsJSValue.Set("ptr", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(float64(ptrFromView(argAt(args, 0), int(numberArg(argAt(args, 1))))))
	}))
	AsJSValue.Set("toArrayBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return bytesToArray(readBytes(uintptrArg(argAt(args, 0)), int(numberArg(argAt(args, 1))), lengthArg(argAt(args, 2))))
	}))
	AsJSValue.Set("toBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		data := string(readBytes(uintptrArg(argAt(args, 0)), int(numberArg(argAt(args, 1))), lengthArg(argAt(args, 2))))
		return buffer.Buffer.Get("from").Call(jsvalue.NewString(data))
	}))
	AsJSValue.Set("read", readObject())
	AsJSValue.Set("viewSource", jsvalue.NewFunction(viewSource))
}

func dlopen(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	name := ""
	if v := argAt(args, 0); v != nil {
		name = v.String()
	}
	handle, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}
	state := &libraryState{handle: handle}
	return buildLibrary(state, argAt(args, 1), false)
}

func linkSymbols(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	return buildLibrary(nil, argAt(args, 0), true)
}

func cc(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	options := argAt(args, 0)
	if options == nil || options.TypeString() != "object" {
		panic(jserror.InvalidArgType("bun:ffi cc options must be an object"))
	}
	ctx, err := tinycc.NewContext()
	if err != nil {
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}
	state := &libraryState{tinycc: ctx}

	if err := ctx.SetOutputMode(tinycc.OutputToMemory); err != nil {
		ctx.Close()
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}
	applyCCOptions(ctx, options)
	source := ccSource(options.Get("source"))
	if strings.TrimSpace(source) == "" {
		ctx.Close()
		panic(jserror.InvalidArgType("bun:ffi cc options.source must not be empty"))
	}
	if err := ctx.CompileString(source); err != nil {
		ctx.Close()
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}
	if err := ctx.Relocate(); err != nil {
		ctx.Close()
		panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
	}
	return buildLibrary(state, options.Get("symbols"), false)
}

func applyCCOptions(ctx *tinycc.Context, options *jsvalue.JSValue) {
	for _, item := range jsStringList(options.Get("include")) {
		if err := ctx.AddIncludePath(item); err != nil {
			panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
		}
	}
	for _, item := range jsStringList(options.Get("library")) {
		if err := ctx.AddLibrary(item); err != nil {
			panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
		}
	}
	for _, item := range jsStringList(options.Get("libraryPath")) {
		if err := ctx.AddLibraryPath(item); err != nil {
			panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
		}
	}
	if flags := options.Get("flags"); flags != nil {
		for _, item := range jsStringList(flags) {
			if strings.HasPrefix(item, "-I") && len(item) > 2 {
				if err := ctx.AddIncludePath(item[2:]); err != nil {
					panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
				}
			}
			if strings.HasPrefix(item, "-L") && len(item) > 2 {
				if err := ctx.AddLibraryPath(item[2:]); err != nil {
					panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
				}
			}
			if strings.HasPrefix(item, "-l") && len(item) > 2 {
				if err := ctx.AddLibrary(item[2:]); err != nil {
					panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
				}
			}
		}
	}
	if defs := options.Get("define"); defs != nil && defs.TypeString() == "object" {
		for _, key := range defs.OwnKeys() {
			if err := ctx.DefineSymbol(key, defs.Get(key).String()); err != nil {
				panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
			}
		}
	}
}

func ccSource(v *jsvalue.JSValue) string {
	if v == nil || v.TypeString() == "undefined" {
		return ""
	}
	if text := v.Get("text"); text != nil && text.TypeString() == "function" {
		p := v.MethodCall("text")
		value := promise.Await(p)
		if promise.IsRejected(p) {
			panic(value)
		}
		return value.String()
	}
	return v.String()
}

func jsStringList(v *jsvalue.JSValue) []string {
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "object" && v.String() == "null" {
		return nil
	}
	if v.IsArray() {
		out := make([]string, 0, v.Len())
		for _, item := range v.Array() {
			out = append(out, item.String())
		}
		return out
	}
	return []string{v.String()}
}

func buildLibrary(state *libraryState, symbols *jsvalue.JSValue, linked bool) *jsvalue.JSValue {
	lib := jsvalue.NewObject()
	symObj := jsvalue.NewObject()
	if symbols != nil && symbols.TypeString() == "object" {
		for _, name := range symbols.OwnKeys() {
			desc := symbols.Get(name)
			if desc == nil || desc.TypeString() != "object" {
				continue
			}
			if linked {
				symObj.Set(name, makeFunctionFromDescriptor(desc, nil))
				continue
			}
			ptr, err := lookupSymbol(state, name)
			if err != nil {
				panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
			}
			copyDesc := cloneDescriptor(desc)
			copyDesc.Set("ptr", jsvalue.NewNumber(float64(ptr)))
			symObj.Set(name, makeFunctionFromDescriptor(copyDesc, state))
		}
	}
	lib.Set("symbols", symObj)
	lib.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if state != nil && !state.closed {
			state.closed = true
			if state.tinycc != nil {
				state.tinycc.Close()
				return jsvalue.NewUndefined()
			}
			if state.handle != 0 {
				if err := purego.Dlclose(state.handle); err != nil {
					panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
				}
			}
		}
		return jsvalue.NewUndefined()
	}))
	return lib
}

func lookupSymbol(state *libraryState, name string) (uintptr, error) {
	if state == nil {
		return 0, fmt.Errorf("bun:ffi library is closed")
	}
	if state.tinycc != nil {
		return state.tinycc.Symbol(name)
	}
	return purego.Dlsym(state.handle, name)
}

func makeFunctionFromDescriptor(desc *jsvalue.JSValue, lib *libraryState) *jsvalue.JSValue {
	if desc == nil || desc.TypeString() != "object" {
		panic(jserror.InvalidArgType("bun:ffi function descriptor must be an object"))
	}
	fn := &ffiFunction{ptr: uintptrArg(desc.Get("ptr")), args: parseArgs(desc.Get("args")), returns: parseType(desc.Get("returns")), lib: lib}
	if fn.ptr == 0 {
		panic(jserror.InvalidArgType("bun:ffi function descriptor requires a non-zero ptr"))
	}
	callable := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return fn.call(args...)
	})
	callable.Set("ptr", jsvalue.NewNumber(float64(fn.ptr)))
	callable.Set("close", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		fn.closed = true
		return jsvalue.NewUndefined()
	}))
	return callable
}

func (fn *ffiFunction) call(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if fn.closed || (fn.lib != nil && fn.lib.closed) {
		panic(jserror.TypeError.Call(jsvalue.NewString("bun:ffi function is closed")))
	}
	argv := make([]uintptr, len(fn.args))
	keepAlive := make([]any, 0, len(fn.args))
	for i, typ := range fn.args {
		v := argAt(args, i)
		argv[i] = marshalArg(v, typ, &keepAlive)
	}
	r1, _, _ := purego.SyscallN(fn.ptr, argv...)
	runtime.KeepAlive(keepAlive)
	return unmarshalReturn(r1, fn.returns)
}

func marshalArg(v *jsvalue.JSValue, typ int, keepAlive *[]any) uintptr {
	switch typ {
	case typeBool:
		if v.Bool() {
			return 1
		}
		return 0
	case typeFloat:
		return uintptr(math.Float32bits(float32(numberArg(v))))
	case typeDouble:
		return uintptr(math.Float64bits(numberArg(v)))
	case typeCString:
		fallthrough
	case typePtr, typeFunction, typeBuffer:
		return pointerLike(v, keepAlive)
	default:
		if v != nil && v.TypeString() == "bigint" {
			return uintptr(v.BigInt())
		}
		return uintptr(int64(numberArg(v)))
	}
}

func unmarshalReturn(r uintptr, typ int) *jsvalue.JSValue {
	switch typ {
	case typeVoid:
		return jsvalue.NewUndefined()
	case typeBool:
		return jsvalue.NewBool(r != 0)
	case typeCString:
		return CStringCtor.New(jsvalue.NewNumber(float64(r)))
	case typeI64, typeU64:
		return jsvalue.NewBigInt(int64(r))
	case typeFloat:
		return jsvalue.NewNumber(float64(math.Float32frombits(uint32(r))))
	case typeDouble:
		return jsvalue.NewNumber(math.Float64frombits(uint64(r)))
	case typePtr, typeFunction:
		if r == 0 {
			return jsvalue.NewNull()
		}
		return jsvalue.NewNumber(float64(r))
	default:
		return jsvalue.NewNumber(float64(int64(r)))
	}
}

func readObject() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for name, typ := range map[string]int{"u8": typeU8, "i8": typeI8, "u16": typeU16, "i16": typeI16, "u32": typeU32, "i32": typeI32, "u64": typeU64, "i64": typeI64, "ptr": typePtr, "intptr": typePtr, "f32": typeFloat, "f64": typeDouble} {
		t := typ
		obj.Set(name, jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return readPrimitive(uintptrArg(argAt(args, 0)), int(numberArg(argAt(args, 1))), t)
		}))
	}
	return obj
}

func readPrimitive(ptr uintptr, offset int, typ int) *jsvalue.JSValue {
	if ptr == 0 {
		panic(jserror.TypeError.Call(jsvalue.NewString("bun:ffi cannot read from null pointer")))
	}
	b := readBytes(ptr, offset, primitiveSize(typ))
	switch typ {
	case typeU8:
		return jsvalue.NewNumber(float64(b[0]))
	case typeI8:
		return jsvalue.NewNumber(float64(int8(b[0])))
	case typeU16:
		return jsvalue.NewNumber(float64(nativeEndian().Uint16(b)))
	case typeI16:
		return jsvalue.NewNumber(float64(int16(nativeEndian().Uint16(b))))
	case typeU32:
		return jsvalue.NewNumber(float64(nativeEndian().Uint32(b)))
	case typeI32:
		return jsvalue.NewNumber(float64(int32(nativeEndian().Uint32(b))))
	case typeU64:
		return jsvalue.NewBigInt(int64(nativeEndian().Uint64(b)))
	case typeI64:
		return jsvalue.NewBigInt(int64(nativeEndian().Uint64(b)))
	case typeFloat:
		return jsvalue.NewNumber(float64(math.Float32frombits(nativeEndian().Uint32(b))))
	case typeDouble:
		return jsvalue.NewNumber(math.Float64frombits(nativeEndian().Uint64(b)))
	default:
		if unsafe.Sizeof(uintptr(0)) == 4 {
			return jsvalue.NewNumber(float64(nativeEndian().Uint32(b)))
		}
		return jsvalue.NewNumber(float64(nativeEndian().Uint64(b)))
	}
}

func primitiveSize(typ int) int {
	switch typ {
	case typeU8, typeI8:
		return 1
	case typeU16, typeI16:
		return 2
	case typeU32, typeI32, typeFloat:
		return 4
	case typeU64, typeI64, typeDouble:
		return 8
	default:
		return int(unsafe.Sizeof(uintptr(0)))
	}
}

func ptrFromView(v *jsvalue.JSValue, offset int) uintptr {
	if v == nil {
		return 0
	}
	if data := v.Get("_data"); data != nil && data.TypeString() == "string" {
		b := []byte(data.String())
		if offset >= len(b) {
			return 0
		}
		return uintptr(unsafe.Pointer(&b[offset]))
	}
	if v.IsArray() {
		b := bytesFromArray(v)
		if offset >= len(b) {
			return 0
		}
		return uintptr(unsafe.Pointer(&b[offset]))
	}
	return uintptrArg(v) + uintptr(offset)
}

func pointerLike(v *jsvalue.JSValue, keepAlive *[]any) uintptr {
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "object" && v.String() == "null" {
		return 0
	}
	if v.TypeString() == "string" {
		b := append([]byte(v.String()), 0)
		*keepAlive = append(*keepAlive, b)
		return uintptr(unsafe.Pointer(&b[0]))
	}
	if data := v.Get("_value"); data != nil && data.TypeString() == "string" {
		b := append([]byte(data.String()), 0)
		*keepAlive = append(*keepAlive, b)
		return uintptr(unsafe.Pointer(&b[0]))
	}
	return ptrFromView(v, 0)
}

func readBytes(ptr uintptr, offset int, length int) []byte {
	if ptr == 0 {
		return nil
	}
	if length < 0 {
		var out []byte
		for i := 0; ; i++ {
			b := copyBytes(ptr, offset+i, 1)[0]
			if b == 0 {
				return out
			}
			out = append(out, b)
		}
	}
	return copyBytes(ptr, offset, length)
}

func copyBytes(ptr uintptr, offset int, length int) []byte {
	if length <= 0 {
		return nil
	}
	out := make([]byte, length)
	memcpy := loadMemcpy()
	memcpy(uintptr(unsafe.Pointer(&out[0])), ptr+uintptr(offset), uintptr(length))
	runtime.KeepAlive(out)
	return out
}

func loadMemcpy() func(uintptr, uintptr, uintptr) uintptr {
	memcpyOnce.Do(func() {
		handle, err := purego.Dlopen(libcName(), purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(jserror.TypeError.Call(jsvalue.NewString(err.Error())))
		}
		for _, name := range []string{"memcpy", "memmove"} {
			if sym, err := purego.Dlsym(handle, name); err == nil {
				purego.RegisterFunc(&memcpyFn, sym)
				return
			}
		}
		panic(jserror.TypeError.Call(jsvalue.NewString("bun:ffi could not resolve memcpy")))
	})
	return memcpyFn
}

func libcName() string {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/lib/libSystem.B.dylib"
	case "windows":
		return "msvcrt.dll"
	default:
		return "libc.so.6"
	}
}

func readCString(ptr uintptr, offset int, length int) string {
	return string(readBytes(ptr, offset, length))
}

func ffiTypeObject() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for name, val := range map[string]int{"char": typeChar, "i8": typeI8, "int8_t": typeI8, "u8": typeU8, "uint8_t": typeU8, "i16": typeI16, "int16_t": typeI16, "u16": typeU16, "uint16_t": typeU16, "i32": typeI32, "int": typeI32, "int32_t": typeI32, "u32": typeU32, "uint32_t": typeU32, "i64": typeI64, "int64_t": typeI64, "u64": typeU64, "uint64_t": typeU64, "double": typeDouble, "f64": typeDouble, "float": typeFloat, "f32": typeFloat, "bool": typeBool, "ptr": typePtr, "pointer": typePtr, "void": typeVoid, "cstring": typeCString, "i64_fast": typeI64Fast, "u64_fast": typeU64Fast, "function": typeFunction, "napi_env": typeNapiEnv, "napi_value": typeNapiValue, "buffer": typeBuffer} {
		obj.Set(name, jsvalue.NewNumber(float64(val)))
	}
	return obj
}

func parseArgs(v *jsvalue.JSValue) []int {
	if v == nil || !v.IsArray() {
		return nil
	}
	out := make([]int, 0, v.Len())
	for _, item := range v.Array() {
		out = append(out, parseType(item))
	}
	return out
}

func parseType(v *jsvalue.JSValue) int {
	if v == nil || v.TypeString() == "undefined" {
		return typeVoid
	}
	if v.TypeString() == "number" {
		return int(v.Number())
	}
	return typeByName(v.String())
}

func typeByName(name string) int {
	switch strings.ToLower(name) {
	case "char":
		return typeChar
	case "i8", "int8_t":
		return typeI8
	case "u8", "uint8_t":
		return typeU8
	case "i16", "int16_t":
		return typeI16
	case "u16", "uint16_t":
		return typeU16
	case "i32", "int", "int32_t":
		return typeI32
	case "u32", "uint32_t":
		return typeU32
	case "i64", "int64_t":
		return typeI64
	case "u64", "uint64_t", "usize":
		return typeU64
	case "double", "f64":
		return typeDouble
	case "float", "f32":
		return typeFloat
	case "bool":
		return typeBool
	case "ptr", "pointer", "callback":
		return typePtr
	case "cstring":
		return typeCString
	case "i64_fast":
		return typeI64Fast
	case "u64_fast":
		return typeU64Fast
	case "function":
		return typeFunction
	case "napi_env":
		return typeNapiEnv
	case "napi_value":
		return typeNapiValue
	case "buffer":
		return typeBuffer
	default:
		return typeVoid
	}
}

func viewSource(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) > 1 && args[1].Bool() {
		return jsvalue.NewString("/* bun:ffi binding generated by Gun via purego */")
	}
	return jsvalue.NewArray(jsvalue.NewString("/* bun:ffi bindings generated by Gun via purego */"))
}

func cloneDescriptor(desc *jsvalue.JSValue) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for _, key := range desc.OwnKeys() {
		obj.Set(key, desc.Get(key))
	}
	return obj
}

func cstringValue(this *jsvalue.JSValue) string {
	if this == nil {
		return ""
	}
	if v := this.Get("_value"); v != nil {
		return v.String()
	}
	return this.String()
}

func charAt(s string, idx int, relative bool) *jsvalue.JSValue {
	r := []rune(s)
	if relative && idx < 0 {
		idx = len(r) + idx
	}
	if idx < 0 || idx >= len(r) {
		if relative {
			return jsvalue.NewUndefined()
		}
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(string(r[idx]))
}

func bytesToArray(b []byte) *jsvalue.JSValue {
	elems := make([]*jsvalue.JSValue, len(b))
	for i, v := range b {
		elems[i] = jsvalue.NewNumber(float64(v))
	}
	return jsvalue.NewArray(elems...)
}

func bytesFromArray(v *jsvalue.JSValue) []byte {
	b := make([]byte, v.Len())
	for i := 0; i < v.Len(); i++ {
		b[i] = byte(v.Index(i).Number())
	}
	return b
}

func argAt(args []*jsvalue.JSValue, i int) *jsvalue.JSValue {
	if i < 0 || i >= len(args) || args[i] == nil {
		return jsvalue.NewUndefined()
	}
	return args[i]
}

func numberArg(v *jsvalue.JSValue) float64 {
	if v == nil || v.TypeString() == "undefined" {
		return 0
	}
	return v.Number()
}

func uintptrArg(v *jsvalue.JSValue) uintptr {
	if v == nil || v.TypeString() == "undefined" {
		return 0
	}
	if v.TypeString() == "bigint" {
		return uintptr(v.BigInt())
	}
	return uintptr(uint64(v.Number()))
}

func lengthArg(v *jsvalue.JSValue) int {
	if v == nil || v.TypeString() == "undefined" {
		return -1
	}
	return int(v.Number())
}

func librarySuffix() string {
	switch runtime.GOOS {
	case "darwin":
		return "dylib"
	case "windows":
		return "dll"
	default:
		return "so"
	}
}

func nativeEndian() binary.ByteOrder {
	var x uint16 = 0x0102
	if *(*byte)(unsafe.Pointer(&x)) == 0x02 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func init() {
	_ = fmt.Sprintf
	_ = nativeEndian
}
