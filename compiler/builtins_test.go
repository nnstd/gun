package compiler

import "testing"

func TestConsoleLog(t *testing.T) {
	out := compile(t, `console.log("hello");`)
	assertContains(t, out, `console.Log(jsvalue.NewString("hello"))`)
	assertContains(t, out, `runtime/builtin/console`)
}

func TestConsoleError(t *testing.T) {
	out := compile(t, `console.error("fail");`)
	assertContains(t, out, `console.Error(jsvalue.NewString("fail"))`)
	assertContains(t, out, `runtime/builtin/console`)
}

func TestMathFloor(t *testing.T) {
	ts := `function f(x: number): number { return Math.floor(x); }`
	out := compile(t, ts)
	assertContains(t, out, `math.AsJSValue.Get("floor").Call(`)
	assertContains(t, out, "runtime/builtin/math")
}

func TestMathRandom(t *testing.T) {
	ts := `function r(): number { return Math.random(); }`
	out := compile(t, ts)
	assertContains(t, out, `math.AsJSValue.Get("random").Call()`)
	assertContains(t, out, "runtime/builtin/math")
}

func TestJSONStringify(t *testing.T) {
	ts := `function ser(x: any): any { return JSON.stringify(x); }`
	out := compile(t, ts)
	assertContains(t, out, `json.AsJSValue.Get("stringify").Call(`)
	assertContains(t, out, `runtime/builtin/json`)
}

func TestJSONParse(t *testing.T) {
	ts := `function deser(s: string): any { return JSON.parse(s); }`
	out := compile(t, ts)
	assertContains(t, out, `json.AsJSValue.Get("parse").Call(`)
	assertContains(t, out, `runtime/builtin/json`)
}

