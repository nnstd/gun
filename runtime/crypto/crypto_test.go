package crypto

import "testing"

func TestAsJSValueExportsCryptoSurface(t *testing.T) {
	if AsJSValue.Get("createHash").TypeString() != "function" {
		t.Fatal("expected createHash export")
	}
	hash := AsJSValue.Get("createHash").Call()
	if hash.Get("update").TypeString() != "function" || hash.Get("digest").TypeString() != "function" {
		t.Fatal("expected hash object with update/digest")
	}
	if AsJSValue.Get("randomBytes").TypeString() != "function" {
		t.Fatal("expected randomBytes export")
	}
}
