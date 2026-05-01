package crypto

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"sync"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// CryptoKey internal storage
// ---------------------------------------------------------------------------

// cryptoKeyData stores the internal key material for a CryptoKey.
type cryptoKeyData struct {
	keyType     string // "secret", "public", "private"
	extractable bool
	algorithm   map[string]interface{}
	usages      []string
	rawKey      []byte
	publicKey   interface{} // *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey
	privateKey  interface{} // *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey
}

// cryptoKeyRegistry maps JSValue pointer strings to their Go-side key data.
// Uses a sentinel object approach for stable pointers.
var cryptoKeyRegistry sync.Map

func storeCryptoKeyData(key *jsvalue.JSValue, data *cryptoKeyData) {
	ptr := fmt.Sprintf("%p", key)
	cryptoKeyRegistry.Store(ptr, data)
}

func getCryptoKeyData(key *jsvalue.JSValue) *cryptoKeyData {
	ptr := fmt.Sprintf("%p", key)
	if v, ok := cryptoKeyRegistry.Load(ptr); ok {
		return v.(*cryptoKeyData)
	}
	// Fallback: check _cryptoKeyData sentinel property
	if v := key.Get("_cryptoKeyData"); v != nil && v.TypeString() != "undefined" {
		ptr2 := fmt.Sprintf("%p", v)
		if d, ok := cryptoKeyRegistry.Load(ptr2); ok {
			return d.(*cryptoKeyData)
		}
	}
	return nil
}

// newCryptoKey creates a CryptoKey JSValue and stores its internal data.
func newCryptoKey(keyType string, extractable bool, algorithm map[string]interface{}, usages []string, rawKey []byte, pubKey, privKey interface{}) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	// Set properties
	obj.Set("type", jsvalue.NewString(keyType))
	obj.Set("extractable", jsvalue.NewBool(extractable))

	// Build algorithm object
	algoObj := jsvalue.NewObject()
	for k, v := range algorithm {
			switch val := v.(type) {
			case string:
				algoObj.Set(k, jsvalue.NewString(val))
			case int:
				algoObj.Set(k, jsvalue.NewNumber(float64(val)))
			case float64:
				algoObj.Set(k, jsvalue.NewNumber(val))
			case bool:
				algoObj.Set(k, jsvalue.NewBool(val))
			}
		}
	obj.Set("algorithm", algoObj)

	// Build usages array
	usageVals := make([]*jsvalue.JSValue, len(usages))
	for i, u := range usages {
		usageVals[i] = jsvalue.NewString(u)
	}
	obj.Set("usages", jsvalue.NewArray(usageVals...))

	// Store internal data via sentinel for stable pointer
	data := &cryptoKeyData{
		keyType:     keyType,
		extractable: extractable,
		algorithm:   algorithm,
		usages:      usages,
		rawKey:      rawKey,
		publicKey:   pubKey,
		privateKey:  privKey,
	}

	sentinel := jsvalue.NewObject()
	obj.Set("_cryptoKeyData", sentinel)
	storeCryptoKeyData(sentinel, data)

	return obj
}

// ---------------------------------------------------------------------------
// KeyObject internal storage
// ---------------------------------------------------------------------------

// keyObjectData holds the Go-side data for a KeyObject JSValue.
type keyObjectData struct {
	keyType    string // "secret", "public", "private"
	symmetric  []byte
	publicKey  interface{}
	privateKey interface{}
}

// keyObjectRegistry maps JSValue pointer strings to their Go-side key object data.
var keyObjectRegistry sync.Map

func storeKeyObjectData(sentinel *jsvalue.JSValue, data *keyObjectData) {
	ptr := fmt.Sprintf("%p", sentinel)
	keyObjectRegistry.Store(ptr, data)
}

