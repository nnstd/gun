package jsvalue

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
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
	regexVal   interface{} // stores *regexp.Regexp to avoid import cycle
	mapVal     *jsMap
	setVal     *jsSet
	isMethod   bool // true for class methods that expect this as _args[0]
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

// NewArray creates an array JSValue (stored as TypeObject).
func NewArray(elems ...*JSValue) *JSValue {
	arr := elems
	if arr == nil {
		arr = []*JSValue{}
	}
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ArrayPrototype,
		arrayVal:   arr,
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

// NewRegex creates a regex JSValue from a compiled regexp.
// The regex parameter should be *regexp.Regexp but is typed as interface{}
// to avoid import cycles. The returned JSValue has test() and exec() methods.
func NewRegex(regex interface{}) *JSValue {
	v := &JSValue{
		typ:        TypeRegex,
		properties: make(map[string]*PropertyDescriptor),
		regexVal:   regex,
	}
	// Add test() method: regex.test(str) → boolean
	v.Set("test", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 {
			return NewBool(false)
		}
		return NewBool(v.MatchString(args[0]))
	}))
	// Add exec() method: regex.exec(str) → array of matches or null
	v.Set("exec", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 || v.regexVal == nil {
			return NewNull()
		}
		str := fmt.Sprint(args[0])
		if re, ok := v.regexVal.(interface {
			FindStringSubmatch(string) []string
		}); ok {
			matches := re.FindStringSubmatch(str)
			if matches == nil {
				return NewNull()
			}
			return FromStrings(matches)
		}
		return NewNull()
	}))
	return v
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

