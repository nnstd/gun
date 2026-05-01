package buffer

import (
	"encoding/base64"
	"encoding/hex"
	"math"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/web"
)

var Buffer *jsvalue.JSValue
var Blob *jsvalue.JSValue

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// newBufferFromBytes creates a new Buffer instance backed by a copy of data.
func newBufferFromBytes(data []byte) *jsvalue.JSValue {
	ab := jsvalue.NewArrayBuffer(len(data))
	if len(data) > 0 {
		copy(ab.Bytes(), data)
	}
	instance := jsvalue.NewObjectWithPrototype(Buffer.Get("prototype"))
	jsvalue.SetBufferBytes(instance, 0, len(data), ab, jsvalue.TypedArrayUint8, 1)
	return instance
}

// newBufferViewInto creates a Buffer view (shared backing) into an existing ArrayBuffer.
func newBufferViewInto(ab *jsvalue.JSValue, offset, length int) *jsvalue.JSValue {
	instance := jsvalue.NewObjectWithPrototype(Buffer.Get("prototype"))
	jsvalue.SetBufferBytes(instance, offset, length, ab, jsvalue.TypedArrayUint8, 1)
	return instance
}

// bufferBytes extracts the raw bytes from a Buffer or typed array value.
func bufferBytes(v *jsvalue.JSValue) []byte {
	return v.Bytes()
}

// normalizeEncoding returns a canonical encoding name.
func normalizeEncoding(enc string) string {
	switch strings.ToLower(enc) {
	case "utf8", "utf-8", "":
		return "utf8"
	case "ascii", "latin1", "binary":
		return "ascii"
	case "hex":
		return "hex"
	case "base64":
		return "base64"
	case "ucs2", "ucs-2", "utf16le", "utf-16le":
		return "utf16le"
	default:
		return "utf8"
	}
}

// encodeBytes encodes a string to bytes using the given encoding.
func encodeBytes(s string, enc string) []byte {
	switch normalizeEncoding(enc) {
	case "hex":
		data, err := hex.DecodeString(s)
		if err != nil {
			return nil
		}
		return data
	case "base64":
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil
		}
		return data
	default: // utf8, ascii, latin1, binary
		return []byte(s)
	}
}

// decodeBytes decodes bytes to a string using the given encoding.
func decodeBytes(data []byte, enc string) string {
	switch normalizeEncoding(enc) {
	case "hex":
		return hex.EncodeToString(data)
	case "base64":
		return base64.StdEncoding.EncodeToString(data)
	default: // utf8, ascii, latin1, binary
		return string(data)
	}
}

