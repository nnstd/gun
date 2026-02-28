package object

import (
	"fmt"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// Prototype is the Object.prototype singleton.
var Prototype = jsvalue.ObjectPrototype

func Keys(obj *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.Keys(obj)
}

func Values(obj *jsvalue.JSValue) *jsvalue.JSValue {
	if obj == nil {
		return jsvalue.NewArray()
	}
	keys := obj.OwnKeys()
	result := make([]*jsvalue.JSValue, 0, len(keys))
	for _, key := range keys {
		result = append(result, obj.Get(key))
	}
	return jsvalue.NewArray(result...)
}

func Entries(obj *jsvalue.JSValue) *jsvalue.JSValue {
	if obj == nil {
		return jsvalue.NewArray()
	}
	keys := obj.OwnKeys()
	result := make([]*jsvalue.JSValue, 0, len(keys))
	for _, key := range keys {
		entry := jsvalue.NewArray(jsvalue.NewString(key), obj.Get(key))
		result = append(result, entry)
	}
	return jsvalue.NewArray(result...)
}

func Assign(target any, sources ...any) *jsvalue.JSValue {
	t := toJSValue(target)
	for _, src := range sources {
		s := toJSValue(src)
		if s == nil {
			continue
		}
		for _, key := range s.OwnKeys() {
			t.Set(key, s.Get(key))
		}
	}
	return t
}

func toJSValue(v any) *jsvalue.JSValue {
	if v == nil {
		return jsvalue.NewObject()
	}
	if jv, ok := v.(*jsvalue.JSValue); ok {
		return jv
	}
	return jsvalue.From(v)
}

// Create creates a new object with the specified prototype.
func Create(proto *jsvalue.JSValue) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	if proto != nil {
		obj.SetPrototype(proto)
	}
	return obj
}

// SetPrototypeOf sets the prototype of an object.
func SetPrototypeOf(obj, proto *jsvalue.JSValue) *jsvalue.JSValue {
	if obj != nil {
		obj.SetPrototype(proto)
	}
	return obj
}

// GetPrototypeOf returns the prototype of an object.
func GetPrototypeOf(obj *jsvalue.JSValue) *jsvalue.JSValue {
	if obj != nil {
		return obj.GetPrototype()
	}
	return jsvalue.NewNull()
}

// Freeze prevents modification of existing property values and prevents
// addition of new properties. Returns the frozen object.
func Freeze(obj *jsvalue.JSValue) *jsvalue.JSValue {
	// Stub for now — full implementation would set all properties to non-writable
	return obj
}

func DefineProperty(obj any, prop any, desc any) *jsvalue.JSValue {
	o := toJSValue(obj)
	d := toJSValue(desc)
	p := fmt.Sprint(prop)
	if d != nil {
		val := d.Get("value")
		if val != nil {
			o.Set(p, val)
		}
	}
	return o
}