// Index returns the element at position i in an array JSValue.
// Returns undefined if out of bounds or not an array.
func (v *JSValue) Index(i int) *JSValue {
	if v.arrayVal != nil && i >= 0 && i < len(v.arrayVal) {
		return v.arrayVal[i]
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

// MatchString tests whether the JSValue (as a regex) matches the given value.
// This is a core bridge method for regex.test() when the regex is a JSValue.
// The argument is coerced to string if needed.
// Returns false if the JSValue is not a regex.
func (v *JSValue) MatchString(s any) bool {
	if v == nil || v.typ != TypeRegex || v.regexVal == nil {
		return false
	}

	// Coerce argument to string
	var str string
	switch val := s.(type) {
	case string:
		str = val
	case *JSValue:
		str = val.String()
	default:
		str = fmt.Sprint(val)
	}

	// Type assert to *regexp.Regexp
	// We use interface{} in the struct to avoid import cycles
	if re, ok := v.regexVal.(interface{ MatchString(string) bool }); ok {
		return re.MatchString(str)
	}
	return false
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

// ToSlice converts an any value to []*JSValue. Handles []*JSValue passthrough
// and *JSValue (via .Array()). Used when an IIFE returns any but the target is []*JSValue.
func ToSlice(v any) []*JSValue {
	switch val := v.(type) {
	case []*JSValue:
		return val
	case *JSValue:
		return val.Array()
	default:
		return nil
	}
}


// From wraps an arbitrary Go value as a *JSValue.
// If the value is already a *JSValue, it is returned as-is.
func From(v any) *JSValue {
	if v == nil {
		return NewNull()
	}
	switch val := v.(type) {
	case *JSValue:
		return val
	case string:
		return NewString(val)
	case int:
		return NewInt(val)
	case float64:
		return NewNumber(val)
	case bool:
		return NewBool(val)
	case func(...*JSValue) *JSValue:
		return NewFunction(val)
	case map[string]*JSValue:
		obj := NewObject()
		for k, v := range val {
			obj.Set(k, v)
		}
		return obj
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

// FromStrings converts a []string into an array JSValue.
func FromStrings(ss []string) *JSValue {
	elems := make([]*JSValue, len(ss))
	for i, s := range ss {
		elems[i] = NewString(s)
	}
	return NewArray(elems...)
}

// SpreadIntoArray spreads a JSValue into array elements.
// For arrays, returns the elements as-is (like [...arr]).
// For strings, splits into individual characters (like [...str]).
// For other types, returns a single-element slice.
func SpreadIntoArray(v *JSValue) []*JSValue {
	if v == nil {
		return nil
	}
	if v.arrayVal != nil {
		return v.arrayVal
	}
	if v.typ == TypeString {
		runes := []rune(v.strVal)
		elems := make([]*JSValue, len(runes))
		for i, r := range runes {
			elems[i] = NewString(string(r))
		}
		return elems
	}
	return []*JSValue{v}
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

// Splice removes/replaces elements in an array and returns the removed elements.
// Implements JavaScript Array.prototype.splice(start, deleteCount, ...items).
func Splice(arrAny any, args ...*JSValue) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewArray()
	}
	length := len(arr.arrayVal)
	start := 0
	if len(args) >= 1 && args[0] != nil {
		start = int(args[0].Number())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		}
		if start > length {
			start = length
		}
	}
	deleteCount := length - start
	if len(args) >= 2 && args[1] != nil {
		deleteCount = int(args[1].Number())
		if deleteCount < 0 {
			deleteCount = 0
		}
		if start+deleteCount > length {
			deleteCount = length - start
		}
	}
	// Extract removed elements
	removed := make([]*JSValue, deleteCount)
	copy(removed, arr.arrayVal[start:start+deleteCount])
	// Build new items to insert
	var newItems []*JSValue
	for i := 2; i < len(args); i++ {
		newItems = append(newItems, args[i])
	}
	// Rebuild array: before + newItems + after
	result := make([]*JSValue, 0, length-deleteCount+len(newItems))
	result = append(result, arr.arrayVal[:start]...)
	result = append(result, newItems...)
	result = append(result, arr.arrayVal[start+deleteCount:]...)
	arr.arrayVal = result
	return NewArray(removed...)
}

// asArray converts any to *JSValue, handling []*JSValue from Go rest params.
func asArray(v any) *JSValue {
	switch val := v.(type) {
	case *JSValue:
		return val
	case []*JSValue:
		return NewArray(val...)
	default:
		return nil
	}
}

// Map applies fn to each element and returns a new array.
// fn can be func(*JSValue) *JSValue or func(*JSValue, *JSValue) *JSValue (value, index).
func Map(arrAny any, fn any) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewArray()
	}
	results := make([]*JSValue, len(arr.arrayVal))
	switch f := fn.(type) {
	case func(*JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			results[i] = f(elem)
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			results[i] = f(elem, NewNumber(float64(i)))
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for i, elem := range arr.arrayVal {
				results[i] = f.funcVal(elem, NewNumber(float64(i)))
			}
		}
	}
	return NewArray(results...)
}

// Filter returns a new array containing elements for which fn returns truthy.
// fn can be func(*JSValue) bool or func(*JSValue) *JSValue.
func Filter(arrAny any, fn any) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewArray()
	}
	var results []*JSValue
	switch f := fn.(type) {
	case func(*JSValue) bool:
		for _, elem := range arr.arrayVal {
			if f(elem) {
				results = append(results, elem)
			}
		}
	case func(*JSValue) *JSValue:
		for _, elem := range arr.arrayVal {
			if f(elem).Bool() {
				results = append(results, elem)
			}
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			if f(elem, NewNumber(float64(i))).Bool() {
				results = append(results, elem)
			}
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for i, elem := range arr.arrayVal {
				r := f.funcVal(elem, NewNumber(float64(i)))
				if r != nil && r.Bool() {
					results = append(results, elem)
				}
			}
		}
	}
	return NewArray(results...)
}

// ForEach calls fn for each element in the array.
// fn can be func(*JSValue), func(*JSValue) *JSValue, or 2-param variants.
func ForEach(arrAny any, fn any) {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return
	}
	switch f := fn.(type) {
	case func(*JSValue):
		for _, elem := range arr.arrayVal {
			f(elem)
		}
	case func(*JSValue) *JSValue:
		for _, elem := range arr.arrayVal {
			f(elem)
		}
	case func(*JSValue, *JSValue):
		for i, elem := range arr.arrayVal {
			f(elem, NewNumber(float64(i)))
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			f(elem, NewNumber(float64(i)))
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for i, elem := range arr.arrayVal {
				f.funcVal(elem, NewNumber(float64(i)))
			}
		}
	}
}

// Find returns the first element for which fn returns truthy, or undefined.
func Find(arrAny any, fn any) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewUndefined()
	}
	switch f := fn.(type) {
	case func(*JSValue) *JSValue:
		for _, elem := range arr.arrayVal {
			if Truthy(f(elem)) {
				return elem
			}
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			if Truthy(f(elem, NewNumber(float64(i)))) {
				return elem
			}
		}
	case func(*JSValue) bool:
		for _, elem := range arr.arrayVal {
			if f(elem) {
				return elem
			}
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for _, elem := range arr.arrayVal {
				if Truthy(f.funcVal(elem)) {
					return elem
				}
			}
		}
	}
	return NewUndefined()
}

