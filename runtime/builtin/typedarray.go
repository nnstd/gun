package jsvalue

import (
	"encoding/base64"
	"encoding/hex"
	"math"
	"strings"
)

// ArrayBufferCtor is the ArrayBuffer constructor.
var ArrayBufferCtor *JSValue

// SharedArrayBufferCtor is the SharedArrayBuffer constructor.
var SharedArrayBufferCtor *JSValue

// TypedArrayCtors maps typed array names to their constructors.
var TypedArrayCtors map[string]*JSValue

// typedArrayDesc describes a TypedArray variant.
type typedArrayDesc struct {
	name string
	kind typedArrayKind
	bpe  int // bytes per element
}

var typedArrayDescs = []typedArrayDesc{
	{"Int8Array", taInt8, 1},
	{"Uint8Array", taUint8, 1},
	{"Uint8ClampedArray", taUint8Clamped, 1},
	{"Int16Array", taInt16, 2},
	{"Uint16Array", taUint16, 2},
	{"Int32Array", taInt32, 4},
	{"Uint32Array", taUint32, 4},
	{"Float32Array", taFloat32, 4},
	{"Float64Array", taFloat64, 8},
	{"BigInt64Array", taBigInt64, 8},
	{"BigUint64Array", taBigUint64, 8},
}

// newArrayBuffer creates a new ArrayBuffer with the given byte length.
func newArrayBuffer(byteLength int) *JSValue {
	proto := ArrayBufferCtor.Get("prototype")
	ab := NewObjectWithPrototype(proto)
	data := make([]byte, byteLength)
	ab.SetByteSlice(&byteSlice{
		data:    data,
		offset:  0,
		length:  byteLength,
		kind:    taNone,
		detached: false,
	})
	return ab
}

// newSharedArrayBuffer creates a new SharedArrayBuffer with the given byte length.
func newSharedArrayBuffer(byteLength int) *JSValue {
	proto := SharedArrayBufferCtor.Get("prototype")
	ab := NewObjectWithPrototype(proto)
	data := make([]byte, byteLength)
	bs := &byteSlice{
		data:     data,
		offset:   0,
		length:   byteLength,
		kind:     taNone,
		detached: false,
		isShared: true,
	}
	ab.SetByteSlice(bs)
	ab.Set("byteLength", NewNumber(float64(byteLength)))
	return ab
}

// typedArrayDescForKind returns the descriptor for the given kind, or nil.
func typedArrayDescForKind(kind typedArrayKind) *typedArrayDesc {
	for i := range typedArrayDescs {
		if typedArrayDescs[i].kind == kind {
			return &typedArrayDescs[i]
		}
	}
	return nil
}

// newTypedArray is the factory for creating typed array instances.
func newTypedArray(desc typedArrayDesc, args ...*JSValue) *JSValue {
	if len(args) == 0 || args[0] == nil {
		return newTypedArrayFromLength(desc, 0)
	}

	first := args[0]

	// 1. Number → length overload
	if first.Type() == TypeNumber {
		n := int(first.Number())
		if n < 0 {
			panic(newRangeErrorJSValue("Invalid typed array length: " + first.String()))
		}
		return newTypedArrayFromLength(desc, n)
	}

	// 2. Has byteSlice → ArrayBuffer/SharedArrayBuffer overload
	if first.ByteSliceData() != nil {
		bs := first.ByteSliceData()
		byteOffset := 0
		if len(args) > 1 && args[1] != nil && args[1].Type() == TypeNumber {
			byteOffset = int(args[1].Number())
			if byteOffset < 0 {
				byteOffset = 0
			}
		}
		byteLength := bs.length - byteOffset
		if len(args) > 2 && args[2] != nil && args[2].Type() == TypeNumber {
			byteLength = int(args[2].Number())
		}
		// Adjust byteLength to be element-aligned
		elemCount := byteLength / desc.bpe
		byteLength = elemCount * desc.bpe
		return newTypedArrayView(desc, first, byteOffset, byteLength)
	}

	// 3. Array → from array
	if first.IsArray() {
		return newTypedArrayFromArray(desc, first.Array())
	}

	// 4. Object with EnumerableOwnKeys → from iterable
	keys := first.EnumerableOwnKeys()
	if len(keys) > 0 {
		elems := make([]*JSValue, 0, len(keys))
		for _, k := range keys {
			elems = append(elems, first.Get(k))
		}
		return newTypedArrayFromArray(desc, elems)
	}

	return newTypedArrayFromLength(desc, 0)
}

