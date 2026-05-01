package runner

// HarnessSource returns Go source code for the test262 harness functions.
// This is written as a package main file into the temp directory for each
// batch so the compiled test binary can call assert, print, $DONE, etc.
// The implementations match test262/harness/ but are self-contained.
const harnessSource = `package main

import (
	"fmt"
	"math"
	"os"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

const (
	ExitCodeAsyncPass = 42
	ExitCodeAsyncFail = 43
)

// Test262Error is the error type used by test262 harness assertions.
type Test262Error struct {
	Message string
}

func (e *Test262Error) Error() string {
	return e.Message
}

// Assert panics with Test262Error if condition is falsy.
func Assert(condition *jsvalue.JSValue) {
	if !condition.Bool() {
		panic(&Test262Error{Message: "assertion failed"})
	}
}

// AssertSameValue checks SameValue equality (distinguishes +0/-0, NaN).
func AssertSameValue(actual, expected *jsvalue.JSValue) {
	if !isSameValue(actual, expected) {
		panic(&Test262Error{
			Message: fmt.Sprintf("assert.sameValue failed: expected %s, got %s",
				formatValue(expected), formatValue(actual)),
		})
	}
}

// AssertNotSameValue checks that values are NOT SameValue equal.
func AssertNotSameValue(actual, unexpected *jsvalue.JSValue) {
	if isSameValue(actual, unexpected) {
		panic(&Test262Error{
			Message: fmt.Sprintf("assert.notSameValue failed: values are the same (%s)",
				formatValue(actual)),
		})
	}
}

func isSameValue(a, b *jsvalue.JSValue) bool {
	if a == nil || b == nil {
		return a == b
	}
	at, bt := a.Type(), b.Type()

	// Both NaN
	if at == jsvalue.TypeNumber && bt == jsvalue.TypeNumber {
		an, bn := a.Number(), b.Number()
		if math.IsNaN(an) && math.IsNaN(bn) {
			return true
		}
		// Distinguish +0 and -0
		if an == 0 && bn == 0 {
			return math.Signbit(an) == math.Signbit(bn)
		}
		return an == bn
	}

	if at != bt {
		return false
	}
	switch at {
	case jsvalue.TypeUndefined, jsvalue.TypeNull:
		return true
	case jsvalue.TypeBoolean:
		return a.Bool() == b.Bool()
	case jsvalue.TypeString:
		return a.String() == b.String()
	default:
		return a == b
	}
}

func formatValue(v *jsvalue.JSValue) string {
	if v == nil {
		return "nil"
	}
	switch v.Type() {
	case jsvalue.TypeUndefined:
		return "undefined"
	case jsvalue.TypeNull:
		return "null"
	case jsvalue.TypeBoolean:
		return fmt.Sprintf("%t", v.Bool())
	case jsvalue.TypeNumber:
		return fmt.Sprintf("%v", v.Number())
	case jsvalue.TypeString:
		return v.String()
	default:
		return v.TypeString()
	}
}

// AssertThrows catches a panic from fn and checks if the error type matches expectedErrorName.
func AssertThrows(expectedErrorName string, fn func()) {
	defer func() {
		r := recover()
		if r == nil {
			panic(&Test262Error{
				Message: fmt.Sprintf("assert.throws: expected %s but no error was thrown", expectedErrorName),
			})
		}
		errorName := extractErrorName(r)
		if errorName != expectedErrorName {
			panic(&Test262Error{
				Message: fmt.Sprintf("assert.throws: expected %s but got %s (%v)", expectedErrorName, errorName, r),
			})
		}
	}()
	fn()
}

func extractErrorName(r interface{}) string {
	switch v := r.(type) {
	case *jsvalue.JSValue:
		if v.Type() == jsvalue.TypeObject || v.Type() == jsvalue.TypeFunction {
			if name := v.Get("name"); name != nil && name.Type() == jsvalue.TypeString {
				return name.String()
			}
		}
		return v.TypeString()
	case *Test262Error:
		return "Test262Error"
	case error:
		return fmt.Sprintf("%T", v)
	default:
		return fmt.Sprintf("%T", v)
	}
}

// Print outputs arguments to stdout, mimicking the test262 print() host function.
func Print(args ...*jsvalue.JSValue) {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprint(os.Stdout, " ")
		}
		if arg == nil {
			fmt.Fprint(os.Stdout, "undefined")
		} else {
			fmt.Fprint(os.Stdout, arg.String())
		}
	}
	fmt.Fprintln(os.Stdout)
}

// Done signals async test completion ($DONE in test262).
func Done(err *jsvalue.JSValue) {
	if err == nil || err.Type() == jsvalue.TypeUndefined || err.Type() == jsvalue.TypeNull {
		os.Exit(ExitCodeAsyncPass)
	}
	fmt.Fprintf(os.Stderr, "Test262:AsyncTestFailure: %s\n", err.String())
	os.Exit(ExitCodeAsyncFail)
}

// DontEvaluate panics immediately. Used in negative parse-phase tests.
func DontEvaluate() {
	panic("Test262: This statement should not be evaluated.")
}

// Global262 returns a JSValue representing the $262 global object stub.
func Global262() *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	obj.Set("global", obj)

	obj.Set("evalScript", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))

	obj.Set("createRealm", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))

	obj.Set("detachArrayBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))

	obj.Set("gc", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))

	return obj
}

// Test262ErrorNew creates a new Test262Error JSValue.
func Test262ErrorNew(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewUndefined()
}
`

// HarnessSource returns the harness Go source code.
func HarnessSource() string {
	return harnessSource
}
