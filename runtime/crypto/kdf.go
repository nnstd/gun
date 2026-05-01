package crypto

import (
	"crypto/sha1"
	"crypto/subtle"
	"crypto/sha256"
	"crypto/sha512"
	"hash"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func scryptSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("password, salt, and keylen"))
	}

	password := inputBytes(args[0])
	salt := inputBytes(args[1])
	keylen := int(args[2].Number())

	if len(salt) == 0 {
		panic(errInvalidSaltLen())
	}
	if keylen <= 0 {
		panic(errInvalidKeyLen())
	}

	N := int64(16384)
	r := int64(8)
	p := int64(1)

	if len(args) > 3 && args[3] != nil && args[3].TypeString() == "object" {
		opts := args[3]
		if v := opts.Get("N"); v != nil && v.TypeString() != "undefined" {
			N = int64(v.Number())
		}
		if v := opts.Get("cost"); v != nil && v.TypeString() != "undefined" {
			N = int64(v.Number())
		}
		if v := opts.Get("r"); v != nil && v.TypeString() != "undefined" {
			r = int64(v.Number())
		}
		if v := opts.Get("blockSize"); v != nil && v.TypeString() != "undefined" {
			r = int64(v.Number())
		}
		if v := opts.Get("p"); v != nil && v.TypeString() != "undefined" {
			p = int64(v.Number())
		}
		if v := opts.Get("parallelization"); v != nil && v.TypeString() != "undefined" {
			p = int64(v.Number())
		}
	}

	if N < 2 || (N&(N-1)) != 0 {
		panic(errInvalidMemoryCost())
	}
	if r <= 0 {
		panic(errInvalidBlockSize())
	}
	if p <= 0 {
		panic(errInvalidCPUCost())
	}

	key, err := scrypt.Key(password, salt, int(N), int(r), int(p), keylen)
	if err != nil {
		panic(errOperationFailed(err.Error()))
	}
	return buffer.NewBufferFromBytes(key)
}

func scryptJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("password, salt, and keylen"))
	}

	password := inputBytes(args[0])
	salt := inputBytes(args[1])
	keylen := int(args[2].Number())

	if keylen <= 0 {
		panic(errInvalidKeyLen())
	}

	N := int64(16384)
	r := int64(8)
	p := int64(1)

	if len(args) > 3 && args[3] != nil && args[3].TypeString() == "object" {
		opts := args[3]
		if v := opts.Get("N"); v != nil && v.TypeString() != "undefined" {
			N = int64(v.Number())
		}
		if v := opts.Get("cost"); v != nil && v.TypeString() != "undefined" {
			N = int64(v.Number())
		}
		if v := opts.Get("r"); v != nil && v.TypeString() != "undefined" {
			r = int64(v.Number())
		}
		if v := opts.Get("blockSize"); v != nil && v.TypeString() != "undefined" {
			r = int64(v.Number())
		}
		if v := opts.Get("p"); v != nil && v.TypeString() != "undefined" {
			p = int64(v.Number())
		}
		if v := opts.Get("parallelization"); v != nil && v.TypeString() != "undefined" {
			p = int64(v.Number())
		}
	}

	if N < 2 || (N&(N-1)) != 0 {
		panic(errInvalidMemoryCost())
	}
	if r <= 0 {
		panic(errInvalidBlockSize())
	}
	if p <= 0 {
		panic(errInvalidCPUCost())
	}

	// Check for callback variant: scrypt(password, salt, keylen, [options], callback)
	cbIdx := 3
	if len(args) > 3 && args[3] != nil && args[3].TypeString() == "object" {
		cbIdx = 4
	}
	if len(args) > cbIdx && args[cbIdx] != nil && args[cbIdx].TypeString() == "function" {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			key, err := scrypt.Key(password, salt, int(N), int(r), int(p), keylen)
			if err != nil {
				return nil, err
			}
			return buffer.NewBufferFromBytes(key), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	// Promise variant
	return resolvePromise(scryptSyncJS(args...))
}

func hashFactoryForDigest(algo string) func() hash.Hash {
	switch algo {
	case "sha1":
		return sha1.New
	case "sha224":
		return sha256.New224
	case "sha256":
		return sha256.New
	case "sha384":
		return sha512.New384
	case "sha512":
		return sha512.New
	default:
		return sha256.New
	}
}

func pbkdf2SyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("password, salt, iterations, keylen, and digest"))
	}

	password := inputBytes(args[0])
	salt := inputBytes(args[1])
	iterations := int(args[2].Number())
	keylen := int(args[3].Number())
	digest := args[4].String()

	if iterations <= 0 {
		panic(errInvalidArgType("iterations must be > 0"))
	}
	if keylen <= 0 {
		panic(errInvalidKeyLen())
	}

	key := pbkdf2.Key(password, salt, iterations, keylen, hashFactoryForDigest(digest))
	return buffer.NewBufferFromBytes(key)
}

func pbkdf2JS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("password, salt, iterations, keylen, and digest"))
	}

	// Check for callback: pbkdf2(password, salt, iterations, keylen, digest, callback)
	if len(args) > 5 && args[5] != nil && args[5].TypeString() == "function" {
		cb := args[5]
		password := inputBytes(args[0])
		salt := inputBytes(args[1])
		iterations := int(args[2].Number())
		keylen := int(args[3].Number())
		digest := args[4].String()

		asyncCrypto(func() (*jsvalue.JSValue, error) {
			key := pbkdf2.Key(password, salt, iterations, keylen, hashFactoryForDigest(digest))
			return buffer.NewBufferFromBytes(key), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(pbkdf2SyncJS(args...))
}

func hkdfSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("digest, ikm, salt, info, and keylen"))
	}

	digest := args[0].String()
	ikm := inputBytes(args[1])
	salt := inputBytes(args[2])
	info := inputBytes(args[3])
	keylen := int(args[4].Number())

	if keylen <= 0 {
		panic(errInvalidKeyLen())
	}

	h := hashFactoryForDigest(digest)
	reader := hkdf.New(h, ikm, salt, info)
	key := make([]byte, keylen)
	_, _ = reader.Read(key)
	return buffer.NewBufferFromBytes(key)
}

func hkdfJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("digest, ikm, salt, info, and keylen"))
	}

	// Check for callback: hkdf(digest, ikm, salt, info, keylen, callback)
	if len(args) > 5 && args[5] != nil && args[5].TypeString() == "function" {
		cb := args[5]
		digest := args[0].String()
		ikm := inputBytes(args[1])
		salt := inputBytes(args[2])
		info := inputBytes(args[3])
		keylen := int(args[4].Number())

		asyncCrypto(func() (*jsvalue.JSValue, error) {
			h := hashFactoryForDigest(digest)
			reader := hkdf.New(h, ikm, salt, info)
			key := make([]byte, keylen)
			_, _ = reader.Read(key)
			return buffer.NewBufferFromBytes(key), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(hkdfSyncJS(args...))
}

func timingSafeEqualJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("two Buffer arguments"))
	}
	a := inputBytes(args[0])
	b := inputBytes(args[1])
	if len(a) != len(b) {
		panic(errInvalidArgType("buffers must be the same length"))
	}
	return jsvalue.NewBool(subtle.ConstantTimeCompare(a, b) == 1)
}

func hashOneShot(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("algorithm and data"))
	}
	algo := args[0].String()
	data := inputBytes(args[1])

	factory := hashFactory(algo)
	h := factory()
	h.Write(data)
	encoding, _ := readEncoding(args, 2)
	return encodeOutput(h.Sum(nil), encoding)
}
