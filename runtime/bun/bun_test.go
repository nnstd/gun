package bun

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/web"
)

type fakeAddr struct{ value string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.value }

type fakeListener struct {
	addr   net.Addr
	closed chan struct{}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, errors.New("closed")
}

func (l *fakeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *fakeListener) Addr() net.Addr { return l.addr }

func TestServeReturnsServerObject(t *testing.T) {
	orig := listenFn
	l := &fakeListener{addr: fakeAddr{value: "127.0.0.1:43110"}, closed: make(chan struct{})}
	listenFn = func(network, address string) (net.Listener, error) {
		return l, nil
	}
	defer func() { listenFn = orig }()

	server := Serve(jsvalue.ObjectFrom(
		"port", jsvalue.NewNumber(43110),
		"fetch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		}),
	))

	if got := int(server.Get("port").Number()); got != 43110 {
		t.Fatalf("unexpected port: %d", got)
	}
	if server.Get("stop").TypeString() != "function" {
		t.Fatal("expected stop() on Bun server object")
	}
	server.MethodCall("stop")
}

func TestYAMLParse(t *testing.T) {
	parsed := AsJSValue.Get("YAML").Get("parse").Call(jsvalue.NewString("name: Jane\nhobbies:\n  - reading\n  - coding\n"))
	if got := parsed.Get("name").String(); got != "Jane" {
		t.Fatalf("name = %q", got)
	}
	if got := parsed.Get("hobbies").Index(1).String(); got != "coding" {
		t.Fatalf("hobbies[1] = %q", got)
	}
}

func TestYAMLParseInvalidThrowsSyntaxError(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "SyntaxError" {
			t.Fatalf("error name = %q, want SyntaxError", got)
		}
	}()
	AsJSValue.Get("YAML").Get("parse").Call(jsvalue.NewString("invalid: yaml: content:"))
}

func TestYAMLStringifyFlowStyleByDefault(t *testing.T) {
	obj := jsvalue.ObjectFrom(
		"abc", jsvalue.NewString("def"),
		"num", jsvalue.NewNumber(123),
	)
	got := AsJSValue.Get("YAML").Get("stringify").Call(obj).String()
	if !strings.Contains(got, "{") || !strings.Contains(got, "abc: def") || !strings.Contains(got, "num: 123") {
		t.Fatalf("unexpected flow YAML: %q", got)
	}
}

func TestYAMLStringifyBlockStyleWithSpace(t *testing.T) {
	obj := jsvalue.ObjectFrom(
		"abc", jsvalue.NewString("def"),
		"nested", jsvalue.ObjectFrom("num", jsvalue.NewNumber(123)),
	)
	got := AsJSValue.Get("YAML").Get("stringify").Call(obj, jsvalue.NewNull(), jsvalue.NewNumber(2)).String()
	if !strings.Contains(got, "abc: def") || !strings.Contains(got, "\nnested:\n  num: 123") {
		t.Fatalf("unexpected block YAML: %q", got)
	}
}

func TestWriteResponseFromFetchResult(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	result := jsvalue.ObjectFrom(
		"status", jsvalue.NewNumber(201),
		"_bodyInit", jsvalue.NewString("bun-ok"),
		"headers", jsvalue.ObjectFrom("content-type", jsvalue.NewString("text/plain")),
	)

	web.WriteResponse(rec, result)

	if rec.Code != 201 {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if body := rec.Body.String(); body != "bun-ok" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := rec.Header().Get("content-type"); got != "text/plain" {
		t.Fatalf("unexpected content-type: %q", got)
	}
	_ = req
}

func TestEventLoopRunReturnsWhenNoActiveServers(t *testing.T) {
	done := make(chan struct{})
	go func() {
		eventloop.Default.Run()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("eventloop.Run blocked with no active servers")
	}
}

func TestServeRejectsNonObjectOptions(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("error name = %q, want TypeError", got)
		}
		if got := err.Get("message").String(); got != "Bun.serve expects an object" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
	}()
	Serve(jsvalue.NewNumber(123))
}

func TestServeRejectsMissingFetchOrRoutes(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("error name = %q, want TypeError", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
		if got := err.Get("message").String(); !strings.Contains(got, "Bun.serve() needs either:") {
			t.Fatalf("message = %q", got)
		}
	}()
	Serve(jsvalue.ObjectFrom("port", jsvalue.NewNumber(0)))
}

func TestServeRejectsNonFunctionFetch(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("message").String(); got != "Expected fetch() to be a function" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("code").String(); got != "ERR_INVALID_ARG_TYPE" {
			t.Fatalf("code = %q", got)
		}
	}()
	Serve(jsvalue.ObjectFrom("port", jsvalue.NewNumber(0), "fetch", jsvalue.NewNumber(123)))
}

func TestServeRejectsPortAlreadyInUseLikeBun(t *testing.T) {
	orig := listenFn
	listenFn = func(network, address string) (net.Listener, error) {
		return nil, &net.OpError{Op: "listen", Net: network, Err: &os.SyscallError{Syscall: "listen", Err: syscall.EADDRINUSE}}
	}
	defer func() { listenFn = orig }()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic = %T, want *jsvalue.JSValue", r)
		}
		if got := err.Get("name").String(); got != "Error" {
			t.Fatalf("error name = %q, want Error", got)
		}
		if got := err.Get("message").String(); got != "Failed to start server. Is port 3029 in use?" {
			t.Fatalf("message = %q", got)
		}
		if got := err.Get("syscall").String(); got != "listen" {
			t.Fatalf("syscall = %q", got)
		}
		if got := err.Get("errno").Number(); got != 0 {
			t.Fatalf("errno = %v", got)
		}
		if got := err.Get("code").String(); got != "EADDRINUSE" {
			t.Fatalf("code = %q", got)
		}
	}()

	Serve(jsvalue.ObjectFrom(
		"port", jsvalue.NewNumber(3029),
		"fetch", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return jsvalue.NewUndefined() }),
	))
}
