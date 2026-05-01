package crypto

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"math/big"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// ECDH state
// ---------------------------------------------------------------------------

// ecdhState holds the internal state for an ECDH object.
type ecdhState struct {
	curve      elliptic.Curve
	ecdhCurve  ecdh.Curve
	curveName  string
	privateKey *ecdsa.PrivateKey
}

// ecdhCurveForName returns the elliptic.Curve for a Node.js curve name.
func ecdhCurveForName(name string) elliptic.Curve {
	switch name {
	case "prime256v1", "secp256r1", "P-256":
		return elliptic.P256()
	case "secp384r1", "P-384":
		return elliptic.P384()
	case "secp521r1", "P-521":
		return elliptic.P521()
	case "secp256k1":
		return nil
	default:
		return nil
	}
}

func ecdhCurveForNameECDH(name string) ecdh.Curve {
	switch name {
	case "prime256v1", "secp256r1", "P-256":
		return ecdh.P256()
	case "secp384r1", "P-384":
		return ecdh.P384()
	case "secp521r1", "P-521":
		return ecdh.P521()
	default:
		return nil
	}
}

// marshalECDHPublicKey marshals a public key point in the specified format.
// format: "uncompressed" (default, 0x04 prefix), "compressed" (0x02/0x03), "hybrid".
func marshalECDHPublicKey(x, y *big.Int, curve elliptic.Curve, format string) []byte {
	byteLen := (curve.Params().BitSize + 7) / 8

	switch format {
	case "compressed":
		result := make([]byte, 1+byteLen)
		result[0] = 0x02
		if y.Bit(0) == 1 {
			result[0] = 0x03
		}
		xBytes := x.Bytes()
		copy(result[1+byteLen-len(xBytes):], xBytes)
		return result
	case "hybrid":
		result := make([]byte, 1+2*byteLen)
		result[0] = 0x06
		if y.Bit(0) == 1 {
			result[0] = 0x07
		}
		xBytes := x.Bytes()
		yBytes := y.Bytes()
		copy(result[1+2*byteLen-2*len(xBytes):1+byteLen-len(xBytes)], xBytes)
		copy(result[1+byteLen+2*byteLen-len(yBytes):], yBytes)
		// Correct: copy x into position, then y
		result2 := make([]byte, 1+2*byteLen)
		result2[0] = result[0]
		copy(result2[1+byteLen-len(xBytes):1+byteLen], xBytes)
		copy(result2[1+byteLen+byteLen-len(yBytes):], yBytes)
		return result2
	default:
		// uncompressed (0x04 prefix)
		result := make([]byte, 1+2*byteLen)
		result[0] = 0x04
		xBytes := x.Bytes()
		yBytes := y.Bytes()
		copy(result[1+byteLen-len(xBytes):1+byteLen], xBytes)
		copy(result[1+byteLen+byteLen-len(yBytes):], yBytes)
		return result
	}
}

// unmarshalECDHPublicKey parses a public key from bytes (uncompressed or compressed format).
func unmarshalECDHPublicKey(data []byte, curve elliptic.Curve) (x, y *big.Int) {
	byteLen := (curve.Params().BitSize + 7) / 8

	if len(data) == 0 {
		panic(errInvalidArgType("a valid public key"))
	}

	switch data[0] {
	case 0x04:
		// Uncompressed
		if len(data) != 1+2*byteLen {
			panic(errInvalidArgType("a valid uncompressed public key"))
		}
		x = new(big.Int).SetBytes(data[1 : 1+byteLen])
		y = new(big.Int).SetBytes(data[1+byteLen:])
	case 0x02, 0x03:
		x, y = elliptic.UnmarshalCompressed(curve, data)
		if x == nil {
			panic(errInvalidArgType("a valid compressed public key"))
		}
	default:
		// Try hex decoding if it looks like a hex string
		panic(errInvalidArgType("a valid public key (expected 0x04 uncompressed or 0x02/0x03 compressed format)"))
	}

	return
}

// ---------------------------------------------------------------------------
// newECDHObject creates a JSValue ECDH object wrapping ecdhState.
// ---------------------------------------------------------------------------

