package module

import "testing"

func TestDgramIsBuiltin(t *testing.T) {
	if !IsBuiltin("dgram") {
		t.Fatal("expected dgram to be builtin")
	}
	if !IsBuiltin("node:dgram") {
		t.Fatal("expected node:dgram to normalize to builtin dgram")
	}
}

func TestDgramRegisteredInModuleRegistry(t *testing.T) {
	if mod, ok := lookupRegistry("dgram"); !ok || mod == nil {
		t.Fatal("expected dgram in module registry")
	}
}

func TestURLRegisteredInModuleRegistry(t *testing.T) {
	if !IsBuiltin("url") || !IsBuiltin("node:url") {
		t.Fatal("expected url and node:url to be builtin")
	}
	if mod, ok := lookupRegistry("url"); !ok || mod == nil || mod.Get("URL") == nil {
		t.Fatal("expected url in module registry")
	}
	if mod, ok := lookupRegistry("node:url"); !ok || mod == nil || mod.Get("URL") == nil {
		t.Fatal("expected node:url in module registry")
	}
}