// Some returns true if at least one element satisfies the predicate.
func Some(arrAny any, fn any) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewBool(false)
	}
	switch f := fn.(type) {
	case func(*JSValue) *JSValue:
		for _, elem := range arr.arrayVal {
			if Truthy(f(elem)) {
				return NewBool(true)
			}
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			if Truthy(f(elem, NewNumber(float64(i)))) {
				return NewBool(true)
			}
		}
	case func(*JSValue) bool:
		for _, elem := range arr.arrayVal {
			if f(elem) {
				return NewBool(true)
			}
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for _, elem := range arr.arrayVal {
				if Truthy(f.funcVal(elem)) {
					return NewBool(true)
				}
			}
		}
	}
	return NewBool(false)
}

// Every returns true if all elements satisfy the predicate.
func Every(arrAny any, fn any) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		return NewBool(true)
	}
	switch f := fn.(type) {
	case func(*JSValue) *JSValue:
		for _, elem := range arr.arrayVal {
			if !Truthy(f(elem)) {
				return NewBool(false)
			}
		}
	case func(*JSValue, *JSValue) *JSValue:
		for i, elem := range arr.arrayVal {
			if !Truthy(f(elem, NewNumber(float64(i)))) {
				return NewBool(false)
			}
		}
	case func(*JSValue) bool:
		for _, elem := range arr.arrayVal {
			if !f(elem) {
				return NewBool(false)
			}
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for _, elem := range arr.arrayVal {
				if !Truthy(f.funcVal(elem)) {
					return NewBool(false)
				}
			}
		}
	}
	return NewBool(true)
}

// Reduce applies fn against an accumulator and each element. Returns the final accumulated value.
func Reduce(arrAny any, fn any, initial ...*JSValue) *JSValue {
	arr := asArray(arrAny)
	if arr == nil || arr.arrayVal == nil {
		if len(initial) > 0 {
			return initial[0]
		}
		return NewUndefined()
	}
	var acc *JSValue
	startIdx := 0
	if len(initial) > 0 {
		acc = initial[0]
	} else if len(arr.arrayVal) > 0 {
		acc = arr.arrayVal[0]
		startIdx = 1
	} else {
		return NewUndefined()
	}
	switch f := fn.(type) {
	case func(*JSValue, *JSValue) *JSValue:
		for i := startIdx; i < len(arr.arrayVal); i++ {
			acc = f(acc, arr.arrayVal[i])
		}
	case func(*JSValue, *JSValue, *JSValue) *JSValue:
		for i := startIdx; i < len(arr.arrayVal); i++ {
			acc = f(acc, arr.arrayVal[i], NewNumber(float64(i)))
		}
	case *JSValue:
		if f != nil && f.funcVal != nil {
			for i := startIdx; i < len(arr.arrayVal); i++ {
				acc = f.funcVal(acc, arr.arrayVal[i])
			}
		}
	}
	return acc
}

// MatchString tests whether a regex JSValue matches the given value.
// This implements JavaScript's regex.test(s) method.
// The value argument is coerced to string if it's a JSValue.
// Returns false if the regex is not a valid regex JSValue.
func MatchString(regex *JSValue, value any) bool {
	if regex == nil || regex.typ != TypeRegex || regex.regexVal == nil {
		return false
	}

	// Coerce argument to string
	var str string
	switch val := value.(type) {
	case string:
		str = val
	case *JSValue:
		str = val.String()
	default:
		str = fmt.Sprint(val)
	}

	// Type assert to *regexp.Regexp
	// We use interface{} in the struct to avoid import cycles
	if re, ok := regex.regexVal.(interface{ MatchString(string) bool }); ok {
		return re.MatchString(str)
	}
	return false
}

// RegexExec executes a regex match and returns an array of matches or null.
// This implements JavaScript's regex.exec(s) method.
func RegexExec(regex *JSValue, value any) *JSValue {
	if regex == nil || regex.typ != TypeRegex || regex.regexVal == nil {
		return NewNull()
	}
	var str string
	switch val := value.(type) {
	case string:
		str = val
	case *JSValue:
		str = val.String()
	default:
		str = fmt.Sprint(val)
	}
	if re, ok := regex.regexVal.(interface {
		FindStringSubmatch(string) []string
	}); ok {
		matches := re.FindStringSubmatch(str)
		if matches == nil {
			return NewNull()
		}
		return FromStrings(matches)
	}
	return NewNull()
}

