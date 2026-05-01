package jsvalue

import (
	"encoding/binary"
	"math"
)

// DataViewCtor is the DataView constructor.
var DataViewCtor *JSValue

// dataviewGetBytes reads size bytes from the DataView at the given offset.
func dataviewGetBytes(v *JSValue, offset int, size int) []byte {
	dv := v.DataViewState()
	bs := dv.buffer.ByteSliceData()
	absOff := dv.byteOffset + offset
	if offset < 0 || absOff+size > dv.byteOffset+dv.byteLength {
		panic(newRangeErrorJSValue("Offset is outside the bounds of the DataView"))
	}
	return bs.data[absOff : absOff+size]
}

// dataviewSetBytes writes b into the DataView at the given offset.
func dataviewSetBytes(v *JSValue, offset int, b []byte) {
	dv := v.DataViewState()
	bs := dv.buffer.ByteSliceData()
	absOff := dv.byteOffset + offset
	if offset < 0 || absOff+len(b) > dv.byteOffset+dv.byteLength {
		panic(newRangeErrorJSValue("Offset is outside the bounds of the DataView"))
	}
	copy(bs.data[absOff:], b)
}

// dataviewIsLittleEndian returns true if the littleEndian argument is true.
// Defaults to false (big-endian).
func dataviewIsLittleEndian(args []*JSValue, leIndex int) bool {
	if len(args) > leIndex && args[leIndex] != nil {
		return args[leIndex].Bool()
	}
	return false
}

