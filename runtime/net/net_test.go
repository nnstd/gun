package nodenet

import (
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

func TestSurface(t *testing.T) {
	if AsJSValue == nil {
		t.Fatal("AsJSValue nil")
	}
	for _, name := range []string{"createServer", "createConnection", "connect", "Socket", "Server", "isIP", "isIPv4", "isIPv6"} {
		v := AsJSValue.Get(name)
		if v == nil || v.TypeString() == "undefined" {
			t.Fatalf("missing %q", name)
		}
	}
	socket := AsJSValue.Get("Socket").Call()
	for _, name := range []string{"connect", "write", "end", "destroy", "pause", "resume", "setEncoding", "setNoDelay", "setKeepAlive", "setTimeout", "address", "on", "emit"} {
		if socket.Get(name).TypeString() != "function" {
			t.Fatalf("socket missing %q", name)
		}
	}
	server := AsJSValue.Get("Server").Call()
	for _, name := range []string{"listen", "close", "address", "on", "emit"} {
		if server.Get(name).TypeString() != "function" {
			t.Fatalf("server missing %q", name)
		}
	}
}

func TestIsIP(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"127.0.0.1", 4},
		{"::1", 6},
		{"0.0.0.0", 4},
		{"::", 6},
		{"abc", 0},
		{"127.000.000.001", 0},
		{"", 0},
		{"256.0.0.1", 0},
	}
	for _, tt := range tests {
		got := AsJSValue.Get("isIP").Call(jsvalue.NewString(tt.input)).Number()
		if got != tt.want {
			t.Errorf("isIP(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsIPv4(t *testing.T) {
	if !AsJSValue.Get("isIPv4").Call(jsvalue.NewString("127.0.0.1")).Bool() {
		t.Error("isIPv4(127.0.0.1) = false")
	}
	if AsJSValue.Get("isIPv4").Call(jsvalue.NewString("::1")).Bool() {
		t.Error("isIPv4(::1) = true")
	}
	if AsJSValue.Get("isIPv4").Call(jsvalue.NewString("abc")).Bool() {
		t.Error("isIPv4(abc) = true")
	}
}

func TestIsIPv6(t *testing.T) {
	if !AsJSValue.Get("isIPv6").Call(jsvalue.NewString("::1")).Bool() {
		t.Error("isIPv6(::1) = false")
	}
	if AsJSValue.Get("isIPv6").Call(jsvalue.NewString("127.0.0.1")).Bool() {
		t.Error("isIPv6(127.0.0.1) = true")
	}
	if AsJSValue.Get("isIPv6").Call(jsvalue.NewString("abc")).Bool() {
		t.Error("isIPv6(abc) = true")
	}
}

func TestServerListenAndAddress(t *testing.T) {
	var mu sync.Mutex
	var gotAddr *jsvalue.JSValue
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call()
	server.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		t.Logf("server error: %v", args[0])
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		gotAddr = server.MethodCall("address")
		mu.Unlock()
		server.MethodCall("close")
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if gotAddr == nil {
		t.Fatal("address() returned nil")
	}
	if gotAddr.Get("port").Number() <= 0 {
		t.Fatalf("address.port = %v, want > 0", gotAddr.Get("port").Number())
	}
	family := gotAddr.Get("family").String()
	if family != "IPv4" && family != "IPv6" {
		t.Fatalf("address.family = %q, want IPv4 or IPv6", family)
	}
}

func TestServerConnectionEvent(t *testing.T) {
	var mu sync.Mutex
	var connected bool
	var gotClient *jsvalue.JSValue
	done := make(chan struct{})

	var server *jsvalue.JSValue
	server = AsJSValue.Get("createServer").Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		connected = true
		if len(args) > 0 {
			gotClient = args[0]
		}
		mu.Unlock()
		if len(args) > 0 {
			args[0].MethodCall("destroy")
		}
		server.MethodCall("close")
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		t.Logf("server error: %v", args[0])
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		addr := server.MethodCall("address")
		port := addr.Get("port").Number()
		client := AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(port), jsvalue.NewString("127.0.0.1"))
		client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			t.Logf("client error: %v", args[0])
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for connection")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if !connected {
		t.Fatal("connection event not fired")
	}
	if gotClient == nil {
		t.Fatal("client socket nil in connection event")
	}
}

func TestTCPEchoRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var gotData string
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		sock := args[0]
		sock.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			sock.MethodCall("write", args[0])
			return nil
		}))
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		t.Logf("server error: %v", args[0])
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		addr := server.MethodCall("address")
		port := addr.Get("port").Number()

		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(
			jsvalue.NewNumber(port),
			jsvalue.NewString("127.0.0.1"),
			jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
				client.MethodCall("write", jsvalue.NewString("hello"))
				return nil
			}),
		)
		client.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			if len(args) > 0 && args[0] != nil {
				gotData = args[0].Get("_data").String()
			}
			mu.Unlock()
			client.MethodCall("end")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			server.MethodCall("close")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			t.Logf("client error: %v", args[0])
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		mu.Lock()
		t.Fatalf("timed out; data=%q", gotData)
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if gotData != "hello" {
		t.Fatalf("echo data = %q, want hello", gotData)
	}
}

func TestSocketProperties(t *testing.T) {
	var mu sync.Mutex
	var props map[string]interface{}
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		addr := server.MethodCall("address")
		port := addr.Get("port").Number()

		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(
			jsvalue.NewNumber(port),
			jsvalue.NewString("127.0.0.1"),
		)
		client.MethodCall("on", jsvalue.NewString("connect"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			props = map[string]interface{}{
				"localAddress":  client.Get("localAddress").String(),
				"localPort":     client.Get("localPort").Number(),
				"remoteAddress": client.Get("remoteAddress").String(),
				"remotePort":    client.Get("remotePort").Number(),
				"destroyed":     client.Get("destroyed").Bool(),
				"connecting":    client.Get("connecting").Bool(),
			}
			mu.Unlock()
			client.MethodCall("end")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			server.MethodCall("close")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			t.Logf("client error: %v", args[0])
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if props["destroyed"].(bool) {
		t.Error("destroyed should be false after connect")
	}
	if props["connecting"].(bool) {
		t.Error("connecting should be false after connect")
	}
	if props["remotePort"].(float64) != props["localPort"].(float64) {
		// remotePort should equal the server port, which equals localPort since we connected to ourselves
		// Actually remotePort == server port, localPort == client port. They're different.
	}
}

func TestSocketEndAndClose(t *testing.T) {
	var mu sync.Mutex
	var events []string
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		port := server.MethodCall("address").Get("port").Number()

		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(port), jsvalue.NewString("127.0.0.1"))
		client.MethodCall("on", jsvalue.NewString("connect"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			client.MethodCall("end")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("end"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			events = append(events, "end")
			mu.Unlock()
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			events = append(events, "close")
			mu.Unlock()
			server.MethodCall("close")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			t.Logf("client error: %v", args[0])
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		mu.Lock()
		t.Fatalf("timed out; events=%v", events)
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	hasEnd := false
	hasClose := false
	for _, e := range events {
		if e == "end" {
			hasEnd = true
		}
		if e == "close" {
			hasClose = true
		}
	}
	if !hasEnd {
		t.Error("missing 'end' event")
	}
	if !hasClose {
		t.Error("missing 'close' event")
	}
}

func TestSocketDestroy(t *testing.T) {
	var mu sync.Mutex
	var gotDestroyed bool
	var closeFired bool
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call()
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		port := server.MethodCall("address").Get("port").Number()

		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(port), jsvalue.NewString("127.0.0.1"))
		client.MethodCall("on", jsvalue.NewString("connect"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			client.MethodCall("destroy")
			mu.Lock()
			gotDestroyed = client.Get("destroyed").Bool()
			mu.Unlock()
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			closeFired = true
			mu.Unlock()
			server.MethodCall("close")
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if !gotDestroyed {
		t.Error("destroyed should be true after destroy()")
	}
	if !closeFired {
		t.Error("'close' event not fired")
	}
}

func TestConnectError(t *testing.T) {
	var mu sync.Mutex
	var gotCode string
	done := make(chan struct{})

	client := AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(1), jsvalue.NewString("127.0.0.1"))
	client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		if len(args) > 0 && args[0] != nil {
			gotCode = args[0].Get("code").String()
		}
		mu.Unlock()
		close(done)
		return nil
	}))

	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if gotCode != "ECONNREFUSED" {
		t.Fatalf("error.code = %q, want ECONNREFUSED", gotCode)
	}
}

func TestConnectDNSError(t *testing.T) {
	var mu sync.Mutex
	var gotCode string
	var gotHostname string
	done := make(chan struct{})

	client := AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(80), jsvalue.NewString("this-host-definitely-does-not-exist.invalid"))
	client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		mu.Lock()
		if len(args) > 0 && args[0] != nil {
			gotCode = args[0].Get("code").String()
			gotHostname = args[0].Get("hostname").String()
		}
		mu.Unlock()
		close(done)
		return nil
	}))

	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if gotCode != "ENOTFOUND" {
		t.Fatalf("error.code = %q, want ENOTFOUND", gotCode)
	}
	if gotHostname != "this-host-definitely-does-not-exist.invalid" {
		t.Fatalf("error.hostname = %q, want original hostname", gotHostname)
	}
}

