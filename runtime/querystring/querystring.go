package querystring

import (
	"net/url"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func escape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func unescape(s string) string {
	result := strings.Builder{}
	result.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hex := s[i+1 : i+3]
			var b byte
			for _, c := range hex {
				b <<= 4
				switch {
				case c >= '0' && c <= '9':
					b |= byte(c - '0')
				case c >= 'a' && c <= 'f':
					b |= byte(c - 'a' + 10)
				case c >= 'A' && c <= 'F':
					b |= byte(c - 'A' + 10)
				default:
					goto literal
				}
				continue
			}
			result.WriteByte(b)
			i += 2
			continue
		}
	literal:
		result.WriteByte(s[i])
	}
	return result.String()
}

func defaultUnescape(s string) string {
	return unescape(strings.ReplaceAll(s, "+", " "))
}

func Escape(str *jsvalue.JSValue) *jsvalue.JSValue {
	if str == nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(escape(str.String()))
}

func Unescape(str *jsvalue.JSValue) *jsvalue.JSValue {
	if str == nil {
		return jsvalue.NewString("")
	}
	return jsvalue.NewString(unescape(str.String()))
}

func Parse(str *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	input := ""
	if str != nil {
		input = str.String()
	}
	if input == "" {
		return jsvalue.NewObject()
	}

	sep := "&"
	if len(args) > 0 && args[0] != nil && args[0].String() != "" {
		sep = args[0].String()
	}
	eq := "="
	if len(args) > 1 && args[1] != nil && args[1].String() != "" {
		eq = args[1].String()
	}

	maxKeys := 1000
	var customDecoder *jsvalue.JSValue
	if len(args) > 2 && args[2] != nil {
		opts := args[2]
		mk := opts.Get("maxKeys")
		if !jsvalue.IsNullish(mk) {
			maxKeys = int(mk.Number())
		}
		d := opts.Get("decodeURIComponent")
		if d != nil && d.TypeString() == "function" {
			customDecoder = d
		}
	}

	decode := defaultUnescape
	if customDecoder != nil {
		decode = func(s string) string {
			return customDecoder.Call(jsvalue.NewString(s)).String()
		}
	}

	result := jsvalue.NewObject()
	pairs := strings.Split(input, sep)
	count := 0

	for _, pair := range pairs {
		if maxKeys > 0 && count >= maxKeys {
			break
		}

		parts := strings.SplitN(pair, eq, 2)
		key := decode(parts[0])
		var val string
		if len(parts) == 2 {
			val = decode(parts[1])
		}

		existing := result.Get(key)
		if jsvalue.IsNullish(existing) {
			result.Set(key, jsvalue.NewString(val))
		} else if existing.IsArray() {
			arr := existing.Array()
			arr = append(arr, jsvalue.NewString(val))
			result.Set(key, jsvalue.NewArray(arr...))
		} else {
			result.Set(key, jsvalue.NewArray(existing, jsvalue.NewString(val)))
		}
		count++
	}

	return result
}

func Stringify(obj *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if obj == nil {
		return jsvalue.NewString("")
	}

	sep := "&"
	if len(args) > 0 && args[0] != nil && args[0].String() != "" {
		sep = args[0].String()
	}
	eq := "="
	if len(args) > 1 && args[1] != nil && args[1].String() != "" {
		eq = args[1].String()
	}

	encoder := escape
	if len(args) > 2 && args[2] != nil {
		enc := args[2].Get("encodeURIComponent")
		if enc != nil && enc.TypeString() == "function" {
			encoder = func(s string) string {
				return enc.Call(jsvalue.NewString(s)).String()
			}
		}
	}

	keys := obj.EnumerableOwnKeys()
	pairs := make([]string, 0, len(keys))

	for _, key := range keys {
		val := obj.Get(key)
		if jsvalue.IsNullish(val) {
			pairs = append(pairs, encoder(key)+eq)
			continue
		}
		if val.IsArray() {
			for _, elem := range val.Array() {
				pairs = append(pairs, encoder(key)+eq+encoder(stringifyValue(elem)))
			}
		} else {
			pairs = append(pairs, encoder(key)+eq+encoder(stringifyValue(val)))
		}
	}

	return jsvalue.NewString(strings.Join(pairs, sep))
}

func stringifyValue(v *jsvalue.JSValue) string {
	if jsvalue.IsNullish(v) {
		return ""
	}
	switch v.Type() {
	case jsvalue.TypeString:
		return v.String()
	case jsvalue.TypeNumber:
		return v.String()
	case jsvalue.TypeBoolean:
		if v.Bool() {
			return "true"
		}
		return "false"
	}
	return ""
}

var AsJSValue = jsvalue.ObjectFrom(
	"parse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewObject()
		}
		return Parse(args[0], args[1:]...)
	}),
	"stringify", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return Stringify(args[0], args[1:]...)
	}),
	"escape", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return Escape(args[0])
	}),
	"unescape", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return Unescape(args[0])
	}),
	"encode", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return Stringify(args[0], args[1:]...)
	}),
	"decode", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewObject()
		}
		return Parse(args[0], args[1:]...)
	}),
)
