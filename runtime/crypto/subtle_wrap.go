package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"

	"fmt"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// subtleWrapKey implements SubtleCrypto.wrapKey().
func subtleWrapKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 4 {
		panic(errInvalidArgType("format, key, wrappingKey, and wrapAlgo"))
	}
	format := args[0].String()
	key := args[1]
	wrappingKey := args[2]
	wrapAlgo := args[3]

	kd := getCryptoKeyData(key)
	if kd == nil {
		panic(errInvalidKeyType("CryptoKey"))
	}
	wkd := getCryptoKeyData(wrappingKey)
	if wkd == nil {
		panic(errInvalidKeyType("CryptoKey (wrapping)"))
	}

	// Export the key first
	var keyBytes []byte
	switch strings.ToLower(format) {
	case "raw":
		keyBytes = kd.rawKey
	default:
		panic(errOsslUnsupported("wrapKey format: " + format))
	}

	algoName := parseAlgorithmName(wrapAlgo)

	switch strings.ToUpper(algoName) {
	case "AES-KW":
		wrapped, err := aesKeyWrap(wkd.rawKey, keyBytes)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return resolvePromise(buffer.NewBufferFromBytes(wrapped))

	case "AES-GCM":
		iv := getAlgoBytes(wrapAlgo, "iv")
		block, err := aes.NewCipher(wkd.rawKey)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		encrypted := aead.Seal(nil, iv, keyBytes, nil)
		return resolvePromise(buffer.NewBufferFromBytes(encrypted))

	default:
		panic(errOsslUnsupported("wrapKey algorithm: " + algoName))
	}
}

// subtleUnwrapKey implements SubtleCrypto.unwrapKey().
func subtleUnwrapKey(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 7 {
		panic(errInvalidArgType("format, wrappedKey, unwrappingKey, unwrapAlgo, unwrappedKeyAlgo, extractable, keyUsages"))
	}
	format := args[0].String()
	wrappedKey := inputBytes(args[1])
	unwrappingKey := args[2]
	unwrapAlgo := args[3]
	unwrappedKeyAlgo := args[4]
	extractable := args[5]
	keyUsages := args[6]

	ukd := getCryptoKeyData(unwrappingKey)
	if ukd == nil {
		panic(errInvalidKeyType("CryptoKey (unwrapping)"))
	}

	algoName := parseAlgorithmName(unwrapAlgo)
	var keyBytes []byte

	switch strings.ToUpper(algoName) {
	case "AES-KW":
		unwraped, err := aesKeyUnwrap(ukd.rawKey, wrappedKey)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		keyBytes = unwraped

	case "AES-GCM":
		iv := getAlgoBytes(unwrapAlgo, "iv")
		block, err := aes.NewCipher(ukd.rawKey)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		unwraped, err := aead.Open(nil, iv, wrappedKey, nil)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		keyBytes = unwraped

	default:
		panic(errOsslUnsupported("unwrapKey algorithm: " + algoName))
	}

	// Import the unwrapped key
	importArgs := []*jsvalue.JSValue{
		jsvalue.NewString(format),
		buffer.NewBufferFromBytes(keyBytes),
		unwrappedKeyAlgo,
		extractable,
		keyUsages,
	}
	return subtleImportKey(importArgs...)
}

// aesKeyWrap implements RFC 3394 AES Key Wrap.
func aesKeyWrap(kek, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	if len(plaintext)%8 != 0 {
		return nil, fmt.Errorf("plaintext must be a multiple of 8 bytes")
	}

	// RFC 3394 initial value
	a := make([]byte, 8)
	copy(a, []byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6})

	n := len(plaintext) / 8
	r := make([][]byte, n)
	for i := 0; i < n; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], plaintext[i*8:])
	}

	for j := 0; j < 6; j++ {
		for i := 0; i < n; i++ {
			// B = AES(A || R[i])
			b := make([]byte, 16)
			copy(b, a)
			copy(b[8:], r[i])
			block.Encrypt(b, b)
			copy(a, b[:8])
			copy(r[i], b[8:])
			// XOR A with t
			t := uint64(n*(j+1) + i + 1)
			a[7] ^= byte(t)
			a[6] ^= byte(t >> 8)
			a[5] ^= byte(t >> 16)
			a[4] ^= byte(t >> 24)
			a[3] ^= byte(t >> 32)
			a[2] ^= byte(t >> 40)
			a[1] ^= byte(t >> 48)
			a[0] ^= byte(t >> 56)
		}
	}

	result := make([]byte, 8+len(plaintext))
	copy(result, a)
	for i := 0; i < n; i++ {
		copy(result[8+i*8:], r[i])
	}
	return result, nil
}

// aesKeyUnwrap implements RFC 3394 AES Key Unwrap.
func aesKeyUnwrap(kek, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < 16 || len(ciphertext)%8 != 0 {
		return nil, fmt.Errorf("ciphertext must be at least 16 bytes and a multiple of 8")
	}

	a := make([]byte, 8)
	copy(a, ciphertext[:8])

	n := (len(ciphertext) - 8) / 8
	r := make([][]byte, n)
	for i := 0; i < n; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], ciphertext[8+i*8:])
	}

	for j := 5; j >= 0; j-- {
		for i := n - 1; i >= 0; i-- {
			t := uint64(n*(j+1) + i + 1)
			a[7] ^= byte(t)
			a[6] ^= byte(t >> 8)
			a[5] ^= byte(t >> 16)
			a[4] ^= byte(t >> 24)
			a[3] ^= byte(t >> 32)
			a[2] ^= byte(t >> 40)
			a[1] ^= byte(t >> 48)
			a[0] ^= byte(t >> 56)
			// B = AES_decrypt(A || R[i])
			b := make([]byte, 16)
			copy(b, a)
			copy(b[8:], r[i])
			block.Decrypt(b, b)
			copy(a, b[:8])
			copy(r[i], b[8:])
		}
	}

	// Check integrity
	for i := 0; i < 8; i++ {
		if a[i] != 0xA6 {
			return nil, fmt.Errorf("AES-KW integrity check failed")
		}
	}

	result := make([]byte, n*8)
	for i := 0; i < n; i++ {
		copy(result[i*8:], r[i])
	}
	return result, nil
}

