package jsvalue

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
)

// ValueType represents the JavaScript type tag.
type ValueType int

const (
	TypeUndefined ValueType = iota
	TypeNull
	TypeBoolean
	TypeNumber
	TypeBigInt
	TypeString
	TypeSymbol
	TypeObject
	TypeFunction
	TypeRegex
	TypeMap
	TypeSet
)

// JSValue models a JavaScript value with typed storage and prototype chain.
type JSValue struct {
	typ        ValueType
	boolVal    bool
	numVal     float64
	strVal     string
	intVal     int
	bigIntVal  int64
	symbolDesc string
	symbolID   uint64
	properties map[string]*PropertyDescriptor
	prototype  *JSValue
	funcVal    func(...*JSValue) *JSValue
	arrayVal   []*JSValue
	regexVal   interface{} // stores GoRegex (see jsregex.go)
	mapVal     *jsMap
	setVal     *jsSet
	isMethod   bool // true for class methods that expect this as _args[0]
	classInit  func(this *JSValue, args ...*JSValue) *JSValue // raw constructor for super() calls
}

// MarkAsMethod marks this function as a class method that expects 'this'
// as the first argument when called via MethodCall.
func (v *JSValue) MarkAsMethod() *JSValue {
	if v != nil {
		v.isMethod = true
	}
	return v
}

var symbolCounter uint64

// NewString creates a string JSValue.
func NewString(s string) *JSValue {
	return &JSValue{typ: TypeString, strVal: s, prototype: StringPrototype}
}

// NewNumber creates a number JSValue.
func NewNumber(f float64) *JSValue {
	return &JSValue{typ: TypeNumber, numVal: f, prototype: NumberPrototype}
}

// NewInt creates a number JSValue from an int.
func NewInt(i int) *JSValue {
	return &JSValue{typ: TypeNumber, numVal: float64(i), intVal: i, prototype: NumberPrototype}
}

// NewBigInt creates a bigint JSValue.
func NewBigInt(i int64) *JSValue {
	return &JSValue{typ: TypeBigInt, bigIntVal: i, prototype: BigIntPrototype}
}

// NewBool creates a boolean JSValue.
func NewBool(b bool) *JSValue {
	return &JSValue{typ: TypeBoolean, boolVal: b, prototype: BooleanPrototype}
}

// NewSymbol creates a symbol JSValue with a unique ID.
func NewSymbol(description string) *JSValue {
	id := atomic.AddUint64(&symbolCounter, 1)
	return &JSValue{typ: TypeSymbol, symbolDesc: description, symbolID: id, prototype: SymbolPrototype}
}

// PropertyKey returns a string suitable for use as a property key on JSValue objects.
// For Symbols, includes the internal ID to guarantee uniqueness (since Symbol('a') !== Symbol('a')).
// For all other types, returns the standard string representation.
// Accepts any type for convenience — non-JSValue values use fmt.Sprint.
func PropertyKey(v any) string {
	if jsv, ok := v.(*JSValue); ok && jsv != nil && jsv.typ == TypeSymbol {
		return fmt.Sprintf("@@sym%d:%s", jsv.symbolID, jsv.symbolDesc)
	}
	return fmt.Sprint(v)
}

// NewNull creates a null JSValue.
func NewNull() *JSValue {
	return &JSValue{typ: TypeNull}
}

// NewUndefined creates an undefined JSValue.
func NewUndefined() *JSValue {
	return &JSValue{typ: TypeUndefined}
}

// NewObject creates an empty object JSValue.
func NewObject() *JSValue {
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
}

// NewFunction creates a function JSValue.
func NewFunction(fn func(...*JSValue) *JSValue) *JSValue {
	return &JSValue{
		typ:        TypeFunction,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  FunctionPrototype,
		funcVal:    fn,
	}
}

// Type returns the pre-computed type tag.
func (v *JSValue) Type() ValueType {
	return v.typ
}

