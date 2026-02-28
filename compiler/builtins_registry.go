package compiler

// MethodReturnType represents the return type of a built-in method.
type MethodReturnType int

const (
	ReturnString MethodReturnType = iota
	ReturnNumber
	ReturnBoolean
	ReturnArray
	ReturnJSValue
	ReturnVoid
)

// MethodInfo contains metadata about a built-in method.
type MethodInfo struct {
	Name       string
	ReturnType MethodReturnType
	// For untyped receivers (JSValue), should the result be wrapped in JSValue?
	WrapForJSValue bool
	// Is this an array method (vs string method)?
	IsArrayMethod bool
	// Is this a regex method?
	IsRegexMethod bool
}

// BuiltinRegistry maintains metadata about all built-in methods.
type BuiltinRegistry struct {
	methods              map[string]*MethodInfo
	jsvaluePackageFuncs  map[string]bool // JSValue package-level functions that return *jsvalue.JSValue
}

// NewBuiltinRegistry creates and initializes the built-in method registry.
func NewBuiltinRegistry() *BuiltinRegistry {
	r := &BuiltinRegistry{
		methods:             make(map[string]*MethodInfo),
		jsvaluePackageFuncs: make(map[string]bool),
	}
	r.registerStringMethods()
	r.registerArrayMethods()
	r.registerRegexMethods()
	r.registerJSValuePackageFunctions()
	return r
}

func (r *BuiltinRegistry) registerStringMethods() {
	stringMethods := []*MethodInfo{
		{Name: "toLowerCase", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "toUpperCase", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "trim", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "trimStart", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "trimEnd", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "toString", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "replace", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "replaceAll", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "charAt", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "substring", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "substr", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "repeat", ReturnType: ReturnString, WrapForJSValue: true},
		{Name: "indexOf", ReturnType: ReturnNumber, WrapForJSValue: true},
		{Name: "lastIndexOf", ReturnType: ReturnNumber, WrapForJSValue: true},
		{Name: "search", ReturnType: ReturnNumber, WrapForJSValue: true},
		{Name: "startsWith", ReturnType: ReturnBoolean, WrapForJSValue: true},
		{Name: "endsWith", ReturnType: ReturnBoolean, WrapForJSValue: true},
		{Name: "includes", ReturnType: ReturnBoolean, WrapForJSValue: true},
		{Name: "match", ReturnType: ReturnJSValue, WrapForJSValue: true},
		{Name: "split", ReturnType: ReturnArray, WrapForJSValue: true},
	}
	for _, m := range stringMethods {
		r.methods[m.Name] = m
	}
}

func (r *BuiltinRegistry) registerArrayMethods() {
	arrayMethods := []*MethodInfo{
		{Name: "push", ReturnType: ReturnVoid, IsArrayMethod: true},
		{Name: "pop", ReturnType: ReturnJSValue, IsArrayMethod: true},
		{Name: "shift", ReturnType: ReturnJSValue, IsArrayMethod: true},
		{Name: "unshift", ReturnType: ReturnNumber, IsArrayMethod: true},
		{Name: "slice", ReturnType: ReturnArray, IsArrayMethod: true},
		{Name: "concat", ReturnType: ReturnArray, IsArrayMethod: true},
		{Name: "join", ReturnType: ReturnString, IsArrayMethod: true},
		{Name: "map", ReturnType: ReturnArray, IsArrayMethod: true},
		{Name: "filter", ReturnType: ReturnArray, IsArrayMethod: true},
		{Name: "forEach", ReturnType: ReturnVoid, IsArrayMethod: true},
		{Name: "reduce", ReturnType: ReturnJSValue, IsArrayMethod: true},
		{Name: "find", ReturnType: ReturnJSValue, IsArrayMethod: true},
		{Name: "some", ReturnType: ReturnBoolean, IsArrayMethod: true},
		{Name: "every", ReturnType: ReturnBoolean, IsArrayMethod: true},
		{Name: "includes", ReturnType: ReturnBoolean, IsArrayMethod: true},
		{Name: "length", ReturnType: ReturnNumber, IsArrayMethod: true},
	}
	for _, m := range arrayMethods {
		r.methods[m.Name] = m
	}
}

