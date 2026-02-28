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
	assertContains(t, out, "math.Floor(")
	assertContains(t, out, `"math"`)
}

func TestMathRandom(t *testing.T) {
	ts := `function r(): number { return Math.random(); }`
	out := compile(t, ts)
	assertContains(t, out, "rand.Float64()")
	assertContains(t, out, `"math/rand"`)
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
	assertContains(t, out, `errors.New("bad input")`)
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
	assertContains(t, out, `process.Env["HOME"]`)
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
	assertContains(t, out, ".MatchString(")
	assertNotContains(t, out, ".Test(")
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