// TypeString returns the JavaScript typeof string.
func (v *JSValue) TypeString() string {
	switch v.typ {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "object"
	case TypeBoolean:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeBigInt:
		return "bigint"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeObject:
		return "object"
	case TypeFunction:
		return "function"
	case TypeRegex:
		return "object" // In JavaScript, typeof /regex/ === "object"
	case TypeMap, TypeSet:
		return "object"
	default:
		return "undefined"
	}
}

// String returns the string value (or a string representation).
func (v *JSValue) String() string {
	switch v.typ {
	case TypeString:
		return v.strVal
	case TypeNumber:
		if v.numVal == float64(int64(v.numVal)) {
			return fmt.Sprintf("%d", int64(v.numVal))
		}
		return fmt.Sprintf("%g", v.numVal)
	case TypeBigInt:
		return fmt.Sprintf("%d", v.bigIntVal)
	case TypeBoolean:
		if v.boolVal {
			return "true"
		}
		return "false"
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeSymbol:
		return fmt.Sprintf("Symbol(%s)", v.symbolDesc)
	case TypeObject:
		// Arrays: JS Array.toString() joins elements with commas
		if v.arrayVal != nil {
			strs := make([]string, len(v.arrayVal))
			for i, elem := range v.arrayVal {
				if elem == nil || elem.typ == TypeNull || elem.typ == TypeUndefined {
					strs[i] = ""
				} else {
					strs[i] = elem.String()
				}
			}
			return strings.Join(strs, ",")
		}
		return "[object Object]"
	case TypeFunction:
		return "function"
	case TypeRegex:
		if re, ok := v.regexVal.(interface{ String() string }); ok {
			return re.String()
		}
		return ""
	case TypeMap:
		return "[object Map]"
	case TypeSet:
		return "[object Set]"
	default:
		return "undefined"
	}
}

// Number returns the numeric value. Nil-safe: returns 0 for nil.
func (v *JSValue) Number() float64 {
	if v == nil {
		return 0
	}
	return v.numVal
}

// Bool returns the JavaScript truthiness value. Nil-safe: returns false for nil.
// Follows JS semantics: false, 0, NaN, "", null, undefined are falsy; everything else is truthy.
func (v *JSValue) Bool() bool {
	if v == nil {
		return false
	}
	switch v.typ {
	case TypeBoolean:
		return v.boolVal
	case TypeNumber:
		return v.numVal != 0 && v.numVal == v.numVal // false for 0 and NaN
	case TypeString:
		return v.strVal != ""
	case TypeNull, TypeUndefined:
		return false
	case TypeObject, TypeFunction, TypeRegex, TypeMap, TypeSet:
		return true
	case TypeBigInt:
		return v.bigIntVal != 0
	case TypeSymbol:
		return true
	default:
		return false
	}
}

// Int returns the int value. Nil-safe: returns 0 for nil.
func (v *JSValue) Int() int {
	if v == nil {
		return 0
	}
	return v.intVal
}

// BigInt returns the bigint value.
func (v *JSValue) BigInt() int64 {
	return v.bigIntVal
}

// SymbolDesc returns the symbol description.
func (v *JSValue) SymbolDesc() string {
	return v.symbolDesc
}

// SetPrototype sets the [[Prototype]] internal slot.
// This is the ONLY way to change the prototype chain. Set("__proto__", v)
// creates an own property — it does NOT modify the chain (prototype pollution safe).
func (v *JSValue) SetPrototype(proto *JSValue) {
	v.prototype = proto
}

// GetPrototype returns the [[Prototype]] internal slot.
func (v *JSValue) GetPrototype() *JSValue {
	if v.prototype != nil {
		return v.prototype
	}
	return NewNull()
}

// Array returns the underlying array elements, or nil if not an array.
func (v *JSValue) Array() []*JSValue {
	return v.arrayVal
}

