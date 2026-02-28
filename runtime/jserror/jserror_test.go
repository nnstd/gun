package jserror

import (
	"strings"
	"testing"

	"github.com/nnstd/gun/runtime/jsvalue"
)

func TestErrorConstructor(t *testing.T) {
	err := Error.Call(jsvalue.NewString("test message"))
	if err.Get("message").String() != "test message" {
		t.Errorf("expected message 'test message', got %q", err.Get("message").String())
	}
	if err.Get("name").String() != "Error" {
		t.Errorf("expected name 'Error', got %q", err.Get("name").String())
	}
	stack := err.Get("stack").String()
	if !strings.Contains(stack, "Error: test message") {
		t.Errorf("stack should contain 'Error: test message', got %q", stack)
	}
	if !strings.Contains(stack, "at ") {
		t.Errorf("stack should contain stack frames, got %q", stack)
	}
}

func TestErrorEmptyMessage(t *testing.T) {
	err := Error.Call()
	if err.Get("message").String() != "" {
		t.Errorf("expected empty message, got %q", err.Get("message").String())
	}
	stack := err.Get("stack").String()
	if !strings.HasPrefix(stack, "Error\n") && stack != "Error" {
		t.Errorf("stack should start with 'Error', got %q", stack)
	}
}

func TestTypeErrorConstructor(t *testing.T) {
	err := TypeError.Call(jsvalue.NewString("bad type"))
	if err.Get("name").String() != "TypeError" {
		t.Errorf("expected name 'TypeError', got %q", err.Get("name").String())
	}
	if err.Get("message").String() != "bad type" {
		t.Errorf("expected message 'bad type', got %q", err.Get("message").String())
	}
	stack := err.Get("stack").String()
	if !strings.Contains(stack, "TypeError: bad type") {
		t.Errorf("stack should contain 'TypeError: bad type', got %q", stack)
	}
}

func TestRangeErrorConstructor(t *testing.T) {
	err := RangeError.Call(jsvalue.NewString("out of range"))
	if err.Get("name").String() != "RangeError" {
		t.Errorf("expected name 'RangeError', got %q", err.Get("name").String())
	}
}

func TestReferenceErrorConstructor(t *testing.T) {
	err := ReferenceError.Call(jsvalue.NewString("not defined"))
	if err.Get("name").String() != "ReferenceError" {
		t.Errorf("expected name 'ReferenceError', got %q", err.Get("name").String())
	}
}

func TestSyntaxErrorConstructor(t *testing.T) {
	err := SyntaxError.Call(jsvalue.NewString("unexpected token"))
	if err.Get("name").String() != "SyntaxError" {
		t.Errorf("expected name 'SyntaxError', got %q", err.Get("name").String())
	}
}

func TestURIErrorConstructor(t *testing.T) {
	err := URIError.Call(jsvalue.NewString("bad uri"))
	if err.Get("name").String() != "URIError" {
		t.Errorf("expected name 'URIError', got %q", err.Get("name").String())
	}
}

func TestEvalErrorConstructor(t *testing.T) {
	err := EvalError.Call(jsvalue.NewString("eval failed"))
	if err.Get("name").String() != "EvalError" {
		t.Errorf("expected name 'EvalError', got %q", err.Get("name").String())
	}
}

func TestErrorCause(t *testing.T) {
	cause := jsvalue.NewString("root cause")
	opts := jsvalue.NewObject()
	opts.Set("cause", cause)
	err := Error.Call(jsvalue.NewString("wrapped"), opts)

	c := err.Get("cause")
	if c == nil || c.String() != "root cause" {
		t.Errorf("expected cause 'root cause', got %v", c)
	}
}

func TestErrorCauseNotSetWithoutOption(t *testing.T) {
	err := Error.Call(jsvalue.NewString("no cause"))
	c := err.Get("cause")
	if c != nil && c.Type() != jsvalue.TypeUndefined {
		t.Errorf("expected no cause property, got %v", c)
	}
}

