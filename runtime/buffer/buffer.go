package buffer

import (
	"encoding/base64"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/web"
)

var Buffer *jsvalue.JSValue
var Blob *jsvalue.JSValue

func newBufferInstance(data string) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.SetPrototype(Buffer.Get("prototype"))
	obj.Set("_data", jsvalue.NewString(data))
	obj.Set("length", jsvalue.NewNumber(float64(len([]byte(data)))))
	return obj
}

func bufferData(v *jsvalue.JSValue) string {
	if v == nil {
		return ""
	}
	if data := v.Get("_data"); data != nil && data.TypeString() == "string" {
		return data.String()
	}
	return v.String()
}

func init() {
	Buffer = jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferInstance("")
		}
		return newBufferInstance(args[0].String())
	})
	Buffer.Set("prototype", jsvalue.NewObject())
	Buffer.Get("prototype").Set("constructor", Buffer)
	Buffer.Get("prototype").Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewString("")
		}
		return jsvalue.NewString(bufferData(args[0]))
	}).MarkAsMethod())
	Buffer.Get("prototype").Set("slice", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferInstance("")
		}
		data := []rune(bufferData(args[0]))
		start := 0
		end := len(data)
		if len(args) > 1 && args[1] != nil {
			start = int(args[1].Number())
		}
		if len(args) > 2 && args[2] != nil {
			end = int(args[2].Number())
		}
		if start < 0 {
			start = 0
		}
		if end > len(data) {
			end = len(data)
		}
		if end < start {
			end = start
		}
		return newBufferInstance(string(data[start:end]))
	}).MarkAsMethod())

	Buffer.Set("from", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return newBufferInstance("")
		}
		return newBufferInstance(args[0].String())
	}))
	Buffer.Set("alloc", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		size := 0
		fill := "\x00"
		if len(args) > 0 && args[0] != nil {
			size = int(args[0].Number())
		}
		if len(args) > 1 && args[1] != nil {
			fill = args[1].String()
			if fill == "" {
				fill = "\x00"
			}
		}
		return newBufferInstance(strings.Repeat(string(fill[0]), size))
	}))
	Buffer.Set("isBuffer", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewBool(false)
		}
		return jsvalue.InstanceOf(args[0], Buffer)
	}))
	Buffer.Set("byteLength", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(float64(len([]byte(args[0].String()))))
	}))
	Buffer.Set("concat", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil || !args[0].IsArray() {
			return newBufferInstance("")
		}
		var b strings.Builder
		for _, item := range args[0].Array() {
			b.WriteString(bufferData(item))
		}
		return newBufferInstance(b.String())
	}))

	Blob = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		parts := jsvalue.NewArray()
		if len(args) > 0 && args[0] != nil {
			parts = args[0]
		}
		this.Set("parts", parts)
		this.Set("size", jsvalue.NewNumber(float64(parts.Len())))
		this.Set("type", jsvalue.NewString(""))
		return nil
	}, nil)

	AsJSValue = jsvalue.ObjectFrom(
		"Buffer", Buffer,
		"Blob", Blob,
		"File", web.File,
		"atob", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewString("")
			}
			data, err := base64.StdEncoding.DecodeString(args[0].String())
			if err != nil {
				return jsvalue.NewString("")
			}
			return jsvalue.NewString(string(data))
		}),
		"btoa", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			if len(args) == 0 || args[0] == nil {
				return jsvalue.NewString("")
			}
			return jsvalue.NewString(base64.StdEncoding.EncodeToString([]byte(args[0].String())))
		}),
	)
}

var AsJSValue *jsvalue.JSValue