// Index returns the element at position i in an array or string JSValue.
// For arrays: returns the element at position i.
// For strings: returns a single-character string (matching JS "str"[i] semantics).
// Returns undefined if out of bounds or not an array/string.
func (v *JSValue) Index(i int) *JSValue {
	if v.arrayVal != nil && i >= 0 && i < len(v.arrayVal) {
		return v.arrayVal[i]
	}
	if v.typ == TypeString && i >= 0 {
		runes := []rune(v.strVal)
		if i < len(runes) {
			return NewString(string(runes[i]))
		}
	}
	return NewUndefined()
}


// IsArray returns true if the JSValue holds an array.
func (v *JSValue) IsArray() bool {
	return v != nil && v.arrayVal != nil
}

// Len returns the length of the JSValue, matching JavaScript semantics.
// For strings: returns character count (not byte count).
// For arrays: returns element count.
// For objects with a length property: returns that property's numeric value.
// Otherwise: returns 0.
func (v *JSValue) Len() int {
	if v == nil {
		return 0
	}
	switch v.typ {
	case TypeString:
		// JavaScript .length on strings returns character count (UTF-16 code units)
		// In Go, we approximate this with rune count
		return len([]rune(v.strVal))
	case TypeObject:
		if v.arrayVal != nil {
			return len(v.arrayVal)
		}
	case TypeMap:
		if v.mapVal != nil {
			return len(v.mapVal.entries)
		}
	case TypeSet:
		if v.setVal != nil {
			return len(v.setVal.items)
		}
	}
	// Check for length property on objects
	if prop := v.Get("length"); prop != nil && prop.typ == TypeNumber {
		return int(prop.numVal)
	}
	return 0
}


// Call invokes the JSValue as a function with the given arguments.
// Returns undefined if the value is not a function or nil.
func (v *JSValue) Call(args ...*JSValue) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	if v.funcVal != nil {
		return v.funcVal(args...)
	}
	return NewUndefined()
}

// MethodCall invokes a method on a JSValue object with the given arguments.
// For class methods (marked with MarkAsMethod), prepends the receiver as 'this'
// so the method can extract it from _args[0]. For plain functions, passes
// args directly without prepending this.
func (v *JSValue) MethodCall(method string, args ...*JSValue) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	// Handle Function.prototype.apply(thisArg, argsArray) and .call(thisArg, ...args)
	if v.typ == TypeFunction && method == "apply" && len(args) >= 2 {
		argsArray := args[1]
		if argsArray != nil && argsArray.arrayVal != nil {
			return v.Call(argsArray.arrayVal...)
		}
		return v.Call()
	}
	if v.typ == TypeFunction && method == "call" && len(args) >= 1 {
		return v.Call(args[1:]...)
	}
	fn := v.Get(method)
	if fn == nil {
		return NewUndefined()
	}
	// Only prepend 'this' for functions that expect it (class methods)
	if fn.isMethod {
		allArgs := make([]*JSValue, 0, 1+len(args))
		allArgs = append(allArgs, v)
		allArgs = append(allArgs, args...)
		return fn.Call(allArgs...)
	}
	return fn.Call(args...)
}

// CallSuper invokes a class constructor's raw init function on an existing
// 'this' object, implementing JavaScript's super(args) semantics.
// The parent constructor sets properties directly on the child's 'this'.
func (v *JSValue) CallSuper(this *JSValue, args ...*JSValue) {
	if v == nil {
		return
	}
	// Walk the prototype chain to find classInit (handles grandparent constructors)
	for cur := v; cur != nil; cur = cur.prototype {
		if cur.classInit != nil {
			cur.classInit(this, args...)
			return
		}
	}
}


