package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// ---------------------------------------------------------------------------
// generateKeySync — synchronous symmetric key generation
// ---------------------------------------------------------------------------

// generateKeySyncJS generates a symmetric CryptoKey synchronously.
// type: "aes" or "hmac"
// options: { length: number }
func generateKeySyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("type and options"))
	}

	keyType := args[0].String()
	opts := args[1]

	length := 256 // default
	if opts != nil && opts.TypeString() == "object" {
		if v := opts.Get("length"); v != nil && v.TypeString() != "undefined" {
			length = int(v.Number())
		}
	}

	var algo map[string]interface{}
	var rawKey []byte

	switch strings.ToLower(keyType) {
	case "aes":
		if length != 128 && length != 192 && length != 256 {
			panic(errInvalidLength("invalid key length for " + keyType))
		}
		algo = map[string]interface{}{"name": "AES-GCM", "length": length}
		rawKey = randomBytesGo(length / 8)

	case "hmac":
		// HMAC key length defaults to hash output size if not specified.
		hashName := "sha256"
		if opts != nil && opts.TypeString() == "object" {
			if h := opts.Get("hash"); h != nil && h.TypeString() != "undefined" {
				hashName = h.String()
			}
		}
		hashLen := hashOutputSize(hashName)
		if length > 0 {
			rawKey = randomBytesGo(length / 8)
		} else {
			rawKey = randomBytesGo(hashLen)
			length = hashLen * 8
		}
		algo = map[string]interface{}{"name": "HMAC", "hash": hashName, "length": length}

	default:
		panic(errKeygenUnsupported(keyType))
	}

	return newCryptoKey("secret", true, algo, []string{"sign", "verify"}, rawKey, nil, nil)
}

// generateKeyJS generates a symmetric CryptoKey asynchronously.
func generateKeyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("type and options"))
	}

	// Check for callback variant: generateKey(type, options, callback)
	cbIdx := -1
	for i := len(args) - 1; i >= 2; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		// Capture args for closure
		keyType := args[0]
		opts := args[1]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			result := generateKeySyncJS(keyType, opts)
			return result, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(generateKeySyncJS(args...))
}

// hashOutputSize returns the output size in bytes for a hash algorithm name.
func hashOutputSize(name string) int {
	switch strings.ToLower(name) {
	case "sha1":
		return 20
	case "sha224":
		return 28
	case "sha256":
		return 32
	case "sha384":
		return 48
	case "sha512":
		return 64
	case "sha3-256":
		return 32
	case "sha3-384":
		return 48
	case "sha3-512":
		return 64
	case "md5":
		return 16
	case "blake2s256":
		return 32
	case "blake2b256":
		return 32
	case "blake2b384":
		return 48
	case "blake2b512":
		return 64
	default:
		return 32 // default to SHA-256 size
	}
}

// ---------------------------------------------------------------------------
// generateKeyPairSync — synchronous asymmetric key pair generation
// ---------------------------------------------------------------------------

// generateKeyPairSyncJS generates an asymmetric key pair synchronously.
// type: "rsa", "ec", "ed25519", "ed448", "x25519"
func generateKeyPairSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 {
		panic(errInvalidArgType("type"))
	}

	kpType := args[0].String()
	var opts *jsvalue.JSValue
	if len(args) > 1 && args[1] != nil {
		opts = args[1]
	}

	// Collect usages from options
	var usages []string
	if opts != nil && opts.TypeString() == "object" {
		if u := opts.Get("publicKeyUsages"); u != nil && u.TypeString() == "object" {
			length := int(u.Get("length").Number())
			usages = make([]string, length)
			for i := 0; i < length; i++ {
				usages[i] = u.Get(fmt.Sprintf("%d", i)).String()
			}
		}
	}

	switch strings.ToLower(kpType) {
	case "rsa":
		return generateRSAKeyPairSync(opts, usages)
	case "ec":
		return generateECKeyPairSync(opts, usages)
	case "ed25519":
		return generateEd25519KeyPairSync(usages)
	case "ed448":
		panic(errKeygenUnsupported("ed448"))
	case "x25519":
		return generateX25519KeyPairSync(usages)
	default:
		panic(errKeygenUnsupported(kpType))
	}
}

// generateKeyPairJS generates an asymmetric key pair asynchronously.
func generateKeyPairJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 {
		panic(errInvalidArgType("type"))
	}

	// Check for callback variant: generateKeyPair(type, options, callback)
	cbIdx := -1
	for i := len(args) - 1; i >= 1; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		// Build a fresh args slice without the callback for the sync version
		syncArgs := make([]*jsvalue.JSValue, len(args))
		copy(syncArgs, args)
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			result := generateKeyPairSyncJS(syncArgs...)
			return result, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(generateKeyPairSyncJS(args...))
}

// ---------------------------------------------------------------------------
// RSA key pair generation
// ---------------------------------------------------------------------------

func generateRSAKeyPairSync(opts *jsvalue.JSValue, usages []string) *jsvalue.JSValue {
	modulusLength := 2048
	if opts != nil && opts.TypeString() == "object" {
		if v := opts.Get("modulusLength"); v != nil && v.TypeString() != "undefined" {
			modulusLength = int(v.Number())
		}
	}

	if modulusLength < 2048 {
		panic(errInvalidArgType("RSA modulus length >= 2048"))
	}

	pubExp := rsaPublicExponentBig(opts)
	algo := map[string]interface{}{
		"name":          "RSASSA-PKCS1-v1_5",
		"modulusLength": modulusLength,
	}

	pub, priv, err := generateRSAKeyPair(modulusLength, pubExp)
	if err != nil {
		panic(errOperationFailed("RSA key generation failed: " + err.Error()))
	}

	return newKeyPairResult(pub, priv, algo, usages)
}

// ---------------------------------------------------------------------------
// EC key pair generation
// ---------------------------------------------------------------------------

func generateECKeyPairSync(opts *jsvalue.JSValue, usages []string) *jsvalue.JSValue {
	curveName := "P-256"
	if opts != nil && opts.TypeString() == "object" {
		if v := opts.Get("namedCurve"); v != nil && v.TypeString() != "undefined" {
			curveName = v.String()
		}
	}

	curve := namedCurve(curveName)
	algo := map[string]interface{}{
		"name":        "ECDSA",
		"namedCurve":  curveName,
	}

	pub, priv, err := generateECKeyPair(curve)
	if err != nil {
		panic(errOperationFailed("EC key generation failed: " + err.Error()))
	}

	return newKeyPairResult(pub, priv, algo, usages)
}

// ---------------------------------------------------------------------------
// Ed25519 key pair generation
// ---------------------------------------------------------------------------

func generateEd25519KeyPairSync(usages []string) *jsvalue.JSValue {
	algo := map[string]interface{}{
		"name": "Ed25519",
	}

	pub, priv, err := generateEd25519KeyPair()
	if err != nil {
		panic(errOperationFailed("Ed25519 key generation failed: " + err.Error()))
	}

	return newKeyPairResult(pub, priv, algo, usages)
}

// ---------------------------------------------------------------------------
// X25519 key pair generation
// ---------------------------------------------------------------------------

func generateX25519KeyPairSync(usages []string) *jsvalue.JSValue {
	curve := ecdh.X25519()

	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		panic(errOperationFailed("X25519 key generation failed: " + err.Error()))
	}
	pub := priv.PublicKey

	algo := map[string]interface{}{
		"name": "X25519",
	}

	return newKeyPairResult(pub, priv, algo, usages)
}
