package crypto

import (
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.ObjectFrom(
		// Hash
		"createHash", jsvalue.NewFunction(createHash),
		"getHashes", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return getHashes()
		}),
		"hash", jsvalue.NewFunction(hashOneShot),

		// HMAC
		"createHmac", jsvalue.NewFunction(createHmac),

		// Random
		"randomBytes", jsvalue.NewFunction(randomBytesJS),
		"randomFill", jsvalue.NewFunction(randomFillJS),
		"randomFillSync", jsvalue.NewFunction(randomFillSyncJS),
		"randomInt", jsvalue.NewFunction(randomIntJS),
		"randomUUID", jsvalue.NewFunction(randomUUIDJS),
		"getRandomValues", jsvalue.NewFunction(getRandomValuesJS),

		// KDF
		"scrypt", jsvalue.NewFunction(scryptJS),
		"scryptSync", jsvalue.NewFunction(scryptSyncJS),
		"pbkdf2", jsvalue.NewFunction(pbkdf2JS),
		"pbkdf2Sync", jsvalue.NewFunction(pbkdf2SyncJS),
		"hkdf", jsvalue.NewFunction(hkdfJS),
		"hkdfSync", jsvalue.NewFunction(hkdfSyncJS),

		// Utilities
		"timingSafeEqual", jsvalue.NewFunction(timingSafeEqualJS),

		// Constants
		"constants", buildConstants(),

		// Cipher
		"createCipheriv", jsvalue.NewFunction(createCipherivJS),
		"createDecipheriv", jsvalue.NewFunction(createDecipherivJS),
		"getCiphers", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return getCiphersJS()
		}),
		"getCipherInfo", jsvalue.NewFunction(getCipherInfoJS),

		// Sign/Verify
		"createSign", jsvalue.NewFunction(createSignJS),
		"createVerify", jsvalue.NewFunction(createVerifyJS),
		"sign", jsvalue.NewFunction(signOneShotJS),
		"verify", jsvalue.NewFunction(verifyOneShotJS),

		// Keys
		"createPublicKey", jsvalue.NewFunction(createPublicKeyJS),
		"createPrivateKey", jsvalue.NewFunction(createPrivateKeyJS),
		"createSecretKey", jsvalue.NewFunction(createSecretKeyJS),

		// Key generation
		"generateKeySync", jsvalue.NewFunction(generateKeySyncJS),
		"generateKey", jsvalue.NewFunction(generateKeyJS),
		"generateKeyPairSync", jsvalue.NewFunction(generateKeyPairSyncJS),
		"generateKeyPair", jsvalue.NewFunction(generateKeyPairJS),

		// RSA public/private encrypt/decrypt
		"publicEncrypt", jsvalue.NewFunction(publicEncryptJS),
		"privateDecrypt", jsvalue.NewFunction(privateDecryptJS),
		"privateEncrypt", jsvalue.NewFunction(privateEncryptJS),
		"publicDecrypt", jsvalue.NewFunction(publicDecryptJS),

		// Diffie-Hellman
		"createDiffieHellman", jsvalue.NewFunction(createDiffieHellmanJS),
		"createDiffieHellmanGroup", jsvalue.NewFunction(createDiffieHellmanGroupJS),
		"getDiffieHellman", jsvalue.NewFunction(getDiffieHellmanJS),
		"diffieHellman", jsvalue.NewFunction(diffieHellmanAsyncJS),

		// ECDH
		"createECDH", jsvalue.NewFunction(createECDHJS),
		"getCurves", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return getCurvesJS()
		}),
		"ecdhConvertKey", jsvalue.NewFunction(ecdhConvertKeyJS),

		// Argon2
		"argon2", jsvalue.NewFunction(argon2JS),
		"argon2Sync", jsvalue.NewFunction(argon2SyncJS),

		// Prime generation and checking
		"generatePrimeSync", jsvalue.NewFunction(generatePrimeSyncJS),
		"generatePrime", jsvalue.NewFunction(generatePrimeJS),
		"checkPrimeSync", jsvalue.NewFunction(checkPrimeSyncJS),
		"checkPrime", jsvalue.NewFunction(checkPrimeJS),

		// FIPS stubs
		"getFips", jsvalue.NewFunction(getFipsJS),
		"setFips", jsvalue.NewFunction(setFipsJS),
		"secureHeapUsed", jsvalue.NewFunction(secureHeapUsedJS),
		"setEngine", jsvalue.NewFunction(setEngineJS),
		"fips", jsvalue.NewBool(false),
	)

	// Wire subtle (same object for crypto.subtle and crypto.webcrypto.subtle)
	subtle := buildSubtleCrypto()
	obj.Set("subtle", subtle)

	// Build webcrypto with same subtle object
	webcrypto := jsvalue.NewObject()
	webcrypto.Set("subtle", subtle)
	webcrypto.Set("getRandomValues", jsvalue.NewFunction(getRandomValuesJS))
	webcrypto.Set("randomUUID", jsvalue.NewFunction(randomUUIDJS))
	obj.Set("webcrypto", webcrypto)

	return obj
}()