// From wraps an arbitrary Go value as a *JSValue.
// If the value is already a *JSValue, it is returned as-is.
// Go nil maps to undefined (not null) — matching JS semantics where
// unset variables are undefined. Use NewNull() for explicit JS null.
func From(v any) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	switch val := v.(type) {
	case *JSValue:
		if val == nil {
			return NewUndefined()
		}
		return val
	case string:
		return NewString(val)
	case int:
		return NewInt(val)
	case float64:
		return NewNumber(val)
	case bool:
		return NewBool(val)
	case []*JSValue:
		return NewArray(val...)
	case func(...*JSValue) *JSValue:
		return NewFunction(val)
	case map[string]*JSValue:
		obj := NewObject()
		for k, v := range val {
			obj.Set(k, v)
		}
		return obj
	case []string:
		// Handle Go string slices (e.g. from regexp.FindStringSubmatch).
		// nil []string (no regex match) → undefined (falsy).
		// Non-nil []string → JSValue array of strings.
		if val == nil {
			return NewUndefined()
		}
		elems := make([]*JSValue, len(val))
		for i, s := range val {
			elems[i] = NewString(s)
		}
		return NewArray(elems...)
	default:
		// Check if it's a regex (has MatchString method)
		if re, ok := v.(interface{ MatchString(string) bool }); ok {
			return NewRegex(re)
		}
		// Check if it's any Go function and wrap it via reflection
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Func {
			return wrapGoFunc(rv)
		}
		// Nil slices/maps/pointers passed as any are typed nils (not caught by v == nil).
		// Treat them as undefined to match JS semantics (e.g. regex FindStringSubmatch returning nil).
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map || rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
			if rv.IsNil() {
				return NewUndefined()
			}
		}
		return NewString(fmt.Sprint(val))
	}
}

// wrapGoFunc wraps an arbitrary Go function (detected via reflection) into a
// JSValue function. This allows Go functions like fmt.Sprintf to be stored as
// callable JSValue functions and used with .Call(), .MethodCall("apply", ...), etc.
func wrapGoFunc(rv reflect.Value) *JSValue {
	rt := rv.Type()
	return NewFunction(func(args ...*JSValue) *JSValue {
		numIn := rt.NumIn()
		isVariadic := rt.IsVariadic()

		var goArgs []reflect.Value
		if isVariadic {
			// Fixed params first
			fixedCount := numIn - 1
			for i := 0; i < fixedCount; i++ {
				if i < len(args) {
					goArgs = append(goArgs, jsvalueToReflect(args[i], rt.In(i)))
				} else {
					goArgs = append(goArgs, reflect.Zero(rt.In(i)))
				}
			}
			// Variadic params
			elemType := rt.In(numIn - 1).Elem()
			for i := fixedCount; i < len(args); i++ {
				goArgs = append(goArgs, jsvalueToReflect(args[i], elemType))
			}
		} else {
			for i := 0; i < numIn; i++ {
				if i < len(args) {
					goArgs = append(goArgs, jsvalueToReflect(args[i], rt.In(i)))
				} else {
					goArgs = append(goArgs, reflect.Zero(rt.In(i)))
				}
			}
		}

		results := rv.Call(goArgs)
		if len(results) == 0 {
			return NewUndefined()
		}
		return reflectToJSValue(results[0])
	})
}

// jsvalueToReflect converts a *JSValue to a reflect.Value of the target type.
func jsvalueToReflect(v *JSValue, t reflect.Type) reflect.Value {
	// If the target type is *JSValue, pass through directly
	jsvalueType := reflect.TypeOf((*JSValue)(nil))
	if t == jsvalueType {
		if v == nil {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(v)
	}
	if v == nil || v.typ == TypeNull || v.typ == TypeUndefined {
		return reflect.Zero(t)
	}
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(v.String())
	case reflect.Int:
		return reflect.ValueOf(int(v.Number()))
	case reflect.Int64:
		return reflect.ValueOf(int64(v.Number()))
	case reflect.Float64:
		return reflect.ValueOf(v.Number())
	case reflect.Float32:
		return reflect.ValueOf(float32(v.Number()))
	case reflect.Bool:
		return reflect.ValueOf(v.Bool())
	case reflect.Interface:
		// Convert to most natural Go value for interface{}/any params
		switch v.typ {
		case TypeString:
			return reflect.ValueOf(v.strVal)
		case TypeNumber:
			n := v.numVal
			if n == float64(int64(n)) {
				return reflect.ValueOf(int(n))
			}
			return reflect.ValueOf(n)
		case TypeBoolean:
			return reflect.ValueOf(v.boolVal)
		default:
			return reflect.ValueOf(v.String())
		}
	case reflect.Ptr:
		// For pointer types (e.g. *SomeStruct), pass the JSValue as-is if it's the right type
		if reflect.TypeOf(v).AssignableTo(t) {
			return reflect.ValueOf(v)
		}
		return reflect.Zero(t)
	default:
		// Try direct assignment first
		if reflect.TypeOf(v.String()).ConvertibleTo(t) {
			return reflect.ValueOf(v.String()).Convert(t)
		}
		return reflect.Zero(t)
	}
}

