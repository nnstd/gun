package os

import (
	stdos "os"
	"runtime"
	"strings"

	"github.com/nnstd/gun/runtime/builtin"
)

// EOL is the platform-specific end-of-line marker.
var EOL = jsvalue.NewString(func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}())

// Homedir returns the current user's home directory.
func Homedir() *jsvalue.JSValue {
	dir, _ := stdos.UserHomeDir()
	return jsvalue.NewString(dir)
}

// Tmpdir returns the OS default directory for temporary files.
func Tmpdir() *jsvalue.JSValue {
	return jsvalue.NewString(stdos.TempDir())
}

// Hostname returns the hostname of the OS.
func Hostname() *jsvalue.JSValue {
	name, _ := stdos.Hostname()
	return jsvalue.NewString(name)
}

// Platform returns the operating system platform using Node.js naming.
func Platform() *jsvalue.JSValue {
	switch runtime.GOOS {
	case "windows":
		return jsvalue.NewString("win32")
	default:
		return jsvalue.NewString(runtime.GOOS)
	}
}

// Arch returns the CPU architecture using Node.js naming.
func Arch() *jsvalue.JSValue {
	switch runtime.GOARCH {
	case "amd64":
		return jsvalue.NewString("x64")
	case "386":
		return jsvalue.NewString("ia32")
	default:
		return jsvalue.NewString(runtime.GOARCH)
	}
}

// Cpus returns the number of CPUs.
func Cpus() *jsvalue.JSValue {
	return jsvalue.NewNumber(float64(runtime.NumCPU()))
}

// Environ returns environment variables as a JSValue object.
func Environ() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			obj.Set(k, jsvalue.NewString(v))
		}
	}
	return obj
}

// Getenv returns the value of an environment variable.
func Getenv(key *jsvalue.JSValue) *jsvalue.JSValue {
	k := ""
	if key != nil {
		k = key.String()
	}
	return jsvalue.NewString(stdos.Getenv(k))
}

// Exit terminates the process with the given status code.
func Exit(code *jsvalue.JSValue) {
	c := 0
	if code != nil {
		c = int(code.Number())
	}
	stdos.Exit(c)
}

// Cwd returns the current working directory.
func Cwd() *jsvalue.JSValue {
	dir, _ := stdos.Getwd()
	return jsvalue.NewString(dir)
}
