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
