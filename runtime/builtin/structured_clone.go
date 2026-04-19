package jsvalue

// StructuredClone performs a deep clone of v following the HTML structured clone algorithm.
//
// options (optional) may contain a "transfer" array of transferable objects. Each item
// in the transfer list is moved (not copied) into the result — the original is detached
// (neutered) after the clone completes.
//
// Throws (panics) with DataCloneError for non-cloneable types (functions, symbols).
// Circular references are preserved correctly.
func StructuredClone(v *JSValue, options ...*JSValue) *JSValue {
	seen := map[*JSValue]*JSValue{}
	clone := structuredCloneInner(v, seen)

	// Process transfer list from options.transfer
	if len(options) > 0 && options[0] != nil {
		transfer := options[0].Get("transfer")
		if transfer != nil && transfer.isArr {
			n := transfer.arrayVal.Len()
			for i := 0; i < n; i++ {
				orig := transfer.arrayVal.Get(i)
				if orig == nil {
					continue
				}
				// The clone of orig is already in seen (cloned as part of v's graph).
				// Detach the original by clearing its mutable state.
				detach(orig)
			}
		}
	}

	return clone
}

// detach neuters a transferable JSValue in-place, matching browser behavior where
// the original becomes unusable after transfer (e.g. ArrayBuffer.byteLength === 0).
func detach(v *JSValue) {
	if v == nil {
		return
	}
	v.lock()
	defer v.unlock()
	switch v.typ {
	case TypeObject:
		if v.isArr {
			v.arrayVal.ReplaceAll(nil)
		} else {
			v.properties = SmallPropMap{}
		}
	case TypeMap:
		if v.mapVal != nil {
			v.mapVal.entries = nil
		}
	case TypeSet:
		if v.setVal != nil {
			v.setVal.items = nil
		}
	}
	// Bump gen so any cached Gets are invalidated.
	v.gen.Add(1)
}

func structuredCloneInner(v *JSValue, seen map[*JSValue]*JSValue) *JSValue {
	if v == nil {
		return nil
	}
	switch v.typ {
	case TypeUndefined:
		return NewUndefined()
	case TypeNull:
		return NewNull()
	case TypeBoolean:
		return NewBool(v.boolVal)
	case TypeNumber:
		return NewNumber(v.numVal)
	case TypeBigInt:
		return &JSValue{typ: TypeBigInt, bigIntVal: v.bigIntVal}
	case TypeString:
		return NewString(v.strVal)
	case TypeSymbol:
		panic("DataCloneError: Symbol cannot be cloned")
	case TypeFunction:
		panic("DataCloneError: function cannot be cloned")
	case TypeRegex:
		// Regex is treated as a cloneable object in the spec.
		return &JSValue{typ: TypeRegex, regexVal: v.regexVal, prototype: RegexpPrototype}
	case TypeMap:
		if existing, ok := seen[v]; ok {
			return existing
		}
		clone := NewMap()
		seen[v] = clone
		if v.mapVal != nil {
			for _, e := range v.mapVal.entries {
				clonedKey := structuredCloneInner(e.key, seen)
				clonedVal := structuredCloneInner(e.value, seen)
				clone.mapVal.entries = append(clone.mapVal.entries, &jsMapEntry{clonedKey, clonedVal})
			}
		}
		return clone
	case TypeSet:
		if existing, ok := seen[v]; ok {
			return existing
		}
		clone := NewSet()
		seen[v] = clone
		if v.setVal != nil {
			for _, item := range v.setVal.items {
				clone.setVal.items = append(clone.setVal.items, structuredCloneInner(item, seen))
			}
		}
		return clone
	case TypeObject:
		if existing, ok := seen[v]; ok {
			return existing
		}
		var clone *JSValue
		if v.isArr {
			clone = NewArray()
			seen[v] = clone
			n := v.arrayVal.Len()
			for i := 0; i < n; i++ {
				clone.arrayVal.Push(structuredCloneInner(v.arrayVal.Get(i), seen))
			}
		} else {
			clone = NewObject()
			seen[v] = clone
		}
		// Clone own properties (excluding prototype-inherited ones).
		for _, key := range v.OwnKeys() {
			val := v.Get(key)
			if val != nil {
				clone.Set(key, structuredCloneInner(val, seen))
			}
		}
		return clone
	default:
		return NewUndefined()
	}
}
