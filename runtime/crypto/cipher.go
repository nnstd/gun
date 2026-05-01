package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func createCipherivJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, key, and iv"))
	}
	algo := strings.ToLower(args[0].String())
	key := inputBytes(args[1])
	iv := inputBytes(args[2])

	return newCipherObject(algo, key, iv, true)
}

func createDecipherivJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, key, and iv"))
	}
	algo := strings.ToLower(args[0].String())
	key := inputBytes(args[1])
	iv := inputBytes(args[2])

	return newCipherObject(algo, key, iv, false)
}

func newCipherObject(algo string, key, iv []byte, encrypt bool) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	autoPadding := true
	var aad []byte
	var authTag []byte
	finished := false
	var outputBuf []byte

	isAEAD := strings.HasSuffix(algo, "-gcm") || algo == "chacha20-poly1305"
	var aeadAccum []byte

	obj.Set("update", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if finished {
			panic(errUnsupportedOperation("update after final"))
		}
		if len(args) == 0 || args[0] == nil {
			return buffer.NewBufferFromBytes(nil)
		}
		data := inputBytes(args[0])
		if len(data) == 0 {
			return buffer.NewBufferFromBytes(nil)
		}

		if isAEAD {
			aeadAccum = append(aeadAccum, data...)
			return buffer.NewBufferFromBytes(nil)
		}

		result := processCipherChunk(algo, key, iv, data, aad, authTag, autoPadding, encrypt)
		outputBuf = append(outputBuf, result...)

		encoding, hasEnc := readEncoding(args, 1)
		if hasEnc && encoding != "" {
			return encodeOutput(result, encoding)
		}
		return buffer.NewBufferFromBytes(result)
	}))

	obj.Set("final", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if finished {
			panic(errUnsupportedOperation("final already called"))
		}
		finished = true

		var finalData []byte
		if isAEAD {
			finalData = aeadAccum
		} else if encrypt && autoPadding && (strings.HasSuffix(algo, "-cbc")) {
			finalData = pkcs7Pad(nil, aes.BlockSize)
		}

		result := processCipherFinal(algo, key, iv, finalData, aad, authTag, encrypt)
		outputBuf = append(outputBuf, result...)

		encoding, hasEnc := readEncoding(args, 0)
		if hasEnc && encoding != "" {
			return encodeOutput(result, encoding)
		}
		return buffer.NewBufferFromBytes(result)
	}))

	obj.Set("setAutoPadding", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			autoPadding = args[0].Bool()
		}
		return obj
	}))

	obj.Set("getAuthTag", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if !strings.HasSuffix(algo, "-gcm") && algo != "chacha20-poly1305" {
			panic(errUnsupportedOperation("getAuthTag only for authenticated modes"))
		}
		if authTag == nil && encrypt {
			return buffer.NewBufferFromBytes(nil)
		}
		return buffer.NewBufferFromBytes(authTag)
	}))

	obj.Set("setAAD", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			aad = inputBytes(args[0])
		}
		return obj
	}))

	obj.Set("setAuthTag", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			authTag = inputBytes(args[0])
		}
		return obj
	}))

	return obj
}

func processCipherChunk(algo string, key, iv, data, aad, authTag []byte, autoPadding bool, encrypt bool) []byte {
	switch {
	case strings.HasSuffix(algo, "-cbc"):
		block, err := aes.NewCipher(key)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		mode := cipher.NewCBCEncrypter(block, iv)
		if !encrypt {
			mode = cipher.NewCBCDecrypter(block, iv)
		}
		result := make([]byte, len(data))
		mode.CryptBlocks(result, data)
		return result

	case strings.HasSuffix(algo, "-ctr"):
		block, err := aes.NewCipher(key)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		stream := cipher.NewCTR(block, iv)
		result := make([]byte, len(data))
		stream.XORKeyStream(result, data)
		return result

	case strings.HasSuffix(algo, "-cfb"):
		block, err := aes.NewCipher(key)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		var stream cipher.Stream
		if encrypt {
			stream = cipher.NewCFBEncrypter(block, iv) //lint:ignore SA1019 // CFB required for Node.js compat
		} else {
			stream = cipher.NewCFBDecrypter(block, iv) //lint:ignore SA1019 // CFB required for Node.js compat
		}
		result := make([]byte, len(data))
		stream.XORKeyStream(result, data)
		return result

	default:
		panic(errUnknownCipher(algo))
	}
}

