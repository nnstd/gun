package compiler

import (
	"strings"
	"testing"
)

func TestIfStatement(t *testing.T) {
	ts := `function check(x: number): string {
		if (x > 0) { return "pos"; } else { return "neg"; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "if x > 0")
	assertContains(t, out, `return "pos"`)
	assertContains(t, out, `return "neg"`)
}

func TestForOfLoop(t *testing.T) {
	ts := `function sum(items: number[]): number {
		let total = 0;
		for (const item of items) { total += item; }
		return total;
	}`
	out := compile(t, ts)
	assertContains(t, out, "for _, item := range items")
}

func TestForInLoop(t *testing.T) {
	ts := `function keys(obj: any): void {
		for (const k in obj) { console.log(k); }
	}`
	out := compile(t, ts)
	assertContains(t, out, "for k := range obj")
}

func TestWhileLoop(t *testing.T) {
	ts := `function countdown(n: number): void {
		while (n > 0) { n--; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "for n > 0")
}

func TestDoWhile(t *testing.T) {
	ts := `function f(): void {
		let i = 0;
		do { i++; } while (i < 10);
	}`
	out := compile(t, ts)
	assertContains(t, out, "for {")
	assertContains(t, out, "break")
}

func TestSwitchStatement(t *testing.T) {
	ts := `function describe(x: number): string {
		switch (x) {
			case 1: return "one";
			case 2: return "two";
			default: return "other";
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "switch x")
	assertContains(t, out, "case 1:")
	assertContains(t, out, "case 2:")
	assertContains(t, out, "default:")
}

func TestSwitchBreakStripped(t *testing.T) {
	ts := `function f(x: number): void {
		switch (x) {
			case 1: console.log("one"); break;
			case 2: console.log("two"); break;
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "case 1:")
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "case 1:") || strings.Contains(line, "case 2:") {
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed == "break" {
					t.Errorf("found bare 'break' in switch case body at line %d", j)
				}
			}
		}
	}
}

func TestBreakAndContinue(t *testing.T) {
	ts := `function f(): void {
		for (let i = 0; i < 10; i++) {
			if (i === 5) break;
			if (i === 3) continue;
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "break")
	assertContains(t, out, "continue")
}

func TestForLoopInitUsesShortVarDecl(t *testing.T) {
	ts := `function f(): void {
		for (let i = 0; i < 10; i++) { console.log(i); }
	}`
	out := compile(t, ts)
	assertContains(t, out, "for i := 0;")
	assertNotContains(t, out, "var i")
}

func TestThrowStatement(t *testing.T) {
	ts := `function fail(): void { throw new Error("boom"); }`
	out := compile(t, ts)
	assertContains(t, out, `panic(errors.New("boom"))`)
}

func TestTryCatch(t *testing.T) {
	ts := `function safe(): void {
		try { console.log("try"); } catch (e) { console.log("catch"); }
	}`
	out := compile(t, ts)
	assertContains(t, out, "defer func()")
	assertContains(t, out, "recover()")
}

func TestUseStrictSkipped(t *testing.T) {
	ts := `"use strict";
console.log("hello");`
	out := compile(t, ts)
	assertNotContains(t, out, "use strict")
	assertContains(t, out, "hello")
}

func TestNilExpressionStatementsSkipped(t *testing.T) {
	ts := `function f(): void {
	undefined;
	null;
	console.log("ok");
}`
	out := compile(t, ts)
	assertContains(t, out, `"ok"`)
	assertNotContains(t, out, "\tnil\n")
}

func TestForOfDestructuring(t *testing.T) {
	ts := `function f(items: any[]): void {
		for (const {name: label} of items) {
			console.log(label);
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "for _, _item := range items")
	assertContains(t, out, "label := _item.Name")
}

func TestModuleExportsFunction(t *testing.T) {
	ts := `module.exports = function getCallerFile(position) {
		return position;
	}`
	out := compile(t, ts)
	assertContains(t, out, "func Default(")
	assertNotContains(t, out, "module")
}

func TestFuncVarMemberAssignmentSkipped(t *testing.T) {
	// JS functions are objects and can have properties attached.
	// Go functions cannot, so member assignments on function vars should be skipped.
	ts := `var myFunc = (x) => { return x; };
myFunc.extra = "hello";`
	out := compile(t, ts)
	assertNotContains(t, out, "myFunc.Extra")
	assertNotContains(t, out, `myFunc.extra`)
}

func TestForLoopUpdateExpression(t *testing.T) {
	// i++ in for-loop post should compile as IncDecStmt, not i + 1.
	ts := `function f(): void {
	for (let i = 0; i < 10; i++) { console.log(i); }
}`
	out := compile(t, ts)
	assertContains(t, out, "i++")
	assertNotContains(t, out, "i + 1")
}

func TestAugmentedAssignJSValueToString(t *testing.T) {
	// When a typed string local is combined with a JSValue via +=,
	// the RHS should be coerced with fmt.Sprint().
	ts := `function f(item) {
	let result = "";
	result += item;
	return result;
}`
	out := compile(t, ts)
	assertContains(t, out, "result += fmt.Sprint(item)")
}

func TestAssignToUntypedLocalWrapsJSValue(t *testing.T) {
	// Assigning a string expression to an untyped local (JSValue) should
	// wrap with jsvalue.From().
	ts := `function f(s) {
	s = s.toLowerCase();
	return s;
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.From(")
}

func TestNilInitVarGetsJSValueType(t *testing.T) {
	// Variables initialized with null should get *jsvalue.JSValue type,
	// not produce "use of untyped nil".
	ts := `function f() {
	let x = null;
	x = "hello";
	return x;
}`
	out := compile(t, ts)
	assertContains(t, out, "*jsvalue.JSValue")
	assertNotContains(t, out, "var x = nil\n")
}

func TestNilAssignmentNotWrapped(t *testing.T) {
	// Assigning null to a JSValue var should emit nil, not jsvalue.From(nil).
	ts := `function f(x) { x = null; return x; }`
	out := compile(t, ts)
	assertContains(t, out, "x = nil")
	assertNotContains(t, out, "jsvalue.From(nil)")
}

func TestTypedLocalFromStringMethod(t *testing.T) {
	// Variables initialized from string methods (toLowerCase etc.) should
	// be typed locals, not treated as JSValue.
	ts := `function f(s) {
	const lower = s.toLowerCase();
	return lower.indexOf("x");
}`
	out := compile(t, ts)
	assertContains(t, out, "strings.Index(lower")
	assertNotContains(t, out, "lower.IndexOf")
}

func TestForLoopJSValueIncrement(t *testing.T) {
	// ii++ where ii is *jsvalue.JSValue should use jsvalue.NewNumber, not Go ++.
	ts := `function f(args) {
	var ii;
	for (ii = 0; ii < args.length; ii++) {
		console.log(ii);
	}
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewNumber(ii.Number() + 1)")
	assertNotContains(t, out, "ii++")
}
