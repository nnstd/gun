package intl

import (
	"unicode/utf8"

	"github.com/nnstd/gun/runtime/builtin"
)

// NewSegmenter creates a new Intl.Segmenter as a JSValue class.
// The segmenter has a segment() method that splits strings into rune segments.
var Segmenter = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
	// Constructor — store options if provided
	if len(args) > 0 {
		this.Set("locale", args[0])
	}
	if len(args) > 1 {
		this.Set("options", args[1])
	}

	// Set segment method on the instance directly (since we need closure over this)
	this.Set("segment", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		str := ""
		if len(args) > 0 && args[0] != nil {
			str = args[0].String()
		}

		var segments []*jsvalue.JSValue
		for i := 0; i < len(str); {
			r, size := utf8.DecodeRuneInString(str[i:])
			seg := jsvalue.NewObject()
			seg.Set("segment", jsvalue.NewString(string(r)))
			seg.Set("index", jsvalue.NewNumber(float64(i)))
			segments = append(segments, seg)
			i += size
		}
		return jsvalue.NewArray(segments...)
	}))

	return nil
}, nil)
