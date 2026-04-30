package nodetty

import (
	"os"
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func skipIfNotTTY(t *testing.T) {
	t.Helper()
	if !isatty(0) {
		t.Skip("skipping: stdin is not a TTY")
	}
}

func TestIsatty(t *testing.T) {
	// Pipe fd should return false
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isatty(int(r.Fd())) {
		t.Error("pipe fd should not be a tty")
	}
	// Invalid fd
	if isatty(999) {
		t.Error("invalid fd should not be a tty")
	}
}

func TestIsattyFunction(t *testing.T) {
	isattyFn := AsJSValue.Get("isatty")
	if isattyFn == nil || isattyFn.TypeString() != "function" {
		t.Fatal("isatty not found on AsJSValue")
	}
	// isatty(0)
	result := isattyFn.Call(jsvalue.NewNumber(0))
	if result.TypeString() != "boolean" {
		t.Fatalf("expected boolean, got %s", result.TypeString())
	}
	// isatty(undefined) returns false
	result = isattyFn.Call(jsvalue.NewUndefined())
	if result.Bool() != false {
		t.Error("isatty(undefined) should return false")
	}
	// isatty() with no args returns false
	result = isattyFn.Call()
	if result.Bool() != false {
		t.Error("isatty() should return false")
	}
}

func TestReadStream(t *testing.T) {
	rs := ReadStream.Call(jsvalue.NewNumber(0))
	if rs == nil {
		t.Fatal("ReadStream returned nil")
	}
	if rs.Get("isTTY").Bool() != true {
		t.Error("isTTY should be true")
	}
	if rs.Get("isRaw").Bool() != false {
		t.Error("isRaw should be false")
	}
	fd := rs.Get("_fd")
	if fd == nil || fd.Number() != 0 {
		t.Errorf("expected _fd=0, got %v", fd)
	}
}

func TestWriteStream(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	if ws == nil {
		t.Fatal("WriteStream returned nil")
	}
	if ws.Get("isTTY").Bool() != true {
		t.Error("isTTY should be true")
	}
	cols := ws.Get("columns").Number()
	rows := ws.Get("rows").Number()
	if cols <= 0 || rows <= 0 {
		t.Errorf("expected positive dimensions, got cols=%v rows=%v", cols, rows)
	}
	fd := ws.Get("_fd")
	if fd == nil || fd.Number() != 2 {
		t.Errorf("expected _fd=2, got %v", fd)
	}
}

func TestReadStreamNoFd(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing fd")
		}
		errVal, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("expected *JSValue panic, got %T: %v", r, r)
		}
		msg := errVal.String()
		if msg == "" {
			t.Fatalf("expected error message, got: %v", errVal)
		}
	}()
	ReadStream.Call()
}

func TestWriteStreamBadFd(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for bad fd")
		}
	}()
	WriteStream.Call(jsvalue.NewString("bad"))
}

func TestSetRawMode(t *testing.T) {
	skipIfNotTTY(t)
	rs := ReadStream.Call(jsvalue.NewNumber(0))
	result := rs.MethodCall("setRawMode", jsvalue.NewBool(true))
	if result == nil || result != rs {
		t.Error("setRawMode should return this")
	}
	if rs.Get("isRaw").Bool() != true {
		t.Error("isRaw should be true after setRawMode(true)")
	}
	rs.MethodCall("setRawMode", jsvalue.NewBool(false))
	if rs.Get("isRaw").Bool() != false {
		t.Error("isRaw should be false after setRawMode(false)")
	}
}

func TestGetWindowSize(t *testing.T) {
	skipIfNotTTY(t)
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	arr := ws.MethodCall("getWindowSize")
	if arr == nil {
		t.Fatal("getWindowSize returned nil")
	}
	if arr.TypeString() != "object" {
		t.Fatalf("expected array, got %s", arr.TypeString())
	}
	cols := arr.MethodCall("shift").Number()
	rows := arr.MethodCall("shift").Number()
	if cols <= 0 || rows <= 0 {
		t.Errorf("expected positive dimensions, got cols=%v rows=%v", cols, rows)
	}
}