// IsTruthy returns true if the JSValue is truthy in JavaScript semantics.
// This is a method version of the Truthy function.
func (v *JSValue) IsTruthy() bool {
	return Truthy(v)
}

// normalizeIndex converts negative indices to positive ones.
// In JavaScript, arr[-1] means arr[len(arr)-1].
func normalizeIndex(idx int, length int) int {
	if idx < 0 {
		return length + idx
	}
	return idx
}

// Pop returns the last element of an array, or undefined if empty.
func Pop(arr *JSValue) *JSValue {
	if arr == nil || arr.arrayVal == nil || len(arr.arrayVal) == 0 {
		return NewUndefined()
	}
	last := arr.arrayVal[len(arr.arrayVal)-1]
	arr.arrayVal = arr.arrayVal[:len(arr.arrayVal)-1]
	return last
}

// Join joins array elements into a string with separator.
func Join(arr *JSValue, sep *JSValue) *JSValue {
	if arr == nil || arr.arrayVal == nil {
		return NewString("")
	}
	s := ","
	if sep != nil {
		s = sep.String()
	}
	strs := make([]string, len(arr.arrayVal))
	for i, elem := range arr.arrayVal {
		strs[i] = fmt.Sprint(elem)
	}
	return NewString(strings.Join(strs, s))
}

// Includes checks if array contains a value.
func Includes(arr *JSValue, val *JSValue) *JSValue {
	if arr == nil {
		return NewBool(false)
	}
	// String.prototype.includes(searchString)
	if arr.typ == TypeString {
		if val == nil {
			return NewBool(false)
		}
		return NewBool(strings.Contains(arr.strVal, val.String()))
	}
	// Array.prototype.includes(value) — uses SameValueZero comparison
	if arr.arrayVal == nil {
		return NewBool(false)
	}
	valStr := ""
	if val != nil {
		valStr = val.String()
	}
	for _, elem := range arr.arrayVal {
		if elem == val {
			return NewBool(true)
		}
		// Value comparison for primitives (string, number, boolean)
		if elem != nil && val != nil && elem.typ == val.typ {
			switch elem.typ {
			case TypeString:
				if elem.strVal == valStr {
					return NewBool(true)
				}
			case TypeNumber:
				if elem.numVal == val.numVal {
					return NewBool(true)
				}
			case TypeBoolean:
				if elem.boolVal == val.boolVal {
					return NewBool(true)
				}
			}
		}
	}
	return NewBool(false)
}

// OrDefault implements JavaScript || operator with truthiness semantics.
func OrDefault(val *JSValue, fallback *JSValue) *JSValue {
	if val == nil || !val.IsTruthy() {
		return fallback
	}
	return val
}

// Slice slices array (or string) with support for negative indices.
func Slice(arr *JSValue, args ...*JSValue) *JSValue {
	if arr == nil {
		return NewArray()
	}

	// String slicing
	if arr.typ == TypeString {
		runes := []rune(arr.strVal)
		length := len(runes)
		start := 0
		end := length
		if len(args) >= 1 && args[0] != nil {
			start = normalizeIndex(int(args[0].Number()), length)
		}
		if len(args) >= 2 && args[1] != nil {
			end = normalizeIndex(int(args[1].Number()), length)
		}
		if start < 0 {
			start = 0
		}
		if start > length {
			start = length
		}
		if end < 0 {
			end = 0
		}
		if end > length {
			end = length
		}
		if end < start {
			end = start
		}
		return NewString(string(runes[start:end]))
	}

	// Array slicing
	if arr.arrayVal == nil {
		return NewArray()
	}

	length := len(arr.arrayVal)
	start := 0
	end := length

	if len(args) >= 1 && args[0] != nil {
		start = normalizeIndex(int(args[0].Number()), length)
	}
	if len(args) >= 2 && args[1] != nil {
		end = normalizeIndex(int(args[1].Number()), length)
	}

	// Clamp to valid range
	if start < 0 {
		start = 0
	}
	if start > length {
		start = length
	}
	if end < 0 {
		end = 0
	}
	if end > length {
		end = length
	}
	if end < start {
		end = start
	}

	return NewArray(arr.arrayVal[start:end]...)
}

