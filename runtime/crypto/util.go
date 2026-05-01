package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
	"github.com/nnstd/gun/runtime/eventloop"
	"github.com/nnstd/gun/runtime/promise"
)

// inputBytes extracts raw bytes from a string or Buffer/Uint8Array JSValue.
func inputBytes(v *jsvalue.JSValue) []byte {
	if v == nil || v.TypeString() == "undefined" || v.TypeString() == "null" {
		return nil
	}
	if b := v.Bytes(); b != nil {
		return b
	}
	return []byte(v.String())
}

// encodeOutput returns a Buffer when encoding is empty, or an encoded string.
func encodeOutput(data []byte, encoding string) *jsvalue.JSValue {
	switch encoding {
	case "hex":
		return jsvalue.NewString(hex.EncodeToString(data))
	case "base64":
		return jsvalue.NewString(base64.StdEncoding.EncodeToString(data))
	case "base64url":
		return jsvalue.NewString(base64.RawURLEncoding.EncodeToString(data))
	case "binary", "latin1":
		return jsvalue.NewString(string(data))
	default:
		return buffer.NewBufferFromBytes(data)
	}
}

// readEncoding extracts the encoding string from an optional argument.
// Returns ("", false) when no encoding specified.
func readEncoding(args []*jsvalue.JSValue, idx int) (string, bool) {
	if idx >= len(args) || args[idx] == nil || args[idx].TypeString() == "undefined" {
		return "", false
	}
	return args[idx].String(), true
}

// resolvePromise creates a resolved Promise.
func resolvePromise(result *jsvalue.JSValue) *jsvalue.JSValue {
	return promise.Promise.Get("resolve").Call(result)
}

// asyncCrypto runs fn in a goroutine with eventloop integration.
// Calls callback(err, null) on failure or callback(null, result) on success.
func asyncCrypto(fn func() (*jsvalue.JSValue, error), cb *jsvalue.JSValue) {
	eventloop.Default.RegisterHandle()
	go func() {
		defer eventloop.Default.UnregisterHandle()
		result, err := fn()
		eventloop.Default.ScheduleCallback(func() {
			if err != nil {
				errVal := cryptoError("ERR_OPERATION_FAILED", err.Error())
				cb.Call(errVal, jsvalue.NewNull())
			} else {
				cb.Call(jsvalue.NewNull(), result)
			}
		})
	}()
}

// randomBytesGo generates cryptographically random bytes.
func randomBytesGo(n int) []byte {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return buf
}
