// Package context provides the TranspilerContext — a unified registry for all
// builtin global objects, functions, constructors, identifiers, and modules
// that the Gun transpiler recognizes.
//
// Instead of hardcoded switch/case dispatch scattered across the compiler,
// all builtins are registered through this API, making the system extensible
// and centralizing the knowledge of what globals exist.
package context

import "go/ast"

// Imports is the interface for adding Go imports during transformation.
// The Transformer implements this interface.
type Imports interface {
	AddImport(path string)
	AddAliasedImport(path, alias string)
}

// GlobalObjectTransform transforms a method call on a global object.
// Returns nil if the method is not handled.
type GlobalObjectTransform func(method string, args []ast.Expr, imp Imports) ast.Expr

// GlobalObjectMemberTransform transforms a property access on a global object.
// Returns nil if the property is not handled.
type GlobalObjectMemberTransform func(prop string, imp Imports) ast.Expr

// GlobalObject represents a known global object like console, Math, JSON, Object.
type GlobalObject struct {
	Name            string
	TransformCall   GlobalObjectTransform       // method calls: obj.method(args)
	TransformMember GlobalObjectMemberTransform // property access: obj.prop
}

// GlobalFuncTransform transforms a bare global function call.
// Returns nil if not handled.
type GlobalFuncTransform func(args []ast.Expr, imp Imports) ast.Expr

// GlobalFunction represents a bare global function like parseInt, isNaN.
type GlobalFunction struct {
	Name      string
	Transform GlobalFuncTransform
}

// ConstructorTransform transforms a new X(args) expression.
// Returns nil if not handled.
type ConstructorTransform func(args []ast.Expr, imp Imports) ast.Expr

// Constructor represents a known constructor (new Error(), new Map(), etc.).
type Constructor struct {
	Name      string
	Transform ConstructorTransform
}

// IdentifierTransform maps a global identifier to its Go equivalent.
type IdentifierTransform func(imp Imports) ast.Expr

// IdentifierMapping maps a global name (undefined, Infinity, console, etc.)
// to its Go expression.
type IdentifierMapping struct {
	Name      string
	Transform IdentifierTransform
}

// ModuleMapping maps a TS module to a Go package.
type ModuleMapping struct {
	GoImportPath    string
	GoPkgName       string
	SymbolOverrides map[string]SymbolOverride
}

// SymbolOverride maps a specific (module, symbol) pair to a Go translation.
type SymbolOverride struct {
	GoSymbol string // empty for namespace imports
}

// TranspilerContext is the central registry for all builtin globals, functions,
// constructors, identifiers, and modules.
type TranspilerContext struct {
	globals      map[string]*GlobalObject
	globalFuncs  map[string]*GlobalFunction
	constructors map[string]*Constructor
	identifiers  map[string]*IdentifierMapping
	modules      map[string]*ModuleMapping

	// knownGlobals is the set of all global names that should not be treated
	// as JSValue property access (i.e., obj.Get("prop")). It is derived from
	// the above registries plus any explicitly added names.
	knownGlobals map[string]bool
}

// New creates a new TranspilerContext with empty registries.
// Call RegisterDefaults() to populate it with the standard JS builtins.
func New() *TranspilerContext {
	return &TranspilerContext{
		globals:      make(map[string]*GlobalObject),
		globalFuncs:  make(map[string]*GlobalFunction),
		constructors: make(map[string]*Constructor),
		identifiers:  make(map[string]*IdentifierMapping),
		modules:      make(map[string]*ModuleMapping),
		knownGlobals: make(map[string]bool),
	}
}

// RegisterGlobal registers a global object (console, Math, JSON, Object, etc.).
func (c *TranspilerContext) RegisterGlobal(obj *GlobalObject) {
	c.globals[obj.Name] = obj
	c.knownGlobals[obj.Name] = true
}

// RegisterGlobalFunc registers a bare global function (parseInt, isNaN, etc.).
func (c *TranspilerContext) RegisterGlobalFunc(fn *GlobalFunction) {
	c.globalFuncs[fn.Name] = fn
}

