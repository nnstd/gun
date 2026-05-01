package crypto

import (
	"crypto/hmac"
	"crypto/subtle"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"hash"
	"strings"

	"golang.org/x/crypto/sha3"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// subtleDigestAlgo maps WebCrypto algorithm names to Go hash constructors.
func subtleDigestAlgo(algo string) func() hash.Hash {
	switch strings.ToUpper(algo) {
	case "SHA-1":
		return sha1.New
	case "SHA-256":
		return sha256.New
	case "SHA-384":
		return sha512.New384
	case "SHA-512":
		return sha512.New
	case "SHA3-256":
		return sha3.New256
	case "SHA3-384":
		return sha3.New384
	case "SHA3-512":
		return sha3.New512
	default:
		return nil
	}
}

// parseAlgorithmName extracts the algorithm name from a string or object.
func parseAlgorithmName(algo *jsvalue.JSValue) string {
	if algo == nil {
		return ""
	}
	if algo.TypeString() == "string" {
		return algo.String()
	}
	if algo.TypeString() == "object" {
		if name := algo.Get("name"); name != nil && name.TypeString() != "undefined" {
			return name.String()
		}
	}
	return ""
}

// subtleDigest implements SubtleCrypto.digest().
func subtleDigest(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("algorithm and data"))
	}
	algoName := parseAlgorithmName(args[0])
	factory := subtleDigestAlgo(algoName)
	if factory == nil {
		panic(errOsslUnsupported("digest algorithm not supported: " + algoName))
	}
	data := inputBytes(args[1])
	h := factory()
	h.Write(data)
	return resolvePromise(buffer.NewBufferFromBytes(h.Sum(nil)))
}

// subtleGenerateKey implements SubtleCrypto.generateKey() for symmetric keys.
func subtleGenerateKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, extractable, and keyUsages"))
	}
	algoObj := args[0]
	extractable := args[1]
	keyUsages := args[2]

	algoName := parseAlgorithmName(algoObj)

	var keyLen int
	var keyAlgo map[string]interface{}

	switch strings.ToUpper(algoName) {
	case "AES-CBC", "AES-GCM", "AES-CTR", "AES-KW":
		length := 256
		if v := algoObj.Get("length"); v != nil && v.TypeString() != "undefined" {
			length = int(v.Number())
		}
		keyLen = length / 8
		keyAlgo = map[string]interface{}{
			"name":   algoName,
			"length": length,
		}
	case "HMAC":
		hashName := "SHA-256"
		if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
			hashName = parseAlgorithmName(v)
		}
		length := 0
		if v := algoObj.Get("length"); v != nil && v.TypeString() != "undefined" {
			length = int(v.Number())
		}
		// Default length = hash output size
		factory := subtleDigestAlgo(hashName)
		if factory == nil {
			panic(errOsslUnsupported("HMAC hash not supported: " + hashName))
		}
		h := factory()
		h.Write([]byte{})
		defaultLen := len(h.Sum(nil))
		if length == 0 {
			length = defaultLen * 8
		}
		keyLen = length / 8
		keyAlgo = map[string]interface{}{
			"name": "HMAC",
			"hash": hashName,
			"length": length,
		}
	default:
		panic(errKeygenUnsupported(algoName))
	}

	rawKey := randomBytesGo(keyLen)
	ext := extractable != nil && extractable.Bool()

	var usages []string
	if keyUsages != nil && keyUsages.IsArray() {
		for _, u := range keyUsages.Array() {
			usages = append(usages, u.String())
		}
	}

	ck := newCryptoKey("secret", ext, keyAlgo, usages, rawKey, nil, nil)
	return resolvePromise(ck)
}

