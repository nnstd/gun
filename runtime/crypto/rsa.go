package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// RSA public encrypt / private decrypt helpers
// ---------------------------------------------------------------------------

// parseRSAPublicKey extracts an *rsa.PublicKey from a PEM string, KeyObject, or CryptoKey.
func parseRSAPublicKey(key *jsvalue.JSValue) *rsa.PublicKey {
	if key == nil {
		panic(errInvalidArgType("a public key (PEM string, KeyObject, or CryptoKey)"))
	}

	// CryptoKey
	if kd := getCryptoKeyData(key); kd != nil {
		if pub, ok := kd.publicKey.(*rsa.PublicKey); ok {
			return pub
		}
		if priv, ok := kd.privateKey.(*rsa.PrivateKey); ok {
			return &priv.PublicKey
		}
		panic(errInvalidKeyType("RSA public key"))
	}

	// KeyObject
	if kod := getKeyObjectData(key); kod != nil {
		if pub, ok := kod.publicKey.(*rsa.PublicKey); ok {
			return pub
		}
		if priv, ok := kod.privateKey.(*rsa.PrivateKey); ok {
			return &priv.PublicKey
		}
		panic(errInvalidKeyType("RSA public key"))
	}

	// PEM string
	if key.TypeString() == "string" {
		block, _ := pem.Decode([]byte(key.String()))
		if block == nil {
			panic(errInvalidArgType("valid PEM-encoded public key"))
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			// Try PKCS1 fallback
			pkcs1Pub, err2 := x509.ParsePKCS1PublicKey(block.Bytes)
			if err2 != nil {
				panic(errOsslUnsupported("failed to parse RSA public key: " + err.Error()))
			}
			return pkcs1Pub
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			panic(errInvalidKeyType("RSA public key"))
		}
		return rsaPub
	}

	// Buffer / raw bytes
	if b := key.Bytes(); b != nil {
		pub, err := x509.ParsePKIXPublicKey(b)
		if err != nil {
			pkcs1Pub, err2 := x509.ParsePKCS1PublicKey(b)
			if err2 != nil {
				panic(errOsslUnsupported("failed to parse RSA public key: " + err.Error()))
			}
			return pkcs1Pub
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			panic(errInvalidKeyType("RSA public key"))
		}
		return rsaPub
	}

	panic(errInvalidArgType("a public key (PEM string, KeyObject, or CryptoKey)"))
}

// parseRSAPrivateKey extracts an *rsa.PrivateKey from a PEM string, KeyObject, or CryptoKey.
func parseRSAPrivateKey(key *jsvalue.JSValue) *rsa.PrivateKey {
	if key == nil {
		panic(errInvalidArgType("a private key (PEM string, KeyObject, or CryptoKey)"))
	}

	// CryptoKey
	if kd := getCryptoKeyData(key); kd != nil {
		if priv, ok := kd.privateKey.(*rsa.PrivateKey); ok {
			return priv
		}
		panic(errInvalidKeyType("RSA private key"))
	}

	// KeyObject
	if kod := getKeyObjectData(key); kod != nil {
		if priv, ok := kod.privateKey.(*rsa.PrivateKey); ok {
			return priv
		}
		panic(errInvalidKeyType("RSA private key"))
	}

	// PEM string
	if key.TypeString() == "string" {
		block, _ := pem.Decode([]byte(key.String()))
		if block == nil {
			panic(errInvalidArgType("valid PEM-encoded private key"))
		}
		return parseRSAPrivateKeyPEMBlock(block)
	}

	// Buffer / raw bytes
	if b := key.Bytes(); b != nil {
		priv, err := x509.ParsePKCS1PrivateKey(b)
		if err != nil {
			priv8, err2 := x509.ParsePKCS8PrivateKey(b)
			if err2 != nil {
				panic(errOsslUnsupported("failed to parse RSA private key: " + err.Error()))
			}
			rsaPriv, ok := priv8.(*rsa.PrivateKey)
			if !ok {
				panic(errInvalidKeyType("RSA private key"))
			}
			return rsaPriv
		}
		return priv
	}

	panic(errInvalidArgType("a private key (PEM string, KeyObject, or CryptoKey)"))
}

// parseRSAPrivateKeyPEMBlock parses a PEM block into an *rsa.PrivateKey.
func parseRSAPrivateKeyPEMBlock(block *pem.Block) *rsa.PrivateKey {
	switch block.Type {
	case "RSA PRIVATE KEY":
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			panic(errOsslUnsupported("failed to parse RSA private key: " + err.Error()))
		}
		return priv
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			panic(errOsslUnsupported("failed to parse RSA private key: " + err.Error()))
		}
		priv, ok := key.(*rsa.PrivateKey)
		if !ok {
			panic(errInvalidKeyType("RSA private key"))
		}
		return priv
	default:
		// Fallback: try PKCS1 then PKCS8
		if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return priv
		}
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			priv, ok := key.(*rsa.PrivateKey)
			if !ok {
				panic(errInvalidKeyType("RSA private key"))
			}
			return priv
		}
		panic(errOsslUnsupported("failed to parse RSA private key PEM block"))
	}
}

