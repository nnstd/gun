package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

func randomBytesJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	n := 0
	if len(args) > 0 && args[0] != nil {
		n = int(args[0].Number())
	}
	if n < 0 {
		panic(errOutOfRange("The value of \"size\" is out of range. It must be >= 0"))
	}

	buf := randomBytesGo(n)

	// Async variant: randomBytes(size, callback)
	if len(args) > 1 && args[1] != nil && args[1].TypeString() == "function" {
		cb := args[1]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			return buffer.NewBufferFromBytes(buf), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return buffer.NewBufferFromBytes(buf)
}

func randomFillSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("a Buffer or TypedArray"))
	}
	buf := args[0]
	offset := 0
	size := 0
	if bufBytes := buf.Bytes(); bufBytes != nil {
		size = len(bufBytes)
	}

	if len(args) > 1 && args[1] != nil {
		offset = int(args[1].Number())
	}
	if len(args) > 2 && args[2] != nil {
		size = int(args[2].Number())
	}

	bufBytes := buf.Bytes()
	if bufBytes == nil {
		panic(errInvalidArgType("a Buffer or TypedArray"))
	}
	if offset < 0 || offset > len(bufBytes) || size < 0 || offset+size > len(bufBytes) {
		panic(errOutOfRange("offset or size out of range"))
	}

	randomBuf := randomBytesGo(size)
	copy(bufBytes[offset:], randomBuf)
	return buf
}

func randomFillJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("a Buffer or TypedArray"))
	}

	// Find callback — last argument if it's a function
	cbIdx := -1
	for i := len(args) - 1; i >= 1; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	buf := args[0]
	offset := 0
	size := 0
	if bufBytes := buf.Bytes(); bufBytes != nil {
		size = len(bufBytes)
	}

	argIdx := 1
	if argIdx < len(args) && args[argIdx] != nil && args[argIdx].TypeString() != "function" {
		offset = int(args[argIdx].Number())
		argIdx++
	}
	if argIdx < len(args) && args[argIdx] != nil && args[argIdx].TypeString() != "function" {
		size = int(args[argIdx].Number())
	}

	bufBytes := buf.Bytes()
	if bufBytes == nil {
		panic(errInvalidArgType("a Buffer or TypedArray"))
	}
	if offset < 0 || offset > len(bufBytes) || size < 0 || offset+size > len(bufBytes) {
		panic(errOutOfRange("offset or size out of range"))
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			randomBuf := randomBytesGo(size)
			copy(bufBytes[offset:], randomBuf)
			return buf, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	randomBuf := randomBytesGo(size)
	copy(bufBytes[offset:], randomBuf)
	return buf
}

func randomIntJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	min := int64(0)
	max := int64(0)
	cbIdx := -1

	// Parse arguments: randomInt([min, ]max[, callback])
	argIdx := 0
	if len(args) > argIdx && args[argIdx] != nil && args[argIdx].TypeString() != "undefined" {
		first := int64(args[argIdx].Number())
		argIdx++
		if len(args) > argIdx && args[argIdx] != nil && args[argIdx].TypeString() != "undefined" && args[argIdx].TypeString() != "function" {
			min = first
			max = int64(args[argIdx].Number())
			argIdx++
		} else {
			max = first
		}
	}
	if len(args) > argIdx && args[argIdx] != nil && args[argIdx].TypeString() == "function" {
		cbIdx = argIdx
	}

	if max <= min {
		panic(errOutOfRange("max must be greater than min"))
	}

	doRandomInt := func() *jsvalue.JSValue {
		rangeBig := big.NewInt(max - min)
		n, err := rand.Int(rand.Reader, rangeBig)
		if err != nil {
			panic(errOperationFailed(err.Error()))
		}
		return jsvalue.NewNumber(float64(min + n.Int64()))
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			return doRandomInt(), nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return doRandomInt()
}

func randomUUIDJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	uuid := randomBytesGo(16)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return jsvalue.NewString(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]))
}

func getRandomValuesJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		panic(errInvalidArgType("a TypedArray"))
	}
	arr := args[0]
	arrBytes := arr.Bytes()
	if arrBytes == nil {
		panic(errInvalidArgType("a TypedArray"))
	}
	randomBuf := randomBytesGo(len(arrBytes))
	copy(arrBytes, randomBuf)
	return arr
}