// reflectToJSValue converts a reflect.Value back to a *JSValue.
func reflectToJSValue(rv reflect.Value) *JSValue {
	if !rv.IsValid() {
		return NewUndefined()
	}
	i := rv.Interface()
	return From(i)
}


// Truthy implements JavaScript truthiness semantics.
// Returns false for nil, undefined, null, false, 0, NaN, and "".
func Truthy(v *JSValue) bool {
	if v == nil {
		return false
	}
	switch v.typ {
	case TypeUndefined, TypeNull:
		return false
	case TypeBoolean:
		return v.boolVal
	case TypeNumber:
		return v.numVal != 0 && v.numVal == v.numVal // NaN != NaN
	case TypeString:
		return v.strVal != ""
	default:
		return true
	}
}


// IsTruthy returns true if the JSValue is truthy in JavaScript semantics.
// This is a method version of the Truthy function.
func (v *JSValue) IsTruthy() bool {
	return Truthy(v)
}

// ToLowerCase converts a JSValue to lowercase string and wraps it.
func ToLowerCase(val *JSValue) *JSValue {
	return NewString(strings.ToLower(fmt.Sprint(val)))
}

// ToUpperCase converts a JSValue to uppercase string and wraps it.
func ToUpperCase(val *JSValue) *JSValue {
	return NewString(strings.ToUpper(fmt.Sprint(val)))
}

// Trim trims whitespace from a JSValue string.
func Trim(val *JSValue) *JSValue {
	return NewString(strings.TrimSpace(fmt.Sprint(val)))
}

// Split splits a JSValue string by separator.
func Split(val *JSValue, sep *JSValue) *JSValue {
	s := fmt.Sprint(val)
	// Regex separator: use GoRegex.Split
	if sep != nil && sep.typ == TypeRegex && sep.regexVal != nil {
		if re, ok := sep.regexVal.(GoRegex); ok {
			parts := re.Split(s, -1)
			return FromStrings(parts)
		}
	}
	// String separator
	sepStr := ","
	if sep != nil {
		sepStr = sep.String()
	}
	parts := strings.Split(s, sepStr)
	return FromStrings(parts)
}

// Replace replaces occurrences of pattern with replacement in a JSValue string.
// pattern can be a string JSValue or a regex JSValue.
func Replace(val *JSValue, pattern, replacement *JSValue) *JSValue {
	s := fmt.Sprint(val)
	repl := ""
	if replacement != nil {
		repl = replacement.String()
	}
	if pattern != nil && pattern.typ == TypeRegex && pattern.regexVal != nil {
		if re, ok := pattern.regexVal.(interface {
			ReplaceAllString(string, string) string
		}); ok {
			return NewString(re.ReplaceAllString(s, repl))
		}
	}
	old := ""
	if pattern != nil {
		old = pattern.String()
	}
	return NewString(strings.Replace(s, old, repl, -1))
}

// CharAt returns the character at the given index.
func CharAt(val *JSValue, index *JSValue) *JSValue {
	s := fmt.Sprint(val)
	runes := []rune(s)
	idx := 0
	if index != nil {
		idx = int(index.Number())
	}
	if idx < 0 || idx >= len(runes) {
		return NewString("")
	}
	return NewString(string(runes[idx]))
}

