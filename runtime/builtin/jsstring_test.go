package jsvalue

import "testing"

func TestStringIndexOfWithFromIndex(t *testing.T) {
	s := NewString("http://127.0.0.1:3017/")

	if got := s.MethodCall("indexOf", NewString(":"), NewNumber(0)).Number(); got != 4 {
		t.Fatalf("indexOf(':', 0) = %v, want 4", got)
	}

	if got := s.MethodCall("indexOf", NewString("/"), NewNumber(8)).Number(); got != 21 {
		t.Fatalf("indexOf('/', 8) = %v, want 21", got)
	}
}

func TestStringSliceMethodWithExplicitBounds(t *testing.T) {
	s := NewString("http://127.0.0.1:3017/")

	if got := s.MethodCall("slice", NewNumber(21), NewNumber(22)).String(); got != "/" {
		t.Fatalf("slice(21, 22) = %q, want %q", got, "/")
	}

	if got := s.MethodCall("slice", NewNumber(21)).String(); got != "/" {
		t.Fatalf("slice(21) = %q, want %q", got, "/")
	}
}

func TestStringSliceMethodWithNegativeEnd(t *testing.T) {
	s := NewString("-u")

	if got := s.MethodCall("slice", NewNumber(1), NewNumber(-1)).String(); got != "" {
		t.Fatalf("slice(1, -1) = %q, want empty string", got)
	}
}
