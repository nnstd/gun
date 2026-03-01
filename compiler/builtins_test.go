package compiler

import "testing"

func TestConsoleLog(t *testing.T) {
	out := compile(t, `console.log("hello");`)
	assertContains(t, out, `console.Log("hello")`)
	assertContains(t, out, `runtime/console`)
}

func TestConsoleError(t *testing.T) {
	out := compile(t, `console.error("fail");`)
	assertContains(t, out, `console.Error("fail")`)
	assertContains(t, out, `runtime/console`)
}

func TestMathFloor(t *testing.T) {
	ts := `function f(x: number): number { return Math.floor(x); }`
	out := compile(t, ts)
	assertContains(t, out, "jsmath.Floor(")
	assertContains(t, out, "runtime/jsmath")
}

func TestMathRandom(t *testing.T) {
	ts := `function r(): number { return Math.random(); }`
	out := compile(t, ts)
	assertContains(t, out, "jsmath.Random()")
	assertContains(t, out, "runtime/jsmath")
}

func TestJSONStringify(t *testing.T) {
	ts := `function ser(x: any): any { return JSON.stringify(x); }`
	out := compile(t, ts)
	assertContains(t, out, "json.Stringify(x)")
	assertContains(t, out, `runtime/json`)
}

func TestJSONParse(t *testing.T) {
	ts := `function deser(s: string): any { return JSON.parse(s); }`
	out := compile(t, ts)
	assertContains(t, out, "json.Parse(s)")
	assertContains(t, out, `runtime/json`)
}

func TestStringMethods(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"split", `function f(s: string): any { return s.split(","); }`, `jsvalue.Split(s,`},
		{"trim", `function f(s: string): any { return s.trim(); }`, `jsvalue.Trim(s)`},
		{"toLowerCase", `function f(s: string): any { return s.toLowerCase(); }`, `jsvalue.ToLowerCase(s)`},
		{"toUpperCase", `function f(s: string): any { return s.toUpperCase(); }`, `jsvalue.ToUpperCase(s)`},
		{"startsWith", `function f(s: string): any { return s.startsWith("a"); }`, `jsvalue.StartsWith(s,`},
		{"endsWith", `function f(s: string): any { return s.endsWith("z"); }`, `jsvalue.EndsWith(s,`},
		{"repeat", `function f(s: string): any { return s.repeat(3); }`, `jsvalue.Repeat(s,`},
		{"trimStart", `function f(s: string): any { return s.trimStart(); }`, `strings.TrimLeft(`},
		{"trimEnd", `function f(s: string): any { return s.trimEnd(); }`, `strings.TrimRight(`},
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
	assertContains(t, out, "jsvalue.Push(")
}

func TestNewTypeError(t *testing.T) {
	ts := `function fail(): void { throw new TypeError("bad input"); }`
	out := compile(t, ts)
	assertContains(t, out, `jserror.TypeError.Call(`)
	assertNotContains(t, out, `errors.New`)
}

func TestNumberIsSafeInteger(t *testing.T) {
	ts := `const ok = Number.isSafeInteger(42);`
	out := compile(t, ts)
	assertContains(t, out, "true")
	assertNotContains(t, out, "Number")
}

func TestProcessArgv(t *testing.T) {
	ts := `const args = process.argv;`
	out := compile(t, ts)
	assertContains(t, out, "process.Argv")
	assertContains(t, out, `runtime/process`)
}

func TestProcessEnvAccess(t *testing.T) {
	ts := `const home = process.env.HOME;`
	out := compile(t, ts)
	assertContains(t, out, `process.Env.Get("HOME")`)
}

func TestProcessExit(t *testing.T) {
	ts := `process.exit(1);`
	out := compile(t, ts)
	assertContains(t, out, "process.Exit(1)")
}

func TestObjectCreateNull(t *testing.T) {
	ts := `const obj = Object.create(null);`
	out := compile(t, ts)
	assertContains(t, out, "object.Create(nil)")
	assertNotContains(t, out, "Object.Create")
}

func TestObjectKeysTransform(t *testing.T) {
	ts := `function f(obj: any): any { return Object.keys(obj); }`
	out := compile(t, ts)
	assertContains(t, out, "object.Keys(obj)")
	assertNotContains(t, out, "Object")
}

