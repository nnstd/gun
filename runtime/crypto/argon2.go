package crypto

import (
	"golang.org/x/crypto/argon2"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func argon2SyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("algorithm and parameters"))
	}
	algo := args[0].String()
	params := args[1]

	password := inputBytes(params.Get("password"))
	salt := inputBytes(params.Get("salt"))
	time := uint32(3)
	memory := uint32(65536)
	threads := uint32(4)
	keyLen := uint32(32)

	if v := params.Get("time"); v != nil && v.TypeString() != "undefined" {
		time = uint32(v.Number())
	}
	if v := params.Get("memoryCost"); v != nil && v.TypeString() != "undefined" {
		memory = uint32(v.Number())
	}
	if v := params.Get("parallelism"); v != nil && v.TypeString() != "undefined" {
		threads = uint32(v.Number())
	}
	if v := params.Get("outputLength"); v != nil && v.TypeString() != "undefined" {
		keyLen = uint32(v.Number())
	}
	if v := params.Get("hashLength"); v != nil && v.TypeString() != "undefined" {
		keyLen = uint32(v.Number())
	}

	var key []byte
	switch algo {
	case "argon2d":
		panic(errOsslUnsupported("argon2d is not supported by golang.org/x/crypto/argon2"))
	case "argon2i":
		key = argon2.Key(password, salt, time, memory, uint8(threads), keyLen)
	case "argon2id":
		key = argon2.IDKey(password, salt, time, memory, uint8(threads), keyLen)
	default:
		panic(errInvalidArgType("argon2d, argon2i, or argon2id"))
	}

	return buffer.NewBufferFromBytes(key)
}

func argon2JS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("algorithm and parameters"))
	}

	// Check for callback
	cbIdx := -1
	for i := len(args) - 1; i >= 2; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		algo := args[0]
		params := args[1]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			result := argon2SyncJS(algo, params)
			return result, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(argon2SyncJS(args...))
}