func newECDHObject(state *ecdhState) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	// generateKeys([encoding[, format]])
	obj.Set("generateKeys", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		priv, err := ecdsa.GenerateKey(state.curve, rand.Reader)
		if err != nil {
			panic(errOperationFailed("failed to generate ECDH key pair: " + err.Error()))
		}
		state.privateKey = priv

		encoding, _ := readEncoding(args, 0)
		format := "uncompressed"
		if len(args) > 1 && args[1] != nil && args[1].TypeString() != "undefined" {
			format = args[1].String()
		}

		pubBytes := marshalECDHPublicKey(
			state.privateKey.PublicKey.X,
			state.privateKey.PublicKey.Y,
			state.curve,
			format,
		)

		if encoding == "hex" {
			return jsvalue.NewString(hex.EncodeToString(pubBytes))
		}
		return buffer.NewBufferFromBytes(pubBytes)
	}))

	// computeSecret(otherPublicKey[, inEnc][, outEnc])
	obj.Set("computeSecret", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("otherPublicKey"))
		}
		if state.privateKey == nil {
			panic(errOperationFailed("ECDH: generateKeys must be called before computeSecret"))
		}

		inEnc, _ := readEncoding(args, 1)
		outEnc, _ := readEncoding(args, 2)

		var pubKeyBytes []byte
		if inEnc == "hex" {
			decoded, err := hex.DecodeString(args[0].String())
			if err != nil {
				panic(errInvalidArgType("a valid hex-encoded public key"))
			}
			pubKeyBytes = decoded
		} else {
			pubKeyBytes = inputBytes(args[0])
		}

			var secret []byte
			if state.ecdhCurve != nil {
				// Use crypto/ecdh (preferred, validates on-curve automatically)
				// Re-marshal the public key to uncompressed bytes for ecdh.NewPublicKey
				x, y := unmarshalECDHPublicKey(pubKeyBytes, state.curve)
				pubForECDH := marshalECDHPublicKey(x, y, state.curve, "uncompressed")
				ecdhPub, err := state.ecdhCurve.NewPublicKey(pubForECDH)
				if err != nil {
					panic(errOperationFailed("ECDH: invalid public key: " + err.Error()))
				}
				ecdhPriv, err := state.ecdhCurve.NewPrivateKey(state.privateKey.D.Bytes())
				if err != nil {
					panic(errOperationFailed("ECDH: invalid private key: " + err.Error()))
				}
				secret, err = ecdhPriv.ECDH(ecdhPub)
				if err != nil {
					panic(errOperationFailed("ECDH: shared secret computation failed: " + err.Error()))
				}
			} else {
				// Fallback for curves not supported by crypto/ecdh (e.g. secp256k1)
				x, y := unmarshalECDHPublicKey(pubKeyBytes, state.curve)
				scalarX, _ := state.curve.ScalarMult(x, y, state.privateKey.D.Bytes()) //lint:ignore SA1019 // fallback for secp256k1
				secret = scalarX.Bytes()
			}

			if outEnc == "hex" {
				return jsvalue.NewString(hex.EncodeToString(secret))
			}
			return buffer.NewBufferFromBytes(secret)
	}))

	// getPrivateKey([encoding])
	obj.Set("getPrivateKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if state.privateKey == nil {
			return jsvalue.NewUndefined()
		}

		encoding, _ := readEncoding(args, 0)
		privBytes := state.privateKey.D.Bytes()

		if encoding == "hex" {
			return jsvalue.NewString(hex.EncodeToString(privBytes))
		}
		return buffer.NewBufferFromBytes(privBytes)
	}))

	// getPublicKey([encoding][, format])
	obj.Set("getPublicKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if state.privateKey == nil {
			return jsvalue.NewUndefined()
		}

		encoding, _ := readEncoding(args, 0)
		format := "uncompressed"
		if len(args) > 1 && args[1] != nil && args[1].TypeString() != "undefined" {
			format = args[1].String()
		}

		pubBytes := marshalECDHPublicKey(
			state.privateKey.PublicKey.X,
			state.privateKey.PublicKey.Y,
			state.curve,
			format,
		)

		if encoding == "hex" {
			return jsvalue.NewString(hex.EncodeToString(pubBytes))
		}
		return buffer.NewBufferFromBytes(pubBytes)
	}))

	// setPrivateKey(key[, encoding])
	obj.Set("setPrivateKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("private key"))
		}

		enc, _ := readEncoding(args, 1)
		var privBytes []byte
		if enc == "hex" {
			decoded, err := hex.DecodeString(args[0].String())
			if err != nil {
				panic(errInvalidArgType("a valid hex-encoded private key"))
			}
			privBytes = decoded
		} else {
			privBytes = inputBytes(args[0])
		}

		d := new(big.Int).SetBytes(privBytes)
		state.privateKey = &ecdsa.PrivateKey{
			D: d,
			PublicKey: ecdsa.PublicKey{
				Curve: state.curve,
			},
		}
			// Compute public key from private key
			if state.ecdhCurve != nil {
				ecdhPriv, err := state.ecdhCurve.NewPrivateKey(d.Bytes())
				if err != nil {
					panic(errOperationFailed("ECDH: invalid private key: " + err.Error()))
				}
				pubRaw := ecdhPriv.PublicKey().Bytes()
				state.privateKey.PublicKey.X, state.privateKey.PublicKey.Y = unmarshalECDHPublicKey(pubRaw, state.curve)
			} else {
				state.privateKey.PublicKey.X, state.privateKey.PublicKey.Y = state.curve.ScalarBaseMult(d.Bytes()) //lint:ignore SA1019 // fallback for secp256k1
			}

		return obj
	}))

	return obj
}