func TestStackTraceLimit(t *testing.T) {
	stl := Error.Get("stackTraceLimit")
	if stl == nil || stl.Type() != jsvalue.TypeNumber {
		t.Fatalf("stackTraceLimit should be a number, got %v", stl)
	}
	if stl.Number() != 10 {
		t.Errorf("default stackTraceLimit should be 10, got %v", stl.Number())
	}
}

func TestStackTraceLimitMutable(t *testing.T) {
	// Save and restore
	orig := stackTraceLimit
	defer func() { stackTraceLimit = orig }()

	Error.Set("stackTraceLimit", jsvalue.NewNumber(0))
	err := Error.Call(jsvalue.NewString("limited"))
	stack := err.Get("stack").String()
	// With limit 0, should just have the header, no frames
	if strings.Contains(stack, "at ") {
		t.Errorf("with stackTraceLimit=0, stack should have no frames, got %q", stack)
	}
}

func TestCaptureStackTrace(t *testing.T) {
	target := jsvalue.NewObject()
	target.Set("name", jsvalue.NewString("CustomError"))
	target.Set("message", jsvalue.NewString("custom"))

	captureFunc := Error.Get("captureStackTrace")
	if captureFunc == nil || captureFunc.Type() != jsvalue.TypeFunction {
		t.Fatal("Error.captureStackTrace should be a function")
	}
	captureFunc.Call(target)

	stack := target.Get("stack")
	if stack == nil || stack.Type() != jsvalue.TypeString {
		t.Fatalf("target.stack should be a string, got %v", stack)
	}
	if !strings.Contains(stack.String(), "CustomError: custom") {
		t.Errorf("stack should contain 'CustomError: custom', got %q", stack.String())
	}
}

func TestPrepareStackTrace(t *testing.T) {
	// Save and restore prepareStackTrace
	orig := Error.Get("prepareStackTrace")
	defer func() { Error.Set("prepareStackTrace", orig) }()

	// Set custom prepareStackTrace that returns array of file names
	Error.Set("prepareStackTrace", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewString("no stack")
		}
		callSites := args[1]
		if callSites == nil || !callSites.IsArray() {
			return jsvalue.NewString("not array")
		}
		// Return the call sites array directly (like get-caller-file does)
		return callSites
	}))

	target := jsvalue.NewObject()
	Error.Get("captureStackTrace").Call(target)

	stack := target.Get("stack")
	if stack == nil {
		t.Fatal("target.stack should be set")
	}
	// prepareStackTrace returns the callSites array
	if !stack.IsArray() {
		t.Fatalf("with custom prepareStackTrace, stack should be array, got type %d", stack.Type())
	}
	// Verify call sites have getFileName method
	if stack.Len() > 0 {
		site := stack.Index(0)
		getFn := site.Get("getFileName")
		if getFn == nil || getFn.Type() != jsvalue.TypeFunction {
			t.Error("call site should have getFileName method")
		} else {
			fileName := getFn.Call()
			if fileName == nil || fileName.Type() != jsvalue.TypeString {
				t.Error("getFileName should return a string")
			}
		}
	}
}

func TestErrorThrownAndCaught(t *testing.T) {
	// Simulate throw/catch via panic/recover
	var caught *jsvalue.JSValue
	func() {
		defer func() {
			if r := recover(); r != nil {
				caught = jsvalue.From(r)
			}
		}()
		panic(Error.Call(jsvalue.NewString("thrown")))
	}()

	if caught == nil {
		t.Fatal("should have caught the error")
	}
	if caught.Get("message").String() != "thrown" {
		t.Errorf("caught error message should be 'thrown', got %q", caught.Get("message").String())
	}
	if caught.Get("name").String() != "Error" {
		t.Errorf("caught error name should be 'Error', got %q", caught.Get("name").String())
	}
}
