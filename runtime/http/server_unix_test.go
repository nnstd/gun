//go:build !windows

package nodehttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func startUnixServer(t *testing.T, sockPath string, handler func(req, res *jsvalue.JSValue)) (srv *jsvalue.JSValue, teardown func()) {
	t.Helper()
	listener := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && handler != nil {
			handler(args[0], args[1])
		}
		return jsvalue.NewUndefined()
	})
	srv = CreateServer(false, listener)

	ready := make(chan struct{}, 1)
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ready <- struct{}{}
		return jsvalue.NewUndefined()
	})
	srv.MethodCall("listen", jsvalue.NewString(sockPath), cb)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("unix listen callback did not fire")
	}
	teardown = func() { srv.MethodCall("close") }
	return
}

func unixHTTPGet(t *testing.T, sockPath, path string) (*http.Response, []byte) {
	t.Helper()
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sockPath)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	resp, err := client.Get("http://unix" + path)
	if err != nil {
		t.Fatalf("unix GET error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

func TestServerUnix(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gun.sock")

	srv, teardown := startUnixServer(t, sockPath, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("unix-ok"))
	})

	addr := srv.MethodCall("address")
	if addr.TypeString() != "string" {
		t.Fatalf("unix address typeof = %s, want string", addr.TypeString())
	}
	if addr.String() != sockPath {
		t.Errorf("address = %q, want %q", addr.String(), sockPath)
	}

	resp, body := unixHTTPGet(t, sockPath, "/")
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "unix-ok" {
		t.Errorf("body = %q, want unix-ok", string(body))
	}

	teardown()
	time.Sleep(20 * time.Millisecond)
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("socket file still exists after close: err=%v", err)
	}
}

func TestServerUnixStaleAutoUnlink(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gun.sock")
	if err := os.WriteFile(sockPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, teardown := startUnixServer(t, sockPath, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	resp, _ := unixHTTPGet(t, sockPath, "/")
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestServerUnixOptionsForm(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gun.sock")
	listener := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			args[1].MethodCall("end", jsvalue.NewString("opts"))
		}
		return jsvalue.NewUndefined()
	})
	srv := CreateServer(false, listener)
	ready := make(chan struct{}, 1)
	cb := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		ready <- struct{}{}
		return jsvalue.NewUndefined()
	})
	srv.MethodCall("listen", jsvalue.ObjectFrom("path", jsvalue.NewString(sockPath)), cb)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("listen({path}) cb did not fire")
	}
	defer srv.MethodCall("close")

	resp, body := unixHTTPGet(t, sockPath, "/")
	if resp.StatusCode != 200 || string(body) != "opts" {
		t.Errorf("status=%d body=%q, want 200/opts", resp.StatusCode, string(body))
	}
}

func TestServerWindowsPipeRejected(t *testing.T) {
	srv := CreateServer(false, jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewUndefined()
	}))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for windows pipe path")
		}
		ev, ok := r.(*jsvalue.JSValue)
		if !ok {
			t.Fatalf("panic type = %T, want *JSValue", r)
		}
		msg := ev.Get("message").String()
		if msg == "" || !contains(msg, "Windows named pipes") {
			t.Errorf("panic message = %q, want substring 'Windows named pipes'", msg)
		}
	}()
	srv.MethodCall("listen", jsvalue.NewString(`\\.\pipe\foo`))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