func TestStringMethods(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"split", `function f(s: string): any { return s.split(","); }`, `MethodCall("split"`},
		{"trim", `function f(s: string): any { return s.trim(); }`, `MethodCall("trim"`},
		{"toLowerCase", `function f(s: string): any { return s.toLowerCase(); }`, `MethodCall("toLowerCase"`},
		{"toUpperCase", `function f(s: string): any { return s.toUpperCase(); }`, `MethodCall("toUpperCase"`},
		{"startsWith", `function f(s: string): any { return s.startsWith("a"); }`, `MethodCall("startsWith"`},
		{"endsWith", `function f(s: string): any { return s.endsWith("z"); }`, `MethodCall("endsWith"`},
		{"repeat", `function f(s: string): any { return s.repeat(3); }`, `MethodCall("repeat"`},
		{"trimStart", `function f(s: string): any { return s.trimStart(); }`, `MethodCall("trimStart"`},
		{"trimEnd", `function f(s: string): any { return s.trimEnd(); }`, `MethodCall("trimEnd"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := compile(t, tt.ts)
			assertContains(t, out, tt.want)
		})
	}
}

func TestArrayLength(t *testing.T) {
	ts := `function size(arr: number[]): number { return arr.length; }`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, so .length becomes .Len()
	assertContains(t, out, ".Len()")
}

func TestArrayPush(t *testing.T) {
	ts := `function add(arr: number[]): any { return arr.push(1); }`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, so uses jsvalue.Push
	assertContains(t, out, `MethodCall("push"`)
}

func TestNewTypeError(t *testing.T) {
	ts := `function fail(): void { throw new TypeError("bad input"); }`
	out := compile(t, ts)
	assertContains(t, out, `error.TypeError.Call(`)
	assertNotContains(t, out, `errors.New`)
}

func TestNumberIsSafeInteger(t *testing.T) {
	ts := `const ok = Number.isSafeInteger(42);`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Number.Get("isSafeInteger").Call(`)
}

func TestProcessArgv(t *testing.T) {
	ts := `const args = process.argv;`
	out := compile(t, ts)
	assertContains(t, out, `process.AsJSValue().Get("argv")`)
	assertContains(t, out, `runtime/process`)
}

func TestProcessEnvAccess(t *testing.T) {
	ts := `const home = process.env.HOME;`
	out := compile(t, ts)
	assertContains(t, out, `process.AsJSValue().Get("env").Get("HOME")`)
}

func TestProcessExit(t *testing.T) {
	ts := `process.exit(1);`
	out := compile(t, ts)
	assertContains(t, out, `process.AsJSValue().Get("exit").Call(`)
}

func TestObjectCreateNull(t *testing.T) {
	ts := `const obj = Object.create(null);`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("create").Call(`)
}

func TestObjectKeysTransform(t *testing.T) {
	ts := `function f(obj: any): any { return Object.keys(obj); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("keys").Call(`)
}

func TestObjectEntriesPassthrough(t *testing.T) {
	ts := `function f(obj: any): any { return Object.entries(obj); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("entries").Call(`)
}

func TestRegexTest(t *testing.T) {
	ts := `const re = /hello/;
function check(s: string): boolean { return re.test(s); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewRegex(jsvalue.CompileRegex(")
	assertContains(t, out, "jsvalue.MatchString(re,")
}

func TestArrayConcat(t *testing.T) {
	ts := `function f(arr: number[]): any { return arr.concat(1); }`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, uses jsvalue.Concat
	assertContains(t, out, `MethodCall("concat"`)
}

func TestMathMinCoercesFloat64(t *testing.T) {
	ts := `function f(a: number, b: number): number { return Math.min(a, b); }`
	out := compile(t, ts)
	assertContains(t, out, `math.AsJSValue.Get("min").Call(`)
}

func TestMathMinCoercesJSValueViaNumber(t *testing.T) {
	// JSValue args to Math.min should use .Number(), not float64()
	ts := `function f(x) { var n = Math.min(x, 10); return n; }`
	out := compile(t, ts)
	assertContains(t, out, "x.Number()")
	assertNotContains(t, out, "float64(x)")
}

func TestBooleanIdentifier(t *testing.T) {
	ts := `const fn = Boolean;`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Truthy")
	assertNotContains(t, out, "Boolean")
}

func TestPushOnMapLocalValue(t *testing.T) {
	ts := `function f(key) {
	const flags = {keys: []};
	flags.keys.push(key);
}`
	out := compile(t, ts)
	// Should use prototype method via MethodCall
	assertContains(t, out, `MethodCall("push"`)
	assertNotContains(t, out, `append(flags["keys"].Array()`)
}

func TestPopOnJSValueArray(t *testing.T) {
	ts := `function f(arr) {
	return arr.pop();
}`
	out := compile(t, ts)
	// Should use jsvalue.Pop wrapper
	assertContains(t, out, `MethodCall("pop"`)
	assertNotContains(t, out, "_arr := arr.Array()")
	assertNotContains(t, out, "if len(_arr) > 0")
}

func TestPopOnMapLocalValue(t *testing.T) {
	ts := `function f() {
	const flags = {keys: [1, 2, 3]};
	return flags.keys.pop();
}`
	out := compile(t, ts)
	// Should use prototype method via MethodCall
	assertContains(t, out, `MethodCall("pop"`)
	assertNotContains(t, out, "_arr :=")
}

func TestNegativeSliceIndex(t *testing.T) {
	ts := `function f(arr: number[]): number[] {
	return arr.slice(0, -1);
}`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, uses jsvalue.Slice wrapper
	assertContains(t, out, `MethodCall("slice"`)
}

func TestNegativeSliceIndexOnJSValue(t *testing.T) {
	ts := `function f(arr) {
	return arr.slice(1, -2);
}`
	out := compile(t, ts)
	// Should use prototype method via MethodCall
	assertContains(t, out, `MethodCall("slice"`)
	assertNotContains(t, out, "len(arr.Array())")
}

func TestJoinOnJSValueArray(t *testing.T) {
	ts := `function f(arr) {
	return arr.join(",");
}`
	out := compile(t, ts)
	// Should use prototype method via MethodCall
	assertContains(t, out, `MethodCall("join"`)
	assertNotContains(t, out, "_arr := arr.Array()")
	assertNotContains(t, out, "make([]string")
}

func TestJoinOnMapResult(t *testing.T) {
	// Test join on the result of map (which returns JSValue)
	ts := `function f(arr) {
	return arr.map(x => x).join(".");
}`
	out := compile(t, ts)
	// Should use prototype methods via MethodCall
	assertContains(t, out, `MethodCall("map"`)
	assertContains(t, out, `MethodCall("join"`)
	assertNotContains(t, out, "_arr :=")
	assertNotContains(t, out, "strings.Join")
}

func TestNumberGlobalCall(t *testing.T) {
	ts := `function f(x) { return Number(x); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Number.Call(`)
}

// --- All-JSValue regression tests ---

func TestReplaceAcceptsRegexJSValue(t *testing.T) {
	// jsvalue.Replace accepts *JSValue for pattern (string or regex).
	ts := `function f(s) { return s.replace(/abc/, "xyz"); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("replace"`)
	assertNotContains(t, out, "strings.Replace(")
}

func TestSplitAcceptsJSValueSeparator(t *testing.T) {
	ts := `function f(key) { return key.split("."); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("split"`)
	// args passed directly to .MethodCall()
}

func TestCharAtAcceptsJSValueIndex(t *testing.T) {
	ts := `function f(s) { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("charAt"`)
}

func TestSliceAcceptsJSValueArgs(t *testing.T) {
	ts := `function f(arr) { return arr.slice(1, 3); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("slice"`)
}

func TestLastIndexOfWrapsArgs(t *testing.T) {
	ts := `function f(s) { return s.lastIndexOf("x"); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("lastIndexOf"`)
}

func TestJoinWrapsJSValueSeparator(t *testing.T) {
	ts := `function f(arr) { return arr.join(","); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("join"`)
}

func TestIsArrayReturnsJSValue(t *testing.T) {
	ts := `function f(x) { return Array.isArray(x); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Array.Get("isArray").Call(`)
}

func TestMatchOnJSValueUsesRegexpCompile(t *testing.T) {
	// arg.match(pattern) where both are JSValue compiles regex from pattern.
	ts := `function f(arg, pattern) { return arg.match(pattern); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("match"`)
	// match is now a prototype method call
	assertNotContains(t, out, "pattern.FindStringSubmatch")
}