// newTypedArrayFromLength creates a typed array by allocating a new ArrayBuffer of n*desc.bpe bytes.
func newTypedArrayFromLength(desc typedArrayDesc, count int) *JSValue {
	byteLength := count * desc.bpe
	ab := newArrayBuffer(byteLength)
	return newTypedArrayView(desc, ab, 0, byteLength)
}

// newTypedArrayFromArray creates a typed array from a slice of JSValues.
func newTypedArrayFromArray(desc typedArrayDesc, elems []*JSValue) *JSValue {
	byteLength := len(elems) * desc.bpe
	ab := newArrayBuffer(byteLength)
	view := newTypedArrayView(desc, ab, 0, byteLength)
	bs := view.ByteSliceData()
	for i, elem := range elems {
		bs.setElement(i, elem)
	}
	return view
}

// newTypedArrayView creates a typed array view over an existing ArrayBuffer.
func newTypedArrayView(desc typedArrayDesc, ab *JSValue, byteOffset, byteLength int) *JSValue {
	ctor := TypedArrayCtors[desc.name]
	var proto *JSValue
	if ctor != nil {
		proto = ctor.Get("prototype")
	} else {
		proto = ObjectPrototype
	}

	view := NewObjectWithPrototype(proto)
	abBS := ab.ByteSliceData()
	if abBS == nil {
		abBS = &byteSlice{}
	}

	view.SetByteSlice(&byteSlice{
		data:            abBS.data,
		offset:          byteOffset,
		length:          byteLength,
		arrayBuffer:     ab,
		detached:        false,
		bytesPerElement: desc.bpe,
		kind:            desc.kind,
		isShared:        abBS.isShared,
	})

	return view
}

// initTypedArrays sets up ArrayBuffer, SharedArrayBuffer, and all TypedArray constructors.
func initTypedArrays() {
	TypedArrayCtors = make(map[string]*JSValue, len(typedArrayDescs))

	initArrayBuffer()
	initSharedArrayBuffer()

	for _, desc := range typedArrayDescs {
		initTypedArrayCtor(desc)
	}

	// Replace the existing Uint8ArrayCtor stub
	Uint8ArrayCtor = TypedArrayCtors["Uint8Array"]
	// Update the global registry
	RegisterGlobal("Uint8Array", Uint8ArrayCtor)
}

func initArrayBuffer() {
	arrayBufferProto := NewObjectWithPrototype(ObjectPrototype)

	// Prototype methods
	defMethod(arrayBufferProto, "slice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return NewUndefined()
		}
		begin := 0
		if len(args) > 1 && args[1] != nil {
			begin = int(args[1].Number())
			if begin < 0 {
				begin = bs.length + begin
			}
			if begin < 0 {
				begin = 0
			}
		}
		end := bs.length
		if len(args) > 2 && args[2] != nil {
			end = int(args[2].Number())
			if end < 0 {
				end = bs.length + end
			}
			if end < 0 {
				end = 0
			}
		}
		if begin > end {
			begin = end
		}
		if begin > bs.length {
			begin = bs.length
		}
		if end > bs.length {
			end = bs.length
		}
		newLen := end - begin
		newAB := newArrayBuffer(newLen)
		src := bs.data[bs.offset+begin : bs.offset+end]
		dst := newAB.ByteSliceData().data
		copy(dst, src)
		return newAB
	})

	defGetter(arrayBufferProto, "resizable", func(this *JSValue) *JSValue {
		return NewBool(false)
	})

	defGetter(arrayBufferProto, "detached", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewBool(false)
		}
		return NewBool(bs.detached)
	})

	defGetter(arrayBufferProto, "maxByteLength", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(bs.length))
	})

	ArrayBufferCtor = NewFunction(func(args ...*JSValue) *JSValue {
		byteLength := 0
		if len(args) > 0 && args[0] != nil {
			n := args[0].Number()
			if math.IsNaN(n) {
				n = 0
			}
			if n < 0 || math.IsInf(n, 0) {
				panic(newRangeErrorJSValue("Invalid array buffer length"))
			}
			byteLength = int(n)
			if byteLength < 0 {
				panic(newRangeErrorJSValue("Invalid array buffer length"))
			}
		}
		return newArrayBuffer(byteLength)
	})
	ArrayBufferCtor.Set("prototype", arrayBufferProto)

	// Static methods
	ArrayBufferCtor.Set("isView", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil {
			return NewBool(false)
		}
		val := args[0]
		bs := val.ByteSliceData()
		if bs != nil && bs.kind != taNone {
			return NewBool(true)
		}
		// DataView instances don't have a byteSlice, check prototype chain
		if DataViewCtor != nil {
			dvProto := DataViewCtor.Get("prototype")
			if dvProto != nil {
				for cur := val; cur != nil; cur = cur.prototype {
					if cur == dvProto {
						return NewBool(true)
					}
				}
			}
		}
		return NewBool(false)
	}))

	RegisterGlobal("ArrayBuffer", ArrayBufferCtor)
}

