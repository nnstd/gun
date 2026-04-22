package v8

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nnstd/gun/runtime/buffer"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/profile"
	"github.com/nnstd/gun/runtime/promise"
)

func TestCachedDataVersionTag(t *testing.T) {
	tag1 := AsJSValue.Get("cachedDataVersionTag").Call()
	tag2 := AsJSValue.Get("cachedDataVersionTag").Call()
	if tag1 == nil || tag1.Number() == 0 {
		t.Fatalf("cachedDataVersionTag() = %v, want non-zero number", tag1)
	}
	if tag1.Number() != tag2.Number() {
		t.Fatalf("cachedDataVersionTag not stable within process: %v vs %v", tag1.Number(), tag2.Number())
	}
}

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	value := jsvalue.ObjectFrom(
		"name", jsvalue.NewString("gun"),
		"nums", jsvalue.NewArray(jsvalue.NewNumber(1), jsvalue.NewNumber(2)),
		"undef", jsvalue.NewUndefined(),
		"big", jsvalue.BigIntCtor.Call(jsvalue.NewString("4")),
	)
	value.Set("self", value)
	m := jsvalue.NewMap()
	m.MethodCall("set", jsvalue.NewString("x"), jsvalue.NewNumber(2))
	value.Set("map", m)
	s := jsvalue.NewSet()
	s.MethodCall("add", jsvalue.NewNumber(3))
	value.Set("set", s)
	buf := AsJSValue.Get("serialize").Call(value)
	if !jsvalue.InstanceOf(buf, bufferCtor()).Bool() {
		t.Fatalf("serialize() did not return Buffer instance")
	}
	out := AsJSValue.Get("deserialize").Call(buf)
	if got := out.Get("name").String(); got != "gun" {
		t.Fatalf("deserialize(name) = %q, want gun", got)
	}
	if got := out.Get("nums").Get("1").Number(); got != 2 {
		t.Fatalf("deserialize(nums[1]) = %v, want 2", got)
	}
	if !out.HasOwnProperty("undef") || out.Get("undef").Type() != jsvalue.TypeUndefined {
		t.Fatalf("deserialize(undef) did not preserve undefined own property")
	}
	if out.Get("self") != out {
		t.Fatalf("deserialize(self) did not preserve cycle")
	}
	if got := out.Get("map").MethodCall("get", jsvalue.NewString("x")).Number(); got != 2 {
		t.Fatalf("deserialize(map.get('x')) = %v, want 2", got)
	}
	if ok := out.Get("set").MethodCall("has", jsvalue.NewNumber(3)).Bool(); !ok {
		t.Fatalf("deserialize(set.has(3)) = false, want true")
	}
	if got := out.Get("big").String(); got != "4" {
		t.Fatalf("deserialize(big) = %q, want 4", got)
	}
}

func TestSerializerDeserializerWorkflow(t *testing.T) {
	ser := AsJSValue.Get("Serializer").New()
	ser.MethodCall("writeHeader")
	ser.MethodCall("writeValue", jsvalue.ObjectFrom("ok", jsvalue.NewBool(true)))
	buf := ser.MethodCall("releaseBuffer")
	des := AsJSValue.Get("Deserializer").New(buf)
	des.MethodCall("readHeader")
	val := des.MethodCall("readValue")
	if !val.Get("ok").Bool() {
		t.Fatal("expected deserialized value to preserve object data")
	}
}

func TestPromiseHooksLifecycle(t *testing.T) {
	events := jsvalue.NewArray()
	push := func(name string) *jsvalue.JSValue {
		return jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			events.MethodCall("push", jsvalue.NewString(name))
			return jsvalue.NewUndefined()
		})
	}
	stop := AsJSValue.Get("promiseHooks").Get("createHook").Call(jsvalue.ObjectFrom(
		"init", push("init"),
		"before", push("before"),
		"after", push("after"),
		"settled", push("settled"),
	))
	p := promise.Promise.Get("resolve").Call(jsvalue.NewNumber(1))
	p.MethodCall("then", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		events.MethodCall("push", jsvalue.NewString("then"))
		return jsvalue.NewUndefined()
	}))
	eventloop.Default.Run()
	stop.Call()
	got := map[string]bool{}
	for _, item := range events.Array() {
		got[item.String()] = true
	}
	for _, want := range []string{"init", "settled", "before", "after", "then"} {
		if !got[want] {
			t.Fatalf("missing event %q in %v", want, got)
		}
	}
}

func TestPromiseHooksRejectAsyncCallback(t *testing.T) {
	asyncFn := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}).MarkAsAsync()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected async promise hook callback to panic")
		}
	}()
	AsJSValue.Get("promiseHooks").Get("onInit").Call(asyncFn)
}

func TestStartCpuProfileHandle(t *testing.T) {
	handle := AsJSValue.Get("startCpuProfile").Call()
	if handle == nil || handle.Get("stop").TypeString() != "function" {
		t.Fatalf("startCpuProfile() did not return handle with stop()")
	}
	leave := profile.EnterFrame(profile.Frame{
		FunctionName: "v8.testWork",
		File:         "/tmp/v8_test.ts",
		Line:         12,
		Column:       3,
	})
	time.Sleep(5 * time.Millisecond)
	leave()
	out := handle.Get("stop").Call()
	if out.TypeString() != "string" {
		t.Fatalf("expected stop() to return string, got %s", out.TypeString())
	}
	if !strings.Contains(out.String(), "\"nodes\"") {
		t.Fatalf("expected stop() string to contain profile json, got %q", out.String())
	}
	var profile map[string]any
	if err := json.Unmarshal([]byte(out.String()), &profile); err != nil {
		t.Fatalf("invalid profile json: %v", err)
	}
	nodes, ok := profile["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatal("expected cpuprofile nodes from handle.stop()")
	}
}

func bufferCtor() *jsvalue.JSValue {
	return buffer.Buffer
}
