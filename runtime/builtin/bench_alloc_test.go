package jsvalue

import "testing"

// BenchmarkArithmeticLoop - tight Add/Sub loop on small integers.
// Operands and results are overwhelmingly in the [-128, 255] cache range.
func BenchmarkArithmeticLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		acc := NewNumber(0)
		for j := 0; j < 100; j++ {
			acc = Add(acc, NewNumber(float64(j)))
			acc = Sub(acc, NewNumber(1))
		}
		_ = acc
	}
}

// BenchmarkArrayMapFilter - simulated map/filter pipeline without relying on
// prototype dispatch. Allocates many NewNumber and NewBool temporaries.
func BenchmarkArrayMapFilter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		src := make([]*JSValue, 0, 50)
		for j := 0; j < 50; j++ {
			src = append(src, NewNumber(float64(j)))
		}
		mapped := make([]*JSValue, len(src))
		for idx, v := range src {
			mapped[idx] = Mul(v, NewNumber(2))
		}
		filtered := mapped[:0]
		for _, v := range mapped {
			if Lt(v, NewNumber(50)).Bool() {
				filtered = append(filtered, v)
			}
		}
		_ = filtered
	}
}

// BenchmarkStringOperations - string concat + split.
func BenchmarkStringOperations(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s := NewString("hello")
		for j := 0; j < 20; j++ {
			s = Add(s, NewString(" world"))
		}
		_ = Split(s, NewString(" "))
	}
}

// BenchmarkClosureHeavy - function creation + repeated calls.
func BenchmarkClosureHeavy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		adder := NewFunction(func(args ...*JSValue) *JSValue {
			return Add(args[0], args[1])
		})
		for j := 0; j < 50; j++ {
			_ = adder.Call(NewNumber(float64(j)), NewNumber(1))
		}
	}
}

// BenchmarkBooleanHeavy - comparison-heavy code (Lt/Gt/Eq/Not).
func BenchmarkBooleanHeavy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			a := NewNumber(float64(j))
			bb := NewNumber(float64(j + 1))
			_ = Lt(a, bb)
			_ = Gt(a, bb)
			_ = Eq(a, bb)
			_ = Not(Lt(a, bb))
		}
	}
}

func BenchmarkObjectFromFlat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ObjectFrom(
			"hello", NewString("world"),
			"count", NewNumber(1),
			"ok", NewBool(true),
		)
	}
}

func BenchmarkObjectFromMap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ObjectFrom(map[string]any{
			"hello": NewString("world"),
			"count": NewNumber(1),
			"ok":    NewBool(true),
		})
	}
}
