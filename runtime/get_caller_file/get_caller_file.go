package get_caller_file

import "runtime"

// Default returns the file name of the caller at the given stack position.
// This is the Go equivalent of the npm get-caller-file package, which uses
// V8's Error.prepareStackTrace to inspect the call stack.
func Default(position int) string {
	if position <= 0 {
		position = 2
	}
	_, file, _, ok := runtime.Caller(position)
	if !ok {
		return ""
	}
	return file
}
