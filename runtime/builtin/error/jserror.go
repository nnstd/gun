package error

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/builtin"
)

// stackTraceLimit is the Go-side backing store for Error.stackTraceLimit.
// The JSValue property on Error reads/writes this variable via an accessor.
var stackTraceLimit = 10

// getStackTraceLimit returns the current stack trace limit.
// During package initialization (before Error is set up), it uses the Go variable.
// At runtime it also uses the Go variable, kept in sync via the JSValue accessor.
func getStackTraceLimit() int {
	return stackTraceLimit
}

// captureStack formats a stack trace like Node.js:
//
//	Error: message
//	    at functionName (file:line:0)
//	    at ...
//
// skip controls how many call frames to omit (to hide error internals).
func captureStack(name, message string, skip int) string {
	limit := getStackTraceLimit()
	if limit <= 0 {
		if message == "" {
			return name
		}
		return name + ": " + message
	}

	pcs := make([]uintptr, limit+skip)
	n := runtime.Callers(skip+1, pcs)
	pcs = pcs[:n]

	var sb strings.Builder
	if message == "" {
		sb.WriteString(name)
	} else {
		sb.WriteString(name + ": " + message)
	}

	frames := runtime.CallersFrames(pcs)
	var collected []runtime.Frame
	hasSourceFrame := false
	for {
		frame, more := frames.Next()
		collected = append(collected, frame)
		if isSourceFrame(frame.File) {
			hasSourceFrame = true
		}
		if !more {
			break
		}
	}
	for _, frame := range collected {
		if shouldSkipFrame(frame, hasSourceFrame) {
			continue
		}
		sb.WriteString("\n    at ")
		sb.WriteString(formatFrame(frame))
	}
	return sb.String()
}

func shouldSkipFrame(frame runtime.Frame, hasSourceFrame bool) bool {
	if strings.HasPrefix(frame.Function, "runtime.") {
		return true
	}
	file := filepath.ToSlash(frame.File)
	if strings.Contains(file, "/runtime/builtin/error/jserror.go") {
		return true
	}
	if hasSourceFrame && strings.Contains(file, "/runtime/") && !isSourceFrame(file) {
		return true
	}
	return false
}

func isSourceFrame(file string) bool {
	file = filepath.ToSlash(file)
	return strings.HasSuffix(file, ".ts") ||
		strings.HasSuffix(file, ".js") ||
		strings.HasSuffix(file, ".tsx") ||
		strings.HasSuffix(file, ".jsx")
}

func formatFrame(frame runtime.Frame) string {
	location := fmt.Sprintf("%s:%d:0", frame.File, frame.Line)
	name := simplifyFunctionName(frame.Function)
	if name == "" {
		return location
	}
	return fmt.Sprintf("%s (%s)", name, location)
}

func simplifyFunctionName(fn string) string {
	if fn == "" {
		return ""
	}
	if idx := strings.LastIndex(fn, "/"); idx >= 0 {
		fn = fn[idx+1:]
	}
	if strings.HasPrefix(fn, "main.main.func") || strings.Contains(fn, ".func") {
		return ""
	}
	if idx := strings.LastIndex(fn, "."); idx >= 0 {
		prefix := fn[:idx]
		suffix := fn[idx+1:]
		if strings.Contains(prefix, ".") {
			if dot := strings.LastIndex(prefix, "."); dot >= 0 {
				prefix = prefix[dot+1:]
			}
		}
		if prefix != "" {
			return prefix + "." + suffix
		}
		return suffix
	}
	return fn
}

// buildCallSites returns a JSValue array of CallSite-like objects.
// Each object has methods: getFileName(), getLineNumber(), getFunctionName(),
// getTypeName(), getColumnNumber() — matching V8's structured stack trace API.
func buildCallSites(skip int) *jsvalue.JSValue {
	limit := getStackTraceLimit()
	if limit <= 0 {
		return jsvalue.NewArray()
	}

	pcs := make([]uintptr, limit+skip)
	n := runtime.Callers(skip+1, pcs)
	pcs = pcs[:n]

	var sites []*jsvalue.JSValue
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if strings.HasPrefix(frame.Function, "runtime.") {
			if !more {
				break
			}
			continue
		}

		fileName := frame.File
		lineNumber := frame.Line
		funcName := frame.Function

		site := jsvalue.NewObject()
		site.Set("getFileName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewString(fileName)
		}))
		site.Set("getLineNumber", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewNumber(float64(lineNumber))
		}))
		site.Set("getFunctionName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewString(funcName)
		}))
		site.Set("getTypeName", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewString("")
		}))
		site.Set("getColumnNumber", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewNumber(0)
		}))

		sites = append(sites, site)
		if !more {
			break
		}
	}
	return jsvalue.NewArray(sites...)
}

