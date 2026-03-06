package assert

import "github.com/nnstd/gun/runtime/builtin"

// StrictEqual asserts that two values are strictly equal.
func StrictEqual(actual, expected *jsvalue.JSValue) *jsvalue.JSValue {
	if jsvalue.Eq(actual, expected).Bool() {
		return jsvalue.NewUndefined()
	}
	panic("assertion failed: values are not strictly equal")
}

// NotStrictEqual asserts that two values are not strictly equal.
func NotStrictEqual(actual, expected *jsvalue.JSValue) *jsvalue.JSValue {
	if jsvalue.NEq(actual, expected).Bool() {
		return jsvalue.NewUndefined()
	}
	panic("assertion failed: values are strictly equal")
}
