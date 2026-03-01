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
	assertContains(t, out, "jsvalue.Gt(")
	assertContains(t, out, `jsvalue.NewString("pos")`)
	assertContains(t, out, `jsvalue.NewString("neg")`)
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
	// for-in on JSValue uses .OwnKeys() for property enumeration
	assertContains(t, out, "for _, k := range obj.OwnKeys()")
}

func TestWhileLoop(t *testing.T) {
	ts := `function countdown(n: number): void {
		while (n > 0) { n--; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Gt(")
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
	// For loop init wraps literal in JSValue
	assertContains(t, out, "for i := jsvalue.NewNumber(float64(0));")
}

func TestThrowStatement(t *testing.T) {
	ts := `function fail(): void { throw new Error("boom"); }`
	out := compile(t, ts)
	assertContains(t, out, `panic(jserror.Error.Call(`)
	assertNotContains(t, out, `errors.New`)
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
	assertContains(t, out, `_item.Get("name")`)
}

func TestModuleExportsFunction(t *testing.T) {
	ts := `module.exports = function getCallerFile(position) {
		return position;
	}`
	out := compile(t, ts)
	assertContains(t, out, "var Default = jsvalue.NewFunction(")
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
	// i++ where i is JSValue becomes i = jsvalue.NewNumber(i.Number() + 1)
	ts := `function f(): void {
	for (let i = 0; i < 10; i++) { console.log(i); }
}`
	out := compile(t, ts)
	assertContains(t, out, "i = jsvalue.NewNumber(i.Number() + 1)")
}

func TestAugmentedAssignJSValueToString(t *testing.T) {
	// In all-JSValue mode, += on JSValue var uses jsvalue.Add
	ts := `function f(item) {
	let result = "";
	result += item;
	return result;
}`
	out := compile(t, ts)
	assertContains(t, out, "result = jsvalue.Add(result,")
}

func TestAugmentedAssignJSValueToNumber(t *testing.T) {
	// When a typed numeric local is combined with a JSValue via +=,
	// the RHS should be coerced with .Number().
	ts := `function f(jsval) {
	let width: number = 0;
	width += jsval;
	return width;
}`
	out := compile(t, ts)
	assertContains(t, out, "width += jsval.Number()")
	assertNotContains(t, out, "width += fmt.Sprint(jsval)")
}

func TestAugmentedAssignImportedFunctionToNumber(t *testing.T) {
	// When an imported function (which returns JSValue) is used in augmented
	// assignment with a numeric local, the RHS should be coerced with .Number().
	ts := `import * as lib from 'some-lib';
function f() {
	let width: number = 0;
	width += lib.getValue();
	return width;
}`
	out := compileWithModule(t, ts, "test")
	assertContains(t, out, "width += some_lib.GetValue().Number()")
	assertNotContains(t, out, "width += some_lib.GetValue())")
}

func TestAugmentedAssignNamedImportToNumber(t *testing.T) {
	// When a named imported function is used in augmented assignment with
	// a numeric local, the RHS should be coerced with .Number().
	ts := `import { getValue } from 'some-lib';
function f() {
	let width: number = 0;
	width += getValue();
	return width;
}`
	out := compileWithModule(t, ts, "test")
	assertContains(t, out, "width += some_lib.GetValue.Call().Number()")
}

func TestAssignToUntypedLocalWrapsJSValue(t *testing.T) {
	// Assigning a string expression to an untyped local (JSValue) should
	// use jsvalue wrapper for the method call.
	ts := `function f(s) {
	s = s.toLowerCase();
	return s;
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ToLowerCase(s)")
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
	// Variables initialized from string methods on JSValue receivers (toLowerCase etc.)
	// should be tracked as JSValue locals and coerced when used with native Go functions.
	ts := `function f(s) {
	const lower = s.toLowerCase();
	return lower.indexOf("x");
}`
	out := compile(t, ts)
	assertContains(t, out, "strings.Index(fmt.Sprint(lower)")
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

// --- All-JSValue regression tests ---

func TestForLoopConditionWithJSValueComparison(t *testing.T) {
	ts := `function f(arr, n) { for (let i = 0; i < n; i++) {} }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Lt(")
}

func TestForOfJSValueRangeUsesArray(t *testing.T) {
	// for-of on JSValue expression should call .Array() for Go range.
	ts := `function f(items) { for (const item of items) { } }`
	out := compile(t, ts)
	assertContains(t, out, ".Array()")
}

func TestMemberAssignmentOnJSValueUsesSet(t *testing.T) {
	// this.name = value → this.Set("name", value)
	ts := `function f(obj) { obj.name = "hello"; }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Set("name",`)
}

func TestModuleExportsParamReassignWrapsLiteral(t *testing.T) {
	// Function params in module.exports functions are JSValue locals,
	// so reassigning a literal must wrap it.
	ts := `module.exports = function f(position) {
		if (position === undefined) { position = 2; }
		return position;
	}`
	out := compile(t, ts)
	assertContains(t, out, "var Default = jsvalue.NewFunction(")
	assertNotContains(t, out, "position = 2")
	assertContains(t, out, "jsvalue")
}

func TestModuleExportsFunctionTrailingReturn(t *testing.T) {
	// module.exports functions with conditional returns need a trailing
	// return to avoid Go "missing return" errors.
	ts := `module.exports = function f(x) {
		if (x) { return x; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "var Default = jsvalue.NewFunction(")
	assertContains(t, out, "return nil")
}

func TestModuleExportsFunctionParamIsLocal(t *testing.T) {
	// Parameters should be tracked as locals so member access uses .Get().
	ts := `module.exports = function f(opts) {
		return opts.name;
	}`
	out := compile(t, ts)
	assertContains(t, out, `.Get("name")`)
}
