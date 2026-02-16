package compiler

import "testing"

func TestConsoleLog(t *testing.T) {
	out := compile(t, `console.log("hello");`)
	assertContains(t, out, `fmt.Println("hello")`)
	assertContains(t, out, `"fmt"`)
}

func TestConsoleError(t *testing.T) {
	out := compile(t, `console.error("fail");`)
	assertContains(t, out, "fmt.Fprintln(os.Stderr")
	assertContains(t, out, `"os"`)
}

func TestMathFloor(t *testing.T) {
	ts := `function f(x: number): number { return Math.floor(x); }`
	out := compile(t, ts)
	assertContains(t, out, "math.Floor(x)")
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
	assertContains(t, out, "json.Marshal(x)")
	assertContains(t, out, `"encoding/json"`)
}

func TestJSONParse(t *testing.T) {
	ts := `function deser(s: string): any { return JSON.parse(s); }`
	out := compile(t, ts)
	assertContains(t, out, "json.Unmarshal")
	assertContains(t, out, "jsvalue.From(v)")
	assertContains(t, out, "*jsvalue.JSValue")
}

func TestStringMethods(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"split", `function f(s: string): any { return s.split(","); }`, `strings.Split(s, ",")`},
		{"trim", `function f(s: string): any { return s.trim(); }`, `strings.TrimSpace(s)`},
		{"toLowerCase", `function f(s: string): any { return s.toLowerCase(); }`, `strings.ToLower(s)`},
		{"toUpperCase", `function f(s: string): any { return s.toUpperCase(); }`, `strings.ToUpper(s)`},
		{"startsWith", `function f(s: string): any { return s.startsWith("a"); }`, `strings.HasPrefix(s, "a")`},
		{"endsWith", `function f(s: string): any { return s.endsWith("z"); }`, `strings.HasSuffix(s, "z")`},
		{"repeat", `function f(s: string): any { return s.repeat(3); }`, `strings.Repeat(s, 3)`},
		{"trimStart", `function f(s: string): any { return s.trimStart(); }`, `strings.TrimLeft(s, " \t\n\r")`},
		{"trimEnd", `function f(s: string): any { return s.trimEnd(); }`, `strings.TrimRight(s, " \t\n\r")`},
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
	assertContains(t, out, "len(arr)")
}

func TestArrayPush(t *testing.T) {
	ts := `function add(arr: number[]): any { return arr.push(1); }`
	out := compile(t, ts)
	assertContains(t, out, "append(arr, 1)")
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
	assertContains(t, out, "os.Args")
	assertContains(t, out, `"os"`)
}

func TestProcessEnvAccess(t *testing.T) {
	ts := `const home = process.env.HOME;`
	out := compile(t, ts)
	assertContains(t, out, `os.Getenv("HOME")`)
}

func TestProcessExit(t *testing.T) {
	ts := `process.exit(1);`
	out := compile(t, ts)
	assertContains(t, out, "os.Exit(1)")
}

func TestObjectCreateNull(t *testing.T) {
	ts := `const obj = Object.create(null);`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewObject()")
	assertNotContains(t, out, "Object.Create")
}

func TestObjectKeysTransform(t *testing.T) {
	ts := `function f(obj: any): any { return Object.keys(obj); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Keys(obj)")
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
	assertContains(t, out, ".MatchString(s)")
	assertNotContains(t, out, ".Test(")
}

func TestArrayConcat(t *testing.T) {
	ts := `function f(arr: number[]): any { return arr.concat(1); }`
	out := compile(t, ts)
	assertContains(t, out, "append(arr, 1)")
	assertNotContains(t, out, ".Concat(")
}

func TestMathMinCoercesFloat64(t *testing.T) {
	ts := `function f(a: number, b: number): number { return Math.min(a, b); }`
	out := compile(t, ts)
	assertContains(t, out, "math.Min(float64(a), float64(b))")
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
	assertContains(t, out, `jsvalue.Push(flags["keys"]`)
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
	assertContains(t, out, `jsvalue.Pop(flags["keys"])`)
	assertNotContains(t, out, "_arr :=")
}

func TestNegativeSliceIndex(t *testing.T) {
	ts := `function f(arr: number[]): number[] {
	return arr.slice(0, -1);
}`
	out := compile(t, ts)
	// Should convert -1 to len(arr)-1 (Go formatter adds spaces around :)
	assertContains(t, out, "len(arr)-1")
	assertNotContains(t, out, ":-1")
}

func TestNegativeSliceIndexOnJSValue(t *testing.T) {
	ts := `function f(arr) {
	return arr.slice(1, -2);
}`
	out := compile(t, ts)
	// Should use jsvalue.Slice wrapper which handles negative indices internally
	assertContains(t, out, "jsvalue.Slice(arr, 1, -2)")
	assertNotContains(t, out, "len(arr.Array())")
}

func TestJoinOnJSValueArray(t *testing.T) {
	ts := `function f(arr) {
	return arr.join(",");
}`
	out := compile(t, ts)
	// Should use jsvalue.Join wrapper
	assertContains(t, out, `jsvalue.Join(arr, ",")`)
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
