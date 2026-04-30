package nodehttp

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
)

// pumpEventLoop starts a background goroutine that drains the event loop's
// job channel so ScheduleCallback work executes. Unlike Run(), Pump never exits,
// which is safe for tests where the process terminates after all tests complete.
func pumpEventLoop() {
	eventloop.Default.Pump()
}

// startTestServer creates a Server, listens on an ephemeral port, and returns
// the address + a teardown func. handler runs as the createServer listener.
func startTestServer(t *testing.T, handler func(req, res *jsvalue.JSValue)) (addr string, srv *jsvalue.JSValue, teardown func()) {
	t.Helper()
	listener := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && handler != nil {
			handler(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	})
	srv = CreateServer(false, listener)
	pumpEventLoop()

	ready := make(chan string, 1)
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		a := srv.MethodCall("address")
		if a.TypeString() == "object" {
			port := int(a.Get("port").Number())
			ready <- ("127.0.0.1:" + itoa(port))
		}
		return jsvalue.NewUndefined()
	})
	srv.MethodCall("listen", jsvalue.NewNumber(0), jsvalue.NewString("127.0.0.1"), cb)

	select {
	case addr = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("server listen callback did not fire")
	}
	teardown = func() {
		srv.MethodCall("close")
	}
	return
}

func itoa(n int) string {
	buf := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

func TestServerTCP(t *testing.T) {
	addr, srv, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("setHeader", jsvalue.NewString("X-Test"), jsvalue.NewString("yes"))
		res.MethodCall("end", jsvalue.NewString("hello"))
	})
	defer teardown()

	a := srv.MethodCall("address")
	if a.TypeString() != "object" {
		t.Fatalf("address typeof = %s, want object", a.TypeString())
	}
	if a.Get("family").String() != "IPv4" {
		t.Errorf("family = %s, want IPv4", a.Get("family").String())
	}
	if a.Get("address").String() != "127.0.0.1" {
		t.Errorf("address = %s, want 127.0.0.1", a.Get("address").String())
	}
	if a.Get("port").Number() <= 0 {
		t.Errorf("port = %v, want >0", a.Get("port").Number())
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q, want hello", string(body))
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Test") != "yes" {
		t.Errorf("X-Test header = %q, want yes", resp.Header.Get("X-Test"))
	}
}

func TestServerEADDRINUSE(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	srv := CreateServer(false, jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() }))
	pumpEventLoop()

	errCh := make(chan *jsvalue.JSValue, 1)
	srv.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			errCh <- args[0]
		}
		return jsvalue.NewUndefined()
	}))
	srv.MethodCall("listen", jsvalue.NewNumber(float64(port)), jsvalue.NewString("127.0.0.1"))

	select {
	case errVal := <-errCh:
		if errVal.Get("code").String() != "EADDRINUSE" {
			t.Errorf("code = %s, want EADDRINUSE", errVal.Get("code").String())
		}
		if errVal.Get("syscall").String() != "listen" {
			t.Errorf("syscall = %s, want listen", errVal.Get("syscall").String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected error event for EADDRINUSE")
	}
}

func TestServerEmitsRequest(t *testing.T) {
	var seen atomic.Int32
	addr, srv, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		seen.Add(1)
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	// 'request' event listener (in addition to createServer listener).
	srv.MethodCall("on", jsvalue.NewString("request"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		seen.Add(10)
		return jsvalue.NewUndefined()
	}))
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// 1 from listener + 10 from explicit 'request' (createServer-registered listener
	// also fires via emit in handler so total = 11+1 = 12 minimum).
	if got := seen.Load(); got < 11 {
		t.Errorf("seen = %d, want >=11", got)
	}
}

func TestServerResponseHeaders(t *testing.T) {
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("writeHead", jsvalue.NewNumber(201), jsvalue.ObjectFrom(
			"Content-Type", jsvalue.NewString("text/plain"),
			"X-Custom", jsvalue.NewString("v"),
		))
		res.MethodCall("write", jsvalue.NewString("part1"))
		res.MethodCall("end", jsvalue.NewString("-part2"))
	})
	defer teardown()

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "part1-part2" {
		t.Errorf("body = %q, want part1-part2", string(body))
	}
	if resp.StatusCode != 201 {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("X-Custom") != "v" {
		t.Errorf("X-Custom = %q, want v", resp.Header.Get("X-Custom"))
	}
}

func TestIncomingMessage(t *testing.T) {
	var mu sync.Mutex
	var collected string
	var endFired atomic.Bool

	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		method := req.Get("method").String()
		url := req.Get("url").String()
		hdr := req.Get("headers").Get("x-foo").String()
		if method != "POST" {
			t.Errorf("method = %q, want POST", method)
		}
		if url != "/path?x=1" {
			t.Errorf("url = %q, want /path?x=1", url)
		}
		if hdr != "bar" {
			t.Errorf("x-foo = %q, want bar", hdr)
		}

		req.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			collected += args[0].String()
			mu.Unlock()
			return jsvalue.NewUndefined()
		}))
		req.MethodCall("on", jsvalue.NewString("end"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			endFired.Store(true)
			res.MethodCall("end", jsvalue.NewString("done"))
			return jsvalue.NewUndefined()
		}))
	})
	defer teardown()

	req, _ := http.NewRequest("POST", "http://"+addr+"/path?x=1", strings.NewReader("hello world"))
	req.Header.Set("X-Foo", "bar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	// give end event time
	time.Sleep(20 * time.Millisecond)

	if !endFired.Load() {
		t.Error("end event did not fire")
	}
	mu.Lock()
	if collected != "hello world" {
		t.Errorf("collected = %q, want hello world", collected)
	}
	mu.Unlock()
}
