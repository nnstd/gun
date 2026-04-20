package jsvalue

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
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

// cacheEntry is a single inline cache slot for Get() optimization.
// Fixed-size array on JSValue avoids allocation for the cache itself.
// Stores the resolved value directly (not a *PropertyDescriptor pointer) to avoid
// data races when Set() mutates desc.Value in-place on shared objects.
type cacheEntry struct {
	key     string
	value   *JSValue                // cached resolved value (snapshot)
	getter  func(*JSValue) *JSValue // cached accessor (may be nil)
	source  *JSValue                // object where property was found
	gen     uint64                  // source.gen when cached
	recvGen uint64                  // receiver's gen when cached
}

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
	properties SmallPropMap
	prototype  *JSValue
	funcVal    func(...*JSValue) *JSValue
	arrayVal   SmallValueList
	isArr      bool // true when this JSValue has array semantics (even if empty)
	regexVal   any  // stores GoRegex (see jsregex.go)
	mapVal     *jsMap
	setVal     *jsSet
	isMethod   bool                                           // true for class methods that expect this as _args[0]
	classInit  func(this *JSValue, args ...*JSValue) *JSValue // raw constructor for super() calls
	mu         sync.RWMutex                                   // per-object RWMutex for concurrent access
	gen        atomic.Uint64                                  // incremented on every mutation for cache invalidation
	cache      [4]cacheEntry                                  // fixed inline cache for Get() optimization
	frozen     bool                                           // true for interned singletons; Set/DefineProperty no-op
}

// MarkAsMethod marks this function as a class method that expects 'this'
// as the first argument when called via MethodCall.
func (v *JSValue) MarkAsMethod() *JSValue {
	if v != nil {
		v.isMethod = true
	}
	return v
}

// lock acquires a write lock on the JSValue's RWMutex.
func (v *JSValue) lock()   { v.mu.Lock() }
func (v *JSValue) unlock() { v.mu.Unlock() }

// rlock acquires a read lock on the JSValue's RWMutex.
func (v *JSValue) rlock()   { v.mu.RLock() }
func (v *JSValue) runlock() { v.mu.RUnlock() }

var symbolCounter uint64

// Interned boolean singletons. Declared without prototype; BooleanPrototype
// is patched in during prototype.init() to avoid init-order dependency.
// frozen=true makes .Set()/.DefineProperty() a no-op on these values, matching
// JS sloppy-mode semantics for primitive mutation.
var (
	_true  = &JSValue{typ: TypeBoolean, boolVal: true, frozen: true}
	_false = &JSValue{typ: TypeBoolean, boolVal: false, frozen: true}
)

// NewBool returns the cached singleton for the given boolean value.
func NewBool(b bool) *JSValue {
	if b {
		return _true
	}
	return _false
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

// Package-level singletons for undefined and null. These are immutable in
// correct JS semantics — type guards in Set()/DefineProperty() prevent
// accidental mutation of the singleton's (nil) properties map.
var (
	_undefined = &JSValue{typ: TypeUndefined, frozen: true}
	_null      = &JSValue{typ: TypeNull, frozen: true}
)

// NewNull returns a singleton null JSValue (zero allocation).
func NewNull() *JSValue {
	return _null
}

// NewUndefined returns a singleton undefined JSValue (zero allocation).
func NewUndefined() *JSValue {
	return _undefined
}

// NewFunction creates a function JSValue.
func NewFunction(fn func(...*JSValue) *JSValue) *JSValue {
	return &JSValue{
		typ:       TypeFunction,
		prototype: FunctionPrototype,
		funcVal:   fn,
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
		if v.isArr {
			n := v.arrayVal.Len()
			strs := make([]string, n)
			for i := 0; i < n; i++ {
				elem := v.arrayVal.Get(i)
				if elem == nil || elem.typ == TypeNull || elem.typ == TypeUndefined {
					strs[i] = ""
				} else {
					strs[i] = elem.String()
				}
			}
			return strings.Join(strs, ",")
		}
		if stack := v.Get("stack"); stack != nil && stack.typ == TypeString && stack.strVal != "" {
			return stack.strVal
		}
		if name := v.Get("name"); name != nil && name.typ == TypeString {
			if msg := v.Get("message"); msg != nil && msg.typ == TypeString {
				if msg.strVal == "" {
					return name.strVal
				}
				return name.strVal + ": " + msg.strVal
			}
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
	switch v.typ {
	case TypeNumber:
		return v.numVal
	case TypeBigInt:
		return float64(v.bigIntVal)
	case TypeBoolean:
		if v.boolVal {
			return 1
		}
		return 0
	case TypeNull:
		return 0
	case TypeUndefined:
		return math.NaN()
	case TypeString:
		s := strings.TrimSpace(v.strVal)
		if s == "" {
			return 0
		}
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			if n, err := strconv.ParseInt(s[2:], 16, 64); err == nil {
				return float64(n)
			}
		}
		if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
			if n, err := strconv.ParseInt(s[2:], 8, 64); err == nil {
				return float64(n)
			}
		}
		if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
			if n, err := strconv.ParseInt(s[2:], 2, 64); err == nil {
				return float64(n)
			}
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n
		}
		return math.NaN()
	default:
		return math.NaN()
	}
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
	v.lock()
	v.prototype = proto
	v.gen.Add(1)
	v.unlock()
}