// subtleImportKey implements SubtleCrypto.importKey().
func subtleImportKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 5 {
		panic(errInvalidArgType("format, keyData, algorithm, extractable, and keyUsages"))
	}
	format := args[0].String()
	keyData := args[1]
	algoObj := args[2]
	extractable := args[3]
	keyUsages := args[4]

	algoName := parseAlgorithmName(algoObj)
	ext := extractable != nil && extractable.Bool()

	var usages []string
	if keyUsages != nil && keyUsages.IsArray() {
		for _, u := range keyUsages.Array() {
			usages = append(usages, u.String())
		}
	}

	var rawKey []byte

	switch strings.ToLower(format) {
	case "raw":
		rawKey = inputBytes(keyData)
	case "jwk":
		jwk := keyData
		if jwk.Get("k") != nil && jwk.Get("k").TypeString() != "undefined" {
			// Symmetric key — base64url decode "k" field
			encoded := jwk.Get("k").String()
			decoded, err := decodeBase64URL(encoded)
			if err != nil {
				panic(errInvalidArgType("valid JWK"))
			}
			rawKey = decoded
		} else {
			panic(errOsslUnsupported("JWK import for asymmetric keys not yet supported"))
		}
	case "spki", "pkcs8":
		panic(errOsslUnsupported(format + " key import not yet supported"))
	default:
		panic(errInvalidArgType("valid key format (raw, jwk, spki, pkcs8)"))
	}

	// Build algorithm object
	var keyAlgo map[string]interface{}
	switch strings.ToUpper(algoName) {
	case "AES-CBC", "AES-GCM", "AES-CTR", "AES-KW":
		length := len(rawKey) * 8
		keyAlgo = map[string]interface{}{
			"name":   algoName,
			"length": length,
		}
	case "HMAC":
		hashName := "SHA-256"
		if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
			hashName = parseAlgorithmName(v)
		}
		length := len(rawKey) * 8
		keyAlgo = map[string]interface{}{
			"name":   "HMAC",
			"hash":   hashName,
			"length": length,
		}
	default:
		keyAlgo = map[string]interface{}{"name": algoName}
	}

	ck := newCryptoKey("secret", ext, keyAlgo, usages, rawKey, nil, nil)
	return resolvePromise(ck)
}

// subtleExportKey implements SubtleCrypto.exportKey().
func subtleExportKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 {
		panic(errInvalidArgType("format and key"))
	}
	format := strings.ToLower(args[0].String())
	key := args[1]

	kd := getCryptoKeyData(key)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}

	switch format {
	case "raw":
		return resolvePromise(buffer.NewBufferFromBytes(kd.rawKey))
	case "jwk":
		jwk := jsvalue.NewObject()
		jwk.Set("kty", jsvalue.NewString("oct"))
		jwk.Set("k", jsvalue.NewString(encodeBase64URL(kd.rawKey)))
		if algoName, ok := kd.algorithm["name"].(string); ok {
			jwk.Set("alg", jsvalue.NewString(jwkAlgName(algoName, kd.algorithm)))
		}
		jwk.Set("ext", jsvalue.NewBool(kd.extractable))
		jwk.Set("key_ops", jsvalue.NewArray(stringSliceToJSValues(kd.usages)...))
		return resolvePromise(jwk)
	default:
		panic(errOsslUnsupported(format + " export not yet supported"))
	}
}

// subtleSign implements SubtleCrypto.sign() for HMAC.
func subtleSign(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, key, and data"))
	}
	algoObj := args[0]
	key := args[1]
	data := inputBytes(args[2])

	kd := getCryptoKeyData(key)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}

	algoName := parseAlgorithmName(algoObj)

	switch strings.ToUpper(algoName) {
	case "HMAC":
		hashName := "SHA-256"
		if v, ok := kd.algorithm["hash"].(string); ok {
			hashName = v
		}
		factory := subtleDigestAlgo(hashName)
		if factory == nil {
			panic(errOsslUnsupported("HMAC hash not supported: " + hashName))
		}
		h := hmac.New(factory, kd.rawKey)
		h.Write(data)
		return resolvePromise(buffer.NewBufferFromBytes(h.Sum(nil)))

	case "RSASSA-PKCS1-V1_5":
		return resolvePromise(subtleRSASign(kd, algoObj, data, false))

	case "RSA-PSS":
		return resolvePromise(subtleRSASign(kd, algoObj, data, true))

	case "ECDSA":
		return resolvePromise(subtleECDSASign(kd, algoObj, data))

	case "ED25519":
		return resolvePromise(subtleEd25519Sign(kd, data))

	default:
		panic(errOsslUnsupported("sign algorithm not yet supported: " + algoName))
	}
}

