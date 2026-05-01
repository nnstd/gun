package crypto

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"strings"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/sha3"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// hashFactory returns a hash constructor for the given algorithm name.
// Panics with ERR_CRYPTO_UNKNOWN_HASH for unknown algorithms.
func hashFactory(algo string) func() hash.Hash {
	switch strings.ToLower(algo) {
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
	case "md5":
		return md5.New
	case "blake2s256":
		return func() hash.Hash {
			h, _ := blake2s.New256(nil)
			return h
		}
	case "blake2b256":
		return func() hash.Hash {
			h, _ := blake2b.New256(nil)
			return h
		}
	case "blake2b384":
		return func() hash.Hash {
			h, _ := blake2b.New384(nil)
			return h
		}
	case "blake2b512":
		return func() hash.Hash {
			h, _ := blake2b.New512(nil)
			return h
		}
	case "sha3-256":
		return sha3.New256
	case "sha3-384":
		return sha3.New384
	case "sha3-512":
		return sha3.New512
	default:
		panic(errUnknownHash(algo))
	}
}

// newHashObject creates a JSValue Hash object wrapping a Go hash.Hash.
func newHashObject(h hash.Hash) *jsvalue.JSValue {
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

	obj.Set("copy", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		algo := ""
		if v := obj.Get("_algo"); v != nil && v.TypeString() != "undefined" {
			algo = v.String()
		}
		if algo == "" {
			panic(errOsslUnsupported("hash state copy not supported"))
		}
		marshaler, ok := h.(interface{ MarshalBinary() ([]byte, error) })
		if !ok {
			panic(errOsslUnsupported("hash state copy not supported"))
		}
		data, err := marshaler.MarshalBinary()
		if err != nil {
			panic(errOsslUnsupported("hash state copy failed"))
		}
		newH := hashFactory(algo)()
		unmarshaler, ok := newH.(interface{ UnmarshalBinary([]byte) error })
		if !ok {
			panic(errOsslUnsupported("hash state copy not supported"))
		}
		if err := unmarshaler.UnmarshalBinary(data); err != nil {
			panic(errOsslUnsupported("hash state copy failed"))
		}
		return newHashObjectWithAlgo(newH, algo)
	}))

	return obj
}

// newHashObjectWithAlgo creates a Hash object that remembers its algorithm for copy().
func newHashObjectWithAlgo(h hash.Hash, algo string) *jsvalue.JSValue {
	obj := newHashObject(h)
	obj.Set("_algo", jsvalue.NewString(algo))
	return obj
}

// createHash creates a Hash object for the given algorithm.
func createHash(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	algo := "sha256"
	if len(args) > 0 && args[0] != nil {
		algo = args[0].String()
	}
	return newHashObjectWithAlgo(hashFactory(algo)(), algo)
}

// getHashes returns an array of supported hash algorithm names.
func getHashes() *jsvalue.JSValue {
	names := []*jsvalue.JSValue{
		jsvalue.NewString("sha1"), jsvalue.NewString("sha224"), jsvalue.NewString("sha256"),
		jsvalue.NewString("sha384"), jsvalue.NewString("sha512"),
		jsvalue.NewString("md5"), jsvalue.NewString("blake2s256"),
		jsvalue.NewString("blake2b256"), jsvalue.NewString("blake2b384"), jsvalue.NewString("blake2b512"),
		jsvalue.NewString("sha3-256"), jsvalue.NewString("sha3-384"), jsvalue.NewString("sha3-512"),
	}
	return jsvalue.NewArray(names...)
}