func getKeyObjectData(obj *jsvalue.JSValue) *keyObjectData {
	// Direct sentinel lookup
	ptr := fmt.Sprintf("%p", obj)
	if v, ok := keyObjectRegistry.Load(ptr); ok {
		return v.(*keyObjectData)
	}
	// Fallback: check _keyObjectData sentinel property
	if v := obj.Get("_keyObjectData"); v != nil && v.TypeString() != "undefined" {
		ptr2 := fmt.Sprintf("%p", v)
		if d, ok := keyObjectRegistry.Load(ptr2); ok {
			return d.(*keyObjectData)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// KeyObject creation
// ---------------------------------------------------------------------------

// newKeyObject creates a KeyObject JSValue with export, equals, and toCryptoKey methods.
func newKeyObject(keyType string, rawKey []byte, pubKey, privKey interface{}) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("_isKeyObject", jsvalue.NewBool(true))
	obj.Set("type", jsvalue.NewString(keyType))

	switch keyType {
	case "secret":
		obj.Set("symmetricKeySize", jsvalue.NewNumber(float64(len(rawKey))))
	case "public":
		obj.Set("asymmetricKeyType", jsvalue.NewString(keyTypeOf(pubKey)))
	case "private":
		obj.Set("asymmetricKeyType", jsvalue.NewString(keyTypeOf(privKey)))
	}

	// export([options]) method
	obj.Set("export", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		format := "buffer"
		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "object" {
			if f := args[0].Get("format"); f != nil && f.TypeString() != "undefined" {
				format = f.String()
			}
		}

		switch keyType {
		case "secret":
			switch format {
			case "hex":
				return jsvalue.NewString(fmt.Sprintf("%x", rawKey))
			case "base64":
				return jsvalue.NewString(base64.StdEncoding.EncodeToString(rawKey))
			case "buffer", "":
				return buffer.NewBufferFromBytes(rawKey)
			default:
				return buffer.NewBufferFromBytes(rawKey)
			}
		case "public":
			return exportPublicKey(pubKey, format)
		case "private":
			return exportPrivateKey(privKey, format)
		}
		return jsvalue.NewNull()
	}))

	// equals(otherKeyObject) method
	obj.Set("equals", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		other := args[0]
		if other.Get("_isKeyObject") == nil {
			return jsvalue.NewBool(false)
		}
		if other.Get("type").String() != keyType {
			return jsvalue.NewBool(false)
		}
		// Compare raw keys for secret type
		if keyType == "secret" && rawKey != nil {
			otherRaw := inputBytes(other.Get("export").Call())
			if len(rawKey) != len(otherRaw) {
				return jsvalue.NewBool(false)
			}
			return jsvalue.NewBool(subtle.ConstantTimeCompare(rawKey, otherRaw) == 1)
		}
		return jsvalue.NewBool(fmt.Sprintf("%p", obj) == fmt.Sprintf("%p", other))
	}))

	// toCryptoKey(algorithm, extractable, keyUsages) method
	obj.Set("toCryptoKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		algo := map[string]interface{}{}
		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "object" {
			algo = jsValueToMap(args[0])
		}
		extractable := false
		if len(args) > 1 && args[1] != nil {
			extractable = args[1].Bool()
		}
		var usages []string
		if len(args) > 2 && args[2] != nil {
			usagesArr := args[2]
			length := int(usagesArr.Get("length").Number())
			usages = make([]string, length)
			for i := 0; i < length; i++ {
				usages[i] = usagesArr.Get(fmt.Sprintf("%d", i)).String()
			}
		}

		var ckPubKey, ckPrivKey interface{}
		var ckRawKey []byte
		switch keyType {
		case "secret":
			ckRawKey = rawKey
		case "public":
			ckPubKey = pubKey
		case "private":
			ckPrivKey = privKey
		}

		return newCryptoKey(keyType, extractable, algo, usages, ckRawKey, ckPubKey, ckPrivKey)
	}))

	// Store key data for CryptoKey conversion
	data := &keyObjectData{
		keyType:    keyType,
		symmetric:  rawKey,
		publicKey:  pubKey,
		privateKey: privKey,
	}
	sentinel := jsvalue.NewObject()
	obj.Set("_keyObjectData", sentinel)
	storeKeyObjectData(sentinel, data)

	return obj
}