// ---------------------------------------------------------------------------
// createECDH(curveName)
// ---------------------------------------------------------------------------

func createECDHJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("curveName"))
	}

	curveName := args[0].String()
	curve := ecdhCurveForName(curveName)
	if curve == nil {
		panic(errOsslUnsupported("unsupported ECDH curve: " + curveName))
	}

	state := &ecdhState{
		curve:     curve,
		ecdhCurve: ecdhCurveForNameECDH(curveName),
		curveName: curveName,
	}

	return newECDHObject(state)
}

// ---------------------------------------------------------------------------
// ECDH.convertKey(key, curve[, inEnc][, outEnc[, format]])
// ---------------------------------------------------------------------------

func ecdhConvertKeyJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 2 || args[0] == nil || args[1] == nil {
		panic(errInvalidArgType("key and curve"))
	}

	key := args[0]
	curveName := args[1].String()

	inEnc, _ := readEncoding(args, 2)
	outEnc, _ := readEncoding(args, 3)
	format := "uncompressed"
	if len(args) > 4 && args[4] != nil && args[4].TypeString() != "undefined" {
		format = args[4].String()
	}

	curve := ecdhCurveForName(curveName)
	if curve == nil {
		panic(errOsslUnsupported("unsupported ECDH curve: " + curveName))
	}

	var pubKeyBytes []byte
	if inEnc == "hex" {
		decoded, err := hex.DecodeString(key.String())
		if err != nil {
			panic(errInvalidArgType("a valid hex-encoded public key"))
		}
		pubKeyBytes = decoded
	} else {
		pubKeyBytes = inputBytes(key)
	}

	x, y := unmarshalECDHPublicKey(pubKeyBytes, curve)

	converted := marshalECDHPublicKey(x, y, curve, format)

	if outEnc == "hex" {
		return jsvalue.NewString(hex.EncodeToString(converted))
	}
	return buffer.NewBufferFromBytes(converted)
}

// ---------------------------------------------------------------------------
// getCurves()
// ---------------------------------------------------------------------------

func getCurvesJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	curves := []*jsvalue.JSValue{
		jsvalue.NewString("prime256v1"),
		jsvalue.NewString("secp256k1"),
		jsvalue.NewString("secp256r1"),
		jsvalue.NewString("secp384r1"),
		jsvalue.NewString("secp521r1"),
	}
	return jsvalue.NewArray(curves...)
}
