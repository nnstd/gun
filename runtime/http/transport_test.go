package nodehttp

import (
	"testing"
	"time"
)

func TestNormalizeTransportRequestFromURL(t *testing.T) {
	rejectUnauthorized := false
	cfg, err := normalizeTransportRequest(&TransportRequest{
		Method:             "post",
		URL:                "https://example.com/path?x=1",
		Headers:            map[string]string{"x-test": "yes"},
		Body:               []byte("payload"),
		TimeoutMsec:        1234,
		RejectUnauthorized: &rejectUnauthorized,
		ServerName:         "example.com",
	})
	if err != nil {
		t.Fatalf("normalizeTransportRequest: %v", err)
	}
	if cfg.Method != "POST" {
		t.Fatalf("Method = %q, want POST", cfg.Method)
	}
	if cfg.Scheme != "https" {
		t.Fatalf("Scheme = %q, want https", cfg.Scheme)
	}
	if cfg.Host != "example.com:443" {
		t.Fatalf("Host = %q, want example.com:443", cfg.Host)
	}
	if cfg.Path != "/path?x=1" {
		t.Fatalf("Path = %q, want /path?x=1", cfg.Path)
	}
	if cfg.Headers["x-test"] != "yes" {
		t.Fatalf("header x-test = %q, want yes", cfg.Headers["x-test"])
	}
	if cfg.TimeoutMsec != 1234 {
		t.Fatalf("TimeoutMsec = %d, want 1234", cfg.TimeoutMsec)
	}
	if cfg.TLSConfig == nil || !cfg.TLSConfig.InsecureSkipVerify {
		t.Fatal("expected TLS config with InsecureSkipVerify=true")
	}
	if cfg.TLSConfig.ServerName != "example.com" {
		t.Fatalf("ServerName = %q, want example.com", cfg.TLSConfig.ServerName)
	}
}

func TestDoTransportAsyncCallbackOnInvalidURL(t *testing.T) {
	done := make(chan error, 1)
	DoTransportAsync(&TransportRequest{URL: "://bad"}, func(resp *TransportResponse, err error) {
		if resp != nil {
			t.Errorf("expected nil response, got %#v", resp)
		}
		done <- err
	})

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for callback")
	}
}

func TestNewTransportResponseMessagePreservesRawMetadata(t *testing.T) {
	msg := newTransportResponseMessage(&TransportResponse{
		StatusCode: 299,
		StatusText: "Custom Status",
		Headers:    map[string]string{"x-test": "yes"},
		RawHeaders: []TransportHeader{{Key: "X-Test", Value: "yes"}},
	})

	if msg.Get("statusMessage").String() != "Custom Status" {
		t.Fatalf("statusMessage = %q, want Custom Status", msg.Get("statusMessage").String())
	}
	raw := msg.Get("rawHeaders")
	if !raw.IsArray() || raw.Len() != 2 {
		t.Fatalf("rawHeaders len = %d, want 2", raw.Len())
	}
	if raw.Index(0).String() != "X-Test" || raw.Index(1).String() != "yes" {
		t.Fatalf("rawHeaders = [%q %q], want [X-Test yes]", raw.Index(0).String(), raw.Index(1).String())
	}
}
