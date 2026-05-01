package crypto

import (
	"crypto/ecdh"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// subtleDeriveBits implements SubtleCrypto.deriveBits().
// Args: algorithm, baseKey, length (in bits)
func subtleDeriveBits(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, baseKey, and length"))
	}

	algoObj := args[0]
	baseKey := args[1]
	lengthBits := int(args[2].Number())

	kd := getCryptoKeyData(baseKey)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}

	algoName := parseAlgorithmName(algoObj)

	var derived []byte
	var err error

	switch strings.ToUpper(algoName) {
	case "PBKDF2":
		derived, err = derivePBKDF2(algoObj, kd, lengthBits)
	case "HKDF":
		derived, err = deriveHKDF(algoObj, kd, lengthBits)
	case "ECDH":
		derived, err = deriveECDH(algoObj, kd)
	case "X25519":
		derived, err = deriveX25519(algoObj, kd)
	default:
		panic(errOsslUnsupported("deriveBits algorithm not supported: " + algoName))
	}

	if err != nil {
		panic(cryptoError("ERR_OPERATION_FAILED", err.Error()))
	}

	// lengthBits is in bits — truncate to ceil(lengthBits/8) bytes then trim
	if lengthBits > 0 {
		byteLen := (lengthBits + 7) / 8
		if len(derived) > byteLen {
			derived = derived[:byteLen]
		}
	}

	return resolvePromise(buffer.NewBufferFromBytes(derived))
}

// subtleDeriveKey implements SubtleCrypto.deriveKey().
// Args: algorithm, baseKey, derivedKeyAlgorithm, extractable, keyUsages
func subtleDeriveKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("algorithm, baseKey, derivedKeyAlgorithm, extractable, and keyUsages"))
	}

	algoObj := args[0]
	baseKey := args[1]
	derivedKeyAlgo := args[2]
	extractable := args[3].Bool()

	var usages []string
	if args[4] != nil && args[4].IsArray() {
		for _, u := range args[4].Array() {
			usages = append(usages, u.String())
		}
	}

	kd := getCryptoKeyData(baseKey)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}

	algoName := parseAlgorithmName(algoObj)

	// Determine the derived key length from the derivedKeyAlgorithm
	derivedAlgoName := parseAlgorithmName(derivedKeyAlgo)
	keyLengthBits := deriveKeyLengthBits(derivedAlgoName, derivedKeyAlgo)

	var derived []byte
	var err error

	switch strings.ToUpper(algoName) {
	case "PBKDF2":
		derived, err = derivePBKDF2(algoObj, kd, keyLengthBits)
	case "HKDF":
		derived, err = deriveHKDF(algoObj, kd, keyLengthBits)
	case "ECDH":
		derived, err = deriveECDH(algoObj, kd)
	case "X25519":
		derived, err = deriveX25519(algoObj, kd)
	default:
		panic(errOsslUnsupported("deriveKey algorithm not supported: " + algoName))
	}

	if err != nil {
		panic(cryptoError("ERR_OPERATION_FAILED", err.Error()))
	}

	// Build the CryptoKey algorithm from the derivedKeyAlgorithm
	keyAlgo := jsValueToMap(derivedKeyAlgo)
	if _, ok := keyAlgo["name"]; !ok {
		keyAlgo["name"] = derivedAlgoName
	}
	if _, ok := keyAlgo["length"]; !ok && keyLengthBits > 0 {
		keyAlgo["length"] = keyLengthBits
	}

	ck := newCryptoKey("secret", extractable, keyAlgo, usages, derived, nil, nil)
	return resolvePromise(ck)
}

// ---------------------------------------------------------------------------
// PBKDF2
// ---------------------------------------------------------------------------

func derivePBKDF2(algoObj *jsvalue.JSValue, kd *cryptoKeyData, lengthBits int) ([]byte, error) {
	salt := getAlgoBytes(algoObj, "salt")

	iterations := 100000
	if v := algoObj.Get("iterations"); v != nil && v.TypeString() != "undefined" {
		iterations = int(v.Number())
	}

	hashName := "SHA-256"
	if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
		hashName = parseAlgorithmName(v)
	}

	hashFunc := subtleDigestAlgo(hashName)
	if hashFunc == nil {
		return nil, fmt.Errorf("unsupported PBKDF2 hash: %s", hashName)
	}

	byteLen := (lengthBits + 7) / 8
	if byteLen <= 0 {
		byteLen = 32 // default to 256 bits
	}

	return pbkdf2.Key(kd.rawKey, salt, iterations, byteLen, hashFunc), nil
}

