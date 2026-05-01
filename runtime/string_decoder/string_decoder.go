package string_decoder

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
)

var StringDecoder *jsvalue.JSValue
var AsJSValue *jsvalue.JSValue

const replacementChar = "\uFFFD"

func normalizeEncoding(enc string) (string, bool) {
	e := strings.ToLower(enc)
	e = strings.ReplaceAll(e, "-", "")
	switch e {
	case "utf8":
		return "utf8", true
	case "utf16le", "ucs2":
		return "utf16le", true
	case "latin1", "binary":
		return "latin1", true
	case "ascii":
		return "ascii", true
	case "base64":
		return "base64", true
	case "base64url":
		return "base64url", true
	case "hex":
		return "hex", true
	}
	return "", false
}

func rawBytes(v *jsvalue.JSValue) []byte {
	if v == nil {
		return nil
	}
	if bs := v.Bytes(); bs != nil {
		return append([]byte(nil), bs...)
	}
	return []byte(v.String())
}

func getBuf(this *jsvalue.JSValue) []byte {
	return []byte(this.Get("_buf").String())
}

func setBuf(this *jsvalue.JSValue, b []byte) {
	this.Set("_buf", jsvalue.NewString(string(b)))
}

func getEnc(this *jsvalue.JSValue) string {
	return this.Get("_enc").String()
}

func utf8PartialLen(b []byte) int {
	n := len(b)
	for i := 0; i < 4 && i < n; i++ {
		c := b[n-1-i]
		if c&0x80 == 0 {
			return 0
		}
		if c&0xC0 == 0xC0 {
			need := 0
			switch {
			case c&0xF8 == 0xF0:
				need = 4
			case c&0xF0 == 0xE0:
				need = 3
			case c&0xE0 == 0xC0:
				need = 2
			default:
				return 0
			}
			have := i + 1
			if have < need {
				return have
			}
			return 0
		}
	}
	return 0
}

func utf16lePartialLen(b []byte) int {
	n := len(b)
	if n == 0 {
		return 0
	}
	partial := n % 2
	complete := n - partial
	if complete >= 2 {
		last := uint16(b[complete-2]) | uint16(b[complete-1])<<8
		if last >= 0xD800 && last <= 0xDBFF {
			partial += 2
		}
	}
	return partial
}

func utf16leDecode(b []byte) string {
	n := len(b) / 2
	if n == 0 {
		return ""
	}
	units := make([]uint16, n)
	for i := 0; i < n; i++ {
		units[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(units))
}

func latin1Decode(b []byte) string {
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}

func asciiDecode(b []byte) string {
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c & 0x7F)
	}
	return string(rs)
}

func decodeWrite(this *jsvalue.JSValue, input []byte) string {
	enc := getEnc(this)
	buf := append(getBuf(this), input...)

	switch enc {
	case "utf8":
		p := utf8PartialLen(buf)
		setBuf(this, buf[len(buf)-p:])
		return string(buf[:len(buf)-p])
	case "utf16le":
		p := utf16lePartialLen(buf)
		setBuf(this, buf[len(buf)-p:])
		return utf16leDecode(buf[:len(buf)-p])
	case "latin1":
		setBuf(this, nil)
		return latin1Decode(buf)
	case "ascii":
		setBuf(this, nil)
		return asciiDecode(buf)
	case "base64":
		p := len(buf) % 3
		setBuf(this, buf[len(buf)-p:])
		return base64.StdEncoding.EncodeToString(buf[:len(buf)-p])
	case "base64url":
		p := len(buf) % 3
		setBuf(this, buf[len(buf)-p:])
		return base64.RawURLEncoding.EncodeToString(buf[:len(buf)-p])
	case "hex":
		setBuf(this, nil)
		return hex.EncodeToString(buf)
	}
	return ""
}

func decodeEnd(this *jsvalue.JSValue, input []byte) string {
	enc := getEnc(this)
	buf := append(getBuf(this), input...)
	setBuf(this, nil)

	switch enc {
	case "utf8":
		p := utf8PartialLen(buf)
		s := string(buf[:len(buf)-p])
		if p > 0 {
			s += replacementChar
		}
		return s
	case "utf16le":
		p := utf16lePartialLen(buf)
		s := utf16leDecode(buf[:len(buf)-p])
		if p > 0 {
			s += replacementChar
		}
		return s
	case "latin1":
		return latin1Decode(buf)
	case "ascii":
		return asciiDecode(buf)
	case "base64":
		return base64.StdEncoding.EncodeToString(buf)
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(buf)
	case "hex":
		return hex.EncodeToString(buf)
	}
	return ""
}

func init() {
	StringDecoder = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		enc := "utf8"
		if len(args) > 0 && args[0] != nil && args[0].TypeString() == "string" {
			raw := args[0].String()
			norm, ok := normalizeEncoding(raw)
			if !ok {
				panic(jserror.TypeError.Call(jsvalue.NewString(fmt.Sprintf("Unknown encoding: %s", raw))))
			}
			enc = norm
		}
		this.Set("_enc", jsvalue.NewString(enc))
		this.Set("_buf", jsvalue.NewString(""))
		this.Set("encoding", jsvalue.NewString(enc))
		return nil
	}, nil)

	proto := StringDecoder.Get("prototype")

	proto.Set("write", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		this := args[0]
		var input []byte
		if len(args) > 1 && args[1] != nil {
			input = rawBytes(args[1])
		}
		return jsvalue.NewString(decodeWrite(this, input))
	}).MarkAsMethod())

	proto.Set("end", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		this := args[0]
		var input []byte
		if len(args) > 1 && args[1] != nil {
			input = rawBytes(args[1])
		}
		return jsvalue.NewString(decodeEnd(this, input))
	}).MarkAsMethod())

	AsJSValue = jsvalue.ObjectFrom("StringDecoder", StringDecoder)
}
