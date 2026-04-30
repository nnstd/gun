package querystring

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// --- Escape ---

func TestEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello world", "hello%20world"},
		{"a+b=c", "a%2Bb%3Dc"},
		{"foo bar baz", "foo%20bar%20baz"},
		{"", ""},
	}
	for _, tt := range tests {
		got := Escape(jsvalue.NewString(tt.in)).String()
		if got != tt.want {
			t.Errorf("escape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeNil(t *testing.T) {
	if Escape(nil).String() != "" {
		t.Error("escape(nil) should return empty string")
	}
}

// --- Unescape ---

func TestUnescape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello%20world", "hello world"},
		{"a%2Bb%3Dc", "a+b=c"},
		{"foo%20bar%20baz", "foo bar baz"},
		{"", ""},
	}
	for _, tt := range tests {
		got := Unescape(jsvalue.NewString(tt.in)).String()
		if got != tt.want {
			t.Errorf("unescape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnescapeNil(t *testing.T) {
	if Unescape(nil).String() != "" {
		t.Error("unescape(nil) should return empty string")
	}
}

func TestUnescapeDoesNotDecodePlus(t *testing.T) {
	got := Unescape(jsvalue.NewString("hello+world")).String()
	if got != "hello+world" {
		t.Errorf("unescape should not decode + as space, got %q", got)
	}
}

func TestUnescapeInvalidHex(t *testing.T) {
	got := Unescape(jsvalue.NewString("hello%GGworld")).String()
	if got != "hello%GGworld" {
		t.Errorf("unescape should pass through invalid hex, got %q", got)
	}
}

// --- Parse ---

func TestParseBasic(t *testing.T) {
	result := Parse(jsvalue.NewString("foo=bar&baz=qux"))
	if result.Get("foo").String() != "bar" {
		t.Error("foo should be bar")
	}
	if result.Get("baz").String() != "qux" {
		t.Error("baz should be qux")
	}
}

func TestParseEmpty(t *testing.T) {
	result := Parse(jsvalue.NewString(""))
	keys := result.EnumerableOwnKeys()
	if len(keys) != 0 {
		t.Error("empty input should produce empty object")
	}
}

func TestParseNil(t *testing.T) {
	result := Parse(nil)
	keys := result.EnumerableOwnKeys()
	if len(keys) != 0 {
		t.Error("nil input should produce empty object")
	}
}

func TestParseNoValue(t *testing.T) {
	result := Parse(jsvalue.NewString("key"))
	if result.Get("key").String() != "" {
		t.Error("key without value should have empty string value")
	}
}

func TestParseNoKey(t *testing.T) {
	result := Parse(jsvalue.NewString("=val"))
	if result.Get("").String() != "val" {
		t.Error("empty key should map to val")
	}
}

func TestParseDuplicateKeys(t *testing.T) {
	result := Parse(jsvalue.NewString("a=1&a=2&a=3"))
	arr := result.Get("a").Array()
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr))
	}
	if arr[0].String() != "1" || arr[1].String() != "2" || arr[2].String() != "3" {
		t.Error("duplicate key values mismatch")
	}
}

func TestParseCustomSep(t *testing.T) {
	result := Parse(jsvalue.NewString("a=1;b=2"), jsvalue.NewString(";"))
	if result.Get("a").String() != "1" || result.Get("b").String() != "2" {
		t.Error("custom sep parse failed")
	}
}

func TestParseCustomEq(t *testing.T) {
	result := Parse(jsvalue.NewString("a:1&b:2"), jsvalue.NewString("&"), jsvalue.NewString(":"))
	if result.Get("a").String() != "1" || result.Get("b").String() != "2" {
		t.Error("custom eq parse failed")
	}
}

func TestParseMaxKeys(t *testing.T) {
	opts := jsvalue.NewObject()
	opts.Set("maxKeys", jsvalue.NewNumber(2))
	result := Parse(jsvalue.NewString("a=1&b=2&c=3"), nil, nil, opts)
	keys := result.EnumerableOwnKeys()
	if len(keys) != 2 {
		t.Errorf("maxKeys=2 should produce 2 keys, got %d", len(keys))
	}
}

func TestParseMaxKeysZero(t *testing.T) {
	opts := jsvalue.NewObject()
	opts.Set("maxKeys", jsvalue.NewNumber(0))
	result := Parse(jsvalue.NewString("a=1&b=2&c=3"), nil, nil, opts)
	keys := result.EnumerableOwnKeys()
	if len(keys) != 3 {
		t.Errorf("maxKeys=0 should produce all keys, got %d", len(keys))
	}
}

func TestParseEncodedChars(t *testing.T) {
	result := Parse(jsvalue.NewString("name=hello%20world"))
	if result.Get("name").String() != "hello world" {
		t.Error("encoded space not decoded")
	}
}

