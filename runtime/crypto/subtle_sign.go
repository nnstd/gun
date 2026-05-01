package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"math/big"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func subtleRSASign(kd *cryptoKeyData, algoObj *jsvalue.JSValue, data []byte, pss bool) *jsvalue.JSValue {
	priv, ok := kd.privateKey.(*rsa.PrivateKey)
	if !ok {
		panic(errInvalidKeyType("RSA private key"))
	}

	hashAlg := subtleHashAlgo(algoObj)
	hasher := subtleHasher(hashAlg)
	if hasher == nil {
		panic(errOsslUnsupported("RSA hash algorithm not supported"))
	}
	hasher.Write(data)
	hashed := hasher.Sum(nil)

	var sig []byte
	var err error
	if pss {
		saltLen := rsa.PSSSaltLengthAuto
		if v := algoObj.Get("saltLength"); v != nil && v.TypeString() != "undefined" {
			saltLen = int(v.Number())
		}
		opts := &rsa.PSSOptions{SaltLength: saltLen, Hash: hashAlg}
		sig, err = rsa.SignPSS(rand.Reader, priv, hashAlg, hashed, opts)
	} else {
		sig, err = rsa.SignPKCS1v15(rand.Reader, priv, hashAlg, hashed)
	}
	if err != nil {
		panic(errOperationFailed(err.Error()))
	}
	return buffer.NewBufferFromBytes(sig)
}

func subtleRSAVerify(kd *cryptoKeyData, algoObj *jsvalue.JSValue, sig, data []byte, pss bool) bool {
	pub, ok := kd.publicKey.(*rsa.PublicKey)
	if !ok {
		panic(errInvalidKeyType("RSA public key"))
	}

	hashAlg := subtleHashAlgo(algoObj)
	hasher := subtleHasher(hashAlg)
	if hasher == nil {
		panic(errOsslUnsupported("RSA hash algorithm not supported"))
	}
	hasher.Write(data)
	hashed := hasher.Sum(nil)

	if pss {
		saltLen := rsa.PSSSaltLengthAuto
		if v := algoObj.Get("saltLength"); v != nil && v.TypeString() != "undefined" {
			saltLen = int(v.Number())
		}
		opts := &rsa.PSSOptions{SaltLength: saltLen, Hash: hashAlg}
		return rsa.VerifyPSS(pub, hashAlg, hashed, sig, opts) == nil
	}
	return rsa.VerifyPKCS1v15(pub, hashAlg, hashed, sig) == nil
}

func subtleECDSASign(kd *cryptoKeyData, algoObj *jsvalue.JSValue, data []byte) *jsvalue.JSValue {
	priv, ok := kd.privateKey.(*ecdsa.PrivateKey)
	if !ok {
		panic(errInvalidKeyType("ECDSA private key"))
	}

	hashAlg := subtleHashAlgo(algoObj)
	hasher := subtleHasher(hashAlg)
	if hasher == nil {
		panic(errOsslUnsupported("ECDSA hash algorithm not supported"))
	}
	hasher.Write(data)
	hashed := hasher.Sum(nil)

	r, s, err := ecdsa.Sign(rand.Reader, priv, hashed)
	if err != nil {
		panic(errOperationFailed(err.Error()))
	}

	// Check for DER format in algorithm
	useDER := false
	if algoObj.Get("hash") != nil {
		useDER = true
	}
	// WebCrypto ECDSA uses DER-encoded signatures by default
	sig := encodeECDSASignatureDER(r, s)
	if !useDER {
		sig = encodeECDSASignatureRaw(r, s)
	}
	return buffer.NewBufferFromBytes(sig)
}

func subtleECDSAVerify(kd *cryptoKeyData, algoObj *jsvalue.JSValue, sig, data []byte) bool {
	pub, ok := kd.publicKey.(*ecdsa.PublicKey)
	if !ok {
		panic(errInvalidKeyType("ECDSA public key"))
	}

	hashAlg := subtleHashAlgo(algoObj)
	hasher := subtleHasher(hashAlg)
	if hasher == nil {
		panic(errOsslUnsupported("ECDSA hash algorithm not supported"))
	}
	hasher.Write(data)
	hashed := hasher.Sum(nil)

	// Try DER first, then raw
	r, s := decodeECDSASignature(sig)
	if r == nil || s == nil {
		return false
	}
	return ecdsa.Verify(pub, hashed, r, s)
}

func subtleEd25519Sign(kd *cryptoKeyData, data []byte) *jsvalue.JSValue {
	priv, ok := kd.privateKey.(ed25519.PrivateKey)
	if !ok {
		panic(errInvalidKeyType("Ed25519 private key"))
	}
	return buffer.NewBufferFromBytes(ed25519.Sign(priv, data))
}

func subtleEd25519Verify(kd *cryptoKeyData, sig, data []byte) bool {
	pub, ok := kd.publicKey.(ed25519.PublicKey)
	if !ok {
		panic(errInvalidKeyType("Ed25519 public key"))
	}
	return ed25519.Verify(pub, data, sig)
}

func subtleHashAlgo(algoObj *jsvalue.JSValue) crypto.Hash {
	hashName := "SHA-256"
	// Algorithm can be a string or an object with "hash" property
	if algoObj.TypeString() == "string" {
		hashName = algoObj.String()
	} else if v := algoObj.Get("hash"); v != nil && v.TypeString() != "undefined" {
		if v.TypeString() == "string" {
			hashName = v.String()
		} else {
			hashName = parseAlgorithmName(v)
		}
	} else if v := algoObj.Get("name"); v != nil && v.TypeString() != "undefined" {
		name := v.String()
		if strings.Contains(strings.ToUpper(name), "SHA-1") {
			return crypto.SHA1
		}
	}

	switch strings.ToUpper(hashName) {
	case "SHA-1":
		return crypto.SHA1
	case "SHA-256":
		return crypto.SHA256
	case "SHA-384":
		return crypto.SHA384
	case "SHA-512":
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

func subtleHasher(hashAlg crypto.Hash) interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
} {
	switch hashAlg {
	case crypto.SHA1:
		return sha1.New()
	case crypto.SHA256:
		return sha256.New()
	case crypto.SHA384:
		return sha512.New384()
	case crypto.SHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

func encodeECDSASignatureDER(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	// DER encode: 0x30 <totalLen> 0x02 <rLen> <r> 0x02 <sLen> <s>
	if rBytes[0]&0x80 != 0 {
		rBytes = append([]byte{0}, rBytes...)
	}
	if sBytes[0]&0x80 != 0 {
		sBytes = append([]byte{0}, sBytes...)
	}
	totalLen := 2 + len(rBytes) + 2 + len(sBytes)
	result := make([]byte, 0, 2+totalLen)
	result = append(result, 0x30, byte(totalLen))
	result = append(result, 0x02, byte(len(rBytes)))
	result = append(result, rBytes...)
	result = append(result, 0x02, byte(len(sBytes)))
	result = append(result, sBytes...)
	return result
}

func encodeECDSASignatureRaw(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 0, len(rBytes)+len(sBytes))
	sig = append(sig, rBytes...)
	sig = append(sig, sBytes...)
	return sig
}

