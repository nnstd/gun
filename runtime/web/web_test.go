package web

import (
	"net/http/httptest"
	"strings"
	"testing"

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
