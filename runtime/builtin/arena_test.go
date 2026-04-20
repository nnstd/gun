package jsvalue

import "testing"

func TestNewArenaStartsLazy(t *testing.T) {
	a := NewArena()
	if a.current != nil {
		t.Fatal("expected lazy arena to start without an allocated chunk")
	}
	a.PushScope()
	if a.current == nil {
		t.Fatal("expected PushScope to allocate the first chunk lazily")
	}
}

func TestArenaPushPopReusesMarkSlice(t *testing.T) {
	a := NewArena()
	for i := 0; i < 32; i++ {
		a.PushScope()
		a.PopScope()
	}
	cap0 := cap(a.marks)
	if cap0 == 0 {
		t.Fatal("expected marks capacity to grow")
	}
	for i := 0; i < 128; i++ {
		a.PushScope()
		a.PopScope()
	}
	if cap(a.marks) != cap0 {
		t.Fatalf("marks capacity changed from %d to %d; expected reuse", cap0, cap(a.marks))
	}
}

func BenchmarkArenaScopePushPop(b *testing.B) {
	a := NewArena()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.PushScope()
		a.PopScope()
	}
}

func BenchmarkGetArenaRelease(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := GetArena()
		ReleaseArena(a)
	}
}

