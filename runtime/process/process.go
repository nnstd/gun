package process

import (
	stdos "os"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/jsvalue"
)

var Argv = jsvalue.FromStrings(stdos.Args)

var Platform = jsvalue.NewString(func() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}())

var Stdout = stdos.Stdout
var Stderr = stdos.Stderr
var Pid = jsvalue.NewNumber(float64(stdos.Getpid()))
var Versions = jsvalue.NewObject()
var Version = jsvalue.NewString(runtime.Version())

var Env = func() map[string]*jsvalue.JSValue {
	env := make(map[string]*jsvalue.JSValue)
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = jsvalue.NewString(v)
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