// ---------------------------------------------------------------------------
// KeyObject.from(key) — static factory from CryptoKey
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// createPublicKey
// ---------------------------------------------------------------------------

func createPublicKeyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("key"))
	}
	key := args[0]

	// If already a KeyObject, return it
	if key.Get("type") != nil && key.Get("_isKeyObject") != nil {
		return key
	}

	// If CryptoKey, convert
	if kd := getCryptoKeyData(key); kd != nil {
		if kd.keyType == "public" {
			return newKeyObject("public", nil, kd.publicKey, nil)
		}
		if kd.keyType == "private" && kd.privateKey != nil {
			return newKeyObject("public", nil, extractPublicKey(kd.privateKey), nil)
		}
	}

	// Parse from PEM or DER
	var pub interface{}
	var err error

	if key.TypeString() == "string" {
		block, _ := pem.Decode([]byte(key.String()))
		if block == nil {
			panic(errInvalidArgType("valid PEM string"))
		}
		pub, err = parsePublicKeyPEMBlock(block)
		if err != nil {
			panic(errOsslUnsupported("failed to parse public key: " + err.Error()))
		}
	} else if b := key.Bytes(); b != nil {
		// Buffer/DER
		pub, err = x509.ParsePKIXPublicKey(b)
		if err != nil {
			panic(errOsslUnsupported("failed to parse public key: " + err.Error()))
		}
	} else if key.TypeString() == "object" {
		// Object with key, pem, or der property
		if keyProp := key.Get("key"); keyProp != nil && keyProp.TypeString() != "undefined" {
			return createPublicKeyJS(keyProp)
		}
		if pemProp := key.Get("pem"); pemProp != nil && pemProp.TypeString() == "string" {
			block, _ := pem.Decode([]byte(pemProp.String()))
			if block == nil {
				panic(errInvalidArgType("valid PEM string"))
			}
			pub, err = parsePublicKeyPEMBlock(block)
			if err != nil {
				panic(errOsslUnsupported("failed to parse public key: " + err.Error()))
			}
		} else if derProp := key.Get("der"); derProp != nil {
			if b := derProp.Bytes(); b != nil {
				pub, err = x509.ParsePKIXPublicKey(b)
				if err != nil {
					panic(errOsslUnsupported("failed to parse public key: " + err.Error()))
				}
			} else {
				panic(errInvalidArgType("valid key material"))
			}
		} else {
			panic(errInvalidArgType("valid key material"))
		}
	} else {
		panic(errInvalidArgType("valid key material"))
	}

	return newKeyObject("public", nil, pub, nil)
}

// ---------------------------------------------------------------------------
// createPrivateKey
// ---------------------------------------------------------------------------

func createPrivateKeyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("key"))
	}
	key := args[0]

	// If already a KeyObject, return it
	if key.Get("type") != nil && key.Get("_isKeyObject") != nil {
		return key
	}

	// If CryptoKey, convert
	if kd := getCryptoKeyData(key); kd != nil {
		if kd.keyType == "private" {
			return newKeyObject("private", nil, nil, kd.privateKey)
		}
	}

	// Parse from PEM or DER
	var priv interface{}
	var err error

	if key.TypeString() == "string" {
		block, _ := pem.Decode([]byte(key.String()))
		if block == nil {
			panic(errInvalidArgType("valid PEM string"))
		}
		priv, err = parsePrivateKeyPEMBlock(block)
		if err != nil {
			panic(errOsslUnsupported("failed to parse private key: " + err.Error()))
		}
	} else if b := key.Bytes(); b != nil {
		// Buffer/DER
		priv, err = parsePrivateKeyDER(b)
		if err != nil {
			panic(errOsslUnsupported("failed to parse private key: " + err.Error()))
		}
	} else if key.TypeString() == "object" {
		// Object with key, pem, or der property
		if keyProp := key.Get("key"); keyProp != nil && keyProp.TypeString() != "undefined" {
			return createPrivateKeyJS(keyProp)
		}
		if pemProp := key.Get("pem"); pemProp != nil && pemProp.TypeString() == "string" {
			block, _ := pem.Decode([]byte(pemProp.String()))
			if block == nil {
				panic(errInvalidArgType("valid PEM string"))
			}
			priv, err = parsePrivateKeyPEMBlock(block)
			if err != nil {
				panic(errOsslUnsupported("failed to parse private key: " + err.Error()))
			}
		} else if derProp := key.Get("der"); derProp != nil {
			if b := derProp.Bytes(); b != nil {
				priv, err = parsePrivateKeyDER(b)
				if err != nil {
					panic(errOsslUnsupported("failed to parse private key: " + err.Error()))
				}
			} else {
				panic(errInvalidArgType("valid key material"))
			}
		} else {
			panic(errInvalidArgType("valid key material"))
		}
	} else {
		panic(errInvalidArgType("valid key material"))
	}

	return newKeyObject("private", nil, nil, priv)
}

