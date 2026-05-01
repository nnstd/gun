package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/pem"
	"hash"
	"math/big"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func createSignJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		panic(errInvalidArgType("algorithm"))
	}
	algo := args[0].String()
	var accumulated []byte

	obj := jsvalue.NewObject()
	obj.Set("update", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			accumulated = append(accumulated, inputBytes(args[0])...)
		}
		return obj
	}))

	obj.Set("sign", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("privateKey"))
		}
		sig := signWithKey(algo, args[0], accumulated)
		encoding, hasEnc := readEncoding(args, 1)
		if hasEnc && encoding != "" {
			return encodeOutput(sig, encoding)
		}
		return buffer.NewBufferFromBytes(sig)
	}))

	return obj
}

func createVerifyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		panic(errInvalidArgType("algorithm"))
	}
	algo := args[0].String()
	var accumulated []byte

	obj := jsvalue.NewObject()
	obj.Set("update", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && args[0] != nil {
			accumulated = append(accumulated, inputBytes(args[0])...)
		}
		return obj
	}))

	obj.Set("verify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			panic(errInvalidArgType("publicKey and signature"))
		}
		sig := inputBytes(args[1])
		encoding, hasEnc := readEncoding(args, 2)
		if hasEnc && encoding != "" {
			decoded, err := decodeBase64URL(string(sig))
			if err == nil {
				sig = decoded
			}
		}
		valid := verifyWithKey(algo, args[0], sig, accumulated)
		return jsvalue.NewBool(valid)
	}))

	return obj
}

func signOneShotJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 3 {
		panic(errInvalidArgType("algorithm, data, and key"))
	}
	algo := args[0].String()
	data := inputBytes(args[1])
	key := args[2]

	sig := signWithKey(algo, key, data)

	cbIdx := 3
	if len(args) > cbIdx && args[cbIdx] != nil && args[cbIdx].TypeString() == "function" {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			return buffer.NewBufferFromBytes(sig), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(buffer.NewBufferFromBytes(sig))
}

func verifyOneShotJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 4 {
		panic(errInvalidArgType("algorithm, data, key, and signature"))
	}
	algo := args[0].String()
	data := inputBytes(args[1])
	key := args[2]
	sig := inputBytes(args[3])

	valid := verifyWithKey(algo, key, sig, data)

	cbIdx := 4
	if len(args) > cbIdx && args[cbIdx] != nil && args[cbIdx].TypeString() == "function" {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			return jsvalue.NewBool(valid), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(jsvalue.NewBool(valid))
}

func signWithKey(algo string, key *jsvalue.JSValue, data []byte) []byte {
	hasher, hashAlg := hashForAlgorithm(algo)
	if hasher != nil {
		hasher.Write(data)
		data = hasher.Sum(nil)
	}

	// Try CryptoKey
	if kd := getCryptoKeyData(key); kd != nil {
		return signWithCryptoKey(kd, hashAlg, data)
	}

	// Try PEM string
	if key.TypeString() == "string" {
		priv := parsePrivateKeyFromPEM(key.String())
		return signWithPrivateKey(priv, hashAlg, data)
	}

	panic(errInvalidKeyType("private key"))
}

func verifyWithKey(algo string, key *jsvalue.JSValue, sig, data []byte) bool {
	hasher, hashAlg := hashForAlgorithm(algo)
	if hasher != nil {
		hasher.Write(data)
		data = hasher.Sum(nil)
	}

	if kd := getCryptoKeyData(key); kd != nil {
		return verifyWithCryptoKey(kd, hashAlg, sig, data)
	}

	if key.TypeString() == "string" {
		pub := parsePublicKeyFromPEM(key.String())
		return verifyWithPublicKey(pub, hashAlg, sig, data)
	}

	panic(errInvalidKeyType("public key"))
}

func hashForAlgorithm(algo string) (hash.Hash, crypto.Hash) {
	algoLower := strings.ToLower(algo)
	switch {
	case strings.Contains(algoLower, "sha256"):
		return sha256.New(), crypto.SHA256
	case strings.Contains(algoLower, "sha384"):
		return sha512.New384(), crypto.SHA384
	case strings.Contains(algoLower, "sha512"):
		return sha512.New(), crypto.SHA512
	case algoLower == "ed25519":
		return nil, 0
	default:
		return sha256.New(), crypto.SHA256
	}
}

