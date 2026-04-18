package nodehttp

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestEventEmitterMixin(t *testing.T) {
	cls := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		initEvents(this)
		return nil
	}, nil)
	mixEventEmitter(cls)

	inst := cls.Call()
	for _, name := range []string{"on", "emit", "once", "off", "removeListener", "addListener", "removeAllListeners", "listeners"} {
		if inst.Get(name).TypeString() != "function" {
			t.Errorf("instance missing %q (typeof=%s)", name, inst.Get(name).TypeString())
		}
	}

	got := []string{}
	handler := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		for _, a := range args {
			got = append(got, a.String())
		}
		return jsvalue.NewUndefined()
	})

	inst.MethodCall("on", jsvalue.NewString("hello"), handler)
	inst.MethodCall("emit", jsvalue.NewString("hello"), jsvalue.NewString("a"), jsvalue.NewString("b"))

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("emit args = %v, want [a b]", got)
	}

	got = nil
	inst.MethodCall("off", jsvalue.NewString("hello"), handler)
	inst.MethodCall("emit", jsvalue.NewString("hello"), jsvalue.NewString("c"))
	if len(got) != 0 {
		t.Errorf("after off, got = %v, want empty", got)
	}
}