func init() {
	// ======================================================================
	// Buffer class — extends Uint8Array
	// ======================================================================
	Buffer = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// Internal constructor. Prefer Buffer.from() or Buffer.alloc().
		if len(args) > 0 && args[0] != nil && args[0].Type() == jsvalue.TypeNumber {
			n := int(args[0].Number())
			if n < 0 {
				n = 0
			}
			ab := jsvalue.NewArrayBuffer(n)
			jsvalue.SetBufferBytes(this, 0, n, ab, jsvalue.TypedArrayUint8, 1)
		}
		return nil
	}, jsvalue.TypedArrayCtors["Uint8Array"])

	proto := Buffer.Get("prototype")

	// ------------------------------------------------------------------
	// Instance methods on Buffer.prototype
	// ------------------------------------------------------------------

	// toString(encoding?, start?, end?)
	proto.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		this := args[0]
		enc := "utf8"
		if len(args) > 1 && args[1] != nil && args[1].Type() == jsvalue.TypeString {
			enc = args[1].String()
		}
		data := this.Bytes()
		if data == nil {
			return jsvalue.NewString("")
		}
		start := 0
		if len(args) > 2 && args[2] != nil {
			start = int(args[2].Number())
			if start < 0 {
				start = 0
			}
		}
		end := len(data)
		if len(args) > 3 && args[3] != nil {
			end = int(args[3].Number())
			if end > len(data) {
				end = len(data)
			}
		}
		if start > end {
			start = end
		}
		return jsvalue.NewString(decodeBytes(data[start:end], enc))
	}).MarkAsMethod())

	// slice(start?, end?) — returns a VIEW (not a copy), per Node.js semantics
	proto.Set("slice", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferFromBytes(nil)
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return newBufferFromBytes(nil)
		}
		length := bs.Length()
		start := 0
		if len(args) > 1 && args[1] != nil {
			start = int(args[1].Number())
			if start < 0 {
				start = length + start
			}
			if start < 0 {
				start = 0
			}
		}
		end := length
		if len(args) > 2 && args[2] != nil {
			end = int(args[2].Number())
			if end < 0 {
				end = length + end
			}
			if end < 0 {
				end = 0
			}
		}
		if start > end {
			start = end
		}
		if start > length {
			start = length
		}
		if end > length {
			end = length
		}
		ab := bs.ArrayBuffer()
		if ab == nil {
			ab = this
		}
		return newBufferViewInto(ab, bs.Offset()+start, end-start)
	}).MarkAsMethod())

	// subarray(start?, end?) — same as slice (view)
	proto.Set("subarray", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferFromBytes(nil)
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return newBufferFromBytes(nil)
		}
		count := bs.Length()
		begin := 0
		if len(args) > 1 && args[1] != nil {
			begin = int(args[1].Number())
			if begin < 0 {
				begin = count + begin
			}
			if begin < 0 {
				begin = 0
			}
		}
		end := count
		if len(args) > 2 && args[2] != nil {
			end = int(args[2].Number())
			if end < 0 {
				end = count + end
			}
			if end < 0 {
				end = 0
			}
		}
		if begin > end {
			begin = end
		}
		if begin > count {
			begin = count
		}
		if end > count {
			end = count
		}
		ab := bs.ArrayBuffer()
		if ab == nil {
			ab = this
		}
		return newBufferViewInto(ab, bs.Offset()+begin, end-begin)
	}).MarkAsMethod())

	// fill(value, start?, end?, encoding?)
	proto.Set("fill", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return this
		}
		length := bs.Length()
		value := args[1]
		start := 0
		if len(args) > 2 && args[2] != nil {
			start = int(args[2].Number())
			if start < 0 {
				start = length + start
			}
			if start < 0 {
				start = 0
			}
		}
		end := length
		if len(args) > 3 && args[3] != nil {
			end = int(args[3].Number())
			if end < 0 {
				end = length + end
			}
			if end < 0 {
				end = 0
			}
		}
		if start > end {
			start = end
		}
		if start > length {
			start = length
		}
		if end > length {
			end = length
		}

		data := bs.DataSlice()
		// Determine fill byte(s)
		if value != nil && value.Type() == jsvalue.TypeString {
			enc := "utf8"
			if len(args) > 4 && args[4] != nil {
				enc = args[4].String()
			}
			encoded := encodeBytes(value.String(), enc)
			if len(encoded) > 0 {
				for i := start; i < end; i++ {
					data[i] = encoded[(i-start)%len(encoded)]
				}
			}
		} else if value != nil && value.Type() == jsvalue.TypeNumber {
			b := byte(int(value.Number()) & 0xFF)
			for i := start; i < end; i++ {
				data[i] = b
			}
		} else if value != nil {
			// Check if it's a Buffer (fill with its contents)
			fillBytes := value.Bytes()
			if len(fillBytes) > 0 {
				for i := start; i < end; i++ {
					data[i] = fillBytes[(i-start)%len(fillBytes)]
				}
			}
		}
		return this
	}).MarkAsMethod())

	// copy(target, targetStart?, sourceStart?, sourceEnd?)
	proto.Set("copy", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		this := args[0]
		target := args[1]
		srcData := this.Bytes()
		if srcData == nil {
			return jsvalue.NewNumber(0)
		}
		tgtBS := target.ByteSliceData()
		if tgtBS == nil {
			return jsvalue.NewNumber(0)
		}
		targetStart := 0
		if len(args) > 2 && args[2] != nil {
			targetStart = int(args[2].Number())
			if targetStart < 0 {
				targetStart = 0
			}
		}
		sourceStart := 0
		if len(args) > 3 && args[3] != nil {
			sourceStart = int(args[3].Number())
			if sourceStart < 0 {
				sourceStart = 0
			}
		}
		sourceEnd := len(srcData)
		if len(args) > 4 && args[4] != nil {
			sourceEnd = int(args[4].Number())
			if sourceEnd < 0 {
				sourceEnd = 0
			}
		}
		if sourceStart > len(srcData) {
			sourceStart = len(srcData)
		}
		if sourceEnd > len(srcData) {
			sourceEnd = len(srcData)
		}
		if sourceStart > sourceEnd {
			sourceStart = sourceEnd
		}
		src := srcData[sourceStart:sourceEnd]
		tgtData := tgtBS.DataSlice()
		avail := len(tgtData) - targetStart
		if avail < 0 {
			avail = 0
		}
		if len(src) > avail {
			src = src[:avail]
		}
		copied := copy(tgtData[targetStart:], src)
		return jsvalue.NewNumber(float64(copied))
	}).MarkAsMethod())

	// write(string, offset?, length?, encoding?)
	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil || args[1] == nil {
			return jsvalue.NewNumber(0)
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return jsvalue.NewNumber(0)
		}
		str := args[1].String()
		enc := "utf8"
		offset := 0
		maxLen := len(str) // byte length upper bound

		if len(args) > 2 && args[2] != nil {
			offset = int(args[2].Number())
			if offset < 0 {
				offset = 0
			}
		}
		if len(args) > 3 && args[3] != nil {
			if args[3].Type() == jsvalue.TypeString {
				enc = args[3].String()
			} else {
				maxLen = int(args[3].Number())
			}
		}
		if len(args) > 4 && args[4] != nil {
			enc = args[4].String()
		}

		data := encodeBytes(str, enc)
		if data == nil {
			return jsvalue.NewNumber(0)
		}
		if maxLen < len(data) {
			data = data[:maxLen]
		}
		bufData := bs.DataSlice()
		avail := len(bufData) - offset
		if avail < 0 {
			avail = 0
		}
		if len(data) > avail {
			data = data[:avail]
		}
		copy(bufData[offset:], data)
		return jsvalue.NewNumber(float64(len(data)))
	}).MarkAsMethod())

	// equals(otherBuffer)
	proto.Set("equals", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		this := args[0]
		other := args[1]
		a := this.Bytes()
		b := other.Bytes()
		if len(a) != len(b) {
			return jsvalue.NewBool(false)
		}
		for i := range a {
			if a[i] != b[i] {
				return jsvalue.NewBool(false)
			}
		}
		return jsvalue.NewBool(true)
	}).MarkAsMethod())

	// compare(otherBuffer)
	proto.Set("compare", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		this := args[0]
		other := args[1]
		a := bufferBytes(this)
		b := bufferBytes(other)
		return jsvalue.NewNumber(float64(compareBytes(a, b)))
	}).MarkAsMethod())

	// indexOf(value, byteOffset?)
	proto.Set("indexOf", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(-1)
		}
		this := args[0]
		data := this.Bytes()
		if data == nil {
			return jsvalue.NewNumber(-1)
		}
		from := 0
		if len(args) > 2 && args[2] != nil {
			from = int(args[2].Number())
			if from < 0 {
				from = 0
			}
		}
		search := args[1]
		// Search by byte value
		if search != nil && search.Type() == jsvalue.TypeNumber {
			b := byte(int(search.Number()) & 0xFF)
			for i := from; i < len(data); i++ {
				if data[i] == b {
					return jsvalue.NewNumber(float64(i))
				}
			}
			return jsvalue.NewNumber(-1)
		}
		// Search by string or Buffer
		var pattern []byte
		if search != nil && search.Type() == jsvalue.TypeString {
			pattern = []byte(search.String())
		} else if search != nil {
			pattern = search.Bytes()
		}
		if pattern == nil {
			return jsvalue.NewNumber(-1)
		}
		if len(pattern) == 0 {
			return jsvalue.NewNumber(float64(from))
		}
		for i := from; i <= len(data)-len(pattern); i++ {
			if bytesEqual(data[i:i+len(pattern)], pattern) {
				return jsvalue.NewNumber(float64(i))
			}
		}
		return jsvalue.NewNumber(-1)
	}).MarkAsMethod())

	// includes(value, byteOffset?)
	proto.Set("includes", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		idxFn := proto.Get("indexOf")
		var idx *jsvalue.JSValue
		if len(args) > 2 && args[2] != nil {
			idx = idxFn.Call(args[0], args[1], args[2])
		} else {
			idx = idxFn.Call(args[0], args[1])
		}
		return jsvalue.NewBool(idx.Number() != -1)
	}).MarkAsMethod())

	// toJSON()
	proto.Set("toJSON", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.ObjectFrom("type", jsvalue.NewString("Buffer"), "data", jsvalue.NewArray())
		}
		this := args[0]
		data := this.Bytes()
		if data == nil {
			return jsvalue.ObjectFrom("type", jsvalue.NewString("Buffer"), "data", jsvalue.NewArray())
		}
		elems := make([]*jsvalue.JSValue, len(data))
		for i, b := range data {
			elems[i] = jsvalue.NewNumber(float64(b))
		}
		return jsvalue.ObjectFrom(
			"type", jsvalue.NewString("Buffer"),
			"data", jsvalue.NewArray(elems...),
		)
	}).MarkAsMethod())

	// entries()
	proto.Set("entries", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewArray()
		}
		data := args[0].Bytes()
		if data == nil {
			return jsvalue.NewArray()
		}
		result := make([]*jsvalue.JSValue, len(data))
		for i, b := range data {
			result[i] = jsvalue.NewArray(jsvalue.NewNumber(float64(i)), jsvalue.NewNumber(float64(b)))
		}
		return jsvalue.NewArray(result...)
	}).MarkAsMethod())

	// keys()
	proto.Set("keys", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewArray()
		}
		data := args[0].Bytes()
		if data == nil {
			return jsvalue.NewArray()
		}
		result := make([]*jsvalue.JSValue, len(data))
		for i := range data {
			result[i] = jsvalue.NewNumber(float64(i))
		}
		return jsvalue.NewArray(result...)
	}).MarkAsMethod())

	// values()
	proto.Set("values", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewArray()
		}
		data := args[0].Bytes()
		if data == nil {
			return jsvalue.NewArray()
		}
		result := make([]*jsvalue.JSValue, len(data))
		for i, b := range data {
			result[i] = jsvalue.NewNumber(float64(b))
		}
		return jsvalue.NewArray(result...)
	}).MarkAsMethod())

	// readUInt8(offset)
	proto.Set("readUInt8", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		data := args[0].Bytes()
		if data == nil {
			return jsvalue.NewNumber(0)
		}
		offset := int(args[1].Number())
		if offset < 0 || offset >= len(data) {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(float64(data[offset]))
	}).MarkAsMethod())

	// writeUInt8(value, offset)
	proto.Set("writeUInt8", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 3 || args[0] == nil {
			return args[0]
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return this
		}
		value := int(args[1].Number()) & 0xFF
		offset := int(args[2].Number())
		data := bs.DataSlice()
		if offset >= 0 && offset < len(data) {
			data[offset] = byte(value)
		}
		return this
	}).MarkAsMethod())

	// readUInt16LE(offset)
	proto.Set("readUInt16LE", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		data := args[0].Bytes()
		if data == nil || len(data) < 2 {
			return jsvalue.NewNumber(0)
		}
		offset := int(args[1].Number())
		if offset < 0 || offset+2 > len(data) {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(float64(uint16(data[offset]) | uint16(data[offset+1])<<8))
	}).MarkAsMethod())

	// readUInt32LE(offset)
	proto.Set("readUInt32LE", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		data := args[0].Bytes()
		if data == nil || len(data) < 4 {
			return jsvalue.NewNumber(0)
		}
		offset := int(args[1].Number())
		if offset < 0 || offset+4 > len(data) {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(float64(uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24))
	}).MarkAsMethod())

	// ------------------------------------------------------------------
	// Static methods on Buffer
	// ------------------------------------------------------------------

	// Buffer.from(string, encoding?)
	// Buffer.from(array)
	// Buffer.from(buffer)
	// Buffer.from(arrayBuffer, byteOffset?, length?)
	Buffer.Set("from", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferFromBytes(nil)
		}
		first := args[0]

		// ArrayBuffer → view
		if first.Type() == jsvalue.TypeObject {
			fbs := first.ByteSliceData()
			if fbs != nil && fbs.Kind() == jsvalue.TypedArrayNone {
				byteOffset := 0
				if len(args) > 1 && args[1] != nil {
					byteOffset = int(args[1].Number())
					if byteOffset < 0 {
						byteOffset = 0
					}
				}
				flen := fbs.Length()
				byteLen := flen - byteOffset
				if len(args) > 2 && args[2] != nil {
					byteLen = int(args[2].Number())
				}
				if byteOffset+byteLen > flen {
					byteLen = flen - byteOffset
				}
				return newBufferViewInto(first, byteOffset, byteLen)
			}

			// Buffer / TypedArray → copy
			if fbs != nil && fbs.Kind() != jsvalue.TypedArrayNone {
				return newBufferFromBytes(first.Bytes())
			}
		}

		// String → encode
		if first.Type() == jsvalue.TypeString {
			enc := "utf8"
			if len(args) > 1 && args[1] != nil && args[1].Type() == jsvalue.TypeString {
				enc = args[1].String()
			}
			data := encodeBytes(first.String(), enc)
			if data == nil {
				return newBufferFromBytes(nil)
			}
			return newBufferFromBytes(data)
		}

		// Array of numbers
		if first.IsArray() {
			elems := first.Array()
			data := make([]byte, len(elems))
			for i, e := range elems {
				if e != nil {
					data[i] = byte(int(e.Number()) & 0xFF)
				}
			}
			return newBufferFromBytes(data)
		}

		return newBufferFromBytes(nil)
	}))

	// Buffer.alloc(size, fill?, encoding?)
	Buffer.Set("alloc", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		size := 0
		if len(args) > 0 && args[0] != nil {
			size = int(args[0].Number())
			if size < 0 {
				size = 0
			}
		}
		buf := newBufferFromBytes(make([]byte, size))
		// Apply fill if specified
		if len(args) > 1 && args[1] != nil && size > 0 {
			fill := args[1]
			enc := "utf8"
			if len(args) > 2 && args[2] != nil {
				enc = args[2].String()
			}
			bs := buf.ByteSliceData()
			data := bs.DataSlice()
			if fill.Type() == jsvalue.TypeString {
				encoded := encodeBytes(fill.String(), enc)
				if len(encoded) > 0 {
					for i := 0; i < size; i++ {
						data[i] = encoded[i%len(encoded)]
					}
				}
			} else if fill.Type() == jsvalue.TypeNumber {
				b := byte(int(fill.Number()) & 0xFF)
				for i := 0; i < size; i++ {
					data[i] = b
				}
			} else {
				fillBytes := fill.Bytes()
				if len(fillBytes) > 0 {
					for i := 0; i < size; i++ {
						data[i] = fillBytes[i%len(fillBytes)]
					}
				}
			}
		}
		return buf
	}))

	// Buffer.allocUnsafe(size)
	Buffer.Set("allocUnsafe", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		size := 0
		if len(args) > 0 && args[0] != nil {
			size = int(args[0].Number())
			if size < 0 {
				size = 0
			}
		}
		return newBufferFromBytes(make([]byte, size))
	}))

	// Buffer.allocUnsafeSlow(size)
	Buffer.Set("allocUnsafeSlow", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		size := 0
		if len(args) > 0 && args[0] != nil {
			size = int(args[0].Number())
			if size < 0 {
				size = 0
			}
		}
		return newBufferFromBytes(make([]byte, size))
	}))

	// Buffer.concat(list, totalLength?)
	Buffer.Set("concat", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil || !args[0].IsArray() {
			return newBufferFromBytes(nil)
		}
		list := args[0].Array()

		// Calculate total length
		totalLen := 0
		if len(args) > 1 && args[1] != nil {
			totalLen = int(args[1].Number())
		} else {
			for _, item := range list {
				bb := bufferBytes(item)
				if bb != nil {
					totalLen += len(bb)
				}
			}
		}

		result := make([]byte, 0, totalLen)
		for _, item := range list {
			bb := bufferBytes(item)
			if bb != nil {
				result = append(result, bb...)
			}
		}
		// Trim to totalLength if specified
		if len(args) > 1 && args[1] != nil && len(result) > totalLen {
			result = result[:totalLen]
		}
		return newBufferFromBytes(result)
	}))

	// Buffer.isBuffer(val)
	Buffer.Set("isBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewBool(false)
		}
		return jsvalue.InstanceOf(args[0], Buffer)
	}))

	// Buffer.byteLength(string, encoding?)
	Buffer.Set("byteLength", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		enc := "utf8"
		if len(args) > 1 && args[1] != nil {
			enc = args[1].String()
		}
		data := encodeBytes(args[0].String(), enc)
		if data == nil {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(float64(len(data)))
	}))

	// Buffer.isEncoding(encoding)
	Buffer.Set("isEncoding", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewBool(false)
		}
		switch strings.ToLower(args[0].String()) {
		case "utf8", "utf-8", "ascii", "latin1", "binary", "hex", "base64", "ucs2", "ucs-2", "utf16le", "utf-16le":
			return jsvalue.NewBool(true)
		default:
			return jsvalue.NewBool(false)
		}
	}))

	// Buffer.compare(buf1, buf2)
	Buffer.Set("compare", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 2 {
			return jsvalue.NewNumber(0)
		}
		a := bufferBytes(args[0])
		b := bufferBytes(args[1])
		return jsvalue.NewNumber(float64(compareBytes(a, b)))
	}))

	// ======================================================================
	// Blob class
	// ======================================================================
	Blob = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		parts := jsvalue.NewArray()
		if len(args) > 0 && args[0] != nil {
			parts = args[0]
		}
		this.Set("parts", parts)
		this.Set("size", jsvalue.NewNumber(float64(parts.Len())))
		this.Set("type", jsvalue.NewString(""))
		return nil
	}, nil)

	// ======================================================================
	// Module exports
	// ======================================================================
	constants := jsvalue.ObjectFrom(
		"MAX_LENGTH", jsvalue.NewNumber(float64(math.MaxUint32)),
		"MAX_STRING_LENGTH", jsvalue.NewNumber(float64(math.MaxInt32)),
	)

	AsJSValue = jsvalue.ObjectFrom(
		"Buffer", Buffer,
		"Blob", Blob,
		"File", web.File,
		"constants", constants,
		"kMaxLength", jsvalue.NewNumber(float64(math.MaxUint32)),
		"kStringMaxLength", jsvalue.NewNumber(float64(math.MaxInt32)),
		"atob", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewString("")
			}
			data, err := base64.StdEncoding.DecodeString(args[0].String())
			if err != nil {
				return jsvalue.NewString("")
			}
			return jsvalue.NewString(string(data))
		}),
		"btoa", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewString("")
			}
			return jsvalue.NewString(base64.StdEncoding.EncodeToString([]byte(args[0].String())))
		}),
	)
}

var AsJSValue *jsvalue.JSValue

// bytesEqual compares two byte slices for equality.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// compareBytes compares two byte slices lexicographically.
func compareBytes(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