func TestParsePlusAsSpace(t *testing.T) {
	result := Parse(jsvalue.NewString("name=hello+world"))
	if result.Get("name").String() != "hello world" {
		t.Error("plus not decoded as space")
	}
}

func TestParseTrailingSep(t *testing.T) {
	result := Parse(jsvalue.NewString("a=1&"))
	if result.Get("a").String() != "1" {
		t.Error("trailing sep should not affect a=1")
	}
}

func TestParseCustomDecoder(t *testing.T) {
	opts := jsvalue.NewObject()
	// Custom decoder that uppercases
	opts.Set("decodeURIComponent", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		s := args[0].String()
		result := make([]byte, len(s))
		for i := range s {
			if s[i] >= 'a' && s[i] <= 'z' {
				result[i] = s[i] - 32
			} else {
				result[i] = s[i]
			}
		}
		return jsvalue.NewString(string(result))
	}))
	result := Parse(jsvalue.NewString("key=value"), nil, nil, opts)
	if result.Get("KEY").String() != "VALUE" {
		t.Errorf("custom decoder failed, keys: %v", result.EnumerableOwnKeys())
	}
}

// --- Stringify ---

func TestStringifyBasic(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("foo", jsvalue.NewString("bar"))
	obj.Set("baz", jsvalue.NewString("qux"))
	got := Stringify(obj).String()
	if got != "foo=bar&baz=qux" && got != "baz=qux&foo=bar" {
		t.Errorf("stringify basic = %q", got)
	}
}

func TestStringifyEmpty(t *testing.T) {
	obj := jsvalue.NewObject()
	got := Stringify(obj).String()
	if got != "" {
		t.Errorf("empty object stringify = %q, want empty", got)
	}
}

func TestStringifyNil(t *testing.T) {
	got := Stringify(nil).String()
	if got != "" {
		t.Error("nil stringify should return empty")
	}
}

func TestStringifyArrayValues(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("a", jsvalue.NewArray(jsvalue.NewString("1"), jsvalue.NewString("2")))
	got := Stringify(obj).String()
	if got != "a=1&a=2" {
		t.Errorf("array values stringify = %q", got)
	}
}

func TestStringifyCustomSep(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("a", jsvalue.NewString("1"))
	obj.Set("b", jsvalue.NewString("2"))
	got := Stringify(obj, jsvalue.NewString(";")).String()
	if got != "a=1;b=2" && got != "b=2;a=1" {
		t.Errorf("custom sep stringify = %q", got)
	}
}

func TestStringifyCustomEq(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("a", jsvalue.NewString("1"))
	got := Stringify(obj, jsvalue.NewString("&"), jsvalue.NewString(":")).String()
	if got != "a:1" {
		t.Errorf("custom eq stringify = %q", got)
	}
}

func TestStringifySpecialChars(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("q", jsvalue.NewString("hello world"))
	got := Stringify(obj).String()
	if got != "q=hello%20world" {
		t.Errorf("special chars stringify = %q", got)
	}
}

func TestStringifyNullValue(t *testing.T) {
	obj := jsvalue.NewObject()
	obj.Set("key", jsvalue.NewNull())
	got := Stringify(obj).String()
	if got != "key=" {
		t.Errorf("null value stringify = %q", got)
	}
}

// --- Round trip ---

func TestRoundTrip(t *testing.T) {
	original := "a=1&b=hello%20world&c=3"
	parsed := Parse(jsvalue.NewString(original))
	result := Stringify(parsed).String()
	// Parse then stringify should contain all key-value pairs
	for _, pair := range []string{"a=1", "b=hello%20world", "c=3"} {
		if !contains(result, pair) {
			t.Errorf("round trip missing %q in %q", pair, result)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Aliases ---

func TestAliases(t *testing.T) {
	input := jsvalue.NewString("a=1&b=2")
	encodeResult := AsJSValue.Get("encode").Call(Parse(input)).String()
	stringifyResult := AsJSValue.Get("stringify").Call(Parse(input)).String()
	if encodeResult != stringifyResult {
		t.Errorf("encode != stringify: %q vs %q", encodeResult, stringifyResult)
	}

	decodeResult := AsJSValue.Get("decode").Call(input)
	parseResult := AsJSValue.Get("parse").Call(input)
	if decodeResult.Get("a").String() != parseResult.Get("a").String() {
		t.Error("decode != parse")
	}
}

// --- Module exports ---

func TestAsJSValueExports(t *testing.T) {
	for _, name := range []string{"parse", "stringify", "escape", "unescape", "encode", "decode"} {
		export := AsJSValue.Get(name)
		if export.TypeString() != "function" {
			t.Errorf("expected %s to be a function, got %s", name, export.TypeString())
		}
	}
}
