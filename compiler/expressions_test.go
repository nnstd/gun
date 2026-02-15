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
	// Assignment-as-expression should be wrapped in an IIFE.
	// a: any maps to *jsvalue.JSValue, so the IIFE returns that type.
	assertContains(t, out, "func() *jsvalue.JSValue")
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

func TestEnsureBoolTypedLocalNotNilChecked(t *testing.T) {
	// Typed bool locals should not get != nil coercion.
	ts := `function f(s) {
	let flag = false;
	if (flag) { return s; }
}`
	out := compile(t, ts)
	assertContains(t, out, "if flag")
	assertNotContains(t, out, "flag != nil")
}

func TestLengthOnJSValueChain(t *testing.T) {
	// .length on a subscript of a JSValue should use .Len(), not len().
	ts := `function f(arr) { return arr[0].length; }`
	out := compile(t, ts)
	assertContains(t, out, ".Index(0).Len()")
	assertNotContains(t, out, "len(")
}

func TestTernaryNumericTypeInference(t *testing.T) {
	// When both ternary branches are numeric, the IIFE should return int.
	ts := `function f(x) { return x ? 1 : 0; }`
	out := compile(t, ts)
	assertContains(t, out, "func() int")
	assertNotContains(t, out, "func() any")
}

func TestJSOrDefaultPattern(t *testing.T) {
	// JS || used as default-value pattern with JSValue should emit
	// a truthiness-checking IIFE, not Go's boolean ||.
	ts := `function f(x) { return x || "default"; }`
	out := compile(t, ts)
	assertContains(t, out, `x.String() != ""`)
	assertContains(t, out, "return x")
	assertNotContains(t, out, `x || "default"`)
}

func TestCharAtOnJSValueUsesRuntime(t *testing.T) {
	// charAt on a JSValue param should coerce to string and use rune indexing.
	ts := `function f(s) { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, "string([]rune(fmt.Sprint(s))[0])")
}

func TestCharAtOnStringUsesBuiltin(t *testing.T) {
	// charAt on a plain string should use the string builtin.
	ts := `function f(s: string): string { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, "string([]rune(s)[0])")
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

func TestCharAtTypedVsJSValue(t *testing.T) {
	// charAt on a typed string local should produce a string result,
	// while charAt on a JSValue param should produce a JSValue result.
	// Comparing the two must coerce the JSValue side to .String().
	ts := `function f(str) {
	const lower = str.toLowerCase();
	const chrLower = lower.charAt(0);
	const chrString = str.charAt(0);
	return chrLower !== chrString;
}`
	out := compile(t, ts)
	assertContains(t, out, "chrString.String()")
	assertNotContains(t, out, "chrLower.String()")
}

func TestNullComparisonNoStringCoercion(t *testing.T) {
	// Comparing a JSValue param with null/undefined should emit x == nil,
	// not x.String() == nil.
	ts := `function f(x) { if (x === null) { return true; } return false; }`
	out := compile(t, ts)
	assertContains(t, out, "x == nil")
	assertNotContains(t, out, "x.String() == nil")
}

func TestRegexTestBoolResult(t *testing.T) {
	// regex.test() returns bool; ensureBool should not wrap with != nil.
	ts := `function f(s) {
	if (/^hello/.test(s)) { return true; }
	return false;
}`
	out := compile(t, ts)
	assertContains(t, out, "MatchString(fmt.Sprint(s))")
	assertNotContains(t, out, "!= nil")
}

func TestNegationOnJSValueUsesNilCheck(t *testing.T) {
	// !jsValue should compile to jsValue == nil, not !jsValue.
	ts := `function f(x) { if (!x) { return true; } return false; }`
	out := compile(t, ts)
	assertContains(t, out, "x == nil")
	assertNotContains(t, out, "!x")
}

func TestJSValuePlusStringCoercion(t *testing.T) {
	// JSValue + "" should coerce the JSValue to string via fmt.Sprint.
	ts := `function f(e) { return e + ""; }`
	out := compile(t, ts)
	assertContains(t, out, "fmt.Sprint(e)")
	assertNotContains(t, out, "e + \"\"")
}

func TestArrayIsArray(t *testing.T) {
	// Array.isArray(x) should compile to x.IsArray().
	ts := `function f(x) { if (Array.isArray(x)) { return x; } }`
	out := compile(t, ts)
	assertContains(t, out, "x.IsArray()")
	assertNotContains(t, out, "Array.")
}

func TestArrayLiteralIndexUsesNativeSubscript(t *testing.T) {
	// Variables initialized from array literals are typed Go slices,
	// so indexing should use native [] not .Index().
	ts := `function f(x) { const args = [x, x]; return args[0]; }`
	out := compile(t, ts)
	assertContains(t, out, "args[0]")
	assertNotContains(t, out, "args.Index(")
}

func TestNegationOnSubscriptUsesNilCheck(t *testing.T) {
	// !arr[i] where arr is []*jsvalue.JSValue should emit arr[i] == nil.
	ts := `function f(x) { const args = [x]; if (!args[0]) { return x; } }`
	out := compile(t, ts)
	assertContains(t, out, "== nil")
	assertNotContains(t, out, "!args[")
}

func TestSliceIndexWrapsFloat64WithInt(t *testing.T) {
	// JS number vars are float64; Go slice indices must be int.
	ts := `function f(x) { const args = [x]; let i = 0; return args[i]; }`
	out := compile(t, ts)
	assertContains(t, out, "args[int(i)]")
}

func TestJSValueSliceElementAssignmentWrapped(t *testing.T) {
	// Assigning a string literal to a JSValue slice element should wrap with jsvalue.From().
	ts := `function f(x) { const args = [x]; args[0] = "hello"; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.From(")
}

func TestJSValueSliceElementPlusString(t *testing.T) {
	// JSValue slice element + "" should coerce the element to string.
	ts := `function f(x) { const args = [x]; return args[0] + ""; }`
	out := compile(t, ts)
	assertContains(t, out, "fmt.Sprint(")
	assertNotContains(t, out, `args[int(0)] + ""`)
}

func TestJSValueComparedWithOwnStringMethod(t *testing.T) {
	// str !== str.toLowerCase() should coerce str to .String() on the left,
	// not treat both sides as JSValue.
	ts := `function f(str) { return str !== str.toLowerCase(); }`
	out := compile(t, ts)
	assertContains(t, out, ".String()")
	assertContains(t, out, "strings.ToLower")
}

func TestLocalVarPropertyAccessUsesGet(t *testing.T) {
	// Local variables declared inside a function body should be registered
	// in scope so property access uses .Get() instead of struct field access.
	ts := `function f(obj) { var result = obj.call(); return result.argv; }`
	out := compile(t, ts)
	assertContains(t, out, `result.Get("argv")`)
	assertNotContains(t, out, "result.Argv")
}

func TestNegatedMatchUsesNilCheck(t *testing.T) {
	// !str.match(regex) should produce FindStringSubmatch(...) == nil, not !FindStringSubmatch(...)
	ts := `function f(s: string) {
	if (!s.match(/^hello/)) { return false; }
	return true;
}`
	out := compile(t, ts)
	assertContains(t, out, "== nil")
	assertNotContains(t, out, "!regexp")
}