func initSharedArrayBuffer() {
	sharedArrayBufferProto := NewObjectWithPrototype(ObjectPrototype)

	// Prototype methods
	defMethod(sharedArrayBufferProto, "slice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return NewUndefined()
		}
		begin := 0
		if len(args) > 1 && args[1] != nil {
			begin = int(args[1].Number())
			if begin < 0 {
				begin = bs.length + begin
			}
			if begin < 0 {
				begin = 0
			}
		}
		end := bs.length
		if len(args) > 2 && args[2] != nil {
			end = int(args[2].Number())
			if end < 0 {
				end = bs.length + end
			}
			if end < 0 {
				end = 0
			}
		}
		if begin > end {
			begin = end
		}
		if begin > bs.length {
			begin = bs.length
		}
		if end > bs.length {
			end = bs.length
		}
		newLen := end - begin
		newAB := newSharedArrayBuffer(newLen)
		src := bs.data[bs.offset+begin : bs.offset+end]
		dst := newAB.ByteSliceData().data
		copy(dst, src)
		return newAB
	})

	defGetter(sharedArrayBufferProto, "growable", func(this *JSValue) *JSValue {
		return NewBool(false)
	})

	defGetter(sharedArrayBufferProto, "detached", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewBool(false)
		}
		return NewBool(bs.detached)
	})

	defGetter(sharedArrayBufferProto, "maxByteLength", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(bs.length))
	})

	defMethod(sharedArrayBufferProto, "grow", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return NewUndefined()
		}
		newLength := bs.length
		if len(args) > 1 && args[1] != nil {
			newLength = int(args[1].Number())
		}
		if newLength <= bs.length {
			// Cannot shrink
			return NewUndefined()
		}
		delta := newLength - len(bs.data)
		if delta > 0 {
			bs.data = append(bs.data, make([]byte, delta)...)
		}
		bs.length = newLength
		this.Set("byteLength", NewNumber(float64(newLength)))
		return NewUndefined()
	})

	SharedArrayBufferCtor = NewFunction(func(args ...*JSValue) *JSValue {
		byteLength := 0
		if len(args) > 0 && args[0] != nil {
			byteLength = int(args[0].Number())
			if byteLength < 0 {
				byteLength = 0
			}
		}
		return newSharedArrayBuffer(byteLength)
	})
	SharedArrayBufferCtor.Set("prototype", sharedArrayBufferProto)

	RegisterGlobal("SharedArrayBuffer", SharedArrayBufferCtor)
}

func initTypedArrayCtor(desc typedArrayDesc) {
	proto := NewObjectWithPrototype(ObjectPrototype)

	// Constant property
	proto.Set("BYTES_PER_ELEMENT", NewNumber(float64(desc.bpe)))

	// Add prototype methods
	setTypedArrayProtoMethods(proto, desc)

	// Create constructor
	ctor := NewFunction(func(args ...*JSValue) *JSValue {
		return newTypedArray(desc, args...)
	})
	ctor.Set("prototype", proto)
	proto.Set("constructor", ctor)

	// Static properties
	ctor.Set("BYTES_PER_ELEMENT", NewNumber(float64(desc.bpe)))

	// Static methods
	ctor.Set("from", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil {
			return newTypedArrayFromLength(desc, 0)
		}
		source := args[0]
		var mapFn *JSValue
		if len(args) > 1 && args[1] != nil {
			mapFn = args[1]
		}

		var elems []*JSValue
		if source.IsArray() {
			elems = source.Array()
		} else {
			keys := source.EnumerableOwnKeys()
			elems = make([]*JSValue, 0, len(keys))
			for _, k := range keys {
				elems = append(elems, source.Get(k))
			}
		}

		if mapFn != nil {
			mapped := make([]*JSValue, len(elems))
			for i, e := range elems {
				mapped[i] = mapFn.Call(e, NewNumber(float64(i)))
			}
			elems = mapped
		}

		return newTypedArrayFromArray(desc, elems)
	}))

	ctor.Set("of", NewFunction(func(args ...*JSValue) *JSValue {
		return newTypedArrayFromArray(desc, args)
	}))

	TypedArrayCtors[desc.name] = ctor
	RegisterGlobal(desc.name, ctor)

	// Uint8Array extras
	if desc.kind == taUint8 {
		initUint8ArrayExtras(proto, ctor)
	}
}

