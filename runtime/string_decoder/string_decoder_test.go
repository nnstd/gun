package string_decoder

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func newDecoder(t *testing.T, enc string) *jsvalue.JSValue {
	t.Helper()
	var arg *jsvalue.JSValue
	if enc != "" {
		arg = jsvalue.NewString(enc)
	}
	obj := StringDecoder.Call(arg)
	if obj == nil {
		t.Fatalf("StringDecoder.Call(%q) returned nil", enc)
	}
	return obj
}

func bytesArg(b []byte) *jsvalue.JSValue {
	return jsvalue.NewString(string(b))
}

func write(t *testing.T, d *jsvalue.JSValue, b []byte) string {
	t.Helper()
	return d.MethodCall("write", bytesArg(b)).String()
}

func end(t *testing.T, d *jsvalue.JSValue, b []byte) string {
	t.Helper()
	if b == nil {
		return d.MethodCall("end").String()
	}
	return d.MethodCall("end", bytesArg(b)).String()
}

func TestExports(t *testing.T) {
	if AsJSValue == nil {
		t.Fatal("AsJSValue nil")
	}
	if AsJSValue.Get("StringDecoder") != StringDecoder {
		t.Fatal("AsJSValue.StringDecoder not the class")
	}
}

func TestEncodingNormalization(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"utf8", "utf8"},
		{"utf-8", "utf8"},
		{"UTF-8", "utf8"},
		{"utf16le", "utf16le"},
		{"utf-16le", "utf16le"},
		{"ucs2", "utf16le"},
		{"ucs-2", "utf16le"},
		{"latin1", "latin1"},
		{"binary", "latin1"},
		{"ascii", "ascii"},
		{"base64", "base64"},
		{"base64url", "base64url"},
		{"hex", "hex"},
	}
	for _, c := range cases {
		d := newDecoder(t, c.in)
		got := d.Get("encoding").String()
		if got != c.want {
			t.Errorf("encoding(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

func TestUnknownEncodingThrows(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown encoding")
		}
	}()
	StringDecoder.Call(jsvalue.NewString("bogus"))
}

func TestUTF8CompleteSingleWrite(t *testing.T) {
	d := newDecoder(t, "utf8")
	got := write(t, d, []byte{0xE2, 0x82, 0xAC})
	if got != "€" {
		t.Errorf("got %q want €", got)
	}
}

func TestUTF8SplitAcrossThreeCalls(t *testing.T) {
	d := newDecoder(t, "utf8")
	if got := write(t, d, []byte{0xE2}); got != "" {
		t.Errorf("write([0xE2]) = %q, want ''", got)
	}
	if got := write(t, d, []byte{0x82}); got != "" {
		t.Errorf("write([0x82]) = %q, want ''", got)
	}
	if got := end(t, d, []byte{0xAC}); got != "€" {
		t.Errorf("end([0xAC]) = %q, want €", got)
	}
}

func TestUTF8HoldsPartialAtTail(t *testing.T) {
	d := newDecoder(t, "utf8")
	if got := write(t, d, []byte{0x48, 0x69, 0xE2}); got != "Hi" {
		t.Errorf("got %q want Hi", got)
	}
	if got := write(t, d, []byte{0x82, 0xAC}); got != "€" {
		t.Errorf("got %q want €", got)
	}
}

func TestUTF8EndReplacementChar(t *testing.T) {
	d := newDecoder(t, "utf8")
	write(t, d, []byte{0xE2})
	got := end(t, d, nil)
	if got != "\uFFFD" {
		t.Errorf("got %q want U+FFFD", got)
	}
}

func TestUTF8ReusableAfterEnd(t *testing.T) {
	d := newDecoder(t, "utf8")
	write(t, d, []byte{0xE2})
	end(t, d, nil)
	if got := write(t, d, []byte{0x41}); got != "A" {
		t.Errorf("reuse: got %q want A", got)
	}
}

func TestUTF16LESimple(t *testing.T) {
	d := newDecoder(t, "utf16le")
	got := write(t, d, []byte{0x61, 0x00, 0x62, 0x00})
	if got != "ab" {
		t.Errorf("got %q want ab", got)
	}
}

func TestUTF16LEOddTrailingByte(t *testing.T) {
	d := newDecoder(t, "utf16le")
	if got := write(t, d, []byte{0x61, 0x00, 0x62}); got != "a" {
		t.Errorf("got %q want a", got)
	}
	if got := write(t, d, []byte{0x00}); got != "b" {
		t.Errorf("got %q want b", got)
	}
}

func TestUTF16LESurrogatePair(t *testing.T) {
	// U+1F642 (🙂) = UTF-16 surrogate pair D83D DE42, LE bytes: 3D D8 42 DE
	d := newDecoder(t, "utf16le")
	if got := write(t, d, []byte{0x3D, 0xD8}); got != "" {
		t.Errorf("high surrogate alone: got %q want ''", got)
	}
	if got := write(t, d, []byte{0x42, 0xDE}); got != "🙂" {
		t.Errorf("pair: got %q want 🙂", got)
	}
}

func TestUTF16LEEndReplacement(t *testing.T) {
	d := newDecoder(t, "utf16le")
	got := end(t, d, []byte{0x61})
	if got != "\uFFFD" {
		t.Errorf("got %q want U+FFFD", got)
	}
}

func TestBase64SplitGroups(t *testing.T) {
	d := newDecoder(t, "base64")
	if got := write(t, d, []byte{0x61, 0x62, 0x63}); got != "YWJj" {
		t.Errorf("3 bytes: got %q want YWJj", got)
	}
}

func TestBase64BuffersPartial(t *testing.T) {
	d := newDecoder(t, "base64")
	if got := write(t, d, []byte{0x61, 0x62}); got != "" {
		t.Errorf("2 bytes buffer: got %q want ''", got)
	}
	if got := end(t, d, nil); got != "YWI=" {
		t.Errorf("flush: got %q want YWI=", got)
	}
}

func TestBase64FourBytesBufferOne(t *testing.T) {
	d := newDecoder(t, "base64")
	if got := write(t, d, []byte{0x61, 0x62, 0x63, 0x64}); got != "YWJj" {
		t.Errorf("got %q want YWJj", got)
	}
	if got := end(t, d, nil); got != "ZA==" {
		t.Errorf("flush: got %q want ZA==", got)
	}
}

func TestBase64URL(t *testing.T) {
	d := newDecoder(t, "base64url")
	// 0xFB 0xFF 0xFE → base64 std = "+//+", base64url = "-__-"
	if got := write(t, d, []byte{0xFB, 0xFF, 0xFE}); got != "-__-" {
		t.Errorf("got %q want -__-", got)
	}
}

func TestHex(t *testing.T) {
	d := newDecoder(t, "hex")
	if got := write(t, d, []byte{0xDE, 0xAD, 0xBE, 0xEF}); got != "deadbeef" {
		t.Errorf("got %q want deadbeef", got)
	}
}

func TestLatin1(t *testing.T) {
	d := newDecoder(t, "latin1")
	// 0xE9 = é in latin1
	if got := write(t, d, []byte{0x68, 0x69, 0xE9}); got != "hié" {
		t.Errorf("got %q want hié", got)
	}
}

func TestBinaryAliasesLatin1(t *testing.T) {
	d := newDecoder(t, "binary")
	if got := write(t, d, []byte{0xE9}); got != "é" {
		t.Errorf("got %q want é", got)
	}
}

func TestASCII(t *testing.T) {
	d := newDecoder(t, "ascii")
	// high bit stripped: 0xC1 → 0x41 ('A')
	if got := write(t, d, []byte{0xC1}); got != "A" {
		t.Errorf("got %q want A", got)
	}
}

func TestDefaultEncodingIsUTF8(t *testing.T) {
	d := newDecoder(t, "")
	if got := d.Get("encoding").String(); got != "utf8" {
		t.Errorf("default: got %q want utf8", got)
	}
	if got := write(t, d, []byte{0x41}); got != "A" {
		t.Errorf("got %q want A", got)
	}
}