// ---------------------------------------------------------------------------
// HKDF
// ---------------------------------------------------------------------------

func deriveHKDF(algoObj *jsvalue.JSValue, kd *cryptoKeyData, lengthBits int) ([]byte, error) {
	salt := getAlgoBytes(algoObj, "salt")
	info := getAlgoBytes(algoObj, "info")

	hashName := "SHA-256"
	if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
		hashName = parseAlgorithmName(v)
	}

	hashFunc := subtleDigestAlgo(hashName)
	if hashFunc == nil {
		return nil, fmt.Errorf("unsupported HKDF hash: %s", hashName)
	}

	byteLen := (lengthBits + 7) / 8
	if byteLen <= 0 {
		byteLen = 32
	}

	reader := hkdf.New(hashFunc, kd.rawKey, salt, info)
	out := make([]byte, byteLen)
	if _, err := io.ReadFull(reader, out); err != nil {
		return nil, err
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// ECDH
// ---------------------------------------------------------------------------

func deriveECDH(algoObj *jsvalue.JSValue, kd *cryptoKeyData) ([]byte, error) {
	// The baseKey must be a private key with a public key to derive against.
	// In WebCrypto, the algorithm has a "public" property containing the peer's public key.
	pubKeyVal := algoObj.Get("public")
	if pubKeyVal == nil || pubKeyVal.TypeString() == "undefined" {
		return nil, fmt.Errorf("ECDH: algorithm.public (peer public key) is required")
	}

	pubKD := getCryptoKeyData(pubKeyVal)
	if pubKD == nil {
		return nil, fmt.Errorf("ECDH: expected CryptoKey (public)")
	}

	curveName := "P-256"
	if v := algoObj.Get("namedCurve"); v != nil && v.TypeString() != "undefined" {
		curveName = v.String()
	}

	var curve ecdh.Curve
	switch strings.ToUpper(curveName) {
	case "P-256":
		curve = ecdh.P256()
	case "P-384":
		curve = ecdh.P384()
	case "P-521":
		curve = ecdh.P521()
	default:
		return nil, fmt.Errorf("unsupported ECDH curve: %s", curveName)
	}

	privKey, err := curve.NewPrivateKey(kd.rawKey)
	if err != nil {
		return nil, err
	}

	pubKey, err := curve.NewPublicKey(pubKD.rawKey)
	if err != nil {
		return nil, err
	}

	return privKey.ECDH(pubKey)
}

// ---------------------------------------------------------------------------
// X25519
// ---------------------------------------------------------------------------

func deriveX25519(algoObj *jsvalue.JSValue, kd *cryptoKeyData) ([]byte, error) {
	pubKeyVal := algoObj.Get("public")
	if pubKeyVal == nil || pubKeyVal.TypeString() == "undefined" {
		return nil, fmt.Errorf("X25519: algorithm.public (peer public key) is required")
	}

	pubKD := getCryptoKeyData(pubKeyVal)
	if pubKD == nil {
		return nil, fmt.Errorf("X25519: expected CryptoKey (public)")
	}

	curve := ecdh.X25519()

	privKey, err := curve.NewPrivateKey(kd.rawKey)
	if err != nil {
		return nil, err
	}

	pubKey, err := curve.NewPublicKey(pubKD.rawKey)
	if err != nil {
		return nil, err
	}

	return privKey.ECDH(pubKey)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// deriveKeyLengthBits determines the key length in bits from a derivedKeyAlgorithm.
func deriveKeyLengthBits(algoName string, algoObj *jsvalue.JSValue) int {
	switch strings.ToUpper(algoName) {
	case "AES-CBC", "AES-GCM", "AES-CTR", "AES-KW":
		if v := algoObj.Get("length"); v != nil && v.TypeString() != "undefined" {
			return int(v.Number())
		}
		return 256 // default AES-256
	case "HMAC":
		if v := algoObj.Get("length"); v != nil && v.TypeString() != "undefined" {
			return int(v.Number())
		}
		// Default to hash output size
		hashName := "SHA-256"
		if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
			hashName = parseAlgorithmName(v)
		}
		switch strings.ToUpper(hashName) {
		case "SHA-256":
			return 256
		case "SHA-384":
			return 384
		case "SHA-512":
			return 512
		case "SHA-1":
			return 160
		default:
			return 256
		}
	default:
		return 256
	}
}

