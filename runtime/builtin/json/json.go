package json

import (
	stdjson "encoding/json"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func Stringify(v *jsvalue.JSValue) *jsvalue.JSValue {
	b, _ := stdjson.Marshal(jsvalToNative(v))
	return jsvalue.NewString(string(b))
}

func Parse(s *jsvalue.JSValue) *jsvalue.JSValue {
	str := ""
	if s != nil {
		str = s.String()
	}
	var v any
	stdjson.Unmarshal([]byte(str), &v)
	return nativeToJSVal(v)
}

// nativeToJSVal converts a native Go value (from json.Unmarshal) to a JSValue.
func nativeToJSVal(v any) *jsvalue.JSValue {
	if v == nil {
		return jsvalue.NewNull()
	}
	switch val := v.(type) {
	case string:
		return jsvalue.NewString(val)
	case float64:
		return jsvalue.NewNumber(val)
	case bool:
		return jsvalue.NewBool(val)
	case []any:
		elems := make([]*jsvalue.JSValue, len(val))
		for i, elem := range val {
			elems[i] = nativeToJSVal(elem)
		}
		return jsvalue.NewArray(elems...)
	case map[string]any:
		obj := jsvalue.NewObject()
		for k, v := range val {
			obj.Set(k, nativeToJSVal(v))
		}
		return obj
	default:
		return jsvalue.From(v)
	}
}

// jsvalToNative converts a JSValue to a native Go value for JSON marshaling.
func jsvalToNative(v *jsvalue.JSValue) any {
	if v == nil {
		return nil
	}
	switch v.TypeString() {
	case "string":
		return v.String()
	case "number":
		return v.Number()
	case "boolean":
		return v.Bool()
	case "null":
		return nil
	case "undefined":
		return nil
	case "object":
		if v.IsArray() {
			arr := make([]any, v.Len())
			for i := 0; i < v.Len(); i++ {
				arr[i] = jsvalToNative(v.Index(i))
			}
			return arr
		}
		m := make(map[string]any)
		for _, k := range v.OwnKeys() {
			m[k] = jsvalToNative(v.Get(k))
		}
		return m
	default:
		return v.String()
	}
}
