package crypto

import (
	"crypto/hmac"
	"hash"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// newHmacObject creates a JSValue Hmac object wrapping a Go hash.Hash (HMAC).
func newHmacObject(h hash.Hash) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	digested := false

	obj.Set("update", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if digested {
			panic(errHashUpdateAfterDigest())
		}
		if len(args) > 0 && args[0] != nil {
			h.Write(inputBytes(args[0]))
		}
		return obj
	}))

	obj.Set("digest", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if digested {
			panic(errHashDigested())
		}
		digested = true
		encoding, _ := readEncoding(args, 0)
		sum := h.Sum(nil)
		return encodeOutput(sum, encoding)
	}))

	return obj
}

// createHmac creates an Hmac object for the given algorithm and key.
func createHmac(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	algo := "sha256"
	key := []byte("")
	if len(args) > 0 && args[0] != nil {
		algo = args[0].String()
	}
	if len(args) > 1 && args[1] != nil {
		key = inputBytes(args[1])
	}

	factory := hashFactory(algo)

	return newHmacObject(hmac.New(factory, key))
}