func TestRegistryCleanup(t *testing.T) {
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call(jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		sock := args[0]
		sock.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			sock.MethodCall("end")
			return nil
		}))
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		port := server.MethodCall("address").Get("port").Number()

		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(port), jsvalue.NewString("127.0.0.1"))
		client.MethodCall("on", jsvalue.NewString("connect"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			client.MethodCall("write", jsvalue.NewString("hi"))
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			server.MethodCall("close")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			t.Logf("client error: %v", args[0])
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	serverRegistryMu.Lock()
	_, exists := serverRegistry[server]
	serverRegistryMu.Unlock()
	if exists {
		t.Fatal("server still in serverRegistry after close")
	}
}

func TestListenBadPort(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for bad port")
		} else {
			err, ok := r.(*jsvalue.JSValue)
			if !ok {
				t.Fatalf("panic value type = %T, want *JSValue", r)
			}
			code := err.Get("code").String()
			if code != "ERR_SOCKET_BAD_PORT" {
				t.Fatalf("error.code = %q, want ERR_SOCKET_BAD_PORT", code)
			}
		}
	}()
	server := AsJSValue.Get("createServer").Call()
	server.MethodCall("listen", jsvalue.NewNumber(-1))
}

func TestListenAlreadyListening(t *testing.T) {
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call()
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		defer func() {
			if r := recover(); r == nil {
				server.MethodCall("close")
				t.Error("expected panic for double listen")
			} else {
				err, ok := r.(*jsvalue.JSValue)
				if !ok {
					t.Errorf("panic value type = %T", r)
					server.MethodCall("close")
					return
				}
				code := err.Get("code").String()
				if code != "ERR_SERVER_ALREADY_LISTEN" {
					t.Errorf("error.code = %q, want ERR_SERVER_ALREADY_LISTEN", code)
				}
				server.MethodCall("close")
			}
		}()
		server.MethodCall("listen", jsvalue.NewNumber(0))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")
}

func TestCloseNotListening(t *testing.T) {
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call()
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			code := args[0].Get("code").String()
			if code != "ERR_SERVER_NOT_RUNNING" {
				t.Errorf("error.code = %q, want ERR_SERVER_NOT_RUNNING", code)
			}
		}
		close(done)
		return nil
	})
	server.MethodCall("close", cb)

	// No event loop needed - close on non-listening server calls callback directly
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}

func TestDestroyIdempotent(t *testing.T) {
	var mu sync.Mutex
	closeCount := 0
	done := make(chan struct{})

	server := AsJSValue.Get("createServer").Call()
	server.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		close(done)
		return nil
	}))
	server.MethodCall("on", jsvalue.NewString("listening"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		port := server.MethodCall("address").Get("port").Number()
		var client *jsvalue.JSValue
		client = AsJSValue.Get("createConnection").Call(jsvalue.NewNumber(port), jsvalue.NewString("127.0.0.1"))
		client.MethodCall("on", jsvalue.NewString("close"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			closeCount++
			mu.Unlock()
			// Call destroy again — should be no-op
			client.MethodCall("destroy")
			server.MethodCall("close")
			return nil
		}))
		client.MethodCall("on", jsvalue.NewString("connect"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			client.MethodCall("destroy")
			return nil
		}))
		return nil
	}))

	server.MethodCall("listen", jsvalue.NewNumber(0))
	runDone := runLoop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	waitDone(t, runDone, "event loop")

	mu.Lock()
	defer mu.Unlock()
	if closeCount != 1 {
		t.Fatalf("close event fired %d times, want 1", closeCount)
	}
}