// StartsWith checks if a JSValue string starts with prefix.
func StartsWith(val *JSValue, prefix *JSValue) *JSValue {
	p := ""
	if prefix != nil {
		p = prefix.String()
	}
	return NewBool(strings.HasPrefix(fmt.Sprint(val), p))
}

// EndsWith checks if a JSValue string ends with suffix.
func EndsWith(val *JSValue, suffix *JSValue) *JSValue {
	s := ""
	if suffix != nil {
		s = suffix.String()
	}
	return NewBool(strings.HasSuffix(fmt.Sprint(val), s))
}

// Repeat repeats a JSValue string count times.
func Repeat(val *JSValue, count *JSValue) *JSValue {
	n := 0
	if count != nil {
		n = int(count.Number())
	}
	return NewString(strings.Repeat(fmt.Sprint(val), n))
}

// LastIndexOf returns the last index of search in str, starting from position.
func LastIndexOf(str *JSValue, search *JSValue, position ...*JSValue) *JSValue {
	s := fmt.Sprint(str)
	sub := fmt.Sprint(search)
	if len(position) > 0 && position[0] != nil {
		pos := int(position[0].Number())
		if pos < len(s) {
			s = s[:pos+1]
		}
	}
	return NewNumber(float64(strings.LastIndex(s, sub)))
}

// Substring returns the part of the string between start and end indices.
func Substring(str *JSValue, start *JSValue, end ...*JSValue) *JSValue {
	s := []rune(fmt.Sprint(str))
	st := 0
	if start != nil {
		st = int(start.Number())
	}
	if st < 0 {
		st = 0
	}
	if st > len(s) {
		st = len(s)
	}
	e := len(s)
	if len(end) > 0 && end[0] != nil {
		e = int(end[0].Number())
	}
	if e < 0 {
		e = 0
	}
	if e > len(s) {
		e = len(s)
	}
	if st > e {
		st, e = e, st
	}
	return NewString(string(s[st:e]))
}

// ObjectFrom creates an object from alternating key (string) and value (*JSValue) pairs.
func ObjectFrom(pairs ...any) *JSValue {
	obj := NewObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := pairs[i].(string)
		val, _ := pairs[i+1].(*JSValue)
		if val == nil {
			val = NewUndefined()
		}
		obj.Set(key, val)
	}
	return obj
}

// Keys returns the keys of an object as a JSValue array.
// Implements Object.keys() semantics.
func Keys(obj *JSValue) *JSValue {
	if obj == nil {
		return NewArray()
	}
	keys := obj.OwnKeys()
	result := make([]*JSValue, len(keys))
	for i, key := range keys {
		result[i] = NewString(key)
	}
	return NewArray(result...)
}

// ---------------------------------------------------------------------------
// Arithmetic operations
// ---------------------------------------------------------------------------

// Add implements the JavaScript + operator.
// If either operand is a string, concatenates; otherwise numeric addition.
func Add(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString || b.typ == TypeString {
		return NewString(a.String() + b.String())
	}
	return NewNumber(a.Number() + b.Number())
}

// Sub implements the JavaScript - operator.
func Sub(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(a.Number() - b.Number())
}

// Mul implements the JavaScript * operator.
func Mul(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(a.Number() * b.Number())
}

// Div implements the JavaScript / operator.
func Div(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	bv := b.Number()
	if bv == 0 {
		av := a.Number()
		if av > 0 {
			return NewNumber(math.Inf(1))
		} else if av < 0 {
			return NewNumber(math.Inf(-1))
		}
		return NewNumber(math.NaN())
	}
	return NewNumber(a.Number() / bv)
}

// Mod implements the JavaScript % operator.
func Mod(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(math.Mod(a.Number(), b.Number()))
}

// Neg implements the JavaScript unary - operator.
func Neg(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(math.NaN())
	}
	return NewNumber(-a.Number())
}

// BitNot implements the JavaScript ~ operator.
func BitNot(a *JSValue) *JSValue {
	if a == nil {
		return NewInt(-1)
	}
	return NewInt(^int(a.Number()))
}

