package compiler

import "testing"

func TestArrayLiteral(t *testing.T) {
	out := compile(t, `const nums = [1, 2, 3];`)
	assertContains(t, out, "jsvalue.NewArray(jsvalue.NewNumber(float64(1)), jsvalue.NewNumber(float64(2)), jsvalue.NewNumber(float64(3)))")
}

func TestObjectLiteral(t *testing.T) {
	ts := `const obj = { name: "go", version: 1 };`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ObjectFrom(")
}

func TestObjectLiteralComputedKey(t *testing.T) {
	ts := `const KEY = "hello";
const obj = { [KEY]: true };`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.PropertyKey(KEY)")
	assertNotContains(t, out, `"[KEY]"`)
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
	assertContains(t, out, "jsvalue.Eq(")
	assertNotContains(t, out, "===")
}

func TestStrictInequality(t *testing.T) {
	ts := `function neq(a: number, b: number): boolean { return a !== b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NEq(")
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
	assertContains(t, out, "jsvalue.CompileRegex")
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
	// new Foo(args) → Foo.Call(args...) since classes are JSValue constructors.
	ts := `const p = new Parser({cwd: "/tmp"});`
	out := compile(t, ts)
	assertContains(t, out, "Parser.Call(")
	assertContains(t, out, "jsvalue.ObjectFrom(")
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
	assertContains(t, out, "intl.Segmenter.Call()")
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
	assertContains(t, out, "x != nil && x.Bool()")
}

func TestLengthOnUntypedParam(t *testing.T) {
	ts := `function f(s) { return s.length; }`
	out := compile(t, ts)
	assertContains(t, out, "s.Len()")
	assertNotContains(t, out, "len(s)")
}

func TestEnsureBoolMethodCallOnLocal(t *testing.T) {
	// Method calls on locals use .Get().Call(this, ...) and .Bool() for truthiness.
	ts := `function f(s) { if (s.Match()) { return s; } }`
	out := compile(t, ts)
	assertContains(t, out, `s.MethodCall("Match")`)
	assertContains(t, out, ".Bool()")
}

func TestEnsureBoolPlainCallNotWrapped(t *testing.T) {
	// Package-level functions are JSValue vars, called via .Call()
	// and use .Bool() for truthiness in boolean context.
	ts := `function isOk(x: number): boolean { return x > 0; }
function f(x: number): string { if (isOk(x)) { return "yes"; } return "no"; }`
	out := compile(t, ts)
	assertContains(t, out, "isOk.Call(")
	assertContains(t, out, ".Bool()")
}

func TestEnsureBoolTypedLocalNotNilChecked(t *testing.T) {
	// Variables initialized from false are now JSValue — uses != nil && .Bool()
	ts := `function f(s) {
	let flag = false;
	if (flag) { return s; }
}`
	out := compile(t, ts)
	assertContains(t, out, "flag != nil && flag.Bool()")
}

func TestLengthOnJSValueChain(t *testing.T) {
	// .length on a subscript of a JSValue should use .Len(), not len().
	ts := `function f(arr) { return arr[0].length; }`
	out := compile(t, ts)
	assertContains(t, out, ".Index(0).Len()")
	assertNotContains(t, out, "len(arr")
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
	assertContains(t, out, "jsvalue.Or(jsvalue.From(x), jsvalue.NewString(")
	assertNotContains(t, out, `x || "default"`)
}

func TestCharAtOnJSValueUsesRuntime(t *testing.T) {
	// charAt on a JSValue param should use jsvalue.CharAt wrapper.
	ts := `function f(s) { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("charAt"`)
}

func TestCharAtOnStringUsesBuiltin(t *testing.T) {
	// All-JSValue: even `: string` params are JSValue, so charAt uses jsvalue.CharAt
	ts := `function f(s: string): string { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("charAt"`)
}

func TestArrowFuncTrailingReturn(t *testing.T) {
	// Arrow functions wrapped in NewFunction with not all paths returning
	// should get a trailing return to avoid "missing return".
	ts := `const f = (x: number): string => { if (x > 0) { return "pos"; } };`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.NewFunction(`)
	assertContains(t, out, `jsvalue.NewString("pos")`)
}

func TestFuncExprTrailingReturn(t *testing.T) {
	ts := `const f = function(x: number): string { if (x > 0) { return "pos"; } };`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.NewFunction(`)
	assertContains(t, out, `jsvalue.NewString("pos")`)
}

func TestCharAtTypedVsJSValue(t *testing.T) {
	// charAt on JSValue locals should produce JSValue results.
	// When comparing two JSValue results, both should be coerced to .String().
	ts := `function f(str) {
	const lower = str.toLowerCase();
	const chrLower = lower.charAt(0);
	const chrString = str.charAt(0);
	return chrLower !== chrString;
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NEq(jsvalue.From(chrLower), jsvalue.From(chrString))")
}

func TestNullComparisonNoStringCoercion(t *testing.T) {
	// Comparing a JSValue param with null/undefined should emit x == nil,
	// not x.String() == nil.
	ts := `function f(x) { if (x === null) { return true; } return false; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Eq(jsvalue.From(x), jsvalue.NewNull()).Bool()")
	assertNotContains(t, out, "x.String() == nil")
}

func TestRegexTestBoolResult(t *testing.T) {
	// Inline regex.test() uses jsvalue.MatchString which returns bool.
	ts := `function f(s) {
	if (/^hello/.test(s)) { return true; }
	return false;
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.MatchString(jsvalue.NewRegex(")
}

func TestNegationOnJSValueUsesNilCheck(t *testing.T) {
	// !jsValue should compile to !jsValue.Bool() for proper JavaScript truthiness semantics.
	ts := `function f(x) { if (!x) { return true; } return false; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(x).Bool()")
}

func TestJSValuePlusStringCoercion(t *testing.T) {
	// JSValue + "" should coerce the JSValue to string via fmt.Sprint.
	ts := `function f(e) { return e + ""; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Add(jsvalue.From(e), jsvalue.NewString(\"\"))")
	assertNotContains(t, out, "e + \"\"")
}

func TestArrayIsArray(t *testing.T) {
	// Array.isArray(x) should compile to jsvalue.IsArrayValue(x) returning *JSValue.
	ts := `function f(x) { if (Array.isArray(x)) { return x; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.IsArrayValue(x)")
	assertNotContains(t, out, "Array.")
}

func TestArrayLiteralIndexUsesNativeSubscript(t *testing.T) {
	// Variables initialized from array literals use jsvalue.NewArray,
	// so indexing uses .Index().
	ts := `function f(x) { const args = [x, x]; return args[0]; }`
	out := compile(t, ts)
	assertContains(t, out, "args.Index(0)")
}

func TestNegationOnSubscriptUsesNilCheck(t *testing.T) {
	// !arr[i] where arr is []*jsvalue.JSValue should use .Bool() for truthiness.
	ts := `function f(x) { const args = [x]; if (!args[0]) { return x; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(args.Index(0)).Bool()")
}

func TestSliceIndexWrapsFloat64WithInt(t *testing.T) {
	// JS number vars are float64; Go slice indices must be int.
	ts := `function f(x) { const args = [x]; let i = 0; return args[i]; }`
	out := compile(t, ts)
	assertContains(t, out, "args.Get(jsvalue.PropertyKey(i))")
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
	assertContains(t, out, "jsvalue.Add(args.Index(0), jsvalue.NewString(\"\"))")
	assertNotContains(t, out, `args[int(0)] + ""`)
}

func TestJSValueComparedWithOwnStringMethod(t *testing.T) {
	// str !== str.toLowerCase() where str is JSValue should use jsvalue wrappers.
	ts := `function f(str) { return str !== str.toLowerCase(); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("toLowerCase"`)
}

func TestLocalVarPropertyAccessUsesGet(t *testing.T) {
	// Local variables declared inside a function body should be registered
	// in scope so property access uses .Get() instead of struct field access.
	ts := `function f(obj) { var result = obj.call(); return result.argv; }`
	out := compile(t, ts)
	assertContains(t, out, `result.Get("argv")`)
	assertNotContains(t, out, "result.Argv")
}

func TestNegatedMatchUsesNot(t *testing.T) {
	// !str.match(regex) should use jsvalue.Not() since match returns *JSValue
	ts := `function f(s: string) {
	if (!s.match(/^hello/)) { return false; }
	return true;
}`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Not(`)
	assertContains(t, out, `MethodCall("match"`)
}

func TestMatchResultInBooleanContext(t *testing.T) {
	// Match results should be checked with != nil, not .String() != ""
	ts := `function f(arg) {
	if (arg.match(/^--.+/)) {
		return true;
	}
	return false;
}`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("match"`)
	// match result handled by prototype method
	assertNotContains(t, out, ".String()")
}

func TestMatchResultNotTreatedAsJSValue(t *testing.T) {
	// Match results — subscript access uses .Index() in all-JSValue mode
	ts := `function f(s) {
	const m = s.match(/^test/);
	if (m) {
		return m[0];
	}
	return "";
}`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("match"`)
	assertContains(t, out, "m != nil && m.Bool()")
	assertContains(t, out, "m.Index(0)")
	// Should not treat match result as JSValue
	assertNotContains(t, out, "m.String()")
}

func TestTopLevelFuncCallFromMain(t *testing.T) {
	ts := `
function main() { increment(); }
function increment(orig) { return orig; }
`
	out := compile(t, ts)
	// main() calls increment which is a JSValue var — uses .Call()
	assertContains(t, out, "increment.Call()")
}

func TestPkgVarMethodCallUsesGetCall(t *testing.T) {
	ts := `
let mixin;
function f(val) { return mixin.normalize(val); }
`
	out := compile(t, ts)
	assertContains(t, out, `.MethodCall("normalize",`)
	assertNotContains(t, out, `.Get("normalize")(`)
}

// --- All-JSValue regression tests ---

func TestBinaryAddUsesJSValueHelper(t *testing.T) {
	ts := `function f(a, b) { return a + b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Add(")
}

func TestBinaryEqUsesJSValueHelper(t *testing.T) {
	ts := `function f(a, b) { return a === b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Eq(")
}

func TestBinaryLtUsesJSValueHelper(t *testing.T) {
	ts := `function f(a, b) { return a < b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Lt(")
}

func TestLogicalOrUsesJSValueHelper(t *testing.T) {
	ts := `function f(x) { return x || "default"; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Or(")
}

func TestLogicalAndUsesJSValueHelper(t *testing.T) {
	ts := `function f(a, b) { return a && b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.And(")
}

func TestUnaryNotUsesJSValueHelper(t *testing.T) {
	ts := `function f(x) { if (!x) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(x).Bool()")
}

func TestUnaryNegUsesJSValueHelper(t *testing.T) {
	ts := `function f(x) { return -x; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Neg(")
}

func TestTypeofUsesJSValueHelper(t *testing.T) {
	ts := `function f(x) { return typeof x; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.TypeOf(")
}

func TestBitNotInBoolContext(t *testing.T) {
	ts := `function f(s) { var i = "abc"; if (~i) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.BitNot(i).Bool()")
}

func TestNullishCoalescingUsesJSValue(t *testing.T) {
	ts := `function f(x) { return x ?? "default"; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Nullish(")
}

func TestEnsureBoolOnJSValueCallUsesBool(t *testing.T) {
	// JSValue expressions in boolean context use .Bool(), not != nil.
	ts := `function f(x) { if (x.get("key")) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, ".Bool()")
}

func TestTernaryWithJSValueBranchesReturnsJSValue(t *testing.T) {
	ts := `function f(x, y) { return x ? x : y; }`
	out := compile(t, ts)
	assertContains(t, out, "func() *jsvalue.JSValue")
}

func TestBinaryExpressionJSValuePropagation(t *testing.T) {
	// Variable from binary expression with JSValue operands → tracked as JSValue.
	ts := `function f(s) {
	var check = s !== s.toLowerCase() && s !== s.toUpperCase();
	if (!check) { return s; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.And(")
	assertContains(t, out, "jsvalue.Not(check)")
}

func TestPackageLevelUntypedVarUsesGet(t *testing.T) {
	ts := `var obj = {};
const x = obj.foo;`
	out := compile(t, ts)
	assertContains(t, out, `.Get("foo")`)
}

func TestUntypedLocalCallUsesJSValueCall(t *testing.T) {
	ts := `function f(fn) { return fn(42); }`
	out := compile(t, ts)
	assertContains(t, out, ".Call(")
}

func TestChainedStringConcatAllUsesJSValueAdd(t *testing.T) {
	// Chained + with JSValue operands should all become jsvalue.Add, not native +.
	ts := `function f(x) { return "a" + x + "b"; }`
	out := compile(t, ts)
	assertNotContains(t, out, `+ "b"`)
	assertContains(t, out, "jsvalue.Add(jsvalue.Add(")
}

func TestChainedConcatWithMemberAccessUsesJSValueAdd(t *testing.T) {
	// String concat involving member access on globals should use jsvalue.Add.
	ts := `function f(pos) {
		return "was: " + pos + " limit: " + Error.stackTraceLimit;
	}`
	out := compile(t, ts)
	assertNotContains(t, out, `+ "`)
	assertContains(t, out, "jsvalue.Add(")
}

func TestNewExpressionPropertyUsesGet(t *testing.T) {
	// Property access on new expression results should use .Get() not Go field access.
	ts := `function f() { return new Error("x").message; }`
	out := compile(t, ts)
	assertContains(t, out, `.Get("message")`)
	assertNotContains(t, out, ".Message")
}

func TestNewExpressionPropertyStackUsesGet(t *testing.T) {
	// new Error().stack → .Get("stack") not .Stack
	ts := `function f() { const s = new Error().stack; return s; }`
	out := compile(t, ts)
	assertContains(t, out, `.Get("stack")`)
	assertNotContains(t, out, ".Stack")
}

func TestNewExpressionChainedPropertyAccess(t *testing.T) {
	// Chained property access on new expression should use .Get()
	ts := `function f() { return new Error("x").name; }`
	out := compile(t, ts)
	assertContains(t, out, `.Get("name")`)
	assertNotContains(t, out, ".Name")
}

func TestMethodCallOnLocalUsesGetCall(t *testing.T) {
	// Method calls on local variables should use .MethodCall("method",this, ...)
	// since all locals are *jsvalue.JSValue in the all-JSValue architecture.
	ts := `import { statSync } from 'fs';
function f(dir) {
	const stats = statSync(dir);
	return stats.isDirectory();
}`
	out := compile(t, ts)
	assertContains(t, out, `stats.MethodCall("isDirectory")`)
	assertNotContains(t, out, "stats.IsDirectory()")
}

func TestMethodCallOnParamUsesGetCall(t *testing.T) {
	// Method calls on function parameters use .MethodCall() for dynamic dispatch.
	ts := `function f(obj) { return obj.doSomething(1, 2); }`
	out := compile(t, ts)
	assertContains(t, out, `obj.MethodCall("doSomething",`)
}

func TestArrowFuncWrappedInNewFunction(t *testing.T) {
	// Arrow functions assigned to variables become jsvalue.NewFunction
	ts := `const greet = (name) => name;`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
}

func TestFuncExprWrappedInNewFunction(t *testing.T) {
	// Function expressions become jsvalue.NewFunction
	ts := `const add = function(a, b) { return a + b; };`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
}

func TestArrowReturningTemplateStringWrapped(t *testing.T) {
	// Arrow returning template string: return value wrapped in jsvalue.From()
	ts := "const greet = (name) => `hello ${name}`;"
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.From(fmt.Sprintf(")
}

func TestTranspiledImportCalledViaCall(t *testing.T) {
	// Transpiled cross-package imports use .Call() for invocation
	ts := `import emojiRegex from 'emoji-regex';
const re = emojiRegex();`
	out := compileWithModule(t, ts, "myapp")
	assertContains(t, out, "emoji_regex.Default.Call(")
}

func TestInOperatorUsesHasOwnProperty(t *testing.T) {
	ts := `function check(key: string, obj: any): boolean {
		return key in obj;
	}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewBool(")
	assertContains(t, out, ".HasOwnProperty(")
}

func TestNegatedInOperatorUsesJSValueNot(t *testing.T) {
	ts := `function missing(key: string, obj: any): boolean {
		return !(key in obj);
	}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(jsvalue.NewBool(")
}

func TestNegatedHasOwnPropertyCallUsesJSValueNot(t *testing.T) {
	// Object.prototype.hasOwnProperty.call(obj, key) is treated as JSValue
	ts := `function check(obj: any, key: string) {
		if (!Object.prototype.hasOwnProperty.call(obj, key)) { return; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(jsvalue.NewBool(")
}

func TestTernaryNegativeLiteralNoNumberCall(t *testing.T) {
	// -1 in a ternary should not get .Number() called on it
	ts := `function cmp(a: boolean): number {
		return a ? 1 : -1;
	}`
	out := compile(t, ts)
	assertNotContains(t, out, "(-1).Number()")
	assertNotContains(t, out, ".Number()")
}

func TestTernaryNestedInfersType(t *testing.T) {
	// Nested ternary with int branches should infer int type
	ts := `function f(a: boolean, b: boolean): number {
		return a ? 0 : b ? 1 : -1;
	}`
	out := compile(t, ts)
	assertNotContains(t, out, ".Number()")
}

func TestAssignmentExprSubscriptOnJSValueUsesSet(t *testing.T) {
	// Assignment expression with subscript LHS on JSValue should use .Set()
	ts := `function f(obj: any, key: string) {
		let x = (obj[key] = true);
	}`
	out := compile(t, ts)
	assertContains(t, out, `.Set(`)
	assertNotContains(t, out, `.Get(jsvalue.PropertyKey(key)) =`)
}

func TestFuncExprWithThisExtractsFromArgs(t *testing.T) {
	// Function expressions that use 'this' should extract it from _args
	ts := `let obj = { greet: function() { return this.name; } };`
	out := compile(t, ts)
	assertContains(t, out, `this`)
	assertContains(t, out, `.Get("name")`)
}

func TestTemplateLiteralAssignedToJSValueParamWrapped(t *testing.T) {
	// Template literal assigned to a JSValue variable should be wrapped
	ts := "function f($0: string) { $0 = `./${$0}`; return $0; }"
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.From(fmt.Sprintf")
}

func TestSpreadInFunctionCalls(t *testing.T) {
	// 1. fn(...args) where fn is a local JSValue function parameter
	// generates fn.Call(args.Array()...)
	t.Run("local_function_spread", func(t *testing.T) {
		ts := `function f(fn, args) { return fn(...args); }`
		out := compile(t, ts)
		assertContains(t, out, "fn.Call(args.Array()...)")
		assertNotContains(t, out, "fn(")
	})

	// 2. this.method(...args) inside a class method
	// generates this.MethodCall("method", args.Array()...)
	t.Run("this_method_spread", func(t *testing.T) {
		ts := `class Foo {
	dispatch(args) {
		return this.method(...args);
	}
}`
		out := compile(t, ts)
		assertContains(t, out, `this.MethodCall("method", args.Array()...)`)
		assertNotContains(t, out, "this.Method(")
	})

	// 3. obj.method(...args) where obj is a package-level untyped var
	// generates obj.MethodCall("method", args.Array()...)
	t.Run("pkg_var_method_spread", func(t *testing.T) {
		ts := `var obj;
function f(args) { return obj.method(...args); }`
		out := compile(t, ts)
		assertContains(t, out, `obj.MethodCall("method", args.Array()...)`)
		assertNotContains(t, out, "obj.Method(")
	})
}

func TestSamePackageTranspiledCallUsesCall(t *testing.T) {
	// Same-package transpiled imports also use .Call()
	ts := `import { helper } from "./utils";
helper();`
	out, err := Compile([]byte(ts), "mypkg", "mymod", true)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(out), "Helper.Call()")
}
