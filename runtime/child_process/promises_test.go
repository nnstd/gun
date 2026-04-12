package child_process

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestPromisesAsJSValueExportsExec(t *testing.T) {
	if PromisesAsJSValue.Get("exec").TypeString() != "function" {
		t.Fatal("expected child_process/promises exec export")
	}
	p := PromisesAsJSValue.Get("exec").Call(jsvalue.NewString("printf hi"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected exec() to return a Promise")
	}
}
