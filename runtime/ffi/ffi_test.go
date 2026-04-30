package ffi

import (
	"runtime"
	"testing"
	"unsafe"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/promise"
)

func TestExportsBunFFISurface(t *testing.T) {
	for _, name := range []string{"dlopen", "linkSymbols", "CFunction", "CString", "JSCallback", "FFIType", "suffix", "cc", "ptr", "toArrayBuffer", "toBuffer", "read", "viewSource"} {
		if got := AsJSValue.Get(name); got == nil || got.TypeString() == "undefined" {
			t.Fatalf("missing export %s", name)
		}
	}
	if got := AsJSValue.Get("FFIType").Get("i32").Number(); got != typeI32 {
		t.Fatalf("FFIType.i32 = %v", got)
	}
}

func TestCCCompilesAndLinksSymbol(t *testing.T) {
	lib := AsJSValue.Get("cc").Call(jsvalue.ObjectFrom(
		"source", jsvalue.NewString("int add(int a, int b) { return a + b; }"),
		"symbols", jsvalue.ObjectFrom(
			"add", jsvalue.ObjectFrom("args", jsvalue.NewArray(jsvalue.NewString("i32"), jsvalue.NewString("i32")), "returns", jsvalue.NewString("i32")),
		),
	))
	defer lib.MethodCall("close")
	if got := lib.Get("symbols").Get("add").Call(jsvalue.NewNumber(2), jsvalue.NewNumber(5)).Number(); got != 7 {
		t.Fatalf("cc add(2, 5) = %v", got)
	}
}

func TestCCSourceAwaitsThisBoundTextMethod(t *testing.T) {
	source := jsvalue.ObjectFrom("_source", jsvalue.NewString("int seven(void) { return 7; }"))
	source.Set("text", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return promise.Promise.Get("resolve").Call(args[0].Get("_source"))
	}).MarkAsMethod())
	if got := ccSource(source); got != "int seven(void) { return 7; }" {
		t.Fatalf("ccSource() = %q", got)
	}
}

func TestCStringClonesPointerString(t *testing.T) {
	data := []byte("hello\x00ignored")
	ptr := jsvalue.NewNumber(float64(uintptr(unsafe.Pointer(&data[0]))))
	cstr := CStringCtor.New(ptr)
	if got := cstr.MethodCall("toString").String(); got != "hello" {
		t.Fatalf("CString = %q", got)
	}
	if got := cstr.Get("length").Number(); got != 5 {
		t.Fatalf("CString.length = %v", got)
	}
	data[0] = 'j'
	if got := cstr.MethodCall("toString").String(); got != "hello" {
		t.Fatalf("CString did not clone data: %q", got)
	}
}

func TestReadHelpers(t *testing.T) {
	data := []byte{0x34, 0x12, 0, 0, 0xff}
	ptr := jsvalue.NewNumber(float64(uintptr(unsafe.Pointer(&data[0]))))
	if got := AsJSValue.Get("read").Get("u16").Call(ptr).Number(); got != 0x1234 {
		t.Fatalf("read.u16 = %v", got)
	}
	if got := AsJSValue.Get("read").Get("i8").Call(ptr, jsvalue.NewNumber(4)).Number(); got != -1 {
		t.Fatalf("read.i8 = %v", got)
	}
}

func TestDlopenCanCallLibcAbs(t *testing.T) {
	var lib string
	switch runtime.GOOS {
	case "darwin":
		lib = "/usr/lib/libSystem.B.dylib"
	case "linux":
		lib = "libc.so.6"
	default:
		t.Skipf("no libc fixture for %s", runtime.GOOS)
	}
	ffiLib := AsJSValue.Get("dlopen").Call(jsvalue.NewString(lib), jsvalue.ObjectFrom(
		"abs", jsvalue.ObjectFrom("args", jsvalue.NewArray(jsvalue.NewString("i32")), "returns", jsvalue.NewString("i32")),
	))
	defer ffiLib.MethodCall("close")
	if got := ffiLib.Get("symbols").Get("abs").Call(jsvalue.NewNumber(-7)).Number(); got != 7 {
		t.Fatalf("abs(-7) = %v", got)
	}
}
