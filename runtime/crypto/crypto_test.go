package crypto

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

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

func TestScryptReturnsPromise(t *testing.T) {
	p := AsJSValue.Get("scrypt").Call(jsvalue.NewString("pass"), jsvalue.NewString("salt"), jsvalue.NewNumber(8))
	if !jsvalue.InstanceOf(p, promise.Promise).Bool() {
		t.Fatal("expected scrypt() to return a Promise")
	}
}
