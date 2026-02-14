package compiler

import "testing"

func TestArrayLiteral(t *testing.T) {
	out := compile(t, `const nums = [1, 2, 3];`)
	assertContains(t, out, "[]float64{")
}

func TestObjectLiteral(t *testing.T) {
	ts := `const obj = { name: "go", version: 1 };`
	out := compile(t, ts)
	assertContains(t, out, "map[string]*jsvalue.JSValue{")
}

func TestTemplateString(t *testing.T) {
	ts := "const msg = `hello ${name}`;"
	out := compile(t, ts)
	assertContains(t, out, "fmt.Sprintf")
	assertContains(t, out, "%v")
}

func TestTemplateStringWithDoubleQuotes(t *testing.T) {
	ts := "const msg = `\"${name}\"`;"
	out := compile(t, ts)
	assertContains(t, out, "fmt.Sprintf")
	assertContains(t, out, `\"%v\"`)
}

func TestTernaryExpression(t *testing.T) {
	ts := `function pick(ok: boolean): string { return ok ? "yes" : "no"; }`
	out := compile(t, ts)
	assertContains(t, out, "if ok")
	assertContains(t, out, `return "yes"`)
	assertContains(t, out, `return "no"`)
}

func TestStrictEquality(t *testing.T) {
	ts := `function eq(a: number, b: number): boolean { return a === b; }`
	out := compile(t, ts)
	assertContains(t, out, "a == b")
	assertNotContains(t, out, "===")
}

func TestStrictInequality(t *testing.T) {
	ts := `function neq(a: number, b: number): boolean { return a !== b; }`
	out := compile(t, ts)
	assertContains(t, out, "a != b")
	assertNotContains(t, out, "!==")
}

func TestNewExpression(t *testing.T) {
	ts := `function make(): any { return new Date(); }`
	out := compile(t, ts)
	assertContains(t, out, "time.Now()")
}

func TestRegexLiteral(t *testing.T) {
	ts := `const re = /^file:\/\//;`
	out := compile(t, ts)
	assertContains(t, out, "regexp.MustCompile")
	assertContains(t, out, "`^file:\\/\\/`")
}

func TestRegexStripsFlagsAndDelimiters(t *testing.T) {
	ts := `const re = /hello/gi;`
	out := compile(t, ts)
	assertContains(t, out, "`hello`")
	assertNotContains(t, out, "/hello/")
}

func TestStringWithEmbeddedDoubleQuotes(t *testing.T) {
	ts := `const msg = '"version" is reserved';`
	out := compile(t, ts)
	assertContains(t, out, `\"version\" is reserved`)
}

func TestAssignmentExpressionInIIFE(t *testing.T) {
	ts := `function f(a: any, b: any): any { return a = b; }`
	out := compile(t, ts)
	// Assignment-as-expression should be wrapped in an IIFE
	assertContains(t, out, "func() any")
	assertNotContains(t, out, "= =")
}

func TestAssignmentExpressionJSValueTarget(t *testing.T) {
	// Package-level untyped var gets *jsvalue.JSValue; the IIFE must match
	// and wrap the RHS with jsvalue.From() so any-typed values convert.
	ts := `var _a;
var x = (_a = someExpr());`
	out := compile(t, ts)
	assertContains(t, out, "func() *jsvalue.JSValue")
	assertContains(t, out, "jsvalue.From(")
}

func TestDollarSignInIdentifier(t *testing.T) {
	ts := `let $0 = "bin";
let default$0 = "node";
console.log($0);`
	out := compile(t, ts)
	assertContains(t, out, "_0")
	assertContains(t, out, "default_0")
	assertNotContains(t, out, "$")
}

func TestNewExpressionWrapsArgsWithJSValue(t *testing.T) {
	ts := `const p = new Parser({cwd: "/tmp"});`
	out := compile(t, ts)
	assertContains(t, out, "NewParser(jsvalue.From(")
}

func TestAwaitStripped(t *testing.T) {
	ts := `async function load(): Promise<string> { return await fetch(); }`
	out := compile(t, ts)
	assertNotContains(t, out, "await")
	assertNotContains(t, out, "async")
	assertContains(t, out, "func")
}

func TestNewMemberExpression(t *testing.T) {
	ts := `const seg = new Intl.Segmenter();`
	out := compile(t, ts)
	assertNotContains(t, out, "Intl.")
	assertContains(t, out, "intl.NewSegmenter()")
}

func TestImportMeta(t *testing.T) {
	ts := `const url = import.meta.url;`
	out := compile(t, ts)
	assertContains(t, out, "module.ImportMeta.Url")
	assertNotContains(t, out, "nil.Url")
}

func TestEnsureBoolTruthinessCheck(t *testing.T) {
	ts := `function f(x: any): string { return x ? "yes" : "no"; }`
	out := compile(t, ts)
	assertContains(t, out, "x != nil")
}

func TestLengthOnUntypedParam(t *testing.T) {
	ts := `function f(s) { return s.length; }`
	out := compile(t, ts)
	assertContains(t, out, "s.Len()")
	assertNotContains(t, out, "len(s)")
}

func TestEnsureBoolMethodCallGetsNilCheck(t *testing.T) {
	// Method calls (e.g. obj.Match()) may return *jsvalue.JSValue → need != nil
	ts := `function f(s) { if (s.Match()) { return s; } }`
	out := compile(t, ts)
	assertContains(t, out, "!= nil")
}

func TestEnsureBoolPlainCallNotWrapped(t *testing.T) {
	// Plain function calls that return bool should NOT get != nil
	ts := `function isOk(x: number): boolean { return x > 0; }
function f(x: number): string { if (isOk(x)) { return "yes"; } return "no"; }`
	out := compile(t, ts)
	assertContains(t, out, "if isOk(x)")
	assertNotContains(t, out, "isOk(x) != nil")
}

func TestArrowFuncTrailingReturn(t *testing.T) {
	// Arrow functions with a return type but not all paths returning
	// should get a trailing zero-value return to avoid "missing return".
	ts := `const f = (x: number): string => { if (x > 0) { return "pos"; } };`
	out := compile(t, ts)
	assertContains(t, out, `return ""`)
}

func TestFuncExprTrailingReturn(t *testing.T) {
	ts := `const f = function(x: number): string { if (x > 0) { return "pos"; } };`
	out := compile(t, ts)
	assertContains(t, out, `return ""`)
}

func TestLocalVarPropertyAccessUsesGet(t *testing.T) {
	// Local variables declared inside a function body should be registered
	// in scope so property access uses .Get() instead of struct field access.
	ts := `function f(obj) { var result = obj.call(); return result.argv; }`
	out := compile(t, ts)
	assertContains(t, out, `result.Get("argv")`)
	assertNotContains(t, out, "result.Argv")
}