func TestGetColorDepth(t *testing.T) {
	// Test with FORCE_COLOR env var
	env := jsvalue.ObjectFrom(
		"FORCE_COLOR", jsvalue.NewString("2"),
	)
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	depth := ws.MethodCall("getColorDepth", env)
	if depth.Number() != 8 {
		t.Errorf("expected depth 8 for FORCE_COLOR=2, got %v", depth.Number())
	}
	// FORCE_COLOR=3
	env = jsvalue.ObjectFrom("FORCE_COLOR", jsvalue.NewString("3"))
	depth = ws.MethodCall("getColorDepth", env)
	if depth.Number() != 24 {
		t.Errorf("expected depth 24 for FORCE_COLOR=3, got %v", depth.Number())
	}
	// NO_COLOR
	env = jsvalue.ObjectFrom("NO_COLOR", jsvalue.NewString("1"))
	depth = ws.MethodCall("getColorDepth", env)
	if depth.Number() != 1 {
		t.Errorf("expected depth 1 for NO_COLOR, got %v", depth.Number())
	}
	// FORCE_COLOR=0 → falls through to default → 1
	env = jsvalue.ObjectFrom("FORCE_COLOR", jsvalue.NewString("0"))
	depth = ws.MethodCall("getColorDepth", env)
	if depth.Number() != 1 {
		t.Errorf("expected depth 1 for FORCE_COLOR=0, got %v", depth.Number())
	}
	// Unrecognized FORCE_COLOR value → 1
	env = jsvalue.ObjectFrom("FORCE_COLOR", jsvalue.NewString("abc"))
	depth = ws.MethodCall("getColorDepth", env)
	if depth.Number() != 1 {
		t.Errorf("expected depth 1 for FORCE_COLOR=abc, got %v", depth.Number())
	}
	// Default (no env overrides)
	depth = ws.MethodCall("getColorDepth")
	if depth.Number() < 1 {
		t.Errorf("expected depth >= 1, got %v", depth.Number())
	}
}

func TestHasColors(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	result := ws.MethodCall("hasColors")
	if result.TypeString() != "boolean" {
		t.Fatalf("expected boolean, got %s", result.TypeString())
	}
	// hasColors with count
	result = ws.MethodCall("hasColors", jsvalue.NewNumber(256))
	if result.TypeString() != "boolean" {
		t.Fatalf("expected boolean, got %s", result.TypeString())
	}
	// hasColors with env forcing low depth
	env := jsvalue.ObjectFrom("NO_COLOR", jsvalue.NewString("1"))
	result = ws.MethodCall("hasColors", jsvalue.NewNumber(256), env)
	if result.Bool() != false {
		t.Error("hasColors(256) should be false with NO_COLOR")
	}
}

func TestClearLine(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	result := ws.MethodCall("clearLine", jsvalue.NewNumber(0))
	if result.Bool() != true {
		t.Error("clearLine should return true")
	}
	// With callback
	called := false
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called = true
		return nil
	})
	ws.MethodCall("clearLine", jsvalue.NewNumber(0), cb)
	if !called {
		t.Error("callback was not called")
	}
}

func TestCursorTo(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	result := ws.MethodCall("cursorTo", jsvalue.NewNumber(0), jsvalue.NewNumber(0))
	if result.Bool() != true {
		t.Error("cursorTo should return true")
	}
	// With callback
	called := false
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called = true
		return nil
	})
	ws.MethodCall("cursorTo", jsvalue.NewNumber(0), jsvalue.NewNumber(0), cb)
	if !called {
		t.Error("callback was not called")
	}
}

func TestMoveCursor(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	result := ws.MethodCall("moveCursor", jsvalue.NewNumber(1), jsvalue.NewNumber(0))
	if result.Bool() != true {
		t.Error("moveCursor should return true")
	}
}

func TestClearScreenDown(t *testing.T) {
	ws := WriteStream.Call(jsvalue.NewNumber(2))
	result := ws.MethodCall("clearScreenDown")
	if result.Bool() != true {
		t.Error("clearScreenDown should return true")
	}
	// With callback
	called := false
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		called = true
		return nil
	})
	ws.MethodCall("clearScreenDown", cb)
	if !called {
		t.Error("callback was not called")
	}
}

func TestAsJSValue(t *testing.T) {
	if AsJSValue == nil {
		t.Fatal("AsJSValue is nil")
	}
	for _, name := range []string{"ReadStream", "WriteStream", "isatty"} {
		v := AsJSValue.Get(name)
		if v == nil || v.TypeString() == "undefined" {
			t.Fatalf("missing %q on AsJSValue", name)
		}
	}
}