func TestSpliceOnJSValue(t *testing.T) {
	ts := `function f(arr) { return arr.splice(1, 2); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("splice"`)
}

func TestIndexOfOnComplexReceiverReturnsJSValue(t *testing.T) {
	// indexOf on a .Get() chain returns JSValue so comparisons use jsvalue.Eq.
	ts := `function f(obj, val) { return obj.items.indexOf(val) === -1; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Eq(")
	assertNotContains(t, out, "== -1")
}

func TestRegExpPatternUnwrapsJSValue(t *testing.T) {
	// new RegExp(jsValueExpr) coerces arg to string via fmt.Sprint.
	ts := `function f(prefix) { return new RegExp("^" + prefix + "$"); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.CompileRegex(fmt.Sprint(")
}

func TestProcessVersionMember(t *testing.T) {
	ts := `const v = process.version;`
	out := compile(t, ts)
	assertContains(t, out, `process.AsJSValue().Get("version")`)
}

func TestProcessAsStandaloneUsesAsJSValue(t *testing.T) {
	ts := `const exists = process ? true : false;`
	out := compile(t, ts)
	assertContains(t, out, "process.AsJSValue()")
}

func TestErrorCallNoUnusedImport(t *testing.T) {
	// Error() as a function call should NOT import "errors" package.
	ts := `function f(msg) { return Error(msg); }`
	out := compile(t, ts)
	assertNotContains(t, out, `"errors"`)
	assertContains(t, out, `error.Error.Call(`)
}

func TestPushOnSliceLocalWrapsLiteralArgs(t *testing.T) {
	// push(true) on a []*jsvalue.JSValue slice wraps the literal.
	ts := `function f() { const arr = []; arr.push(true); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("push"`)
	assertContains(t, out, "jsvalue.From(true)")
}

func TestCodePointAtOnJSValueCoerces(t *testing.T) {
	// codePointAt on JSValue receiver should coerce to string via fmt.Sprint.
	ts := `function f(s) { return s.codePointAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("codePointAt"`)
}

func TestRegexTestOnJSValueRegex(t *testing.T) {
	// regex.test(str) on JSValue regex uses jsvalue.MatchString
	ts := `function f(str) {
	const re = /hello/;
	return re.test(str);
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewRegex(")
	assertContains(t, out, "jsvalue.MatchString(re,")
}

func TestObjectPrototypeHasOwnPropertyCall(t *testing.T) {
	// Object.prototype.hasOwnProperty.call(obj, prop) → obj.HasOwnProperty(prop)
	ts := `function f(obj) { return Object.prototype.hasOwnProperty.call(obj, "key"); }`
	out := compile(t, ts)
	assertContains(t, out, "HasOwnProperty")
	assertNotContains(t, out, "object.Prototype")
}

func TestGenericDotCallOnFunction(t *testing.T) {
	// fn.call(thisArg, args) → fn.Call(thisArg, args)
	ts := `function f(fn, obj) { return fn.call(obj, 42); }`
	out := compile(t, ts)
	assertContains(t, out, ".Call(")
}

func TestJSValueFunctionFromGetChainUsesCall(t *testing.T) {
	// obj.method(42) on local uses .MethodCall("method",) for dynamic dispatch.
	ts := `function f(obj) { return obj.method(42); }`
	out := compile(t, ts)
	assertContains(t, out, `obj.MethodCall("method",`)
}

func TestNewErrorUsesJserror(t *testing.T) {
	ts := `function fail() { throw new Error("bad"); }`
	out := compile(t, ts)
	assertContains(t, out, `error.Error.Call(`)
	assertContains(t, out, `runtime/builtin/error`)
	assertNotContains(t, out, `errors.New`)
}

func TestErrorCallUsesJserror(t *testing.T) {
	ts := `function f(msg) { return Error(msg); }`
	out := compile(t, ts)
	assertContains(t, out, `error.Error.Call(`)
}

func TestErrorMessageAccess(t *testing.T) {
	ts := `function f() {
		const err = new Error("test");
		return err.message;
	}`
	out := compile(t, ts)
	assertContains(t, out, `.Get("message")`)
}

func TestErrorStackTraceLimit(t *testing.T) {
	ts := `Error.stackTraceLimit = 0;`
	out := compile(t, ts)
	assertContains(t, out, `error.Error`)
	assertContains(t, out, `Set("stackTraceLimit"`)
}

func TestErrorPrepareStackTrace(t *testing.T) {
	ts := `Error.prepareStackTrace = (err, stack) => stack;`
	out := compile(t, ts)
	assertContains(t, out, `error.Error`)
	assertContains(t, out, `Set("prepareStackTrace"`)
}

func TestErrorCaptureStackTrace(t *testing.T) {
	ts := `const obj = {}; Error.captureStackTrace(obj);`
	out := compile(t, ts)
	assertContains(t, out, `error.Error`)
	assertContains(t, out, `captureStackTrace`)
}

func TestErrorMemberAccess(t *testing.T) {
	ts := `const limit = Error.stackTraceLimit;`
	out := compile(t, ts)
	assertContains(t, out, `error.Error.Get("stackTraceLimit")`)
}

func TestBunServeUsesJSWrapperCall(t *testing.T) {
	ts := `Bun.serve({ port: 3000, fetch() { return new Response("ok") } })`
	out := compile(t, ts)
	assertContains(t, out, `bun.AsJSValue.Get("serve").Call(`)
	assertNotContains(t, out, `bun.Serve(`)
}

func TestInstanceofUsesRuntimeHelper(t *testing.T) {
	ts := `class A {} const foo = {} as any; const ok = foo instanceof A`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.InstanceOf(`)
}

func TestParseIntGlobal(t *testing.T) {
	ts := `const n = parseInt("42", 10);`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ParseInt(")
}

func TestParseIntWithoutRadix(t *testing.T) {
	ts := `const n = parseInt("42");`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ParseInt(")
}

func TestParseFloatGlobal(t *testing.T) {
	ts := `const n = parseFloat("3.14");`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ParseFloat(")
}

func TestInfinityGlobal(t *testing.T) {
	ts := `const x = Infinity;`
	out := compile(t, ts)
	assertContains(t, out, "math.Inf()")
	assertContains(t, out, `runtime/builtin/math`)
}

func TestNaNGlobal(t *testing.T) {
	ts := `const x = NaN;`
	out := compile(t, ts)
	assertContains(t, out, "math.NaN()")
	assertContains(t, out, `runtime/builtin/math`)
}

func TestPromiseGlobal(t *testing.T) {
	ts := `const p = Promise.resolve(42);`
	out := compile(t, ts)
	assertContains(t, out, `promise.Promise.Get("resolve").Call(`)
	assertNotContains(t, out, "undefined: Promise")
}

func TestObjectKeysUsesJsvalue(t *testing.T) {
	ts := `const keys = Object.keys({});`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("keys").Call(`)
}

func TestObjectValuesUsesJsvalue(t *testing.T) {
	ts := `const vals = Object.values({});`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("values").Call(`)
}

func TestObjectEntriesUsesJsvalue(t *testing.T) {
	ts := `const entries = Object.entries({});`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Object.Get("entries").Call(`)
}

func TestToStringOnJSValueReturnsJSValue(t *testing.T) {
	ts := `function f(x: any) { return x.toString(); }`
	out := compile(t, ts)
	assertContains(t, out, `.MethodCall("toString")`)
}
