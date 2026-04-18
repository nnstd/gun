package nodehttp

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func mustJSString(s string) *jsvalue.JSValue { return jsvalue.NewString(s) }
