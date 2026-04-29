package jsvalue

import (
	"strconv"
	"testing"
)

var benchSink *JSValue
var benchStringSink string

func benchmarkASCIIString() *JSValue {
	return NewString("alpha beta gamma delta epsilon zeta eta theta iota kappa")
}

func BenchmarkFastPathStringCoercionHelpers(b *testing.B) {
	s := benchmarkASCIIString()
	needle := NewString("gamma")
	for b.Loop() {
		benchSink = Split(s, NewString(" "))
		benchSink = Replace(s, needle, NewString("GAMMA"))
		benchSink = StartsWith(s, NewString("alpha"))
		benchSink = EndsWith(s, NewString("kappa"))
		benchSink = Repeat(NewString("ab"), NewInt(3))
	}
}

func BenchmarkFastPathASCIIStringIndexing(b *testing.B) {
	s := benchmarkASCIIString()
	for b.Loop() {
		benchSink = CharAt(s, NewInt(7))
		benchSink = Substring(s, NewInt(6), NewInt(22))
		benchSink = StringSlice(s, NewInt(6), NewInt(22))
		benchSink = s.Index(12)
		benchSink = s.Get("3")
	}
}

func BenchmarkFastPathStringPadding(b *testing.B) {
	s := NewString("id")
	target := NewInt(64)
	pad := NewString("0")
	for b.Loop() {
		benchSink = StringPrototype.Get("padStart").Call(s, target, pad)
		benchSink = StringPrototype.Get("padEnd").Call(s, target, pad)
	}
}

func benchmarkArray(n int) *JSValue {
	elems := make([]*JSValue, n)
	for i := range n {
		elems[i] = NewInt(i)
	}
	return NewArray(elems...)
}

func BenchmarkFastPathArrayLoopAccess(b *testing.B) {
	arr := benchmarkArray(64)
	mapper := NewFunction(func(args ...*JSValue) *JSValue {
		return Add(args[0], NewInt(1))
	})
	pred := NewFunction(func(args ...*JSValue) *JSValue {
		return Lt(args[0], NewInt(32))
	})
	for b.Loop() {
		benchSink = ArrayPrototype.Get("map").Call(arr, mapper)
		benchSink = ArrayPrototype.Get("forEach").Call(arr, mapper)
		benchSink = ArrayPrototype.Get("some").Call(arr, pred)
		benchSink = ArrayPrototype.Get("every").Call(arr, pred)
	}
}

func BenchmarkFastPathArrayResultPreallocation(b *testing.B) {
	arr := benchmarkArray(128)
	pred := NewFunction(func(args ...*JSValue) *JSValue {
		return Lt(args[0], NewInt(96))
	})
	flat := NewArray(arr, arr, arr, arr)
	flatMapper := NewFunction(func(args ...*JSValue) *JSValue {
		return NewArray(args[0], args[0])
	})
	for b.Loop() {
		benchSink = ArrayPrototype.Get("filter").Call(arr, pred)
		benchSink = ArrayPrototype.Get("flat").Call(flat)
		benchSink = ArrayPrototype.Get("flatMap").Call(arr, flatMapper)
	}
}

func BenchmarkFastPathNewArrayLarge(b *testing.B) {
	elems := make([]*JSValue, 128)
	for i := range elems {
		elems[i] = NewInt(i)
	}
	b.ResetTimer()
	for b.Loop() {
		benchSink = NewArray(elems...)
	}
}

func BenchmarkFastPathOwnEnumerableValuesEntries(b *testing.B) {
	obj := NewObject()
	for i := range 64 {
		obj.Set("k"+strconv.Itoa(i), NewInt(i))
	}
	for b.Loop() {
		benchSink = Values(obj)
		benchSink = Entries(obj)
	}
}

func BenchmarkFastPathSingleDigitArrayGet(b *testing.B) {
	arr := benchmarkArray(10)
	for b.Loop() {
		benchSink = arr.Get("0")
		benchSink = arr.Get("1")
		benchSink = arr.Get("2")
		benchSink = arr.Get("3")
		benchSink = arr.Get("4")
		benchSink = arr.Get("5")
		benchSink = arr.Get("6")
		benchSink = arr.Get("7")
		benchSink = arr.Get("8")
		benchSink = arr.Get("9")
	}
}

func BenchmarkFastPathComputedPropertyKeyRuntime(b *testing.B) {
	key := NewString("content-type")
	num := NewInt(42)
	for b.Loop() {
		benchStringSink = PropertyKey(key)
		benchStringSink = PropertyKey(num)
	}
}