// BitAnd implements the JavaScript & operator.
func BitAnd(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) & int(b.Number()))
}

// BitOr implements the JavaScript | operator.
func BitOr(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) | int(b.Number()))
}

// BitXor implements the JavaScript ^ operator.
func BitXor(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) ^ int(b.Number()))
}

// Shl implements the JavaScript << operator.
func Shl(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) << uint(b.Number()))
}

// Shr implements the JavaScript >> operator.
func Shr(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) >> uint(b.Number()))
}

// UShr implements the JavaScript >>> operator (unsigned right shift).
func UShr(a, b *JSValue) *JSValue {
	return NewInt(int(uint32(a.Number()) >> uint(b.Number())))
}

// ---------------------------------------------------------------------------
// Comparison operations (all return *JSValue boolean)
// ---------------------------------------------------------------------------

// Eq implements JavaScript === (strict equality).
func Eq(a, b *JSValue) *JSValue {
	if a == nil && b == nil {
		return NewBool(true)
	}
	if a == nil || b == nil {
		return NewBool(false)
	}
	if a.typ != b.typ {
		return NewBool(false)
	}
	switch a.typ {
	case TypeUndefined, TypeNull:
		return NewBool(true)
	case TypeBoolean:
		return NewBool(a.boolVal == b.boolVal)
	case TypeNumber:
		return NewBool(a.numVal == b.numVal)
	case TypeString:
		return NewBool(a.strVal == b.strVal)
	case TypeSymbol:
		return NewBool(a.symbolID == b.symbolID)
	default:
		return NewBool(a == b) // reference equality for objects
	}
}

// NEq implements JavaScript !== (strict inequality).
func NEq(a, b *JSValue) *JSValue {
	return NewBool(!Eq(a, b).boolVal)
}

// EqLoose implements JavaScript == (abstract/loose equality).
// Key difference from Eq (===): null == undefined is true.
func EqLoose(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	// null == undefined (and vice versa) is true
	if (a.typ == TypeNull || a.typ == TypeUndefined) && (b.typ == TypeNull || b.typ == TypeUndefined) {
		return NewBool(true)
	}
	// For other types, fall back to strict equality
	return Eq(a, b)
}

// NEqLoose implements JavaScript != (abstract/loose inequality).
func NEqLoose(a, b *JSValue) *JSValue {
	return NewBool(!EqLoose(a, b).boolVal)
}

// Lt implements JavaScript < operator.
func Lt(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal < b.strVal)
	}
	return NewBool(a.Number() < b.Number())
}

// Gt implements JavaScript > operator.
func Gt(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal > b.strVal)
	}
	return NewBool(a.Number() > b.Number())
}

// LtE implements JavaScript <= operator.
func LtE(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal <= b.strVal)
	}
	return NewBool(a.Number() <= b.Number())
}

// GtE implements JavaScript >= operator.
func GtE(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal >= b.strVal)
	}
	return NewBool(a.Number() >= b.Number())
}

// ---------------------------------------------------------------------------
// Logical operations
// ---------------------------------------------------------------------------

// Not implements JavaScript ! operator (returns *JSValue boolean).
// Accepts *JSValue or native bool.
func Not(a any) *JSValue {
	switch v := a.(type) {
	case *JSValue:
		return NewBool(!Truthy(v))
	case bool:
		return NewBool(!v)
	default:
		return NewBool(true) // falsy for unknown types
	}
}

// And implements JavaScript && operator (short-circuit: returns a if falsy, else b).
func And(a, b *JSValue) *JSValue {
	if !Truthy(a) {
		return a
	}
	return b
}

// Or implements JavaScript || operator (short-circuit: returns a if truthy, else b).
func Or(a, b *JSValue) *JSValue {
	if Truthy(a) {
		return a
	}
	return b
}