// subtleVerify implements SubtleCrypto.verify() for HMAC.
func subtleVerify(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 4 {
		panic(errInvalidArgType("algorithm, key, signature, and data"))
	}
	algoObj := args[0]
	key := args[1]
	signature := inputBytes(args[2])
	data := inputBytes(args[3])

	kd := getCryptoKeyData(key)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}

	algoName := parseAlgorithmName(algoObj)

	switch strings.ToUpper(algoName) {
	case "HMAC":
		hashName := "SHA-256"
		if v, ok := kd.algorithm["hash"].(string); ok {
			hashName = v
		}
		factory := subtleDigestAlgo(hashName)
		if factory == nil {
			panic(errOsslUnsupported("HMAC hash not supported: " + hashName))
		}
		h := hmac.New(factory, kd.rawKey)
		h.Write(data)
		expected := h.Sum(nil)
		if len(signature) != len(expected) {
			return resolvePromise(jsvalue.NewBool(false))
		}
		return resolvePromise(jsvalue.NewBool(subtle.ConstantTimeCompare(signature, expected) == 1))

	case "RSASSA-PKCS1-V1_5":
		return resolvePromise(jsvalue.NewBool(subtleRSAVerify(kd, algoObj, signature, data, false)))

	case "RSA-PSS":
		return resolvePromise(jsvalue.NewBool(subtleRSAVerify(kd, algoObj, signature, data, true)))

	case "ECDSA":
		return resolvePromise(jsvalue.NewBool(subtleECDSAVerify(kd, algoObj, signature, data)))

	case "ED25519":
		return resolvePromise(jsvalue.NewBool(subtleEd25519Verify(kd, signature, data)))

	default:
		panic(errOsslUnsupported("verify algorithm not yet supported: " + algoName))
	}
}

// buildSubtleCrypto creates the SubtleCrypto object.
func buildSubtleCrypto() *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	obj.Set("digest", jsvalue.NewFunction(subtleDigest))
	obj.Set("generateKey", jsvalue.NewFunction(subtleGenerateKey))
	obj.Set("importKey", jsvalue.NewFunction(subtleImportKey))
	obj.Set("exportKey", jsvalue.NewFunction(subtleExportKey))
	obj.Set("sign", jsvalue.NewFunction(subtleSign))
	obj.Set("verify", jsvalue.NewFunction(subtleVerify))
	obj.Set("encrypt", jsvalue.NewFunction(subtleEncrypt))
	obj.Set("decrypt", jsvalue.NewFunction(subtleDecrypt))
	obj.Set("deriveBits", jsvalue.NewFunction(subtleDeriveBits))
	obj.Set("deriveKey", jsvalue.NewFunction(subtleDeriveKey))
	obj.Set("wrapKey", jsvalue.NewFunction(subtleWrapKey))
	obj.Set("unwrapKey", jsvalue.NewFunction(subtleUnwrapKey))

	// ML-KEM stubs (not yet supported)
	obj.Set("getPublicKey", jsvalue.NewFunction(subtleGetPublicKey))
	obj.Set("encapsulateBits", jsvalue.NewFunction(subtleEncapsulateBits))
	obj.Set("decapsulateBits", jsvalue.NewFunction(subtleDecapsulateBits))
	obj.Set("encapsulateKey", jsvalue.NewFunction(subtleEncapsulateKey))
	obj.Set("decapsulateKey", jsvalue.NewFunction(subtleDecapsulateKey))

	// Node.js 19+ subtle.supports()
	obj.Set("supports", jsvalue.NewFunction(subtleCryptoSupports))

	return obj
}

// Helpers

func decodeBase64URL(s string) ([]byte, error) {
	return base64Decode(s, true)
}

func encodeBase64URL(data []byte) string {
	return base64Encode(data, true)
}

func base64Decode(s string, url bool) ([]byte, error) {
	if url {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.StdEncoding.DecodeString(s)
}

func base64Encode(data []byte, url bool) string {
	if url {
		return base64.RawURLEncoding.EncodeToString(data)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func jwkAlgName(name string, algo map[string]interface{}) string {
	switch strings.ToUpper(name) {
	case "AES-GCM":
		return "A256GCM" // simplified
	case "AES-CBC":
		return "A256CBC"
	case "HMAC":
		if hash, ok := algo["hash"].(string); ok {
			switch strings.ToUpper(hash) {
			case "SHA-256":
				return "HS256"
			case "SHA-384":
				return "HS384"
			case "SHA-512":
				return "HS512"
			}
		}
	}
	return ""
}

func stringSliceToJSValues(s []string) []*jsvalue.JSValue {
	result := make([]*jsvalue.JSValue, len(s))
	for i, v := range s {
		result[i] = jsvalue.NewString(v)
	}
	return result
}
