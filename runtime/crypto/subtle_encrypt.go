package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// subtleEncrypt implements SubtleCrypto.encrypt().
// Args: algorithm, key, data
func subtleEncrypt(args ...*jsvalue.JSValue) *jsvalue.JSValue {
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

	var encrypted []byte
	var err error

	switch strings.ToUpper(algoName) {
	case "AES-CBC":
		encrypted, err = aesCBCEncrypt(kd.rawKey, algoObj, data)
	case "AES-GCM":
		encrypted, err = aesGCMEncrypt(kd.rawKey, algoObj, data)
	case "AES-CTR":
		encrypted, err = aesCTREncrypt(kd.rawKey, algoObj, data)
	case "RSA-OAEP":
		encrypted, err = rsaOAEPEncrypt(kd, algoObj, data)
	default:
		panic(errOsslUnsupported("encrypt algorithm not supported: " + algoName))
	}

	if err != nil {
		panic(cryptoError("ERR_OPERATION_FAILED", err.Error()))
	}

	return resolvePromise(buffer.NewBufferFromBytes(encrypted))
}

// subtleDecrypt implements SubtleCrypto.decrypt().
// Args: algorithm, key, data
func subtleDecrypt(args ...*jsvalue.JSValue) *jsvalue.JSValue {
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

	var decrypted []byte
	var err error

	switch strings.ToUpper(algoName) {
	case "AES-CBC":
		decrypted, err = aesCBCDecrypt(kd.rawKey, algoObj, data)
	case "AES-GCM":
		decrypted, err = aesGCMDecrypt(kd.rawKey, algoObj, data)
	case "AES-CTR":
		decrypted, err = aesCTRDecrypt(kd.rawKey, algoObj, data)
	case "RSA-OAEP":
		decrypted, err = rsaOAEPDecrypt(kd, algoObj, data)
	default:
		panic(errOsslUnsupported("decrypt algorithm not supported: " + algoName))
	}

	if err != nil {
		panic(cryptoError("ERR_OPERATION_FAILED", err.Error()))
	}

	return resolvePromise(buffer.NewBufferFromBytes(decrypted))
}

// ---------------------------------------------------------------------------
// AES-CBC
// ---------------------------------------------------------------------------

func aesCBCEncrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	iv := getAlgoBytes(algoObj, "iv")
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("AES-CBC IV must be %d bytes", aes.BlockSize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// PKCS7 padding
	padded := pkcs7Pad(data, aes.BlockSize)

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return ciphertext, nil
}

func aesCBCDecrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	iv := getAlgoBytes(algoObj, "iv")
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("AES-CBC IV must be %d bytes", aes.BlockSize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("AES-CBC ciphertext must be a multiple of %d bytes", aes.BlockSize)
	}

	plaintext := make([]byte, len(data))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, data)

	// Remove PKCS7 padding
	return pkcs7Unpad(plaintext), nil
}

// ---------------------------------------------------------------------------
// AES-GCM
// ---------------------------------------------------------------------------

func aesGCMEncrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	iv := getAlgoBytes(algoObj, "iv")
	if len(iv) == 0 {
		iv = make([]byte, 12) // standard GCM nonce size
		if _, err := io.ReadFull(rand.Reader, iv); err != nil {
			return nil, err
		}
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Seal appends ciphertext + tag to nonce
	result := aesGCM.Seal(iv, iv, data, nil)
	return result, nil
}

func aesGCMDecrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	iv := getAlgoBytes(algoObj, "iv")
	if len(iv) == 0 {
		return nil, fmt.Errorf("AES-GCM: IV is required for decryption")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("AES-GCM: ciphertext too short")
	}

	// When IV is provided explicitly, the ciphertext does NOT include the nonce prefix.
	// data = ciphertext + tag
	plaintext, err := aesGCM.Open(nil, iv, data, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// ---------------------------------------------------------------------------
// AES-CTR
// ---------------------------------------------------------------------------

func aesCTREncrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	counter := getAlgoBytes(algoObj, "counter")
	if len(counter) != aes.BlockSize {
		return nil, fmt.Errorf("AES-CTR counter must be %d bytes", aes.BlockSize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, len(data))
	stream := cipher.NewCTR(block, counter)
	stream.XORKeyStream(ciphertext, data)

	return ciphertext, nil
}

func aesCTRDecrypt(key []byte, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	// CTR mode is symmetric: encryption and decryption are the same operation
	return aesCTREncrypt(key, algoObj, data)
}

// ---------------------------------------------------------------------------
// RSA-OAEP
// ---------------------------------------------------------------------------

func rsaOAEPEncrypt(kd *cryptoKeyData, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	pubKey, ok := kd.publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("RSA-OAEP encrypt requires an RSA public key")
	}

	hash := rsaOAEPHash(algoObj)
	label := getAlgoBytesOptional(algoObj, "label")

	return rsa.EncryptOAEP(hash, rand.Reader, pubKey, data, label)
}

func rsaOAEPDecrypt(kd *cryptoKeyData, algoObj *jsvalue.JSValue, data []byte) ([]byte, error) {
	privKey, ok := kd.privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("RSA-OAEP decrypt requires an RSA private key")
	}

	hash := rsaOAEPHash(algoObj)
	label := getAlgoBytesOptional(algoObj, "label")

	return rsa.DecryptOAEP(hash, rand.Reader, privKey, data, label)
}

func rsaOAEPHash(algoObj *jsvalue.JSValue) hash.Hash {
	hashName := "SHA-256"
	if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
		hashName = parseAlgorithmName(v)
	}

	switch strings.ToUpper(hashName) {
	case "SHA-1":
		return sha1.New()
	case "SHA-256":
		return sha256.New()
	case "SHA-384":
		return sha512.New384()
	case "SHA-512":
		return sha512.New()
	default:
		return sha256.New()
	}
}


// ---------------------------------------------------------------------------
// Algorithm property helpers
// ---------------------------------------------------------------------------

// getAlgoBytes extracts a byte slice from an algorithm object property.
// Panics if the property is missing or empty.
func getAlgoBytes(algoObj *jsvalue.JSValue, prop string) []byte {
	v := algoObj.Get(prop)
	if v == nil || v.TypeString() == "undefined" {
		return nil
	}
	return inputBytes(v)
}

// getAlgoBytesOptional extracts a byte slice from an algorithm object property.
// Returns nil if the property is missing.
func getAlgoBytesOptional(algoObj *jsvalue.JSValue, prop string) []byte {
	v := algoObj.Get(prop)
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "null" {
		return nil
	}
	b := inputBytes(v)
	if len(b) == 0 {
		return nil
	}
	return b
}

