package strip_ansi

import "github.com/nnstd/gun/runtime/ansi_regex"

// Default strips ANSI escape codes from a string.
func Default(s string) string {
	return ansi_regex.Default().ReplaceAllString(s, "")
}
