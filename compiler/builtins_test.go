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

func TestRegexTest(t *testing.T) {
	ts := `const re = /hello/;
function check(s: string): boolean { return re.test(s); }`
	out := compile(t, ts)
	assertContains(t, out, ".MatchString(s)")
	assertNotContains(t, out, ".Test(")
}
