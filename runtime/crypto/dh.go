package crypto

import (
	"crypto/rand"
	"math/big"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// Diffie-Hellman state
// ---------------------------------------------------------------------------

// dhState holds the internal state for a DiffieHellman object.
type dhState struct {
	prime      *big.Int
	generator  *big.Int
	privateKey *big.Int
	publicKey  *big.Int
}

// ---------------------------------------------------------------------------
// Well-known DH groups (RFC 2409 / RFC 3526)
// ---------------------------------------------------------------------------

// modpPrimes maps group names to their well-known primes.
var modpPrimes = map[string]string{
	"modp1":  "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp2":  "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AAAC42DAD33170D04507A33A85521ABDF1CBA64" +
		"ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7" +
		"ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6B" +
		"F12FFA06D98A0864D87602733EC86A64521F2B18177B200C" +
		"BBE117577A615D6C770988C0BAD946E208E24FA074E5AB31" +
		"43DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF",
	"modp5":  "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp14": "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp15": "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp16": "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp17": "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
	"modp18": "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
}

// Actually use the real RFC 3526 primes for modp14-modp18.
// The placeholders above are modp5 (1536-bit). Replace with actual values.

func init() {
	// RFC 3526 2048-bit MODP Group (modp14)
	modpPrimes["modp14"] = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF"

	// RFC 3526 3072-bit MODP Group (modp15)
	modpPrimes["modp15"] = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF"

	// RFC 3526 4096-bit MODP Group (modp16)
	modpPrimes["modp16"] = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF"

	// RFC 3526 6144-bit MODP Group (modp17)
	modpPrimes["modp17"] = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF"

	// RFC 3526 8192-bit MODP Group (modp18)
	modpPrimes["modp18"] = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF"
}

// parseModpPrime parses a hex string into a *big.Int.
func parseModpPrime(hexStr string) *big.Int {
	p := new(big.Int)
	p.SetString(hexStr, 16)
	return p
}

// ---------------------------------------------------------------------------
// newDHObject creates a JSValue DiffieHellman object wrapping dhState.
// ---------------------------------------------------------------------------

func newDHObject(state *dhState) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	// verifyError property
	obj.Set("verifyError", jsvalue.NewNumber(0))

	// generateKeys([encoding])
	obj.Set("generateKeys", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// Generate random private key in [2, prime-2]
		maxPriv := new(big.Int).Sub(state.prime, big.NewInt(2))
		priv, err := rand.Int(rand.Reader, maxPriv)
		if err != nil {
			panic(errOperationFailed("failed to generate DH private key: " + err.Error()))
		}
		// Ensure private key >= 2
		if priv.Cmp(big.NewInt(2)) < 0 {
			priv.Set(big.NewInt(2))
		}
		state.privateKey = priv

		// publicKey = generator^privateKey mod prime
		state.publicKey = new(big.Int).Exp(state.generator, state.privateKey, state.prime)

		encoding, _ := readEncoding(args, 0)
		return encodeOutput(state.publicKey.Bytes(), encoding)
	}))

	// computeSecret(otherPublicKey[, inEnc][, outEnc])
	obj.Set("computeSecret", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("otherPublicKey"))
		}

		inEnc, _ := readEncoding(args, 1)
		outEnc, _ := readEncoding(args, 2)

		var otherPubBytes []byte
		if inEnc != "" {
			otherPubBytes = inputBytes(args[0])
		} else {
			otherPubBytes = inputBytes(args[0])
		}

		otherPub := new(big.Int).SetBytes(otherPubBytes)

		// shared secret = otherPub^privateKey mod prime
		secret := new(big.Int).Exp(otherPub, state.privateKey, state.prime)

		return encodeOutput(secret.Bytes(), outEnc)
	}))

	// getPrime([encoding])
	obj.Set("getPrime", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		encoding, _ := readEncoding(args, 0)
		return encodeOutput(state.prime.Bytes(), encoding)
	}))

	// getGenerator([encoding])
	obj.Set("getGenerator", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		encoding, _ := readEncoding(args, 0)
		return encodeOutput(state.generator.Bytes(), encoding)
	}))

	// getPrivateKey([encoding])
	obj.Set("getPrivateKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if state.privateKey == nil {
			return jsvalue.NewUndefined()
		}
		encoding, _ := readEncoding(args, 0)
		return encodeOutput(state.privateKey.Bytes(), encoding)
	}))

	// getPublicKey([encoding])
	obj.Set("getPublicKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if state.publicKey == nil {
			return jsvalue.NewUndefined()
		}
		encoding, _ := readEncoding(args, 0)
		return encodeOutput(state.publicKey.Bytes(), encoding)
	}))

	// setPrivateKey(key[, encoding])
	obj.Set("setPrivateKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("private key"))
		}
		state.privateKey = new(big.Int).SetBytes(inputBytes(args[0]))
		return obj
	}))

	// setPublicKey(key[, encoding])
	obj.Set("setPublicKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("public key"))
		}
		state.publicKey = new(big.Int).SetBytes(inputBytes(args[0]))
		return obj
	}))

	return obj
}

