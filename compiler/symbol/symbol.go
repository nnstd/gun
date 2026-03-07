// Package symbol provides a hygienic symbol table for the Gun transpiler.
//
// Instead of tracking identifiers as raw strings (which leads to name collisions),
// each identifier is assigned a unique Symbol with a numeric ID. Final Go names
// are generated only during code emission via EmitName, ensuring collision-free output.
package symbol

import (
	"strings"
	"unicode"
)

// ID is a unique identifier for a symbol.
type ID int

// Kind classifies what a symbol represents.
type Kind int

const (
	KindVariable  Kind = iota
	KindFunction
	KindParameter
	KindClass
	KindEnum
	KindImport
	KindType
)

// FuncInfo holds metadata for function symbols.
type FuncInfo struct {
	ParamCount int
	ReturnType string // Go return type (e.g. "*jsvalue.JSValue", "bool")
}

// ImportInfo holds metadata for imported symbols.
type ImportInfo struct {
	GoImportPath string
	GoPkgName    string
	GoSymbol     string
	IsTranspiled bool
}

// Symbol represents a single named entity in the program.
type Symbol struct {
	ID           ID
	OriginalName string // name as written in TypeScript source
	Kind         Kind
	Exported     bool   // whether the symbol is exported from its module
	Immutable    bool   // const declaration
	IsJSValue    bool   // holds *jsvalue.JSValue (vs native Go type)
	GoType       string // explicit Go type if typed (e.g. "bool", "string")
	ModuleType   string // module type for dispatch (e.g. "hono")
	FuncInfo     *FuncInfo
	ImportInfo   *ImportInfo
}

// goKeywords is the set of Go reserved words and predeclared identifiers.
var goKeywords = map[string]bool{
	// Keywords
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	// Predeclared identifiers
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true, "int8": true,
	"int16": true, "int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "true": true, "false": true, "iota": true, "nil": true,
	"append": true, "cap": true, "close": true, "complex": true, "copy": true,
	"delete": true, "imag": true, "len": true, "make": true, "new": true,
	"panic": true, "print": true, "println": true, "real": true, "recover": true,
}

// Sanitize converts a raw identifier to a valid Go identifier.
// It replaces $ with _, and appends _ to Go keywords.
func Sanitize(name string) string {
	name = strings.ReplaceAll(name, "$", "_")
	if goKeywords[name] {
		return name + "_"
	}
	return name
}

// Capitalize upper-cases the first letter for Go export convention.
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
