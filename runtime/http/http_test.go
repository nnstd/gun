package nodehttp

import (
	"strconv"
	"testing"
)

func TestSurface(t *testing.T) {
	if AsJSValue == nil {
		t.Fatal("AsJSValue nil")
	}
	if HTTPSAsJSValue == nil {
		t.Fatal("HTTPSAsJSValue nil")
	}

	httpKeys := []string{
		"createServer", "request", "get", "METHODS", "STATUS_CODES",
		"globalAgent", "validateHeaderName", "validateHeaderValue",
		"maxHeaderSize", "Server", "ServerResponse", "IncomingMessage",
		"ClientRequest", "Agent",
	}
	for _, k := range httpKeys {
		if AsJSValue.Get(k).TypeString() == "undefined" {
			t.Errorf("http missing key %q", k)
		}
	}

	httpsKeys := []string{
		"createServer", "request", "get", "globalAgent", "Server", "Agent",
	}
	for _, k := range httpsKeys {
		if HTTPSAsJSValue.Get(k).TypeString() == "undefined" {
			t.Errorf("https missing key %q", k)
		}
	}

	methods := AsJSValue.Get("METHODS")
	if !methods.IsArray() {
		t.Fatal("METHODS not array")
	}
	if got := methods.Len(); got != 35 {
		t.Errorf("METHODS length = %d, want 35", got)
	}

	if got := AsJSValue.Get("STATUS_CODES").Get("200").String(); got != "OK" {
		t.Errorf("STATUS_CODES.200 = %q, want OK", got)
	}
	if got := AsJSValue.Get("STATUS_CODES").Get("404").String(); got != "Not Found" {
		t.Errorf("STATUS_CODES.404 = %q, want Not Found", got)
	}

	if got := AsJSValue.Get("globalAgent").Get("defaultPort").Number(); got != 80 {
		t.Errorf("http.globalAgent.defaultPort = %v, want 80", got)
	}
	if got := HTTPSAsJSValue.Get("globalAgent").Get("defaultPort").Number(); got != 443 {
		t.Errorf("https.globalAgent.defaultPort = %v, want 443", got)
	}
}

func TestValidateHeaderName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on bad header name")
		}
	}()
	fn := AsJSValue.Get("validateHeaderName")
	bad := mustJSString("foo\nbar")
	fn.Call(bad)
}

func TestValidateHeaderNameOK(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn := AsJSValue.Get("validateHeaderName")
	fn.Call(mustJSString("Content-Type"))
}

func TestValidateHeaderValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on bad header value")
		}
	}()
	fn := AsJSValue.Get("validateHeaderValue")
	fn.Call(mustJSString("X-Foo"), mustJSString("line1\r\nline2"))
}

func TestStatusCodesAllPopulated(t *testing.T) {
	for code, want := range statusCodeReasons {
		got := AsJSValue.Get("STATUS_CODES").Get(strconv.Itoa(code)).String()
		if got != want {
			t.Errorf("STATUS_CODES[%d] = %q, want %q", code, got, want)
		}
	}
}
