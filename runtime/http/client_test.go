package nodehttp

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// awaitResponse drives a ClientRequest and returns the response IncomingMessage
// + collected body once the response 'end' event fires.
func awaitResponse(t *testing.T, req *jsvalue.JSValue) (*jsvalue.JSValue, string) {
	t.Helper()
	var resp *jsvalue.JSValue
	var mu sync.Mutex
	var bodyBuf string
	respCh := make(chan *jsvalue.JSValue, 1)
	endCh := make(chan struct{}, 1)
	errCh := make(chan string, 1)

	req.MethodCall("on", jsvalue.NewString("response"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewUndefined()
		}
		r := args[0]
		respCh <- r
		r.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
			mu.Lock()
			bodyBuf += a[0].String()
			mu.Unlock()
			return jsvalue.NewUndefined()
		}))
		r.MethodCall("on", jsvalue.NewString("end"), jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
			endCh <- struct{}{}
			return jsvalue.NewUndefined()
		}))
		return jsvalue.NewUndefined()
	}))
	req.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			errCh <- args[0].Get("message").String()
		}
		return jsvalue.NewUndefined()
	}))

	select {
	case resp = <-respCh:
	case msg := <-errCh:
		t.Fatalf("client request error: %s", msg)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for response event")
	}
	select {
	case <-endCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for response end event")
	}
	mu.Lock()
	defer mu.Unlock()
	return resp, bodyBuf
}

func TestClientGet(t *testing.T) {
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("setHeader", jsvalue.NewString("X-Echo"), jsvalue.NewString("v"))
		res.MethodCall("end", jsvalue.NewString("hello-from-server"))
	})
	defer teardown()

	req := ClientRequest(false, true, jsvalue.NewString("http://"+addr+"/path"))
	resp, body := awaitResponse(t, req)

	if resp.Get("statusCode").Number() != 200 {
		t.Errorf("statusCode = %v, want 200", resp.Get("statusCode").Number())
	}
	if resp.Get("httpVersion").String() != "1.1" {
		t.Errorf("httpVersion = %s, want 1.1", resp.Get("httpVersion").String())
	}
	if v := resp.Get("headers").Get("x-echo").String(); v != "v" {
		t.Errorf("x-echo header = %q, want v", v)
	}
	if body != "hello-from-server" {
		t.Errorf("body = %q, want hello-from-server", body)
	}
}

func TestClientRequestPOST(t *testing.T) {
	var seenBody atomic.Value
	seenBody.Store("")
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		buf := ""
		req.MethodCall("on", jsvalue.NewString("data"), jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
			buf += a[0].String()
			return jsvalue.NewUndefined()
		}))
		req.MethodCall("on", jsvalue.NewString("end"), jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
			seenBody.Store(buf)
			res.MethodCall("end", jsvalue.NewString("ack"))
			return jsvalue.NewUndefined()
		}))
	})
	defer teardown()

	req := ClientRequest(false, false, jsvalue.ObjectFrom(
		"method", jsvalue.NewString("POST"),
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"headers", jsvalue.ObjectFrom("Content-Type", jsvalue.NewString("text/plain")),
	))
	req.MethodCall("write", jsvalue.NewString("part1-"))
	req.MethodCall("end", jsvalue.NewString("part2"))

	resp, body := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 {
		t.Errorf("statusCode = %v, want 200", resp.Get("statusCode").Number())
	}
	if body != "ack" {
		t.Errorf("body = %q, want ack", body)
	}
	if got, _ := seenBody.Load().(string); got != "part1-part2" {
		t.Errorf("server saw body = %q, want part1-part2", got)
	}
}

func TestClientCallbackInvocation(t *testing.T) {
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	cbFired := make(chan *jsvalue.JSValue, 1)
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			cbFired <- args[0]
		}
		return jsvalue.NewUndefined()
	})
	req := ClientRequest(false, false, jsvalue.NewString("http://"+addr+"/"), cb)
	req.MethodCall("end")

	select {
	case r := <-cbFired:
		if r.Get("statusCode").Number() != 200 {
			t.Errorf("cb arg statusCode = %v, want 200", r.Get("statusCode").Number())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("response cb did not fire")
	}
}

func TestClientErrorECONNREFUSED(t *testing.T) {
	req := ClientRequest(false, false, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(1), // very unlikely to be open
		"path", jsvalue.NewString("/"),
		"timeout", jsvalue.NewNumber(500),
	))
	errCh := make(chan *jsvalue.JSValue, 1)
	req.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			errCh <- args[0]
		}
		return jsvalue.NewUndefined()
	}))
	req.MethodCall("end")

	select {
	case e := <-errCh:
		if e.Get("message").String() == "" {
			t.Error("error message empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected error event")
	}
}

func parsePort(addr string) int {
	// "127.0.0.1:NNNNN"
	i := 0
	for i < len(addr) && addr[i] != ':' {
		i++
	}
	if i == len(addr) {
		return 0
	}
	n := 0
	for j := i + 1; j < len(addr); j++ {
		n = n*10 + int(addr[j]-'0')
	}
	return n
}