func (r *BuiltinRegistry) registerRegexMethods() {
	regexMethods := []*MethodInfo{
		{Name: "test", ReturnType: ReturnBoolean, IsRegexMethod: true},
		{Name: "exec", ReturnType: ReturnJSValue, IsRegexMethod: true},
	}
	for _, m := range regexMethods {
		r.methods[m.Name] = m
	}
}

// GetMethod returns metadata for a method, or nil if not found.
func (r *BuiltinRegistry) GetMethod(name string) *MethodInfo {
	return r.methods[name]
}

// IsArrayMethod returns true if the method is an array method.
func (r *BuiltinRegistry) IsArrayMethod(name string) bool {
	m := r.methods[name]
	return m != nil && m.IsArrayMethod
}

// IsStringMethod returns true if the method is a string method.
func (r *BuiltinRegistry) IsStringMethod(name string) bool {
	m := r.methods[name]
	return m != nil && !m.IsArrayMethod && !m.IsRegexMethod
}

// IsRegexMethod returns true if the method is a regex method.
func (r *BuiltinRegistry) IsRegexMethod(name string) bool {
	m := r.methods[name]
	return m != nil && m.IsRegexMethod
}

// ReturnsJSValueForUntypedReceiver returns true if the method should return
// JSValue when called on an untyped receiver (JSValue parameter).
func (r *BuiltinRegistry) ReturnsJSValueForUntypedReceiver(name string) bool {
	m := r.methods[name]
	return m != nil && m.WrapForJSValue
}

// GetReturnType returns the return type of a method.
func (r *BuiltinRegistry) GetReturnType(name string) MethodReturnType {
	m := r.methods[name]
	if m == nil {
		return ReturnJSValue // default
	}
	return m.ReturnType
}

// registerJSValuePackageFunctions registers all jsvalue package-level functions
// that return *jsvalue.JSValue. These are used to identify JSValue-returning
// call expressions in nodeReturnsJSValue and isJSValueMethodCall.
func (r *BuiltinRegistry) registerJSValuePackageFunctions() {
	// Factory functions
	funcs := []string{
		"NewString", "NewNumber", "NewBool",
		"NewArray", "NewObject", "NewNull",
		"NewUndefined", "NewSymbol", "NewBigInt",
		"NewFunction", "NewRegex",
		"From", "FromStrings",
		// Object methods
		"Keys",
		// Array wrapper functions
		"Join", "Pop", "Includes",
		"Slice", "Concat", "Push",
		"Shift", "Unshift",
		"Map", "Filter", "ForEach",
		// String wrapper functions
		"ToLowerCase", "ToUpperCase", "Trim",
		"Split", "Replace", "CharAt",
		"StartsWith", "EndsWith", "Repeat",
		// Logical operators
		"OrDefault", "And", "Or", "Not", "Nullish",
		// Arithmetic operators
		"Add", "Sub", "Mul", "Div", "Mod",
		"Neg", "Inc", "Dec",
		// Bitwise operators
		"BitNot", "BitAnd", "BitOr", "BitXor", "Shl", "Shr", "UShr",
		// Comparison operators
		"Eq", "NEq", "Lt", "Gt", "LtE", "GtE",
		// Type checking
		"IsArrayValue", "TypeOf",
		// Substring/LastIndexOf
		"Substring", "LastIndexOf",
	}
	for _, name := range funcs {
		r.jsvaluePackageFuncs[name] = true
	}
}

// IsJSValuePackageFunction returns true if the name is a jsvalue package-level
// function that returns *jsvalue.JSValue.
func (r *BuiltinRegistry) IsJSValuePackageFunction(name string) bool {
	return r.jsvaluePackageFuncs[name]
}