func initUint8ArrayExtras(proto *JSValue, ctor *JSValue) {
	defMethod(proto, "toHex", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return NewString("")
		}
		data := bs.data[bs.offset : bs.offset+bs.length]
		return NewString(hex.EncodeToString(data))
	})

	defMethod(proto, "toBase64", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		this := args[0]
		bs := this.ByteSliceData()
		if bs == nil {
			return NewString("")
		}
		data := bs.data[bs.offset : bs.offset+bs.length]
		return NewString(base64.StdEncoding.EncodeToString(data))
	})

	ctor.Set("fromHex", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil {
			return newTypedArrayFromLength(typedArrayDescs[1], 0) // Uint8Array desc
		}
		s := args[0].String()
		data, err := hex.DecodeString(s)
		if err != nil {
			return newTypedArrayFromLength(typedArrayDescs[1], 0)
		}
		return newTypedArrayFromArray(typedArrayDescs[1], jsBytesToElements(data))
	}))

	ctor.Set("fromBase64", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil {
			return newTypedArrayFromLength(typedArrayDescs[1], 0)
		}
		s := args[0].String()
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return newTypedArrayFromLength(typedArrayDescs[1], 0)
		}
		return newTypedArrayFromArray(typedArrayDescs[1], jsBytesToElements(data))
	}))
}

// GetTypedArrayCtor returns the constructor for the named typed array.
func GetTypedArrayCtor(name string) *JSValue {
	if TypedArrayCtors == nil {
		return nil
	}
	return TypedArrayCtors[name]
}

// jsBytesToElements converts a []byte to []*JSValue (one Number per byte).
func jsBytesToElements(data []byte) []*JSValue {
	elems := make([]*JSValue, len(data))
	for i, b := range data {
		elems[i] = NewNumber(float64(b))
	}
	return elems
}

