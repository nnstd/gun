package os

import (
	stdos "os"
	"runtime"
	"strings"
)

// EOL is the platform-specific end-of-line marker.
var EOL = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

// Homedir returns the current user's home directory.
func Homedir() string {
	dir, _ := stdos.UserHomeDir()
	return dir
}

// Tmpdir returns the OS default directory for temporary files.
func Tmpdir() string {
	return stdos.TempDir()
}

// Hostname returns the hostname of the OS.
func Hostname() (string, error) {
	return stdos.Hostname()
}

// Platform returns the operating system platform using Node.js naming.
func Platform() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// Arch returns the CPU architecture using Node.js naming.
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}

// Cpus returns the number of CPUs.
func Cpus() int {
	return runtime.NumCPU()
}

// Environ returns environment variables as a map.
func Environ() map[string]string {
	env := make(map[string]string)
	for _, e := range stdos.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	return env
}

// Getenv returns the value of an environment variable.
func Getenv(key string) string {
	return stdos.Getenv(key)
}

// Exit terminates the process with the given status code.
func Exit(code int) {
	stdos.Exit(code)
}

// Cwd returns the current working directory.
func Cwd() string {
	dir, _ := stdos.Getwd()
	return dir
}
