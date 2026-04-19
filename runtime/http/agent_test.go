package nodehttp

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestAgentConstructor(t *testing.T) {
	agent := agentClass.Call(jsvalue.ObjectFrom(
		"keepAlive", jsvalue.NewBool(true),
		"maxSockets", jsvalue.NewNumber(8),
		"timeout", jsvalue.NewNumber(5000),
	))
	if agent.Get("keepAlive").Bool() != true {
		t.Errorf("keepAlive = %v, want true", agent.Get("keepAlive").Bool())
	}
	if agent.Get("maxSockets").Number() != 8 {
		t.Errorf("maxSockets = %v, want 8", agent.Get("maxSockets").Number())
	}
}

func TestAgentGetName(t *testing.T) {
	agent := agentClass.Call()
	name := agent.MethodCall("getName", jsvalue.ObjectFrom(
		"host", jsvalue.NewString("example.com"),
		"port", jsvalue.NewNumber(3000),
		"path", jsvalue.NewString("/api"),
	)).String()
	if name != "example.com:3000:/api" {
		t.Errorf("getName = %q, want example.com:3000:/api", name)
	}
}

func TestAgentHostClientPooling(t *testing.T) {
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("ok"))
	})
	defer teardown()

	agent := agentClass.Call()
	ai := agentInternalOf(agent)

	for i := 0; i < 3; i++ {
		req := ClientRequest(false, false, jsvalue.ObjectFrom(
			"hostname", jsvalue.NewString("127.0.0.1"),
			"port", jsvalue.NewNumber(float64(parsePort(addr))),
			"path", jsvalue.NewString("/"),
			"agent", agent,
		))
		req.MethodCall("end")
		resp, body := awaitResponse(t, req)
		if resp.Get("statusCode").Number() != 200 || body != "ok" {
			t.Fatalf("iter %d: status=%v body=%q", i, resp.Get("statusCode").Number(), body)
		}
	}

	if len(ai.hosts) != 1 {
		t.Errorf("expected 1 pooled HostClient, got %d", len(ai.hosts))
	}

	agent.MethodCall("destroy")
	if ai.hosts != nil {
		t.Error("destroy did not clear hosts map")
	}
}

func TestAgentBypassWithFalse(t *testing.T) {
	addr, _, teardown := startTestServer(t, func(req, res *jsvalue.JSValue) {
		res.MethodCall("end", jsvalue.NewString("bypass"))
	})
	defer teardown()

	req := ClientRequest(false, false, jsvalue.ObjectFrom(
		"hostname", jsvalue.NewString("127.0.0.1"),
		"port", jsvalue.NewNumber(float64(parsePort(addr))),
		"path", jsvalue.NewString("/"),
		"agent", jsvalue.NewBool(false),
	))
	req.MethodCall("end")
	resp, body := awaitResponse(t, req)
	if resp.Get("statusCode").Number() != 200 || body != "bypass" {
		t.Errorf("status=%v body=%q, want 200/bypass", resp.Get("statusCode").Number(), body)
	}
}