// GetPrototype returns the [[Prototype]] internal slot.
func (v *JSValue) GetPrototype() *JSValue {
	if v.prototype != nil {
		return v.prototype
	}
	return _null
}

// Array returns a copy of the underlying array elements, or nil if not an array.
// Returns a copy for thread safety — callers can mutate the returned slice freely.
func (v *JSValue) Array() []*JSValue {
	if !v.isArr {
		return nil
	}
	if v.arrayVal.Len() == 0 {
		return []*JSValue{}
	}
	return append([]*JSValue{}, v.arrayVal.Slice()...)
}

// Index returns the element at position i in an array or string JSValue.
// For arrays: returns the element at position i.
// For strings: returns a single-character string (matching JS "str"[i] semantics).
// Returns undefined if out of bounds or not an array/string.
func (v *JSValue) Index(i int) *JSValue {
	if v.isArr && i >= 0 && i < v.arrayVal.Len() {
		return v.arrayVal.Get(i)
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
	return v != nil && v.isArr
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
		if v.isArr {
			return v.arrayVal.Len()
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

// New simulates JavaScript's `new func(args)` semantics.
// For class constructors (created via NewClass), delegates to Call which already
// creates a fresh instance. For method-marked functions (regular functions that
// use `this`), creates a fresh object and prepends it as _args[0] so the function
// can extract it as `this`. For other functions, just calls normally.
func (v *JSValue) New(args ...*JSValue) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	// Classes already handle new semantics in their Call
	if v.classInit != nil {
		return v.Call(args...)
	}
	if v.funcVal != nil {
		// Method-marked function: expects this as _args[0]
		if v.isMethod {
			this := NewObject()
			allArgs := make([]*JSValue, 0, 1+len(args))
			allArgs = append(allArgs, this)
			allArgs = append(allArgs, args...)
			result := v.Call(allArgs...)
			// If function returns an object, use it; otherwise use `this`
			if result != nil && result.typ == TypeObject {
				return result
			}
			return this
		}
		// Non-method function: call normally, return result or fresh object
		result := v.Call(args...)
		if result != nil && (result.typ == TypeObject || result.typ == TypeFunction) {
			return result
		}
		return NewObject()
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
		if argsArray != nil && argsArray.isArr {
			return v.Call(argsArray.arrayVal.Slice()...)
		}
		return v.Call()
	}
	if v.typ == TypeFunction && method == "call" && len(args) >= 1 {
		// .call(thisArg, ...args) — invoke function with thisArg as receiver
		// For methods (isMethod=true), prepend thisArg so it becomes _args[0]
		if v.isMethod {
			allArgs := make([]*JSValue, 0, len(args))
			allArgs = append(allArgs, args[0]) // thisArg
			allArgs = append(allArgs, args[1:]...)
			return v.Call(allArgs...)
		}
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
	// Walk the prototype chain to find the PARENT's classInit.
	// Skip the first classInit (which is the current class's own constructor)
	// to avoid infinite recursion when the constructor calls super().
	skipped := false
	for cur := v; cur != nil; cur = cur.prototype {
		if cur.classInit != nil {
			if !skipped {
				skipped = true
				continue
			}
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

// IsNullish returns true if v is nil, undefined, or null. Used by transpiled
// short-circuit lowerings to avoid evaluating the RHS when LHS is defined.
func IsNullish(v *JSValue) bool {
	return v == nil || v.typ == TypeUndefined || v.typ == TypeNull
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

// InstanceOf implements JavaScript instanceof using prototype-chain lookup.
func InstanceOf(left, right *JSValue) *JSValue {
	if left == nil || right == nil {
		return NewBool(false)
	}
	if left.typ == TypeUndefined || left.typ == TypeNull {
		return NewBool(false)
	}
	proto := right.Get("prototype")
	if proto == nil || proto.typ == TypeUndefined || proto.typ == TypeNull {
		return NewBool(false)
	}
	for cur := left.GetPrototype(); cur != nil && cur.typ != TypeNull; cur = cur.GetPrototype() {
		if cur == proto {
			return NewBool(true)
		}
	}
	return NewBool(false)
}