func signWithCryptoKey(kd *cryptoKeyData, hashAlg crypto.Hash, hashed []byte) []byte {
	switch priv := kd.privateKey.(type) {
	case *rsa.PrivateKey:
		if hashAlg == 0 {
			hashAlg = crypto.SHA256
		}
		sig, err := rsa.SignPKCS1v15(rand.Reader, priv, hashAlg, hashed)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return sig
	case *ecdsa.PrivateKey:
		r, s, err := ecdsa.Sign(rand.Reader, priv, hashed)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return encodeECDSASignature(r, s)
	case ed25519.PrivateKey:
		return ed25519.Sign(priv, hashed)
	default:
		panic(errInvalidKeyType("signing key"))
	}
}

func verifyWithCryptoKey(kd *cryptoKeyData, hashAlg crypto.Hash, sig, hashed []byte) bool {
	switch pub := kd.publicKey.(type) {
	case *rsa.PublicKey:
		if hashAlg == 0 {
			hashAlg = crypto.SHA256
		}
		return rsa.VerifyPKCS1v15(pub, hashAlg, hashed, sig) == nil
	case *ecdsa.PublicKey:
		r, s := decodeECDSASignature(sig)
		return ecdsa.Verify(pub, hashed, r, s)
	case ed25519.PublicKey:
		return ed25519.Verify(pub, hashed, sig)
	default:
		panic(errInvalidKeyType("verification key"))
	}
}

func signWithPrivateKey(priv interface{}, hashAlg crypto.Hash, hashed []byte) []byte {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		sig, err := rsa.SignPKCS1v15(rand.Reader, k, hashAlg, hashed)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return sig
	case *ecdsa.PrivateKey:
		r, s, err := ecdsa.Sign(rand.Reader, k, hashed)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return encodeECDSASignature(r, s)
	case ed25519.PrivateKey:
		return ed25519.Sign(k, hashed)
	default:
		panic(errInvalidKeyType("signing key"))
	}
}

func verifyWithPublicKey(pub interface{}, hashAlg crypto.Hash, sig, hashed []byte) bool {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(k, hashAlg, hashed, sig) == nil
	case *ecdsa.PublicKey:
		r, s := decodeECDSASignature(sig)
		return ecdsa.Verify(k, hashed, r, s)
	case ed25519.PublicKey:
		return ed25519.Verify(k, hashed, sig)
	default:
		panic(errSignatureInvalid())
	}
}

func parsePrivateKeyFromPEM(pemStr string) interface{} {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		panic(errOperationFailed("failed to parse PEM block"))
	}
	key, err := parsePrivateKeyPEMBlock(block)
	if err != nil {
		panic(errOperationFailed(err.Error()))
	}
	return key
}

func parsePublicKeyFromPEM(pemStr string) interface{} {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		panic(errOperationFailed("failed to parse PEM block"))
	}
	key, err := parsePublicKeyPEMBlock(block)
	if err != nil {
		panic(errOperationFailed(err.Error()))
	}
	return key
}

func encodeECDSASignature(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 0, len(rBytes)+len(sBytes))
	sig = append(sig, rBytes...)
	sig = append(sig, sBytes...)
	return sig
}

func decodeECDSASignature(sig []byte) (*big.Int, *big.Int) {
	if len(sig) == 0 {
		return new(big.Int), new(big.Int)
	}
	if sig[0] == 0x30 {
		return decodeECDSASignatureDER(sig)
	}
	half := len(sig) / 2
	if half == 0 {
		return new(big.Int), new(big.Int)
	}
	return new(big.Int).SetBytes(sig[:half]), new(big.Int).SetBytes(sig[half:])
}

func decodeECDSASignatureDER(sig []byte) (*big.Int, *big.Int) {
	if len(sig) < 8 || sig[0] != 0x30 {
		return new(big.Int), new(big.Int)
	}
	idx := 2
	if sig[1]&0x80 != 0 {
		idx = 2 + int(sig[1]&0x7f)
	}
	if idx+2 > len(sig) || sig[idx] != 0x02 {
		return new(big.Int), new(big.Int)
	}
	idx++
	rLen := int(sig[idx])
	idx++
	if idx+rLen > len(sig) {
		return new(big.Int), new(big.Int)
	}
	r := new(big.Int).SetBytes(sig[idx : idx+rLen])
	idx += rLen
	if idx+2 > len(sig) || sig[idx] != 0x02 {
		return r, new(big.Int)
	}
	idx++
	sLen := int(sig[idx])
	idx++
	if idx+sLen > len(sig) {
		return r, new(big.Int)
	}
	s := new(big.Int).SetBytes(sig[idx : idx+sLen])
	return r, s
}