// rsaPaddingFromOpts reads the padding option from an options object.
// Returns 1 for PKCS1v15 (default), 4 for OAEP.
func rsaPaddingFromOpts(opts *jsvalue.JSValue) int {
	if opts == nil {
		return 1 // PKCS1v15 default
	}
	if v := opts.Get("padding"); v != nil && v.TypeString() != "undefined" {
		return int(v.Number())
	}
	return 1
}

// rsaOAEPHashFromOpts reads the OAEP hash name from an options object.
func rsaOAEPHashFromOpts(opts *jsvalue.JSValue) string {
	if opts == nil {
		return "sha256"
	}
	if hashObj := opts.Get("oaepHash"); hashObj != nil && hashObj.TypeString() != "undefined" {
		return hashObj.String()
	}
	return "sha256"
}

// ---------------------------------------------------------------------------
// publicEncrypt(key, buffer[, options])
// ---------------------------------------------------------------------------

func publicEncryptJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 || args[0] == nil || args[1] == nil {
		panic(errInvalidArgType("key and buffer"))
	}

	pub := parseRSAPublicKey(args[0])
	data := inputBytes(args[1])
	opts := args[2]

	padding := rsaPaddingFromOpts(opts)

	var encrypted []byte
	var err error

	switch padding {
	case 1: // PKCS1v15
		encrypted, err = rsa.EncryptPKCS1v15(rand.Reader, pub, data)
	case 4: // OAEP
		hashName := rsaOAEPHashFromOpts(opts)
		h := hashFactory(hashName)()
		label := []byte{}
		if opts != nil {
			if l := opts.Get("oaepLabel"); l != nil && l.TypeString() != "undefined" {
				label = inputBytes(l)
			}
		}
		encrypted, err = rsa.EncryptOAEP(h, rand.Reader, pub, data, label)
	default:
		panic(errOsslUnsupported("unsupported RSA padding: only PKCS1v15 (1) and OAEP (4) are supported"))
	}

	if err != nil {
		panic(errOperationFailed("RSA encryption failed: " + err.Error()))
	}

	return buffer.NewBufferFromBytes(encrypted)
}

// ---------------------------------------------------------------------------
// publicDecrypt(key, buffer) — rare operation
// ---------------------------------------------------------------------------

func publicDecryptJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 || args[0] == nil || args[1] == nil {
		panic(errInvalidArgType("key and buffer"))
	}

	// publicDecrypt expects a private key (it reverses the operation of privateEncrypt)
	// but in Node.js it takes a public key that corresponds to a private key used for signing.
	// We treat the key as having both components available via KeyObject/CryptoKey.
	priv := parseRSAPrivateKey(args[0])
	data := inputBytes(args[1])

	decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, priv, data)
	if err != nil {
		panic(errOperationFailed("RSA public decrypt failed: " + err.Error()))
	}

	return buffer.NewBufferFromBytes(decrypted)
}

// ---------------------------------------------------------------------------
// privateEncrypt(privateKey, buffer)
// ---------------------------------------------------------------------------

func privateEncryptJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 || args[0] == nil || args[1] == nil {
		panic(errInvalidArgType("privateKey and buffer"))
	}

	priv := parseRSAPrivateKey(args[0])
	data := inputBytes(args[1])

	// privateEncrypt signs with PKCS1v15 (hash=0 for raw signing)
	hashed := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, 0, hashed[:])
	if err != nil {
		panic(errOperationFailed("RSA private encrypt failed: " + err.Error()))
	}

	return buffer.NewBufferFromBytes(signature)
}

// ---------------------------------------------------------------------------
// privateDecrypt(privateKey, buffer[, options])
// ---------------------------------------------------------------------------

func privateDecryptJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 || args[0] == nil || args[1] == nil {
		panic(errInvalidArgType("privateKey and buffer"))
	}

	priv := parseRSAPrivateKey(args[0])
	data := inputBytes(args[1])
	opts := args[2]

	padding := rsaPaddingFromOpts(opts)

	var decrypted []byte
	var err error

	switch padding {
	case 1: // PKCS1v15
		decrypted, err = rsa.DecryptPKCS1v15(rand.Reader, priv, data)
	case 4: // OAEP
		hashName := rsaOAEPHashFromOpts(opts)
		h := hashFactory(hashName)()
		label := []byte{}
		if opts != nil {
			if l := opts.Get("oaepLabel"); l != nil && l.TypeString() != "undefined" {
				label = inputBytes(l)
			}
		}
		decrypted, err = rsa.DecryptOAEP(h, rand.Reader, priv, data, label)
	default:
		panic(errOsslUnsupported("unsupported RSA padding: only PKCS1v15 (1) and OAEP (4) are supported"))
	}

	if err != nil {
		panic(errOperationFailed("RSA decryption failed: " + err.Error()))
	}

	return buffer.NewBufferFromBytes(decrypted)
}