// setTypedArrayProtoMethods adds shared prototype methods for a typed array type.
func setTypedArrayProtoMethods(proto *JSValue, desc typedArrayDesc) {
	// Getter properties
	defGetter(proto, "buffer", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil || bs.arrayBuffer == nil {
			return NewUndefined()
		}
		return bs.arrayBuffer
	})

	defGetter(proto, "byteLength", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(bs.length))
	})

	defGetter(proto, "byteOffset", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(bs.offset))
	})

	defGetter(proto, "length", func(this *JSValue) *JSValue {
		bs := this.ByteSliceData()
		if bs == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(bs.elementCount()))
	})

	// --- Instance methods ---

	defMethod(proto, "set", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[1] == nil {
			return NewUndefined()
		}
		this := args[0]
		source := args[1]
		offset := 0
		if len(args) > 2 && args[2] != nil {
			offset = int(args[2].Number())
			if offset < 0 {
				offset = 0
			}
		}
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}

		// Check if source is a typed array
		srcBS := source.ByteSliceData()
		if srcBS != nil && srcBS.kind != taNone {
			srcCount := srcBS.elementCount()
			if offset+srcCount > thisBS.elementCount() {
				panic(newRangeErrorJSValue("offset is out of bounds"))
			}
			for i := 0; i < srcCount; i++ {
				thisBS.setElement(offset+i, srcBS.getElement(i))
			}
		} else if source.IsArray() {
			elems := source.Array()
			if offset+len(elems) > thisBS.elementCount() {
				panic(newRangeErrorJSValue("offset is out of bounds"))
			}
			for i, elem := range elems {
				thisBS.setElement(offset+i, elem)
			}
		}
		return NewUndefined()
	})

	defMethod(proto, "subarray", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}

		count := thisBS.elementCount()
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

		newOffset := thisBS.offset + begin*desc.bpe
		newLength := (end - begin) * desc.bpe

		ab := thisBS.arrayBuffer
		if ab == nil {
			ab = this
		}

		return newTypedArrayView(desc, ab, newOffset, newLength)
	})

	defMethod(proto, "slice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}

		count := thisBS.elementCount()
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

		elemCount := end - begin
		result := newTypedArrayFromLength(desc, elemCount)
		dstBS := result.ByteSliceData()
		if dstBS == nil {
			return result
		}
		for i := 0; i < elemCount; i++ {
			dstBS.setElement(i, thisBS.getElement(begin+i))
		}
		return result
	})

	defMethod(proto, "fill", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}
		value := args[1]
		count := thisBS.elementCount()

		start := 0
		if len(args) > 2 && args[2] != nil {
			start = int(args[2].Number())
			if start < 0 {
				start = count + start
			}
			if start < 0 {
				start = 0
			}
		}
		end := count
		if len(args) > 3 && args[3] != nil {
			end = int(args[3].Number())
			if end < 0 {
				end = count + end
			}
			if end < 0 {
				end = 0
			}
		}
		if start > end {
			start = end
		}
		if start > count {
			start = count
		}
		if end > count {
			end = count
		}

		for i := start; i < end; i++ {
			thisBS.setElement(i, value)
		}
		return this
	})

	defMethod(proto, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(-1)
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewNumber(-1)
		}
		search := args[1]
		count := thisBS.elementCount()
		from := 0
		if len(args) > 2 && args[2] != nil {
			from = int(args[2].Number())
			if from < 0 {
				from = count + from
			}
			if from < 0 {
				from = 0
			}
		}
		for i := from; i < count; i++ {
			elem := thisBS.getElement(i)
			if jsValueLooseEqual(elem, search) {
				return NewNumber(float64(i))
			}
		}
		return NewNumber(-1)
	})

	defMethod(proto, "includes", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewBool(false)
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewBool(false)
		}
		search := args[1]
		count := thisBS.elementCount()
		from := 0
		if len(args) > 2 && args[2] != nil {
			from = int(args[2].Number())
			if from < 0 {
				from = count + from
			}
			if from < 0 {
				from = 0
			}
		}
		for i := from; i < count; i++ {
			elem := thisBS.getElement(i)
			if jsValueLooseEqual(elem, search) {
				return NewBool(true)
			}
		}
		return NewBool(false)
	})

	defMethod(proto, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		fn := args[1]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}
		count := thisBS.elementCount()
		for i := 0; i < count; i++ {
			elem := thisBS.getElement(i)
			fn.Call(elem, NewNumber(float64(i)), this)
		}
		return NewUndefined()
	})

	defMethod(proto, "map", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return newTypedArrayFromLength(desc, 0)
		}
		this := args[0]
		fn := args[1]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return newTypedArrayFromLength(desc, 0)
		}
		count := thisBS.elementCount()
		result := newTypedArrayFromLength(desc, count)
		resultBS := result.ByteSliceData()
		for i := 0; i < count; i++ {
			elem := thisBS.getElement(i)
			mapped := fn.Call(elem, NewNumber(float64(i)), this)
			resultBS.setElement(i, mapped)
		}
		return result
	})

	defMethod(proto, "filter", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return newTypedArrayFromLength(desc, 0)
		}
		this := args[0]
		fn := args[1]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return newTypedArrayFromLength(desc, 0)
		}
		count := thisBS.elementCount()
		var filtered []*JSValue
		for i := 0; i < count; i++ {
			elem := thisBS.getElement(i)
			result := fn.Call(elem, NewNumber(float64(i)), this)
			if result != nil && result.Bool() {
				filtered = append(filtered, elem)
			}
		}
		return newTypedArrayFromArray(desc, filtered)
	})

	defMethod(proto, "join", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewString("")
		}
		sep := ","
		if len(args) > 1 && args[1] != nil {
			sep = args[1].String()
		}
		count := thisBS.elementCount()
		if count == 0 {
			return NewString("")
		}
		parts := make([]string, count)
		for i := 0; i < count; i++ {
			elem := thisBS.getElement(i)
			parts[i] = elem.String()
		}
		return NewString(strings.Join(parts, sep))
	})

	defMethod(proto, "reverse", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}
		count := thisBS.elementCount()
		for i := 0; i < count/2; i++ {
			j := count - 1 - i
			a := thisBS.getElement(i)
			b := thisBS.getElement(j)
			thisBS.setElement(i, b)
			thisBS.setElement(j, a)
		}
		return this
	})

	defMethod(proto, "at", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewUndefined()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewUndefined()
		}
		idx := int(args[1].Number())
		count := thisBS.elementCount()
		if idx < 0 {
			idx = count + idx
		}
		if idx < 0 || idx >= count {
			return NewUndefined()
		}
		return thisBS.getElement(idx)
	})

	defMethod(proto, "toString", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewString("")
		}
		count := thisBS.elementCount()
		if count == 0 {
			return NewString("")
		}
		parts := make([]string, count)
		for i := 0; i < count; i++ {
			parts[i] = thisBS.getElement(i).String()
		}
		return NewString(strings.Join(parts, ","))
	})

	defMethod(proto, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewArray()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewArray()
		}
		count := thisBS.elementCount()
		result := make([]*JSValue, count)
		for i := 0; i < count; i++ {
			result[i] = NewArray(NewNumber(float64(i)), thisBS.getElement(i))
		}
		return NewArray(result...)
	})

	defMethod(proto, "keys", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewArray()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewArray()
		}
		count := thisBS.elementCount()
		result := make([]*JSValue, count)
		for i := 0; i < count; i++ {
			result[i] = NewNumber(float64(i))
		}
		return NewArray(result...)
	})

	defMethod(proto, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewArray()
		}
		this := args[0]
		thisBS := this.ByteSliceData()
		if thisBS == nil {
			return NewArray()
		}
		count := thisBS.elementCount()
		result := make([]*JSValue, count)
		for i := 0; i < count; i++ {
			result[i] = thisBS.getElement(i)
		}
		return NewArray(result...)
	})
}

