package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/eventloop"
	nodehttp "github.com/nnstd/gun/runtime/http"
)

func newLocalServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping fetch test: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	return srv
}

func awaitPromise(t *testing.T, p *jsvalue.JSValue) (*jsvalue.JSValue, *jsvalue.JSValue) {
	t.Helper()
	valueCh := make(chan *jsvalue.JSValue, 1)
	errCh := make(chan *jsvalue.JSValue, 1)

	p.MethodCall("then",
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				valueCh <- args[0]
			} else {
				valueCh <- jsvalue.NewUndefined()
			}
			return jsvalue.NewUndefined()
		}),
		jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) > 0 && args[0] != nil {
				errCh <- args[0]
			} else {
				errCh <- jsvalue.NewUndefined()
			}
			return jsvalue.NewUndefined()
		}),
	)

	done := make(chan struct{})
	go func() {
		eventloop.Default.Run()
		close(done)
	}()

	var value *jsvalue.JSValue
	var errVal *jsvalue.JSValue
	select {
	case value = <-valueCh:
	case errVal = <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for promise settlement")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not exit")
	}
	return value, errVal
}

func TestFetchSuccessAndResponseReaders(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	value, errVal := awaitPromise(t, Fetch.Call(jsvalue.NewString(srv.URL)))
	if errVal != nil {
		t.Fatalf("unexpected fetch rejection: %s", errVal.String())
	}
	if value.Get("status").Number() != 201 {
		t.Fatalf("status = %v, want 201", value.Get("status").Number())
	}
	if !value.Get("ok").Bool() {
		t.Fatal("expected ok=true")
	}
	if got := value.Get("headers").MethodCall("get", jsvalue.NewString("x-test")).String(); got != "yes" {
		t.Fatalf("x-test header = %q, want yes", got)
	}

	textPromise := value.MethodCall("text")
	if textPromise.TypeString() != "object" {
		t.Fatalf("text() should return Promise-like object, got %s", textPromise.TypeString())
	}
	text, textErr := awaitPromise(t, textPromise)
	if textErr != nil {
		t.Fatalf("unexpected text() rejection: %s", textErr.String())
	}
	if text.String() != `{"ok":true}` {
		t.Fatalf("text() = %q", text.String())
	}
	if !value.Get("bodyUsed").Bool() {
		t.Fatal("expected bodyUsed=true after text()")
	}
}

func TestFetchResolvesHTTPError(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer srv.Close()

	value, errVal := awaitPromise(t, Fetch.Call(jsvalue.NewString(srv.URL)))
	if errVal != nil {
		t.Fatalf("unexpected fetch rejection: %s", errVal.String())
	}
	if value.Get("status").Number() != 404 {
		t.Fatalf("status = %v, want 404", value.Get("status").Number())
	}
	if value.Get("ok").Bool() {
		t.Fatal("expected ok=false")
	}
}

func TestFetchRejectsNetworkFailure(t *testing.T) {
	value, errVal := awaitPromise(t, Fetch.Call(jsvalue.NewString("http://127.0.0.1:1/")))
	if value != nil {
		t.Fatalf("expected rejection, got resolution: %v", value)
	}
	if errVal == nil {
		t.Fatal("expected rejection value")
	}
	if errVal.Get("name").String() != "TypeError" {
		t.Fatalf("error name = %q, want TypeError", errVal.Get("name").String())
	}
}

func TestFetchAcceptsURLObjectAndRegistersGlobal(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if jsvalue.Globals()["fetch"] == nil {
		t.Fatal("expected fetch to be globally registered")
	}

	urlObj := ParseURL(jsvalue.NewString(srv.URL))
	value, errVal := awaitPromise(t, Fetch.Call(urlObj))
	if errVal != nil {
		t.Fatalf("unexpected fetch rejection: %s", errVal.String())
	}
	text, textErr := awaitPromise(t, value.MethodCall("text"))
	if textErr != nil {
		t.Fatalf("unexpected text() rejection: %s", textErr.String())
	}
	if text.String() != "ok" {
		t.Fatalf("text() = %q, want ok", text.String())
	}
}

func TestFetchConsumesSourceRequestBody(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	req := Request.Call(jsvalue.NewString(srv.URL), jsvalue.ObjectFrom(
		"method", jsvalue.NewString("POST"),
		"body", jsvalue.NewString("payload"),
	))
	if req.Get("bodyUsed").Bool() {
		t.Fatal("bodyUsed should start false")
	}

	value, errVal := awaitPromise(t, Fetch.Call(req))
	if errVal != nil {
		t.Fatalf("unexpected fetch rejection: %s", errVal.String())
	}
	if !req.Get("bodyUsed").Bool() {
		t.Fatal("expected source request bodyUsed=true after fetch(request)")
	}
	if _, textErr := awaitPromise(t, value.MethodCall("text")); textErr != nil {
		t.Fatalf("unexpected response.text() rejection: %s", textErr.String())
	}
}

