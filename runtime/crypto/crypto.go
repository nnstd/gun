package crypto

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	promise "github.com/nnstd/gun/runtime/promise"
)

func hashFactory(algo string) func() hash.Hash {
	switch strings.ToLower(algo) {
	case "sha1":
		return sha1.New
	default:
		return sha256.New
	}
}

func newDigestObject(h hash.Hash) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("update", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			_, _ = h.Write([]byte(args[0].String()))
		}
		return obj
	}))
	obj.Set("digest", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		encoding := "hex"
		if len(args) > 0 && args[0] != nil {
			encoding = args[0].String()
		}
		sum := h.Sum(nil)
		switch encoding {
		case "base64":
			return jsvalue.NewString(base64.StdEncoding.EncodeToString(sum))
		default:
			return jsvalue.NewString(hex.EncodeToString(sum))
		}
	}))
	return obj
}

var AsJSValue = jsvalue.ObjectFrom(
	"createHash", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		algo := "sha256"
		if len(args) > 0 && args[0] != nil {
			algo = args[0].String()
		}
		return newDigestObject(hashFactory(algo)())
	}),
	"createHmac", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		algo := "sha256"
		key := []byte("")
		if len(args) > 0 && args[0] != nil {
			algo = args[0].String()
		}
		if len(args) > 1 && args[1] != nil {
			key = []byte(args[1].String())
		}
		return newDigestObject(hmac.New(hashFactory(algo), key))
	}),
	"randomBytes", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		n := 0
		if len(args) > 0 && args[0] != nil {
			n = int(args[0].Number())
		}
		buf := make([]byte, n)
		_, _ = cryptorand.Read(buf)
		return jsvalue.NewString(string(buf))
	}),
	"scrypt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 {
			return promise.Promise.Get("resolve").Call(jsvalue.NewString(""))
		}
		pass := args[0].String()
		salt := args[1].String()
		keylen := int(args[2].Number())
		derived := pass + ":" + salt
		if keylen > len(derived) {
			derived = derived + strings.Repeat("\x00", keylen-len(derived))
		}
		if keylen < len(derived) {
			derived = derived[:keylen]
		}
		return promise.Promise.Get("resolve").Call(jsvalue.NewString(derived))
	}),
	"subtle", jsvalue.NewObject(),
)
