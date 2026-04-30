package module

import "testing"

func TestRequireCanLoadBunFFI(t *testing.T) {
	require := CreateRequire(nil)
	mod := require.Call(DataToJSValue("bun:ffi"))
	if mod.Get("dlopen").TypeString() != "function" {
		t.Fatal("require('bun:ffi').dlopen must be a function")
	}
	if got := mod.Get("cc").TypeString(); got != "function" {
		t.Fatalf("require('bun:ffi').cc = %s, want function", got)
	}
}
