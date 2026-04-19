//go:build !windows

package nodehttp

import (
	"path/filepath"
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestClientUnix(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gun.sock")
	_, teardown := startUnixServer(t, sockPath, func(req, res *jsvalue.JSValue) {
		res.MethodCall("setHeader", jsvalue.NewString("X-Sock"), jsvalue.NewString("yes"))
		res.MethodCall("end", jsvalue.NewString("unix-body"))
	})
	defer teardown()

	req := ClientRequest(false, false, jsvalue.ObjectFrom(
		"socketPath", jsvalue.NewString(sockPath),
		"path", jsvalue.NewString("/"),
		"method", jsvalue.NewString("GET"),
	))
	req.MethodCall("end")

	resp, body := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 {
		t.Errorf("statusCode = %v, want 200", resp.Get("statusCode").Number())
	}
	if body != "unix-body" {
		t.Errorf("body = %q, want unix-body", body)
	}
	if resp.Get("headers").Get("x-sock").String() != "yes" {
		t.Errorf("x-sock = %q, want yes", resp.Get("headers").Get("x-sock").String())
	}
}

func TestClientUnixCacheReuse(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gun.sock")
	_, teardown := startUnixServer(t, sockPath, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	c1 := unixClientFor(sockPath)
	c2 := unixClientFor(sockPath)
	if c1 != c2 {
		t.Error("unixClientFor did not return cached client")
	}
}