// TypedArrayKind constants identify element types for byte slices.
type TypedArrayKind int

const (
	TypedArrayNone          TypedArrayKind = iota // ArrayBuffer / SharedArrayBuffer
	TypedArrayInt8
	TypedArrayUint8
	TypedArrayUint8Clamped
	TypedArrayInt16
	TypedArrayUint16
	TypedArrayInt32
	TypedArrayUint32
	TypedArrayFloat32
	TypedArrayFloat64
	TypedArrayBigInt64
	TypedArrayBigUint64
)

// NewArrayBuffer creates a new ArrayBuffer with the given byte length.
func NewArrayBuffer(byteLength int) *JSValue {
	return newArrayBuffer(byteLength)
}

// NewTypedArrayView creates a typed array view over an ArrayBuffer.
func NewTypedArrayView(kind TypedArrayKind, ab *JSValue, byteOffset, byteLength int) *JSValue {
	desc := typedArrayDescForKind(typedArrayKind(kind))
	if desc == nil {
		return NewUndefined()
	}
	return newTypedArrayView(*desc, ab, byteOffset, byteLength)
}

// SetBufferBytes attaches a typed array byte slice to v. Data is shared with arrayBuffer.
func SetBufferBytes(v *JSValue, offset, length int, arrayBuffer *JSValue, kind TypedArrayKind, bytesPerElement int) {
	abBS := arrayBuffer.ByteSliceData()
	var data []byte
	if abBS != nil {
		data = abBS.data
	}
	v.SetByteSlice(&byteSlice{
		data:            data,
		offset:          offset,
		length:          length,
		arrayBuffer:     arrayBuffer,
		detached:        false,
		bytesPerElement: bytesPerElement,
		kind:            typedArrayKind(kind),
	})
}

// jsValueLooseEqual performs a loose equality check for indexOf/includes.
func jsValueLooseEqual(a, b *JSValue) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// For numbers, compare numerically (handles NaN correctly: NaN !== NaN)
	if a.Type() == TypeNumber && b.Type() == TypeNumber {
		// NaN is never equal to anything
		if math.IsNaN(a.Number()) || math.IsNaN(b.Number()) {
			return false
		}
		return a.Number() == b.Number()
	}
	// Fallback: string comparison
	return a.String() == b.String()
}
