package events

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func TestAsJSValueExportsEventsSurface(t *testing.T) {
	if AsJSValue.Get("EventEmitter").TypeString() != "function" {
		t.Fatal("expected EventEmitter export")
	}
	emitter := AsJSValue.Get("EventEmitter").Call()
	if emitter.Get("on").TypeString() != "function" {
		t.Fatal("expected EventEmitter instance methods")
	}
}

func TestOnceReturnsPromise(t *testing.T) {
	emitter := AsJSValue.Get("EventEmitter").Call()
	p := AsJSValue.Get("once").Call(emitter, jsvalue.NewString("ready"))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected events.once to return a Promise")
	}
}

func TestMixinEventEmitterAndInitEventEmitter(t *testing.T) {
	cls := jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		InitEventEmitter(this)
		return nil
	}, nil)
	MixinEventEmitter(cls)

	inst := cls.Call()
	for _, name := range []string{"on", "emit", "once", "off", "removeListener", "addListener", "removeAllListeners", "listeners"} {
		if inst.Get(name).TypeString() != "function" {
			t.Fatalf("instance missing %q", name)
		}
	}

	var got string
	handler := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			got = args[0].String()
		}
		return jsvalue.NewUndefined()
	})

	inst.MethodCall("on", jsvalue.NewString("hello"), handler)
	inst.MethodCall("emit", jsvalue.NewString("hello"), jsvalue.NewString("world"))
	if got != "world" {
		t.Fatalf("emit handler saw %q, want world", got)
	}
}