// RegisterConstructor registers a constructor (new Error(), new Map(), etc.).
func (c *TranspilerContext) RegisterConstructor(ctor *Constructor) {
	c.constructors[ctor.Name] = ctor
}

// RegisterIdentifier registers a global identifier mapping (undefined, Infinity, etc.).
// Note: this does NOT mark the name as a known global for member access dispatch.
// Use MarkKnownGlobal separately for identifiers that should prevent .Get() dispatch
// (e.g. console, Math, JSON — typed objects, not JSValue wrappers like Promise).
func (c *TranspilerContext) RegisterIdentifier(mapping *IdentifierMapping) {
	c.identifiers[mapping.Name] = mapping
}

// RegisterModule registers a TS module → Go package mapping.
func (c *TranspilerContext) RegisterModule(tsModule string, mapping *ModuleMapping) {
	c.modules[tsModule] = mapping
}

// MarkKnownGlobal adds a name to the known globals set without a full registration.
// Use this for names like "globalThis", "require" that are recognized but don't
// need transform logic.
func (c *TranspilerContext) MarkKnownGlobal(name string) {
	c.knownGlobals[name] = true
}

// LookupGlobal returns the GlobalObject for the given name, or nil.
func (c *TranspilerContext) LookupGlobal(name string) *GlobalObject {
	return c.globals[name]
}

// LookupGlobalFunc returns the GlobalFunction for the given name, or nil.
func (c *TranspilerContext) LookupGlobalFunc(name string) *GlobalFunction {
	return c.globalFuncs[name]
}

// LookupConstructor returns the Constructor for the given name, or nil.
func (c *TranspilerContext) LookupConstructor(name string) *Constructor {
	return c.constructors[name]
}

// LookupIdentifier returns the IdentifierMapping for the given name, or nil.
func (c *TranspilerContext) LookupIdentifier(name string) *IdentifierMapping {
	return c.identifiers[name]
}

// LookupModule returns the ModuleMapping for the given TS module, or nil.
func (c *TranspilerContext) LookupModule(tsModule string) *ModuleMapping {
	return c.modules[tsModule]
}

// IsKnownGlobal returns true if the name is a recognized global that should
// not be treated as a JSValue property access target.
func (c *TranspilerContext) IsKnownGlobal(name string) bool {
	return c.knownGlobals[name]
}

// TransformBuiltinCall dispatches a method call on a global object.
// Returns nil if the object or method is not recognized.
func (c *TranspilerContext) TransformBuiltinCall(obj, method string, args []ast.Expr, imp Imports) ast.Expr {
	g := c.globals[obj]
	if g == nil || g.TransformCall == nil {
		return nil
	}
	return g.TransformCall(method, args, imp)
}

// TransformBuiltinMember dispatches a property access on a global object.
// Returns nil if the object or property is not recognized.
func (c *TranspilerContext) TransformBuiltinMember(obj, prop string, imp Imports) ast.Expr {
	g := c.globals[obj]
	if g == nil || g.TransformMember == nil {
		return nil
	}
	return g.TransformMember(prop, imp)
}

// TransformGlobalCall dispatches a bare global function call.
// Returns nil if the function is not recognized.
func (c *TranspilerContext) TransformGlobalCall(name string, args []ast.Expr, imp Imports) ast.Expr {
	fn := c.globalFuncs[name]
	if fn == nil {
		return nil
	}
	return fn.Transform(args, imp)
}

// TransformBuiltinNew dispatches a constructor call (new X(args)).
// Returns nil if the constructor is not recognized.
func (c *TranspilerContext) TransformBuiltinNew(name string, args []ast.Expr, imp Imports) ast.Expr {
	ctor := c.constructors[name]
	if ctor == nil {
		return nil
	}
	return ctor.Transform(args, imp)
}

// TransformIdentifier maps a global identifier to its Go expression.
// Returns nil if the identifier is not recognized.
func (c *TranspilerContext) TransformIdentifier(name string, imp Imports) ast.Expr {
	mapping := c.identifiers[name]
	if mapping == nil {
		return nil
	}
	return mapping.Transform(imp)
}