// Nullish implements JavaScript ?? operator (returns a if not null/undefined, else b).
func Nullish(a, b *JSValue) *JSValue {
	if a == nil || a.typ == TypeUndefined || a.typ == TypeNull {
		return b
	}
	return a
}

// ---------------------------------------------------------------------------
// Increment/Decrement
// ---------------------------------------------------------------------------

// Inc implements JavaScript ++ (prefix/postfix increment).
func Inc(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(1)
	}
	return NewNumber(a.Number() + 1)
}

// Dec implements JavaScript -- (prefix/postfix decrement).
func Dec(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(-1)
	}
	return NewNumber(a.Number() - 1)
}

// ---------------------------------------------------------------------------
// Typeof
// ---------------------------------------------------------------------------

// TypeOf implements JavaScript typeof operator. Returns *JSValue string.
func TypeOf(a *JSValue) *JSValue {
	if a == nil {
		return NewString("undefined")
	}
	return NewString(a.TypeString())
}

// ParseInt parses a string as an integer with the given radix, matching JS parseInt().
func ParseInt(s, radix *JSValue) *JSValue {
	str := strings.TrimSpace(fmt.Sprint(s))
	base := int(radix.Number())
	if base == 0 {
		base = 10
	}
	// Handle 0x prefix for hex
	if base == 16 && len(str) > 2 && str[0] == '0' && (str[1] == 'x' || str[1] == 'X') {
		str = str[2:]
	}
	// Parse digit by digit (JS parseInt stops at first invalid char)
	result := 0
	found := false
	neg := false
	i := 0
	if i < len(str) && (str[i] == '-' || str[i] == '+') {
		if str[i] == '-' {
			neg = true
		}
		i++
	}
	for ; i < len(str); i++ {
		c := str[i]
		digit := -1
		switch {
		case c >= '0' && c <= '9':
			digit = int(c - '0')
		case c >= 'a' && c <= 'z':
			digit = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			digit = int(c-'A') + 10
		}
		if digit < 0 || digit >= base {
			break
		}
		result = result*base + digit
		found = true
	}
	if !found {
		return NewNumber(math.NaN())
	}
	if neg {
		result = -result
	}
	return NewNumber(float64(result))
}

// ParseFloat parses a string as a floating-point number, matching JS parseFloat().
func ParseFloat(s *JSValue) *JSValue {
	str := strings.TrimSpace(fmt.Sprint(s))
	// Parse manually to handle JS-style stopping at invalid chars
	var num float64
	n, _ := fmt.Sscanf(str, "%f", &num)
	if n == 0 {
		return NewNumber(math.NaN())
	}
	return NewNumber(num)
}

// ---------------------------------------------------------------------------
// Class construction
// ---------------------------------------------------------------------------

// NewClass creates a JS class: a constructor function with a prototype object.
// The constructor's "prototype" property holds the prototype that instances inherit from.
// parent is the parent class (or nil for no inheritance).
func NewClass(constructor func(this *JSValue, args ...*JSValue) *JSValue, parent *JSValue) *JSValue {
	proto := NewObject()
	if parent != nil {
		// Inheritance: Child.prototype.__proto__ = Parent.prototype
		parentProto := parent.Get("prototype")
		if parentProto != nil && parentProto.typ != TypeUndefined {
			proto.SetPrototype(parentProto)
		}
	}

	ctor := NewFunction(func(args ...*JSValue) *JSValue {
		// new Class(args): create instance with prototype chain
		instance := NewObject()
		instance.SetPrototype(proto)
		result := constructor(instance, args...)
		// If constructor explicitly returns an object, use it; otherwise use instance
		if result != nil && result.typ == TypeObject {
			return result
		}
		return instance
	})

	// Store the raw constructor for super() calls via CallSuper
	ctor.classInit = constructor

	// Set up Class.prototype and Class.prototype.constructor
	ctor.Set("prototype", proto)
	proto.Set("constructor", ctor)

	// Inherit static methods from parent
	if parent != nil {
		ctor.SetPrototype(parent)
	}

	return ctor
}