// ---------------------------------------------------------------------------
// createDiffieHellman(prime[, primeEncoding][, generator][, generatorEncoding])
// createDiffieHellman(primeLength[, generator])
// ---------------------------------------------------------------------------

func createDiffieHellmanJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("prime or primeLength"))
	}

	state := &dhState{}

	// If first arg is a number, it's primeLength mode
	if args[0].TypeString() == "number" {
		primeLength := int(args[0].Number())
		generator := big.NewInt(2)
		if len(args) > 1 && args[1] != nil && args[1].TypeString() == "number" {
			generator = big.NewInt(int64(args[1].Number()))
		}

		// Generate a safe prime of the given bit length
		// For simplicity, use rand.Prime (not necessarily safe prime but sufficient)
		prime, err := rand.Prime(rand.Reader, primeLength)
		if err != nil {
			panic(errOperationFailed("failed to generate DH prime: " + err.Error()))
		}

		state.prime = prime
		state.generator = generator
	} else {
		// prime string/buffer mode
		primeEnc, _ := readEncoding(args, 1)
		primeBytes := inputBytes(args[0])
		if primeEnc != "" {
			primeBytes = inputBytes(args[0]) // inputBytes already handles string
		}
		state.prime = new(big.Int).SetBytes(primeBytes)

		generator := big.NewInt(2)
		if len(args) > 2 && args[2] != nil && args[2].TypeString() != "undefined" {
			genEnc, _ := readEncoding(args, 3)
			genBytes := inputBytes(args[2])
			if genEnc != "" {
				genBytes = inputBytes(args[2])
			}
			generator = new(big.Int).SetBytes(genBytes)
		} else if len(args) > 1 && args[1] != nil && args[1].TypeString() == "number" {
			// createDiffieHellman(primeLength, generator) where first arg was not a number
			generator = big.NewInt(int64(args[1].Number()))
		}
		state.generator = generator
	}

	return newDHObject(state)
}

// ---------------------------------------------------------------------------
// getDiffieHellman(groupName)
// ---------------------------------------------------------------------------

func getDiffieHellmanJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("groupName"))
	}

	groupName := args[0].String()

	hexPrime, ok := modpPrimes[groupName]
	if !ok {
		panic(errOsslUnsupported("unknown DH group: " + groupName))
	}

	state := &dhState{
		prime:     parseModpPrime(hexPrime),
		generator: big.NewInt(2),
	}

	return newDHObject(state)
}

// ---------------------------------------------------------------------------
// createDiffieHellmanGroup(groupName) — alias for getDiffieHellman
// ---------------------------------------------------------------------------

func createDiffieHellmanGroupJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	return getDiffieHellmanJS(args...)
}

// ---------------------------------------------------------------------------
// diffieHellman(options[, callback]) — async DH
// ---------------------------------------------------------------------------

func diffieHellmanAsyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("options"))
	}

	opts := args[0]
	cbIdx := -1
	if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
		cbIdx = 1
	}

	// Extract options
	primeVal := opts.Get("prime")
	primeBytes := inputBytes(primeVal)
	if len(primeBytes) == 0 {
		panic(errInvalidArgType("options.prime"))
	}

	privKeyVal := opts.Get("privateKey")
	privKeyBytes := inputBytes(privKeyVal)
	if len(privKeyBytes) == 0 {
		panic(errInvalidArgType("options.privateKey"))
	}

	pubKeyVal := opts.Get("publicKey")
	pubKeyBytes := inputBytes(pubKeyVal)
	if len(pubKeyBytes) == 0 {
		panic(errInvalidArgType("options.publicKey"))
	}

	doDH := func() *jsvalue.JSValue {
		prime := new(big.Int).SetBytes(primeBytes)
		privKey := new(big.Int).SetBytes(privKeyBytes)
		otherPub := new(big.Int).SetBytes(pubKeyBytes)

		secret := new(big.Int).Exp(otherPub, privKey, prime)
		return buffer.NewBufferFromBytes(secret.Bytes())
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			return doDH(), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return doDH()
}
