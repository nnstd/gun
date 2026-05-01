package crypto

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func buildConstants() *jsvalue.JSValue {
	c := jsvalue.NewObject()

	// OpenSSL cipher constants
	c.Set("SSL_OP_NO_SSLv2", jsvalue.NewNumber(0))
	c.Set("SSL_OP_NO_SSLv3", jsvalue.NewNumber(0x02000000))
	c.Set("SSL_OP_NO_TLSv1", jsvalue.NewNumber(0x04000000))
	c.Set("SSL_OP_NO_TLSv1_1", jsvalue.NewNumber(0x08000000))
	c.Set("SSL_OP_NO_TLSv1_2", jsvalue.NewNumber(0x10000000))
	c.Set("SSL_OP_NO_TLSv1_3", jsvalue.NewNumber(0x20000000))

	// RSA padding constants
	rsaObj := jsvalue.NewObject()
	rsaObj.Set("PKCS1_PADDING", jsvalue.NewNumber(1))
	rsaObj.Set("SSLV23_PADDING", jsvalue.NewNumber(2))
	rsaObj.Set("NO_PADDING", jsvalue.NewNumber(3))
	rsaObj.Set("PKCS1_OAEP_PADDING", jsvalue.NewNumber(4))
	rsaObj.Set("X931_PADDING", jsvalue.NewNumber(5))
	rsaObj.Set("PKCS1_PSS_PADDING", jsvalue.NewNumber(6))
	c.Set("RSA_PKCS1_PADDING", jsvalue.NewNumber(1))
	c.Set("RSA_SSLV23_PADDING", jsvalue.NewNumber(2))
	c.Set("RSA_NO_PADDING", jsvalue.NewNumber(3))
	c.Set("RSA_PKCS1_OAEP_PADDING", jsvalue.NewNumber(4))
	c.Set("RSA_X931_PADDING", jsvalue.NewNumber(5))
	c.Set("RSA_PKCS1_PSS_PADDING", jsvalue.NewNumber(6))
	c.Set("RSA_PSS_SALTLEN_DIGEST", jsvalue.NewNumber(-1))
	c.Set("RSA_PSS_SALTLEN_MAX_SIGN", jsvalue.NewNumber(-2))
	c.Set("RSA_PSS_SALTLEN_AUTO", jsvalue.NewNumber(-2))
	c.Set("RSA_PSS_SALTLEN_AUTO_DIGEST_MAX", jsvalue.NewNumber(-3))

	// DH constants
	c.Set("DH_CHECK_P_NOT_SAFE_PRIME", jsvalue.NewNumber(2))
	c.Set("DH_CHECK_P_NOT_PRIME", jsvalue.NewNumber(1))
	c.Set("DH_UNABLE_TO_CHECK_GENERATOR", jsvalue.NewNumber(4))
	c.Set("DH_NOT_SUITABLE_GENERATOR", jsvalue.NewNumber(8))
	c.Set("NPN_ENABLED", jsvalue.NewNumber(1))
	c.Set("ALPN_ENABLED", jsvalue.NewNumber(1))

	// Key types
	c.Set("OPENSSL_EC_NAMED_CURVE", jsvalue.NewNumber(0))
	c.Set("OPENSSL_EC_EXPLICIT_CURVE", jsvalue.NewNumber(1))
	c.Set("OPENSSL_DH_MAX_MODULUS_BITS", jsvalue.NewNumber(10000))
	c.Set("OPENSSL_RSA_MAX_MODULUS_BITS", jsvalue.NewNumber(16384))
	c.Set("OPENSSL_DSA_SIGNATURE_MAX_BITS", jsvalue.NewNumber(4096))

	// Core error codes
	c.Set("ERR_CRYPTO_CUSTOM_ENGINE_NOT_SUPPORTED", jsvalue.NewString("ERR_CRYPTO_CUSTOM_ENGINE_NOT_SUPPORTED"))
	c.Set("ERR_CRYPTO_ECDH_INVALID_PUBLIC_KEY", jsvalue.NewString("ERR_CRYPTO_ECDH_INVALID_PUBLIC_KEY"))
	c.Set("ERR_CRYPTO_INCOMPATIBLE_KEY", jsvalue.NewString("ERR_CRYPTO_INCOMPATIBLE_KEY"))
	c.Set("ERR_CRYPTO_INCOMPATIBLE_KEY_OPTIONS", jsvalue.NewString("ERR_CRYPTO_INCOMPATIBLE_KEY_OPTIONS"))
	c.Set("ERR_CRYPTO_INVALID_DIGEST", jsvalue.NewString("ERR_CRYPTO_INVALID_DIGEST"))
	c.Set("ERR_CRYPTO_INVALID_KEY_OBJECT_TYPE", jsvalue.NewString("ERR_CRYPTO_INVALID_KEY_OBJECT_TYPE"))
	c.Set("ERR_CRYPTO_INVALID_KEYLEN", jsvalue.NewString("ERR_CRYPTO_INVALID_KEYLEN"))
	c.Set("ERR_CRYPTO_INVALID_KEYPAIR", jsvalue.NewString("ERR_CRYPTO_INVALID_KEYPAIR"))
	c.Set("ERR_CRYPTO_INVALID_KEYTYPE", jsvalue.NewString("ERR_CRYPTO_INVALID_KEYTYPE"))
	c.Set("ERR_CRYPTO_INVALID_MESSAGE_LEN", jsvalue.NewString("ERR_CRYPTO_INVALID_MESSAGE_LEN"))
	c.Set("ERR_CRYPTO_INVALID_SCRYPT_PARAMS", jsvalue.NewString("ERR_CRYPTO_INVALID_SCRYPT_PARAMS"))
	c.Set("ERR_CRYPTO_INVALID_STATE", jsvalue.NewString("ERR_CRYPTO_INVALID_STATE"))
	c.Set("ERR_CRYPTO_INVALID_AUTH_TAG", jsvalue.NewString("ERR_CRYPTO_INVALID_AUTH_TAG"))
	c.Set("ERR_CRYPTO_IV_REQUIRED", jsvalue.NewString("ERR_CRYPTO_IV_REQUIRED"))
	c.Set("ERR_CRYPTO_KEYLEN_REQUIRED", jsvalue.NewString("ERR_CRYPTO_KEYLEN_REQUIRED"))
	c.Set("ERR_CRYPTO_OPERATION_FAILED", jsvalue.NewString("ERR_CRYPTO_OPERATION_FAILED"))
	c.Set("ERR_CRYPTO_UNKNOWN_CIPHER", jsvalue.NewString("ERR_CRYPTO_UNKNOWN_CIPHER"))
	c.Set("ERR_CRYPTO_UNKNOWN_DH_GROUP", jsvalue.NewString("ERR_CRYPTO_UNKNOWN_DH_GROUP"))
	c.Set("ERR_CRYPTO_UNKNOWN_HASH", jsvalue.NewString("ERR_CRYPTO_UNKNOWN_HASH"))
	c.Set("ERR_CRYPTO_SIGN_KEY_REQUIRED", jsvalue.NewString("ERR_CRYPTO_SIGN_KEY_REQUIRED"))

	// Sub-namespace: RSA
	c.Set("RSA", rsaObj)

	// Sub-namespace: hasCrypto (always true)
	c.Set("hasCrypto", jsvalue.NewBool(true))

	return c
}
