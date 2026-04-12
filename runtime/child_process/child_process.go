package child_process

import (
	"os/exec"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func runCommand(cmd string) *jsvalue.JSValue {
	c := exec.Command("sh", "-lc", cmd)
	out, err := c.CombinedOutput()
	result := jsvalue.NewObject()
	result.Set("stdout", jsvalue.NewString(string(out)))
	result.Set("stderr", jsvalue.NewString(""))
	if err != nil {
		result.Set("error", jsvalue.NewString(err.Error()))
	}
	return result
}

var AsJSValue = jsvalue.ObjectFrom(
	"execSync", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		c := exec.Command("sh", "-lc", args[0].String())
		out, _ := c.CombinedOutput()
		return jsvalue.NewString(string(out))
	}),
	"exec", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		return runCommand(args[0].String())
	}),
	"spawn", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		return runCommand(args[0].String())
	}),
)