// initDataView sets up the DataView constructor and prototype.
func initDataView() {
	proto := NewObjectWithPrototype(ObjectPrototype)

	// --- Getter properties ---

	defGetter(proto, "buffer", func(this *JSValue) *JSValue {
		dv := this.DataViewState()
		if dv == nil { return NewUndefined() }
		return dv.buffer
	})

	defGetter(proto, "byteLength", func(this *JSValue) *JSValue {
		dv := this.DataViewState()
		if dv == nil { return NewNumber(0) }
		return NewNumber(float64(dv.byteLength))
	})

	defGetter(proto, "byteOffset", func(this *JSValue) *JSValue {
		dv := this.DataViewState()
		if dv == nil { return NewNumber(0) }
		return NewNumber(float64(dv.byteOffset))
	})

	// --- Get methods ---

	defMethod(proto, "getInt8", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 1)
		return NewNumber(float64(int8(b[0])))
	})

	defMethod(proto, "getUint8", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 1)
		return NewNumber(float64(b[0]))
	})

	defMethod(proto, "getInt16", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 2)
		var val int16
		if dataviewIsLittleEndian(args, 2) {
			val = int16(binary.LittleEndian.Uint16(b))
		} else {
			val = int16(binary.BigEndian.Uint16(b))
		}
		return NewNumber(float64(val))
	})

	defMethod(proto, "getUint16", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 2)
		var val uint16
		if dataviewIsLittleEndian(args, 2) {
			val = binary.LittleEndian.Uint16(b)
		} else {
			val = binary.BigEndian.Uint16(b)
		}
		return NewNumber(float64(val))
	})

	defMethod(proto, "getInt32", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 4)
		var val int32
		if dataviewIsLittleEndian(args, 2) {
			val = int32(binary.LittleEndian.Uint32(b))
		} else {
			val = int32(binary.BigEndian.Uint32(b))
		}
		return NewNumber(float64(val))
	})

	defMethod(proto, "getUint32", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 4)
		var val uint32
		if dataviewIsLittleEndian(args, 2) {
			val = binary.LittleEndian.Uint32(b)
		} else {
			val = binary.BigEndian.Uint32(b)
		}
		return NewNumber(float64(val))
	})

	defMethod(proto, "getFloat32", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 4)
		var val float32
		if dataviewIsLittleEndian(args, 2) {
			val = math.Float32frombits(binary.LittleEndian.Uint32(b))
		} else {
			val = math.Float32frombits(binary.BigEndian.Uint32(b))
		}
		return NewNumber(float64(val))
	})

	defMethod(proto, "getFloat64", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 8)
		var val float64
		if dataviewIsLittleEndian(args, 2) {
			val = math.Float64frombits(binary.LittleEndian.Uint64(b))
		} else {
			val = math.Float64frombits(binary.BigEndian.Uint64(b))
		}
		return NewNumber(val)
	})

	defMethod(proto, "getBigInt64", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewBigInt(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 8)
		var val int64
		if dataviewIsLittleEndian(args, 2) {
			val = int64(binary.LittleEndian.Uint64(b))
		} else {
			val = int64(binary.BigEndian.Uint64(b))
		}
		return NewBigInt(val)
	})

	defMethod(proto, "getBigUint64", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewBigUint(0)
		}
		this := args[0]
		offset := int(args[1].Number())
		b := dataviewGetBytes(this, offset, 8)
		var val uint64
		if dataviewIsLittleEndian(args, 2) {
			val = binary.LittleEndian.Uint64(b)
		} else {
			val = binary.BigEndian.Uint64(b)
		}
		return NewBigUint(val)
	})

	// --- Set methods ---

	defMethod(proto, "setInt8", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := int8(args[2].Number())
		dataviewSetBytes(this, offset, []byte{byte(val)})
		return NewUndefined()
	})

	defMethod(proto, "setUint8", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := byte(args[2].Number())
		dataviewSetBytes(this, offset, []byte{val})
		return NewUndefined()
	})

	defMethod(proto, "setInt16", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := int16(args[2].Number())
		b := make([]byte, 2)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint16(b, uint16(val))
		} else {
			binary.BigEndian.PutUint16(b, uint16(val))
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setUint16", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := uint16(args[2].Number())
		b := make([]byte, 2)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint16(b, val)
		} else {
			binary.BigEndian.PutUint16(b, val)
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setInt32", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := int32(args[2].Number())
		b := make([]byte, 4)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint32(b, uint32(val))
		} else {
			binary.BigEndian.PutUint32(b, uint32(val))
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setUint32", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := uint32(args[2].Number())
		b := make([]byte, 4)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint32(b, val)
		} else {
			binary.BigEndian.PutUint32(b, val)
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setFloat32", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := float32(args[2].Number())
		b := make([]byte, 4)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint32(b, math.Float32bits(val))
		} else {
			binary.BigEndian.PutUint32(b, math.Float32bits(val))
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setFloat64", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := args[2].Number()
		b := make([]byte, 8)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint64(b, math.Float64bits(val))
		} else {
			binary.BigEndian.PutUint64(b, math.Float64bits(val))
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setBigInt64", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := args[2].BigInt()
		b := make([]byte, 8)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint64(b, uint64(val))
		} else {
			binary.BigEndian.PutUint64(b, uint64(val))
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	defMethod(proto, "setBigUint64", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		offset := int(args[1].Number())
		val := args[2].BigUint()
		b := make([]byte, 8)
		if dataviewIsLittleEndian(args, 3) {
			binary.LittleEndian.PutUint64(b, val)
		} else {
			binary.BigEndian.PutUint64(b, val)
		}
		dataviewSetBytes(this, offset, b)
		return NewUndefined()
	})

	// --- Constructor ---

	DataViewCtor = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(newTypeErrorJSValue("DataView: buffer argument is required"))
		}
		buffer := args[0]
		if buffer.ByteSliceData() == nil {
			panic(newTypeErrorJSValue("DataView: first argument must be an ArrayBuffer or SharedArrayBuffer"))
		}

		bs := buffer.ByteSliceData()
		byteOffset := 0
		if len(args) > 1 && args[1] != nil {
			byteOffset = int(args[1].Number())
			if byteOffset < 0 {
				byteOffset = 0
			}
		}

		byteLength := bs.length - byteOffset
		if len(args) > 2 && args[2] != nil {
			byteLength = int(args[2].Number())
		}

		if byteOffset+byteLength > bs.length {
			panic(newTypeErrorJSValue("DataView: byteOffset + byteLength exceeds buffer size"))
		}

	dv := NewObjectWithPrototype(DataViewCtor.Get("prototype"))
		dv.SetDataViewState(&dataViewState{buffer: buffer, byteOffset: byteOffset, byteLength: byteLength})
		return dv
	})
	DataViewCtor.Set("prototype", proto)
	proto.Set("constructor", DataViewCtor)

	RegisterGlobal("DataView", DataViewCtor)
}
