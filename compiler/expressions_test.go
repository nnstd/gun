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

func TestDollarSignInIdentifier(t *testing.T) {
	ts := `let $0 = "bin";
let default$0 = "node";
console.log($0);`
	out := compile(t, ts)
	assertContains(t, out, "_0")
	assertContains(t, out, "default_0")
	assertNotContains(t, out, "$")
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

func TestLengthOnUntypedParam(t *testing.T) {
	ts := `function f(s) { return s.length; }`
	out := compile(t, ts)
	assertContains(t, out, "s.Len()")
	assertNotContains(t, out, "len(s)")
}
