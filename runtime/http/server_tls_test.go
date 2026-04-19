package nodehttp

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func startTLSServer(t *testing.T, certPEM, keyPEM string, handler func(req, res *jsvalue.JSValue)) (addr string, srv *jsvalue.JSValue, teardown func()) {
	t.Helper()
	listener := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && handler != nil {
			handler(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	})
	srv = CreateServer(true, jsvalue.ObjectFrom(
		"key", jsvalue.NewString(keyPEM),
		"cert", jsvalue.NewString(certPEM),
	), listener)

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
		t.Fatal("tls listen cb did not fire")
	}
	teardown = func() { srv.MethodCall("close") }
	return
}

func TestServerTLS(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCert(t)
	addr, _, teardown := startTLSServer(t, certPEM, keyPEM, func(req, res *jsvalue.JSValue) {
		res.MethodCall("setHeader", jsvalue.NewString("X-TLS"), jsvalue.NewString("yes"))
		res.MethodCall("end", jsvalue.NewString("tls-ok"))
	})
	defer teardown()

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(certPEM))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost"},
	}
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	resp, err := client.Get("https://" + addr + "/")
	if err != nil {
		t.Fatalf("https GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tls-ok" {
		t.Errorf("body = %q, want tls-ok", string(body))
	}
	if resp.Header.Get("X-TLS") != "yes" {
		t.Errorf("X-TLS = %q, want yes", resp.Header.Get("X-TLS"))
	}
}

func TestServerTLSInvalidPEM(t *testing.T) {
	listener := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	})
	srv := CreateServer(true, jsvalue.ObjectFrom(
		"key", jsvalue.NewString("INVALID"),
		"cert", jsvalue.NewString("INVALID"),
	), listener)

	gotErr := make(chan string, 1)
	srv.MethodCall("on", jsvalue.NewString("error"), jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			gotErr <- args[0].Get("message").String()
		}
		return jsvalue.NewUndefined()
	}))

	srv.MethodCall("listen", jsvalue.NewNumber(0), jsvalue.NewString("127.0.0.1"))

	select {
	case msg := <-gotErr:
		if msg == "" {
			t.Error("error message empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected TLS error event for INVALID PEM")
	}
	srv.MethodCall("close")
}
