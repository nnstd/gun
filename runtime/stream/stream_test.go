package stream

import "testing"

func TestAsJSValueExportsStreamSurface(t *testing.T) {
	if AsJSValue.Get("Readable").TypeString() != "function" {
		t.Fatal("expected Readable export")
	}
	if AsJSValue.Get("Writable").TypeString() != "function" {
		t.Fatal("expected Writable export")
	}
	if AsJSValue.Get("pipeline").TypeString() != "function" {
		t.Fatal("expected pipeline export")
	}
	if AsJSValue.Get("finished").TypeString() != "function" {
		t.Fatal("expected finished export")
	}
}
