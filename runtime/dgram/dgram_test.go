package dgram

import (
	"net"
	"sync"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

func runLoop() chan struct{} {
	done := make(chan struct{})
	go func() {
		eventloop.Default.Run()
		close(done)
	}()
	return done
}

func waitDone(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func skipIfUDPUnavailable(t *testing.T, network, address string) {
	t.Helper()
	pc, err := net.ListenPacket(network, address)
	if err != nil {
		t.Skipf("skipping UDP test: %v", err)
	}
	_ = pc.Close()
}

func TestSurface(t *testing.T) {
	if AsJSValue == nil {
		t.Fatal("AsJSValue nil")
	}
	if AsJSValue.Get("createSocket").TypeString() != "function" {
		t.Fatal("missing createSocket")
	}
	if AsJSValue.Get("Socket").TypeString() != "function" {
		t.Fatal("missing Socket class")
	}

	socket := AsJSValue.Get("createSocket").Call(jsvalue.NewString("udp4"))
	for _, name := range []string{"bind", "send", "close", "address", "on", "emit"} {
		if socket.Get(name).TypeString() != "function" {
			t.Fatalf("socket missing %q", name)
		}
	}
}

func TestCreateSocketRejectsUnsupportedOptions(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected unsupported option panic")
		}
	}()
	AsJSValue.Get("createSocket").Call(jsvalue.ObjectFrom(
		"type", jsvalue.NewString("udp4"),
		"signal", jsvalue.NewObject(),
	))
}

func TestSendRejectsUnsupportedPayload(t *testing.T) {
	socket := AsJSValue.Get("createSocket").Call(jsvalue.NewString("udp4"))

	var gotErr *jsvalue.JSValue
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			gotErr = args[0]
		}
		return jsvalue.NewUndefined()
	})

	socket.MethodCall(
		"send",
		jsvalue.NewArray(jsvalue.NewNumber(1)),
		jsvalue.NewNumber(9999),
		jsvalue.NewString("127.0.0.1"),
		cb,
	)

	if gotErr == nil {
		t.Fatal("expected send callback error")
	}
	if gotErr.Get("code").String() != "EDGRAM" {
		t.Fatalf("unexpected error code: %q", gotErr.Get("code").String())
	}
}

func TestBindSendReceiveLifecycle(t *testing.T) {
	skipIfUDPUnavailable(t, "udp4", "0.0.0.0:0")

	var mu sync.Mutex
	var eventsSeen []string
	var gotMsg string
	var gotAddr string
	var gotFamily string
	var gotSize float64
	var sendBytes float64
	var errorsSeen []string
	var receiver *jsvalue.JSValue
	var sender *jsvalue.JSValue

	closeDone := make(chan struct{})
	closeCount := 0
	onClosed := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		defer mu.Unlock()
		closeCount++
		if closeCount == 2 {
			close(closeDone)
		}
		return jsvalue.NewUndefined()
	})

	receiver = AsJSValue.Get("createSocket").Call(
		jsvalue.NewString("udp4"),
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			eventsSeen = append(eventsSeen, "message")
			if len(args) > 0 && args[0] != nil {
				gotMsg = args[0].MethodCall("toString").String()
			}
			if len(args) > 1 && args[1] != nil {
				gotAddr = args[1].Get("address").String()
				gotFamily = args[1].Get("family").String()
				gotSize = args[1].Get("size").Number()
			}
			mu.Unlock()

			receiver.MethodCall("close", onClosed)
			sender.MethodCall("close", onClosed)
			return jsvalue.NewUndefined()
		}),
	)
	sender = AsJSValue.Get("createSocket").Call(jsvalue.NewString("udp4"))

	recordError := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		defer mu.Unlock()
		if len(args) > 0 && args[0] != nil {
			errorsSeen = append(errorsSeen, args[0].String())
		}
		return jsvalue.NewUndefined()
	})
	receiver.MethodCall("on", jsvalue.NewString("error"), recordError)
	sender.MethodCall("on", jsvalue.NewString("error"), recordError)

	receiver.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		eventsSeen = append(eventsSeen, "listening")
		mu.Unlock()

		addr := receiver.MethodCall("address")
		sender.MethodCall("bind", jsvalue.NewNumber(0), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			sender.MethodCall("send",
				jsvalue.NewString("ping"),
				addr.Get("port"),
				jsvalue.NewString("127.0.0.1"),
				jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
					mu.Lock()
					defer mu.Unlock()
					if len(args) > 1 && args[1] != nil {
						sendBytes = args[1].Number()
					}
					return jsvalue.NewUndefined()
				}),
			)
			return jsvalue.NewUndefined()
		}))
		return jsvalue.NewUndefined()
	}))

	receiver.MethodCall("bind", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("timed out waiting for socket close callbacks; events=%v errors=%v msg=%q sendBytes=%v", eventsSeen, errorsSeen, gotMsg, sendBytes)
	}
	waitDone(t, runDone, "event loop completion")

	mu.Lock()
	defer mu.Unlock()
	if len(eventsSeen) == 0 || eventsSeen[0] != "listening" {
		t.Fatalf("expected listening before message, got %v", eventsSeen)
	}
	if gotMsg != "ping" {
		t.Fatalf("message = %q, want ping", gotMsg)
	}
	if gotAddr != "127.0.0.1" {
		t.Fatalf("rinfo.address = %q, want 127.0.0.1", gotAddr)
	}
	if gotFamily != "IPv4" {
		t.Fatalf("rinfo.family = %q, want IPv4", gotFamily)
	}
	if gotSize != 4 {
		t.Fatalf("rinfo.size = %v, want 4", gotSize)
	}
	if sendBytes != 4 {
		t.Fatalf("send callback bytes = %v, want 4", sendBytes)
	}
}
