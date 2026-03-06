package process

import (
	stdos "os"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/builtin"
)

// Argv follows Node.js convention: [runtime, script, ...args].
// GUN_ENTRY_SCRIPT is set by "gun run" to the original .ts entry file path.
// Without it, Args[0] (the binary) is used as the script path.
var Argv = func() *jsvalue.JSValue {
	script := stdos.Getenv("GUN_ENTRY_SCRIPT")
	if script == "" {
		script = stdos.Args[0]
	}
	argv := make([]string, 0, len(stdos.Args)+1)
	argv = append(argv, stdos.Args[0]) // runtime (the binary itself)
	argv = append(argv, script)        // script path
	argv = append(argv, stdos.Args[1:]...)
	return jsvalue.FromStrings(argv)
}()

var Platform = jsvalue.NewString(func() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}())

var Stdout = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	// process.stdout.columns — terminal width (default 80)
	obj.Set("columns", jsvalue.NewNumber(80))
	return obj
}()
var Stderr = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("columns", jsvalue.NewNumber(80))
	return obj
}()
var Pid = jsvalue.NewNumber(float64(stdos.Getpid()))
var Versions = jsvalue.NewObject()
var Version = jsvalue.NewString(runtime.Version())

var Env = func() *jsvalue.JSValue {
	env := jsvalue.NewObject()
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env.Set(k, jsvalue.NewString(v))
		}
	}
	return env
}()

func Exit(code *jsvalue.JSValue) {
	c := 0
	if code != nil {
		c = int(code.Number())
	}
	stdos.Exit(c)
}

func Cwd() *jsvalue.JSValue {
	dir, _ := stdos.Getwd()
	return jsvalue.NewString(dir)
}

// AsJSValue returns a JSValue object representing the process global.
// Used when `process` is referenced as a standalone value (e.g. `process?.version`).
func AsJSValue() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("version", Version)
	obj.Set("versions", Versions)
	obj.Set("platform", Platform)
	obj.Set("pid", Pid)
	obj.Set("argv", Argv)
	obj.Set("env", Env)
	obj.Set("cwd", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return Cwd()
	}))
	obj.Set("exit", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		code := jsvalue.NewNumber(0)
		if len(args) > 0 {
			code = args[0]
		}
		Exit(code)
		return jsvalue.NewUndefined()
	}))
	obj.Set("nextTick", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// In Go, nextTick is executed synchronously (no event loop)
		if len(args) > 0 && args[0] != nil {
			args[0].Call(args[1:]...)
		}
		return jsvalue.NewUndefined()
	}))
	obj.Set("emitWarning", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// No-op for now
		return jsvalue.NewUndefined()
	}))
	exe, _ := stdos.Executable()
	obj.Set("execPath", jsvalue.NewString(exe))
	return obj
}
