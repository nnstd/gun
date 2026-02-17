package process

import (
	stdos "os"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/jsvalue"
)

var Argv = stdos.Args

var Platform = func() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}()

var Stdout = stdos.Stdout
var Stderr = stdos.Stderr
var Pid = stdos.Getpid()
var Versions = map[string]string{}

var Env = func() map[string]*jsvalue.JSValue {
	env := make(map[string]*jsvalue.JSValue)
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = jsvalue.NewString(v)
		}
	}
	return env
}()

func Exit(code int) {
	stdos.Exit(code)
}

func Cwd() string {
	dir, _ := stdos.Getwd()
	return dir
}