// ---------------------------------------------------------------------------
// createSecretKey
// ---------------------------------------------------------------------------

func createSecretKeyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("key"))
	}
	raw := inputBytes(args[0])
	return newKeyObject("secret", raw, nil, nil)
}

// ---------------------------------------------------------------------------
// PEM/DER parsing helpers
// ---------------------------------------------------------------------------

// parsePublicKeyPEMBlock parses a PEM block into a public key.
func parsePublicKeyPEMBlock(block *pem.Block) (interface{}, error) {
	// PKIX (SPKI) format — "PUBLIC KEY" or any variant
	return x509.ParsePKIXPublicKey(block.Bytes)
}

// parsePrivateKeyPEMBlock parses a PEM block into a private key,
// detecting format from the block type header.
func parsePrivateKeyPEMBlock(block *pem.Block) (interface{}, error) {
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		// Fallback: try PKCS8, then PKCS1, then EC
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		return nil, fmt.Errorf("unrecognized PEM block type: %s", block.Type)
	}
}

// parsePrivateKeyDER tries PKCS8, PKCS1, and EC DER formats.
func parsePrivateKeyDER(der []byte) (interface{}, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("failed to parse private key DER (tried PKCS8, PKCS1, EC)")
}

// ---------------------------------------------------------------------------
// Export helpers
// ---------------------------------------------------------------------------

// exportPublicKey exports a public key in the specified format.
func exportPublicKey(pub interface{}, format string) *jsvalue.JSValue {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(errOperationFailed("failed to marshal public key: " + err.Error()))
	}

	switch format {
	case "pem":
		block := &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: der,
		}
		return jsvalue.NewString(string(pem.EncodeToMemory(block)))
	case "der":
		return buffer.NewBufferFromBytes(der)
	default:
		return buffer.NewBufferFromBytes(der)
	}
}

// exportPrivateKey exports a private key in the specified format.
func exportPrivateKey(priv interface{}, format string) *jsvalue.JSValue {
	var der []byte
	var err error
	var pemType string

	switch k := priv.(type) {
	case *rsa.PrivateKey:
		der = x509.MarshalPKCS1PrivateKey(k)
		pemType = "RSA PRIVATE KEY"
	case *ecdsa.PrivateKey:
		der, err = x509.MarshalECPrivateKey(k)
		pemType = "EC PRIVATE KEY"
	case ed25519.PrivateKey:
		der, err = x509.MarshalPKCS8PrivateKey(k)
		pemType = "PRIVATE KEY"
	default:
		der, err = x509.MarshalPKCS8PrivateKey(priv)
		pemType = "PRIVATE KEY"
	}

	if err != nil {
		panic(errOperationFailed("failed to marshal private key: " + err.Error()))
	}

	switch format {
	case "pem":
		block := &pem.Block{
			Type:  pemType,
			Bytes: der,
		}
		return jsvalue.NewString(string(pem.EncodeToMemory(block)))
	case "der":
		return buffer.NewBufferFromBytes(der)
	default:
		return buffer.NewBufferFromBytes(der)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers (used by keygen.go)
// ---------------------------------------------------------------------------

// extractPublicKey extracts a public key from a private key.
func extractPublicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	}
	return nil
}

