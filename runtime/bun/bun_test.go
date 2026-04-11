package bun

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
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

func TestWaitReturnsWhenNoActiveServers(t *testing.T) {
	done := make(chan struct{})
	go func() {
		Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait blocked with no active servers")
	}
}
