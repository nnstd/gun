package web

import (
	"path/filepath"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

type formDataEntry struct {
	name  string
	value *jsvalue.JSValue
}

var formDataStates = map[*jsvalue.JSValue][]formDataEntry{}

var FormData *jsvalue.JSValue

func init() {
	FormData = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		formDataStates[this] = nil
		if len(args) > 0 && args[0] != nil && jsvalue.InstanceOf(args[0], FormData).Bool() {
			formDataStates[this] = append([]formDataEntry(nil), formDataEntries(args[0])...)
		}
		return nil
	}, nil)
	proto := FormData.Get("prototype")
	proto.Set("append", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 3 {
			formDataAppend(args[0], args[1].String(), formDataValue(args[2], argAt(args, 3)))
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	proto.Set("delete", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			name := args[1].String()
			entries := formDataEntries(args[0])
			out := entries[:0]
			for _, entry := range entries {
				if entry.name != name {
					out = append(out, entry)
				}
			}
			formDataStates[args[0]] = out
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	proto.Set("get", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			name := args[1].String()
			for _, entry := range formDataEntries(args[0]) {
				if entry.name == name {
					return entry.value
				}
			}
		}
		return jsvalue.NewNull()
	}).MarkAsMethod())
	proto.Set("getAll", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		values := []*jsvalue.JSValue{}
		if len(args) >= 2 {
			name := args[1].String()
			for _, entry := range formDataEntries(args[0]) {
				if entry.name == name {
					values = append(values, entry.value)
				}
			}
		}
		return jsvalue.NewArray(values...)
	}).MarkAsMethod())
	proto.Set("has", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			name := args[1].String()
			for _, entry := range formDataEntries(args[0]) {
				if entry.name == name {
					return jsvalue.NewBool(true)
				}
			}
		}
		return jsvalue.NewBool(false)
	}).MarkAsMethod())
	proto.Set("set", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 3 {
			name := args[1].String()
			value := formDataValue(args[2], argAt(args, 3))
			entries := formDataEntries(args[0])
			out := entries[:0]
			inserted := false
			for _, entry := range entries {
				if entry.name != name {
					out = append(out, entry)
					continue
				}
				if !inserted {
					out = append(out, formDataEntry{name: name, value: value})
					inserted = true
				}
			}
			if !inserted {
				out = append(out, formDataEntry{name: name, value: value})
			}
			formDataStates[args[0]] = out
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	proto.Set("entries", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return formDataArray(args[0], "entries")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	proto.Set("keys", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return formDataArray(args[0], "keys")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	proto.Set("values", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return formDataArray(args[0], "values")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	proto.Set("forEach", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[1] != nil {
			thisArg := jsvalue.NewUndefined()
			if len(args) >= 3 {
				thisArg = args[2]
			}
			for _, entry := range formDataEntries(args[0]) {
				args[1].Call(entry.value, jsvalue.NewString(entry.name), args[0], thisArg)
			}
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	proto.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString("[object FormData]")
	}).MarkAsMethod())
	jsvalue.RegisterGlobal("FormData", FormData)
}

func formDataEntries(v *jsvalue.JSValue) []formDataEntry {
	if v == nil {
		return nil
	}
	return formDataStates[v]
}

func formDataAppend(this *jsvalue.JSValue, name string, value *jsvalue.JSValue) {
	formDataStates[this] = append(formDataStates[this], formDataEntry{name: name, value: value})
}

func formDataValue(value, filename *jsvalue.JSValue) *jsvalue.JSValue {
	if value == nil {
		return jsvalue.NewString("undefined")
	}
	if jsvalue.InstanceOf(value, File).Bool() {
		if filename != nil && filename.TypeString() != "undefined" {
			return cloneFileWithName(value, filename.String())
		}
		return value
	}
	if value.Get("size").TypeString() == "number" && value.Get("type").TypeString() == "string" {
		name := "blob"
		if filename != nil && filename.TypeString() != "undefined" {
			name = filename.String()
		}
		return cloneBlobAsFile(value, name)
	}
	return jsvalue.NewString(value.String())
}

func cloneFileWithName(file *jsvalue.JSValue, name string) *jsvalue.JSValue {
	clone := jsvalue.NewObjectWithPrototype(File.Get("prototype"))
	for _, key := range file.OwnKeys() {
		clone.Set(key, file.Get(key))
	}
	clone.Set("name", jsvalue.NewString(name))
	return clone
}

func cloneBlobAsFile(blob *jsvalue.JSValue, name string) *jsvalue.JSValue {
	clone := jsvalue.NewObjectWithPrototype(File.Get("prototype"))
	clone.Set("name", jsvalue.NewString(filepath.Base(name)))
	clone.Set("lastModified", jsvalue.NewNumber(0))
	clone.Set("type", jsvalue.NewString(strings.ToLower(blob.Get("type").String())))
	clone.Set("size", blob.Get("size"))
	clone.Set("parts", jsvalue.NewArray(blob))
	return clone
}

func formDataArray(this *jsvalue.JSValue, mode string) *jsvalue.JSValue {
	entries := formDataEntries(this)
	out := make([]*jsvalue.JSValue, 0, len(entries))
	for _, entry := range entries {
		switch mode {
		case "keys":
			out = append(out, jsvalue.NewString(entry.name))
		case "values":
			out = append(out, entry.value)
		default:
			out = append(out, jsvalue.NewArray(jsvalue.NewString(entry.name), entry.value))
		}
	}
	return jsvalue.NewArray(out...)
}

func argAt(args []*jsvalue.JSValue, idx int) *jsvalue.JSValue {
	if idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}
