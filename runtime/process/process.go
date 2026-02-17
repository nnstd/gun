package process

import (
	stdos "os"
	"runtime"
	"strings"
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

var Env = func() map[string]string {
	env := make(map[string]string)
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
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
