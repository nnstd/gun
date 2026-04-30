package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func BenchmarkRequestFromHTTP(b *testing.B) {
	body := `{"name":"Alice","email":"alice@example.com","age":30}`
	req := httptest.NewRequest("POST", "/users?foo=bar", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RequestFromHTTP(req)
	}
}

func BenchmarkRequestFromFastHTTP(b *testing.B) {
	var req fasthttp.Request
	req.SetRequestURI("http://127.0.0.1/users?foo=bar")
	req.Header.SetMethod("POST")
	req.SetBodyString(`{"name":"Alice","email":"alice@example.com","age":30}`)

	ctx := &fasthttp.RequestCtx{}
	req.CopyTo(&ctx.Request)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := RequestFromFastHTTP(ctx)
		ReleaseFastHTTPRequest(r)
	}
}

func BenchmarkRequestCall(b *testing.B) {
	init := jsvalue.ObjectFrom(
		"method", jsvalue.NewString("POST"),
		"headers", Headers.Call(),
		"body", jsvalue.NewString(`{"ok":true}`),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Request.Call(jsvalue.NewString("http://127.0.0.1/users"), init)
	}
}

func BenchmarkResponseCall(b *testing.B) {
	init := jsvalue.ObjectFrom(
		"status", jsvalue.NewNumber(201),
		"headers", Headers.Call(),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Response.Call(jsvalue.NewString(`{"ok":true}`), init)
	}
}

func TestRequestFromHTTPExposesRequestMethods(t *testing.T) {
	req := httptest.NewRequest("POST", "/users?foo=bar", strings.NewReader(`{"ok":true}`))
	got := RequestFromHTTP(req)
	if got.Get("method").String() != "POST" {
		t.Fatalf("method = %q, want POST", got.Get("method").String())
	}
	if got.MethodCall("text").TypeString() != "object" {
		t.Fatalf("text() should return a Promise-like object")
	}
}

func TestResponseCallBuildsResponseObject(t *testing.T) {
	res := Response.Call(
		jsvalue.NewString(`{"ok":true}`),
		jsvalue.ObjectFrom("status", jsvalue.NewNumber(201)),
	)
	if res.Get("status").Number() != 201 {
		t.Fatalf("status = %v, want 201", res.Get("status").Number())
	}
	if !res.Get("ok").Bool() {
		t.Fatalf("ok should be true")
	}
}

func BenchmarkWriteResponse(b *testing.B) {
	res := jsvalue.NewObject()
	res.Set("status", jsvalue.NewNumber(200))
	res.Set("_bodyInit", jsvalue.NewString(`{"hello":"world"}`))
	headers := Headers.Call()
	headers.Set("content-type", jsvalue.NewString("application/json"))
	res.Set("headers", headers)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		WriteResponse(w, res)
	}
}
