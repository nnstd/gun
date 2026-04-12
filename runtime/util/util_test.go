package util

import "testing"

func TestAsJSValueExportsUtilSurface(t *testing.T) {
	if AsJSValue.Get("format").TypeString() != "function" {
		t.Fatal("expected format export")
	}
	if AsJSValue.Get("inspect").TypeString() != "function" {
		t.Fatal("expected inspect export")
	}
	if AsJSValue.Get("inherits").TypeString() != "function" {
		t.Fatal("expected inherits export")
	}
}