func TestResponseJSONRejectsSyntaxError(t *testing.T) {
	res := Response.Call(jsvalue.NewString("not-json"))
	value, errVal := awaitPromise(t, res.MethodCall("json"))
	if value != nil {
		t.Fatalf("expected rejection, got resolution: %v", value)
	}
	if errVal == nil {
		t.Fatal("expected rejection value")
	}
	if errVal.Get("name").String() != "SyntaxError" {
		t.Fatalf("error name = %q, want SyntaxError", errVal.Get("name").String())
	}
}

func TestFetchRejectsInvalidURLAsTypeError(t *testing.T) {
	value, errVal := awaitPromise(t, Fetch.Call(jsvalue.NewString("/relative")))
	if value != nil {
		t.Fatalf("expected rejection, got resolution: %v", value)
	}
	if errVal == nil {
		t.Fatal("expected rejection value")
	}
	if errVal.Get("name").String() != "TypeError" {
		t.Fatalf("error name = %q, want TypeError", errVal.Get("name").String())
	}
}

func TestFetchRejectsConsumedRequestBody(t *testing.T) {
	req := Request.Call(jsvalue.NewString("https://example.com"), jsvalue.ObjectFrom(
		"method", jsvalue.NewString("POST"),
		"body", jsvalue.NewString("payload"),
	))
	if _, errVal := awaitPromise(t, req.MethodCall("text")); errVal != nil {
		t.Fatalf("unexpected request.text() rejection: %s", errVal.String())
	}

	value, errVal := awaitPromise(t, Fetch.Call(req))
	if value != nil {
		t.Fatalf("expected rejection, got resolution: %v", value)
	}
	if errVal == nil {
		t.Fatal("expected rejection value")
	}
	if errVal.Get("name").String() != "TypeError" {
		t.Fatalf("error name = %q, want TypeError", errVal.Get("name").String())
	}
}

func TestFetchRejectsCredentialedURLAsTypeError(t *testing.T) {
	value, errVal := awaitPromise(t, Fetch.Call(jsvalue.NewString("http://user:pass@example.com/path")))
	if value != nil {
		t.Fatalf("expected rejection, got resolution: %v", value)
	}
	if errVal == nil {
		t.Fatal("expected rejection value")
	}
	if errVal.Get("name").String() != "TypeError" {
		t.Fatalf("error name = %q, want TypeError", errVal.Get("name").String())
	}
}

func TestResponseFromTransportPreservesExactURL(t *testing.T) {
	resp := responseFromTransport(&nodehttp.TransportResponse{
		StatusCode: 200,
		StatusText: "OK",
		Headers:    map[string]string{"content-type": "text/plain"},
		Body:       []byte("ok"),
		URL:        "https://example.com/path?x=1",
	})
	if got := resp.Get("url").String(); got != "https://example.com/path?x=1" {
		t.Fatalf("response.url = %q, want https://example.com/path?x=1", got)
	}
}

func TestRequestCopiesExistingRequestAndRejectsInvalidInput(t *testing.T) {
	original := Request.Call(jsvalue.NewString("https://example.com/path"), jsvalue.ObjectFrom(
		"method", jsvalue.NewString("POST"),
		"headers", jsvalue.ObjectFrom("x-test", jsvalue.NewString("yes")),
		"body", jsvalue.NewString("payload"),
	))
	cloned := Request.Call(original)
	if cloned.Get("url").String() != "https://example.com/path" {
		t.Fatalf("cloned url = %q", cloned.Get("url").String())
	}
	if cloned.Get("method").String() != "POST" {
		t.Fatalf("cloned method = %q", cloned.Get("method").String())
	}
	if cloned.Get("headers").MethodCall("get", jsvalue.NewString("x-test")).String() != "yes" {
		t.Fatalf("cloned header x-test = %q", cloned.Get("headers").MethodCall("get", jsvalue.NewString("x-test")).String())
	}
	if cloned.Get("body").String() != "payload" {
		t.Fatalf("cloned body = %q", cloned.Get("body").String())
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected invalid Request input panic")
		}
	}()
	Request.Call(jsvalue.NewString("/relative"))
}