func processCipherFinal(algo string, key, iv, data, aad, authTag []byte, encrypt bool) []byte {
	switch {
	case strings.HasSuffix(algo, "-gcm"), algo == "chacha20-poly1305":
		var aead cipher.AEAD
		var err error
		if algo == "chacha20-poly1305" {
			aead, err = chacha20poly1305.New(key)
		} else {
			block, berr := aes.NewCipher(key)
			if berr != nil {
				panic(errOperationFailed(berr.Error()))
			}
			aead, err = cipher.NewGCM(block)
		}
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		if encrypt {
			return aead.Seal(nil, iv, data, aad)
		}
		plain, err := aead.Open(nil, iv, data, aad)
		if err != nil {
			panic(errOperationFailed("authentication failed"))
		}
		return plain

	case strings.HasSuffix(algo, "-cbc"):
		if len(data) == 0 {
			return nil
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		if encrypt {
			mode := cipher.NewCBCEncrypter(block, iv)
			result := make([]byte, len(data))
			mode.CryptBlocks(result, data)
			return result
		}
		mode := cipher.NewCBCDecrypter(block, iv)
		result := make([]byte, len(data))
		mode.CryptBlocks(result, data)
		return pkcs7Unpad(result)

	default:
		if len(data) == 0 {
			return nil
		}
		return data
	}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

func pkcs7Unpad(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > len(data) {
		return data
	}
	return data[:len(data)-padding]
}

func getCiphersJS() *jsvalue.JSValue {
	names := []*jsvalue.JSValue{
		jsvalue.NewString("aes-128-cbc"), jsvalue.NewString("aes-192-cbc"), jsvalue.NewString("aes-256-cbc"),
		jsvalue.NewString("aes-128-gcm"), jsvalue.NewString("aes-256-gcm"),
		jsvalue.NewString("aes-128-ctr"), jsvalue.NewString("aes-256-ctr"),
		jsvalue.NewString("aes-128-cfb"), jsvalue.NewString("aes-256-cfb"),
		jsvalue.NewString("chacha20-poly1305"),
	}
	return jsvalue.NewArray(names...)
}

func getCipherInfoJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("algorithm name or NID"))
	}
	name := strings.ToLower(args[0].String())

	info := jsvalue.NewObject()
	switch {
	case strings.Contains(name, "128"):
		info.Set("keyLength", jsvalue.NewNumber(128))
		info.Set("ivLength", jsvalue.NewNumber(16))
	case strings.Contains(name, "192"):
		info.Set("keyLength", jsvalue.NewNumber(192))
		info.Set("ivLength", jsvalue.NewNumber(16))
	case strings.Contains(name, "256"):
		info.Set("keyLength", jsvalue.NewNumber(256))
		info.Set("ivLength", jsvalue.NewNumber(16))
	default:
		info.Set("keyLength", jsvalue.NewNumber(256))
		info.Set("ivLength", jsvalue.NewNumber(16))
	}

	switch {
	case strings.HasSuffix(name, "-cbc"):
		info.Set("mode", jsvalue.NewString("cbc"))
	case strings.HasSuffix(name, "-gcm"):
		info.Set("mode", jsvalue.NewString("gcm"))
	case strings.HasSuffix(name, "-ctr"):
		info.Set("mode", jsvalue.NewString("ctr"))
	case strings.HasSuffix(name, "-cfb"):
		info.Set("mode", jsvalue.NewString("cfb"))
	case name == "chacha20-poly1305":
		info.Set("mode", jsvalue.NewString("chacha20-poly1305"))
		info.Set("keyLength", jsvalue.NewNumber(256))
		info.Set("ivLength", jsvalue.NewNumber(12))
	}
	info.Set("name", jsvalue.NewString(name))

	return info
}