// keyTypeOf returns the asymmetric key type name for a Go key.
func keyTypeOf(key interface{}) string {
	switch key.(type) {
	case *rsa.PublicKey, *rsa.PrivateKey:
		return "rsa"
	case *ecdsa.PublicKey, *ecdsa.PrivateKey:
		return "ec"
	case ed25519.PublicKey, ed25519.PrivateKey:
		return "ed25519"
	default:
		return "unknown"
	}
}

// jsValueToMap converts a JSValue object to a Go map[string]interface{}.
func jsValueToMap(obj *jsvalue.JSValue) map[string]interface{} {
	m := make(map[string]interface{})
	if obj == nil {
		return m
	}
	for _, key := range []string{"name", "length", "modulusLength", "publicExponent", "hash", "namedCurve", "saltLength", "iv", "tag"} {
		v := obj.Get(key)
		if v != nil && v.TypeString() != "undefined" {
			switch v.TypeString() {
			case "string":
				m[key] = v.String()
			case "number":
				m[key] = v.Number()
			case "boolean":
				m[key] = v.Bool()
			}
		}
	}
	return m
}

// rsaPublicExponentBig returns the RSA public exponent as *big.Int.
func rsaPublicExponentBig(opts *jsvalue.JSValue) *big.Int {
	if opts != nil {
		if v := opts.Get("publicExponent"); v != nil && v.TypeString() != "undefined" {
			return big.NewInt(int64(v.Number()))
		}
	}
	return big.NewInt(65537)
}

// namedCurve returns the elliptic.Curve for a WebCrypto namedCurve string.
func namedCurve(name string) elliptic.Curve {
	switch strings.ToUpper(name) {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	default:
		panic(errKeygenUnsupported(name))
	}
}

// filterUsages filters key usages by the key type ("public" or "private").
func filterUsages(usages []string, keyType string) []string {
	publicUsages := map[string]bool{
		"encrypt": true, "verify": true, "wrapKey": true, "deriveKey": true, "deriveBits": true,
	}
	privateUsages := map[string]bool{
		"decrypt": true, "sign": true, "unwrapKey": true, "deriveKey": true, "deriveBits": true,
	}
	allowed := privateUsages
	if keyType == "public" {
		allowed = publicUsages
	}
	var filtered []string
	for _, u := range usages {
		if allowed[u] {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// newKeyPairResult creates a {publicKey, privateKey} JSValue object from Go keys.
func newKeyPairResult(pubKey, privKey interface{}, algo map[string]interface{}, usages []string) *jsvalue.JSValue {
	pubCryptoKey := newCryptoKey("public", true, algo, filterUsages(usages, "public"), nil, pubKey, nil)
	privCryptoKey := newCryptoKey("private", true, algo, filterUsages(usages, "private"), nil, nil, privKey)
	return jsvalue.ObjectFrom(
		"publicKey", pubCryptoKey,
		"privateKey", privCryptoKey,
	)
}

// generateRSAKeyPair generates an RSA key pair.
func generateRSAKeyPair(modulusLength int, pubExp *big.Int) (interface{}, interface{}, error) {
	priv, err := rsa.GenerateKey(rand.Reader, modulusLength)
	if err != nil {
		return nil, nil, err
	}
	return &priv.PublicKey, priv, nil
}

// generateECKeyPair generates an ECDSA key pair.
func generateECKeyPair(curve elliptic.Curve) (interface{}, interface{}, error) {
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return &priv.PublicKey, priv, nil
}

// generateEd25519KeyPair generates an Ed25519 key pair.
func generateEd25519KeyPair() (interface{}, interface{}, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