// makeErrorClass creates a JavaScript Error class as a JSValue constructor.
// The constructor sets .message, .name, .stack, and optionally .cause on instances.
func makeErrorClass(errorName string, parent *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		message := ""
		if len(args) > 0 && args[0] != nil && args[0].Type() != jsvalue.TypeUndefined && args[0].Type() != jsvalue.TypeNull {
			message = args[0].String()
		}
		this.Set("message", jsvalue.NewString(message))
		this.Set("name", jsvalue.NewString(errorName))

		// Check for options.cause (second argument)
		if len(args) > 1 && args[1] != nil && args[1].Type() == jsvalue.TypeObject {
			cause := args[1].Get("cause")
			if cause != nil && cause.Type() != jsvalue.TypeUndefined {
				this.Set("cause", cause)
			}
		}

		// Capture stack trace — skip frames to hide error/jsvalue internals
		this.Set("stack", jsvalue.NewString(captureStack(errorName, message, 3)))
		return nil
	}, parent)
}

// Error constructors — package-level variables matching JavaScript global error types.
var Error = makeErrorClass("Error", nil)
var TypeError = makeErrorClass("TypeError", Error)
var RangeError = makeErrorClass("RangeError", Error)
var ReferenceError = makeErrorClass("ReferenceError", Error)
var SyntaxError = makeErrorClass("SyntaxError", Error)
var URIError = makeErrorClass("URIError", Error)
var EvalError = makeErrorClass("EvalError", Error)

func init() {
	jsvalue.RegisterGlobal("Error", Error)
	jsvalue.RegisterGlobal("TypeError", TypeError)
	jsvalue.RegisterGlobal("RangeError", RangeError)
	jsvalue.RegisterGlobal("ReferenceError", ReferenceError)
	jsvalue.RegisterGlobal("SyntaxError", SyntaxError)
	jsvalue.RegisterGlobal("URIError", URIError)
	jsvalue.RegisterGlobal("EvalError", EvalError)
}

func InvalidArgType(message string) *jsvalue.JSValue {
	err := TypeError.Call(jsvalue.NewString(message))
	err.Set("code", jsvalue.NewString("ERR_INVALID_ARG_TYPE"))
	return err
}

func InvalidPrivateField(expr string) *jsvalue.JSValue {
	return TypeError.Call(jsvalue.NewString(fmt.Sprintf("Cannot access invalid private field (evaluating '%s')", expr)))
}

func InvalidPrivateMethodOrAccessor(expr string) *jsvalue.JSValue {
	return TypeError.Call(jsvalue.NewString(fmt.Sprintf("Cannot access private method or acessor (evaluating '%s')", expr)))
}

func RefreshStack(err *jsvalue.JSValue, skip int) *jsvalue.JSValue {
	if err == nil {
		return nil
	}
	name := "Error"
	message := ""
	if n := err.Get("name"); n != nil && n.Type() == jsvalue.TypeString {
		name = n.String()
	}
	if m := err.Get("message"); m != nil && m.Type() == jsvalue.TypeString {
		message = m.String()
	}
	err.Set("stack", jsvalue.NewString(captureStack(name, message, skip+1)))
	return err
}

func AsJSError(v any) (*jsvalue.JSValue, bool) {
	err, ok := v.(*jsvalue.JSValue)
	if !ok || err == nil {
		return nil, false
	}
	if stack := err.Get("stack"); stack != nil && stack.Type() == jsvalue.TypeString {
		return err, true
	}
	if name := err.Get("name"); name != nil && name.Type() == jsvalue.TypeString {
		return err, true
	}
	return nil, false
}

func FormatRecovered(v any) (string, bool) {
	err, ok := AsJSError(v)
	if !ok {
		return "", false
	}
	return err.String(), true
}

func RecoverMain() {
	if r := recover(); r != nil {
		if msg, ok := FormatRecovered(r); ok {
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		}
		panic(r)
	}
}

func init() {
	// Error.stackTraceLimit — accessor property backed by Go variable
	Error.DefineProperty("stackTraceLimit", &jsvalue.PropertyDescriptor{
		Get: func(_ *jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewNumber(float64(stackTraceLimit))
		},
		Set: func(_ *jsvalue.JSValue, v *jsvalue.JSValue) {
			if v != nil && v.Type() == jsvalue.TypeNumber {
				stackTraceLimit = int(v.Number())
			}
		},
		Enumerable:   true,
		Configurable: true,
	})

	// Error.prepareStackTrace — V8 API: callback(error, structuredStackTrace)
	Error.Set("prepareStackTrace", jsvalue.NewUndefined())

	// Error.captureStackTrace(targetObject[, constructorOpt])
	Error.Set("captureStackTrace", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		target := args[0]

		prepare := Error.Get("prepareStackTrace")
		if prepare != nil && prepare.Type() == jsvalue.TypeFunction {
			// V8 API: prepareStackTrace(error, structuredStackTrace)
			callSites := buildCallSites(3)
			result := prepare.Call(target, callSites)
			target.Set("stack", result)
		} else {
			// Default: capture formatted stack string
			name := "Error"
			message := ""
			if n := target.Get("name"); n != nil && n.Type() == jsvalue.TypeString {
				name = n.String()
			}
			if m := target.Get("message"); m != nil && m.Type() == jsvalue.TypeString {
				message = m.String()
			}
			target.Set("stack", jsvalue.NewString(captureStack(name, message, 3)))
		}
		return jsvalue.NewUndefined()
	}))

	// Error.prototype.toString — returns "Name: message" format
	Error.Get("prototype").Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString("Error")
	}))
}
