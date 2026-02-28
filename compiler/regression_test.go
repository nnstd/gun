package compiler

import "testing"

// Regression tests for issues fixed during the all-JSValue refactoring.
// Each test documents a specific bug that was encountered and fixed.

// --- Phase 1: JSValue helper signatures ---

func TestReplaceAcceptsRegexJSValue(t *testing.T) {
	// jsvalue.Replace now accepts *JSValue for pattern (string or regex).
	// Previously failed: cannot use regexp.MustCompile(...) as string.
	ts := `function f(s) { return s.replace(/abc/, "xyz"); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Replace(")
	assertNotContains(t, out, "strings.Replace(")
}

func TestSplitAcceptsJSValueSeparator(t *testing.T) {
	// jsvalue.Split now takes *JSValue separator.
	ts := `function f(key) { return key.split("."); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Split(key,")
	assertContains(t, out, `jsvalue.NewString(".")`)
}

func TestCharAtAcceptsJSValueIndex(t *testing.T) {
	// jsvalue.CharAt now takes *JSValue index.
	// Previously failed: cannot use i (int) as *JSValue.
	ts := `function f(s) { return s.charAt(0); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.CharAt(s,")
}

func TestSliceAcceptsJSValueArgs(t *testing.T) {
	// jsvalue.Slice now takes ...*JSValue args.
	ts := `function f(arr) { return arr.slice(1, 3); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Slice(arr,")
	assertContains(t, out, "jsvalue.NewNumber")
}

// --- Phase 2: Builtin dispatchers wrap args ---

func TestLastIndexOfWrapsArgs(t *testing.T) {
	// LastIndexOf args wrapped as JSValue.
	ts := `function f(s) { return s.lastIndexOf("x"); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.LastIndexOf(s,")
}

func TestJoinWrapsJSValueSeparator(t *testing.T) {
	// Join separator wrapped as JSValue.
	ts := `function f(arr) { return arr.join(","); }`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.Join(arr, jsvalue.NewString(","))`)
}

func TestBitNotInBoolContext(t *testing.T) {
	// Bitwise NOT used in boolean context: ~expr → != 0.
	ts := `function f(s) { var i = "abc"; if (~i) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, "!= 0")
}

func TestIsArrayReturnsJSValue(t *testing.T) {
	// Array.isArray returns *JSValue via jsvalue.IsArrayValue.
	ts := `function f(x) { return Array.isArray(x); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.IsArrayValue(x)")
}

// --- Phase 3: Binary/unary use JSValue helpers ---

func TestBinaryAddUsesJSValueHelper(t *testing.T) {
	// x + y where x is JSValue → jsvalue.Add(x, y).
	ts := `function f(a, b) { return a + b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Add(")
}

func TestBinaryEqUsesJSValueHelper(t *testing.T) {
	// x === y where x is JSValue → jsvalue.Eq(x, y).
	ts := `function f(a, b) { return a === b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Eq(")
}

func TestBinaryLtUsesJSValueHelper(t *testing.T) {
	// x < y where x is JSValue → jsvalue.Lt(x, y).
	ts := `function f(a, b) { return a < b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Lt(")
}

func TestLogicalOrUsesJSValueHelper(t *testing.T) {
	// x || y where x is JSValue → jsvalue.Or(x, y).
	ts := `function f(x) { return x || "default"; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Or(")
}

func TestLogicalAndUsesJSValueHelper(t *testing.T) {
	// x && y where x is JSValue → jsvalue.And(x, y).
	ts := `function f(a, b) { return a && b; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.And(")
}

func TestUnaryNotUsesJSValueHelper(t *testing.T) {
	// !x where x is JSValue → jsvalue.Not(x).
	ts := `function f(x) { if (!x) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Not(x).Bool()")
}

func TestUnaryNegUsesJSValueHelper(t *testing.T) {
	// -x where x is JSValue → jsvalue.Neg(x).
	ts := `function f(x) { return -x; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Neg(")
}

func TestTypeofUsesJSValueHelper(t *testing.T) {
	// typeof x where x is JSValue → jsvalue.TypeOf(x).
	ts := `function f(x) { return typeof x; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.TypeOf(")
}

func TestEnsureBoolUsesJSValueBoolMethod(t *testing.T) {
	// JSValue expressions in boolean context use .Bool(), not != nil.
	ts := `function f(x) { if (x.get("key")) { return true; } }`
	out := compile(t, ts)
	assertContains(t, out, ".Bool()")
}

// --- Phase 3d: Function signatures all-JSValue ---

func TestTypedParamsAreJSValue(t *testing.T) {
	// Typed params (: number, : string) still emit *jsvalue.JSValue.
	ts := `function f(x: number, s: string): boolean { return true; }`
	out := compile(t, ts)
	assertContains(t, out, "x *jsvalue.JSValue")
	assertContains(t, out, "s *jsvalue.JSValue")
	assertContains(t, out, ") *jsvalue.JSValue")
	assertNotContains(t, out, "float64")
	assertNotContains(t, out, ") bool")
}

func TestForLoopConditionWithJSValueComparison(t *testing.T) {
	// For loop with JSValue comparison in condition uses jsvalue helpers.
	ts := `function f(arr, n) { for (let i = 0; i < n; i++) {} }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Lt(")
}

func TestTernaryWithJSValueBranchesReturnsJSValue(t *testing.T) {
	// Ternary where branches involve JSValue → IIFE returns *jsvalue.JSValue.
	ts := `function f(x, y) { return x ? x : y; }`
	out := compile(t, ts)
	assertContains(t, out, "func() *jsvalue.JSValue")
}

func TestBinaryExpressionJSValuePropagation(t *testing.T) {
	// Variable assigned from binary expression with JSValue operands
	// should be tracked as JSValue (not typed bool).
	ts := `function f(s) {
	var check = s !== s.toLowerCase() && s !== s.toUpperCase();
	if (!check) { return s; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.And(")
	assertContains(t, out, "jsvalue.Not(check)")
}

// --- Phase 4: Runtime packages accept JSValue ---

func TestFsReadFileSyncArgsWrapped(t *testing.T) {
	// String literal args to fs.ReadFileSync get wrapped as JSValue.
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `fs.ReadFileSync(jsvalue.NewString("hello.txt"))`)
}

func TestProcessVersionMember(t *testing.T) {
	// process.version → process.Version (not jsvalue.NewBool(true).Version).
	ts := `const v = process.version;`
	out := compile(t, ts)
	assertContains(t, out, "process.Version")
	assertNotContains(t, out, "NewBool(true).Version")
}

func TestProcessAsStandaloneUsesAsJSValue(t *testing.T) {
	// Standalone `process` in comparisons → process.AsJSValue().
	ts := `const exists = process ? true : false;`
	out := compile(t, ts)
	assertContains(t, out, "process.AsJSValue()")
}

// --- Structural fixes ---

func TestRegExpPatternUnwrapsJSValue(t *testing.T) {
	// new RegExp(jsValueExpr) where arg is a jsvalue.Add result → .String() unwrap.
	ts := `function f(prefix) { return new RegExp("^" + prefix + "$"); }`
	out := compile(t, ts)
	assertContains(t, out, "regexp.MustCompile(")
	assertContains(t, out, ".String()")
}

func TestPackageLevelUntypedVarUsesGet(t *testing.T) {
	// Package-level untyped vars use .Get() for property access.
	ts := `var obj = {};
const x = obj.foo;`
	out := compile(t, ts)
	assertContains(t, out, `.Get("foo")`)
}

func TestUntypedLocalCallUsesJSValueCall(t *testing.T) {
	// Calling an untyped local (JSValue function) uses .Call().
	ts := `function f(fn) { return fn(42); }`
	out := compile(t, ts)
	assertContains(t, out, ".Call(")
}

func TestNullishCoalescingUsesJSValue(t *testing.T) {
	// ?? operator with JSValue → jsvalue.Nullish(a, b).
	ts := `function f(x) { return x ?? "default"; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Nullish(")
}
