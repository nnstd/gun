package crypto

import (
	jserror "github.com/nnstd/gun/runtime/builtin/error"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func cryptoError(code, message string) *jsvalue.JSValue {
	errVal := jserror.Error.Call(jsvalue.NewString(message))
	errVal.Set("code", jsvalue.NewString(code))
	return errVal
}

func cryptoTypeError(code, message string) *jsvalue.JSValue {
	errVal := jserror.TypeError.Call(jsvalue.NewString(message))
	errVal.Set("code", jsvalue.NewString(code))
	return errVal
}

func cryptoRangeError(code, message string) *jsvalue.JSValue {
	errVal := jserror.RangeError.Call(jsvalue.NewString(message))
	errVal.Set("code", jsvalue.NewString(code))
	return errVal
}

func errInvalidKeyLen() *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_KEYLEN", "Invalid key length")
}

func errInvalidSaltLen() *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_SALTLEN", "Invalid salt length")
}

func errInvalidMemoryCost() *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_MEMORY_COST", "Invalid scrypt memory cost (N must be power of 2 >= 2)")
}

func errInvalidCPUCost() *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_CPU_COST", "Invalid scrypt CPU cost (p must be > 0)")
}

func errInvalidBlockSize() *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_BLOCK_SIZE", "Invalid scrypt block size (r must be > 0)")
}

func errHashDigested() *jsvalue.JSValue {
	return cryptoError("ERR_CRYPTO_HASH_DIGESTED", "Digest already called")
}

func errHashUpdateAfterDigest() *jsvalue.JSValue {
	return cryptoError("ERR_CRYPTO_HASH_UPDATE_AFTER_DIGEST", "Digest already called")
}

func errUnknownHash(algo string) *jsvalue.JSValue {
	return cryptoTypeError("ERR_CRYPTO_UNKNOWN_HASH", "Unknown hash algorithm: "+algo)
}

func errUnknownCipher(algo string) *jsvalue.JSValue {
	return cryptoError("ERR_CRYPTO_UNKNOWN_CIPHER", "Unknown cipher algorithm: "+algo)
}

func errUnsupportedOperation(op string) *jsvalue.JSValue {
	return cryptoTypeError("ERR_CRYPTO_UNSUPPORTED_OPERATION", "Unsupported crypto operation: "+op)
}

func errInvalidKeyType(expected string) *jsvalue.JSValue {
	return cryptoTypeError("ERR_CRYPTO_INVALID_KEY_TYPE", "Invalid key type, expected "+expected)
}

func errInvalidLength(msg string) *jsvalue.JSValue {
	return cryptoRangeError("ERR_CRYPTO_INVALID_LENGTH", msg)
}

func errKeygenUnsupported(algo string) *jsvalue.JSValue {
	return cryptoTypeError("ERR_CRYPTO_KEYGEN_UNSUPPORTED", "Unsupported key generation algorithm: "+algo)
}

func errSignatureInvalid() *jsvalue.JSValue {
	return cryptoError("ERR_CRYPTO_SIGNATURE_INVALID", "Invalid signature")
}

func errOsslUnsupported(msg string) *jsvalue.JSValue {
	return cryptoError("ERR_OSSL_EVP_UNSUPPORTED", msg)
}

func errInvalidArgType(expected string) *jsvalue.JSValue {
	return cryptoTypeError("ERR_INVALID_ARG_TYPE", "The argument must be "+expected)
}

func errOutOfRange(msg string) *jsvalue.JSValue {
	return cryptoRangeError("ERR_OUT_OF_RANGE", msg)
}

func errOperationFailed(msg string) *jsvalue.JSValue {
	return cryptoError("ERR_OPERATION_FAILED", msg)
}