func TestObjectEntriesPassthrough(t *testing.T) {
	ts := `function f(obj: any): any { return Object.entries(obj); }`
	out := compile(t, ts)
	assertNotContains(t, out, "Object")
}

func TestRegexTest(t *testing.T) {
	ts := `const re = /hello/;
function check(s: string): boolean { return re.test(s); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewRegex(regexp.MustCompile(")
	assertContains(t, out, "jsvalue.MatchString(re,")
}

func TestArrayConcat(t *testing.T) {
	ts := `function f(arr: number[]): any { return arr.concat(1); }`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, uses jsvalue.Concat
	assertContains(t, out, "jsvalue.Concat(")
}

func TestMathMinCoercesFloat64(t *testing.T) {
	ts := `function f(a: number, b: number): number { return Math.min(a, b); }`
	out := compile(t, ts)
	// a, b are now *jsvalue.JSValue, coerced to .Number() for math.Min
	assertContains(t, out, "math.Min(")
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
	// Should use jsvalue.Push wrapper
	assertContains(t, out, `jsvalue.Push(flags.Get("keys")`)
	assertContains(t, out, "jsvalue.From")
	assertNotContains(t, out, `append(flags["keys"].Array()`)
}

func TestPopOnJSValueArray(t *testing.T) {
	ts := `function f(arr) {
	return arr.pop();
}`
	out := compile(t, ts)
	// Should use jsvalue.Pop wrapper
	assertContains(t, out, "jsvalue.Pop(arr)")
	assertNotContains(t, out, "_arr := arr.Array()")
	assertNotContains(t, out, "if len(_arr) > 0")
}

func TestPopOnMapLocalValue(t *testing.T) {
	ts := `function f() {
	const flags = {keys: [1, 2, 3]};
	return flags.keys.pop();
}`
	out := compile(t, ts)
	// Should use jsvalue.Pop wrapper
	assertContains(t, out, `jsvalue.Pop(flags.Get("keys"))`)
	assertNotContains(t, out, "_arr :=")
}

func TestNegativeSliceIndex(t *testing.T) {
	ts := `function f(arr: number[]): number[] {
	return arr.slice(0, -1);
}`
	out := compile(t, ts)
	// arr is now *jsvalue.JSValue, uses jsvalue.Slice wrapper
	assertContains(t, out, "jsvalue.Slice(arr,")
}

func TestNegativeSliceIndexOnJSValue(t *testing.T) {
	ts := `function f(arr) {
	return arr.slice(1, -2);
}`
	out := compile(t, ts)
	// Should use jsvalue.Slice wrapper with JSValue-wrapped args
	assertContains(t, out, "jsvalue.Slice(arr,")
	assertContains(t, out, "jsvalue.NewNumber")
	assertNotContains(t, out, "len(arr.Array())")
}

func TestJoinOnJSValueArray(t *testing.T) {
	ts := `function f(arr) {
	return arr.join(",");
}`
	out := compile(t, ts)
	// Should use jsvalue.Join wrapper with JSValue-wrapped separator
	assertContains(t, out, `jsvalue.Join(arr, jsvalue.NewString(","))`)
	assertNotContains(t, out, "_arr := arr.Array()")
	assertNotContains(t, out, "make([]string")
}

func TestJoinOnMapResult(t *testing.T) {
	// Test join on the result of map (which returns JSValue)
	ts := `function f(arr) {
	return arr.map(x => x).join(".");
}`
	out := compile(t, ts)
	// Should use jsvalue.Map and jsvalue.Join wrappers
	assertContains(t, out, "jsvalue.Map(arr,")
	assertContains(t, out, `jsvalue.Join(`)
	assertNotContains(t, out, "_arr :=")
	assertNotContains(t, out, "strings.Join")
}

func TestNumberGlobalCall(t *testing.T) {
	ts := `function f(x) { return Number(x); }`
	out := compile(t, ts)
	assertContains(t, out, "x.Number()")
	assertNotContains(t, out, "float64(x)")
}

// --- All-JSValue regression tests ---

func TestReplaceAcceptsRegexJSValue(t *testing.T) {
	// jsvalue.Replace accepts *JSValue for pattern (string or regex).
	ts := `function f(s) { return s.replace(/abc/, "xyz"); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Replace(")
	assertNotContains(t, out, "strings.Replace(")
}

func TestSplitAcceptsJSValueSeparator(t *testing.T) {
	ts := `function f(key) { return key.split("."); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Split(key,")
	assertContains(t, out, `jsvalue.NewString(".")`)
}

func TestCharAtAcceptsJSValueIndex(t *testing.T) {
	ts := `function f(s) { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.CharAt(s,")
}

func TestSliceAcceptsJSValueArgs(t *testing.T) {
	ts := `function f(arr) { return arr.slice(1, 3); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Slice(arr,")
	assertContains(t, out, "jsvalue.NewNumber")
}

func TestLastIndexOfWrapsArgs(t *testing.T) {
	ts := `function f(s) { return s.lastIndexOf("x"); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.LastIndexOf(s,")
}

func TestJoinWrapsJSValueSeparator(t *testing.T) {
	ts := `function f(arr) { return arr.join(","); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Join(arr, jsvalue.NewString(","))`)
}

func TestIsArrayReturnsJSValue(t *testing.T) {
	ts := `function f(x) { return Array.isArray(x); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.IsArrayValue(x)")
}

func TestMatchOnJSValueUsesRegexpCompile(t *testing.T) {
	// arg.match(pattern) where both are JSValue compiles regex from pattern.
	ts := `function f(arg, pattern) { return arg.match(pattern); }`
	out := compile(t, ts)
	assertContains(t, out, "regexp.MustCompile(")
	assertContains(t, out, "FindStringSubmatch")
	assertNotContains(t, out, "pattern.FindStringSubmatch")
}

func TestSpliceOnJSValue(t *testing.T) {
	ts := `function f(arr) { return arr.splice(1, 2); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Splice(")
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
	assertContains(t, out, "regexp.MustCompile(fmt.Sprint(")
}

func TestProcessVersionMember(t *testing.T) {
	ts := `const v = process.version;`
	out := compile(t, ts)
	assertContains(t, out, "process.Version")
	assertNotContains(t, out, "NewBool(true).Version")
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
	assertContains(t, out, `jserror.Error.Call(`)
}

func TestPushOnSliceLocalWrapsLiteralArgs(t *testing.T) {
	// push(true) on a []*jsvalue.JSValue slice wraps the literal.
	ts := `function f() { const arr = []; arr.push(true); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Push(")
	assertContains(t, out, "jsvalue.From(true)")
}

func TestCodePointAtOnJSValueCoerces(t *testing.T) {
	// codePointAt on JSValue receiver should coerce to string via fmt.Sprint.
	ts := `function f(s) { return s.codePointAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, "fmt.Sprint(s)")
	assertContains(t, out, "[]rune(")
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
	// obj.method(42) on local uses .Get("method").Call() for dynamic dispatch.
	ts := `function f(obj) { return obj.method(42); }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get("method").Call(`)
}

func TestNewErrorUsesJserror(t *testing.T) {
	ts := `function fail() { throw new Error("bad"); }`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error.Call(`)
	assertContains(t, out, `runtime/jserror`)
	assertNotContains(t, out, `errors.New`)
}

func TestErrorCallUsesJserror(t *testing.T) {
	ts := `function f(msg) { return Error(msg); }`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error.Call(`)
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
	assertContains(t, out, `jserror.Error`)
	assertContains(t, out, `Set("stackTraceLimit"`)
}

func TestErrorPrepareStackTrace(t *testing.T) {
	ts := `Error.prepareStackTrace = (err, stack) => stack;`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error`)
	assertContains(t, out, `Set("prepareStackTrace"`)
}

func TestErrorCaptureStackTrace(t *testing.T) {
	ts := `const obj = {}; Error.captureStackTrace(obj);`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error`)
	assertContains(t, out, `captureStackTrace`)
}

func TestErrorMemberAccess(t *testing.T) {
	ts := `const limit = Error.stackTraceLimit;`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error.Get("stackTraceLimit")`)
}
