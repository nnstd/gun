package crypto

import (
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// supportedAlgorithms tracks which algorithms are supported for each operation.
var supportedAlgorithms = map[string]map[string]bool{
	"digest": {
		"SHA-1": true, "SHA-256": true, "SHA-384": true, "SHA-512": true,
		"SHA3-256": true, "SHA3-384": true, "SHA3-512": true,
	},
	"generateKey": {
		"AES-CBC": true, "AES-CTR": true, "AES-GCM": true, "AES-KW": true,
		"HMAC": true, "ECDH": true, "ECDSA": true, "Ed25519": true,
		"RSA-OAEP": true, "RSA-PSS": true, "RSASSA-PKCS1-v1_5": true,
	},
	"importKey": {
		"AES-CBC": true, "AES-CTR": true, "AES-GCM": true, "AES-KW": true,
		"HMAC": true, "ECDH": true, "ECDSA": true, "Ed25519": true,
		"PBKDF2": true, "HKDF": true,
	},
	"exportKey": {
		"AES-CBC": true, "AES-CTR": true, "AES-GCM": true, "AES-KW": true,
		"HMAC": true,
	},
	"sign": {
		"HMAC": true, "RSASSA-PKCS1-v1_5": true, "RSA-PSS": true,
		"ECDSA": true, "Ed25519": true,
	},
	"verify": {
		"HMAC": true, "RSASSA-PKCS1-v1_5": true, "RSA-PSS": true,
		"ECDSA": true, "Ed25519": true,
	},
	"encrypt": {
		"AES-CBC": true, "AES-CTR": true, "AES-GCM": true, "RSA-OAEP": true,
	},
	"decrypt": {
		"AES-CBC": true, "AES-CTR": true, "AES-GCM": true, "RSA-OAEP": true,
	},
	"deriveBits": {
		"PBKDF2": true, "HKDF": true, "ECDH": true, "X25519": true,
	},
	"deriveKey": {
		"PBKDF2": true, "HKDF": true, "ECDH": true, "X25519": true,
	},
	"wrapKey": {
		"AES-KW": true, "AES-GCM": true, "RSA-OAEP": true,
	},
	"unwrapKey": {
		"AES-KW": true, "AES-GCM": true, "RSA-OAEP": true,
	},
}

// subtleCryptoSupports implements SubtleCrypto.supports() static method.
func subtleCryptoSupports(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("operation and algorithm"))
	}
	operation := args[0].String()
	algoName := parseAlgorithmName(args[1])

	ops, ok := supportedAlgorithms[strings.ToLower(operation)]
	if !ok {
		return jsvalue.NewBool(false)
	}
	return jsvalue.NewBool(ops[algoName])
}

// subtleGetPublicKey stub — ML-KEM not yet supported.
func subtleGetPublicKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	panic(errOsslUnsupported("ML-KEM getPublicKey is not yet supported"))
}

// subtleEncapsulateBits stub — ML-KEM not yet supported.
func subtleEncapsulateBits(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	panic(errOsslUnsupported("ML-KEM encapsulateBits is not yet supported"))
}

// subtleDecapsulateBits stub — ML-KEM not yet supported.
func subtleDecapsulateBits(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	panic(errOsslUnsupported("ML-KEM decapsulateBits is not yet supported"))
}

// subtleEncapsulateKey stub — ML-KEM not yet supported.
func subtleEncapsulateKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	panic(errOsslUnsupported("ML-KEM encapsulateKey is not yet supported"))
}

// subtleDecapsulateKey stub — ML-KEM not yet supported.
func subtleDecapsulateKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	panic(errOsslUnsupported("ML-KEM decapsulateKey is not yet supported"))
}