// Concat concatenates arrays and values.
func Concat(arr *JSValue, items ...*JSValue) *JSValue {
	var result []*JSValue
	if arr != nil && arr.arrayVal != nil {
		result = make([]*JSValue, len(arr.arrayVal))
		copy(result, arr.arrayVal)
	}
	// Flatten array items (JS Array.concat spreads arrays)
	for _, item := range items {
		if item != nil && item.arrayVal != nil {
			result = append(result, item.arrayVal...)
		} else {
			result = append(result, item)
		}
	}
	return NewArray(result...)
}

// Push appends items to array (returns new array in Go).
// arr accepts *JSValue or []*JSValue (from rest params).
func Push(arrAny any, items ...*JSValue) *JSValue {
	arr := asArray(arrAny)
	if arr == nil {
		return NewNumber(float64(len(items)))
	}
	// Mutate in place — JS Array.push modifies the array
	arr.arrayVal = append(arr.arrayVal, items...)
	return NewNumber(float64(len(arr.arrayVal)))
}

// Shift removes and returns first element, or undefined if empty. Mutates in place.
func Shift(arr *JSValue) *JSValue {
	if arr == nil || arr.arrayVal == nil || len(arr.arrayVal) == 0 {
		return NewUndefined()
	}
	first := arr.arrayVal[0]
	arr.arrayVal = arr.arrayVal[1:]
	return first
}

// Unshift prepends items to array. Mutates in place. Returns new length.
func Unshift(arr *JSValue, items ...*JSValue) *JSValue {
	if arr == nil {
		return NewNumber(float64(len(items)))
	}
	arr.arrayVal = append(items, arr.arrayVal...)
	return NewNumber(float64(len(arr.arrayVal)))
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
	s := ","
	if sep != nil {
		s = sep.String()
	}
	parts := strings.Split(fmt.Sprint(val), s)
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

// jsUnicodeEscapeRe matches JS-style \uNNNN Unicode escapes in regex patterns.
var jsUnicodeEscapeRe = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// jsUnicodePropMap maps JS Unicode property names to Go equivalents.
var jsUnicodePropMap = map[string]string{
	"Default_Ignorable_Code_Point": "Cf",
	"Emoji_Presentation":           "So",
	"Extended_Pictographic":        "So",
	"Emoji_Modifier_Base":          "So",
	"Emoji_Modifier":               "Sk",
	"Emoji_Component":              "So",
	"Regional_Indicator":           "So",
}

// jsUnicodePropRe matches \p{PropertyName} or \P{PropertyName} in regex patterns.
var jsUnicodePropRe = regexp.MustCompile(`\\[pP]\{([^}]+)\}`)

// CompileRegex compiles a regex pattern, converting JS-style \uNNNN Unicode
// escapes and unsupported Unicode property names to Go-compatible equivalents.
// If the pattern still fails to compile, returns a regex that matches nothing.
func CompileRegex(pattern string) *regexp.Regexp {
	// Convert \uNNNN escapes to literal characters
	converted := jsUnicodeEscapeRe.ReplaceAllStringFunc(pattern, func(match string) string {
		hex := match[2:] // strip \u prefix
		code, err := strconv.ParseInt(hex, 16, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})
	// Convert unsupported Unicode property names to Go equivalents
	converted = jsUnicodePropRe.ReplaceAllStringFunc(converted, func(match string) string {
		prefix := match[:2]  // \p or \P
		name := match[3 : len(match)-1] // extract property name
		if goName, ok := jsUnicodePropMap[name]; ok {
			return prefix + "{" + goName + "}"
		}
		return match
	})
	re, err := regexp.Compile(converted)
	if err != nil {
		// Fallback: return a regex that matches nothing rather than panicking
		// Use a pattern that's valid in Go RE2 and never matches
		return regexp.MustCompile(`\A\z(?:never)`)
	}
	return re
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

	// Set up Class.prototype and Class.prototype.constructor
	ctor.Set("prototype", proto)
	proto.Set("constructor", ctor)

	// Inherit static methods from parent
	if parent != nil {
		ctor.SetPrototype(parent)
	}

	return ctor
}

// IsArrayValue returns a *JSValue boolean indicating whether v is an array.
// This is the JSValue-returning wrapper for Array.isArray().
func IsArrayValue(v *JSValue) *JSValue {
	if v == nil {
		return NewBool(false)
	}
	return NewBool(v.IsArray())
}
