package nodehttp

import (
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestClientHTTPSWithCA(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCert(t)
	addr, _, teardown := startTLSServer(t, certPEM, keyPEM, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("tls-client-ok"))
	})
	defer teardown()

	req := ClientRequest(true, false, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"ca", jsvalue.NewString(certPEM),
		"servername", jsvalue.NewString("localhost"),
	))
	req.MethodCall("end")

	resp, body := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 {
		t.Errorf("statusCode = %v, want 200", resp.Get("statusCode").Number())
	}
	if body != "tls-client-ok" {
		t.Errorf("body = %q, want tls-client-ok", body)
	}
}

func TestClientHTTPSRejectUnauthorized(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCert(t)
	addr, _, teardown := startTLSServer(t, certPEM, keyPEM, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	// rejectUnauthorized:false → request succeeds against self-signed cert.
	req := ClientRequest(true, false, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"rejectUnauthorized", jsvalue.NewBool(false),
	))
	req.MethodCall("end")
	resp, _ := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 {
		t.Errorf("rejectUnauthorized:false statusCode = %v, want 200", resp.Get("statusCode").Number())
	}

	// Default rejectUnauthorized → expect cert error.
	req2 := ClientRequest(true, false, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"timeout", jsvalue.NewNumber(1500),
	))
	errCh := make(chan *jsvalue.JSValue, 1)
	req2.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			errCh <- args[0]
		}
		return jsvalue.NewUndefined()
	}))
	req2.MethodCall("end")

	select {
	case e := <-errCh:
		if e.Get("message").String() == "" {
			t.Error("expected non-empty error message for self-signed cert without override")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected cert error event, got none")
	}
}

func TestClientHTTPSGetAutoEnd(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCert(t)
	addr, _, teardown := startTLSServer(t, certPEM, keyPEM, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("get-ok"))
	})
	defer teardown()

	req := ClientRequest(true, true, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"ca", jsvalue.NewString(certPEM),
		"servername", jsvalue.NewString("localhost"),
	))
	resp, body := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 || body != "get-ok" {
		t.Errorf("status=%v body=%q, want 200/get-ok", resp.Get("statusCode").Number(), body)
	}
}
