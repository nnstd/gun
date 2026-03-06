package compiler

import "testing"

func TestConstDeclaration(t *testing.T) {
	out := compile(t, `const x: number = 42;`)
	assertContains(t, out, "var x float64 = 42")
}

func TestLetDeclaration(t *testing.T) {
	out := compile(t, `let name: string = "hello";`)
	assertContains(t, out, `var name string = "hello"`)
}

func TestVarDeclarationInferred(t *testing.T) {
	out := compile(t, `var flag = true;`)
	assertContains(t, out, "var flag = jsvalue.NewBool(true)")
}

func TestConstStringDeclaration(t *testing.T) {
	out := compile(t, `const msg = "hi";`)
	assertContains(t, out, `var msg = jsvalue.NewString("hi")`)
}

func TestFunctionDeclaration(t *testing.T) {
	out := compile(t, `function add(a: number, b: number): number { return a + b; }`)
	assertContains(t, out, "var add = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
	assertContains(t, out, "jsvalue.Add(")
}

func TestArrowFunction(t *testing.T) {
	out := compile(t, `const double = (x: number): number => x * 2;`)
	// Arrow functions are wrapped in jsvalue.NewFunction for all-JSValue consistency
	assertContains(t, out, "jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
	assertContains(t, out, "jsvalue.Mul(")
}

func TestExportCapitalizesName(t *testing.T) {
	out := compile(t, `export function greet(name: string): string { return name; }`)
	assertContains(t, out, "var Greet = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
}

func TestInterfaceWithMethods(t *testing.T) {
	out := compile(t, `interface Reader { read(buf: string): number; }`)
	assertContains(t, out, "type Reader interface")
	assertContains(t, out, "Read(buf *jsvalue.JSValue) *jsvalue.JSValue")
}

func TestInterfaceWithProperties(t *testing.T) {
	out := compile(t, `interface User { name: string; age: number; }`)
	assertContains(t, out, "type User struct")
	assertContains(t, out, "Name string")
	assertContains(t, out, "Age")
}

func TestClassDeclaration(t *testing.T) {
	ts := `class Dog {
		name: string;
		constructor(name: string) { this.name = name; }
		bark(): string { return this.name; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewClass(")
	assertContains(t, out, `this.Set("name",`)
	assertContains(t, out, `Dog.Get("prototype").Set("bark"`)
}

func TestClassExtends(t *testing.T) {
	ts := `class Animal { name: string; }
	class Dog extends Animal { bark(): string { return this.name; } }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewClass(")
	assertContains(t, out, "Animal)") // parent class passed to NewClass
}

func TestNumericEnum(t *testing.T) {
	out := compile(t, `enum Color { Red, Green, Blue }`)
	assertContains(t, out, "type Color int")
	assertContains(t, out, "ColorRed Color = iota")
	assertContains(t, out, "ColorGreen")
	assertContains(t, out, "ColorBlue")
}

func TestStringEnum(t *testing.T) {
	ts := `enum Direction { Up = "UP", Down = "DOWN" }`
	out := compile(t, ts)
	assertContains(t, out, "type Direction string")
	assertContains(t, out, `"UP"`)
	assertContains(t, out, `"DOWN"`)
}

func TestTypeAlias(t *testing.T) {
	out := compile(t, `type ID = string;`)
	assertContains(t, out, "type ID string")
}

func TestParamGoKeywordEscaped(t *testing.T) {
	ts := `const emitWarning = (warning: string, type: string) => process.emitWarning(warning, type)`
	out := compile(t, ts)
	assertContains(t, out, "type_ *jsvalue.JSValue")
	assertNotContains(t, out, "type string")
}

func TestRestParameter(t *testing.T) {
	ts := `function sum(...nums: number[]): number { return 0; }`
	out := compile(t, ts)
	// Rest param passed as slice from _args to inner function
	assertContains(t, out, "nums ...*jsvalue.JSValue")
	assertContains(t, out, "_args[0:]")
}

func TestOptionalParameter(t *testing.T) {
	ts := `function greet(name?: string): void {}`
	out := compile(t, ts)
	assertContains(t, out, "name *jsvalue.JSValue")
}

func TestNullableUnionType(t *testing.T) {
	ts := `function maybe(x: string | null): void {}`
	out := compile(t, ts)
	assertContains(t, out, "x *jsvalue.JSValue")
}

func TestBooleanType(t *testing.T) {
	ts := `function check(b: boolean): boolean { return b; }`
	out := compile(t, ts)
	assertContains(t, out, "var check = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
}

func TestMainFunctionGenerated(t *testing.T) {
	out := compile(t, `console.log("hi");`)
	assertContains(t, out, "func main()")
}

func TestNonMainPackageUsesInit(t *testing.T) {
	out, err := Compile([]byte(`console.log("hi");`), "lib", "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	assertContains(t, s, "package lib")
	assertContains(t, s, "func init()")
	assertNotContains(t, s, "func main()")
}

func TestVarNoTypeNoValue(t *testing.T) {
	ts := `var _foo, _bar;`
	out := compile(t, ts)
	assertContains(t, out, "*jsvalue.JSValue")
	assertContains(t, out, "var _foo")
	assertContains(t, out, "var _bar")
}

func TestParamGoKeywordMap(t *testing.T) {
	ts := `function f(map: string): void {}`
	out := compile(t, ts)
	assertContains(t, out, "map_ *jsvalue.JSValue")
}

func TestParamGoKeywordRange(t *testing.T) {
	ts := `function f(range: number): void {}`
	out := compile(t, ts)
	assertContains(t, out, "range_ *jsvalue.JSValue")
}

func TestClassComputedMethodSkipped(t *testing.T) {
	ts := `const sym = Symbol('test');
class Foo {
	name: string;
	[sym]() { return 1; }
	greet(): string { return this.name; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewClass(")
	assertContains(t, out, `Foo.Get("prototype").Set("greet"`)
	assertNotContains(t, out, "[sym]")
}

func TestClassComputedPropertyWeakMapInit(t *testing.T) {
	// TS private fields compiled as WeakMaps in computed property names
	ts := `var _Foo_bar, _Foo_baz;
class Foo {
	constructor() {
		_Foo_bar.set(this, void 0);
		_Foo_baz.set(this, void 0);
	}
	[(_Foo_bar = new WeakMap(), _Foo_baz = new WeakMap(), Symbol("key"))]() { return 1; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewMap()")
	assertContains(t, out, "_Foo_bar = jsvalue")
	assertContains(t, out, "_Foo_baz = jsvalue")
}

func TestExportStringAliasDoubleQuote(t *testing.T) {
	ts := `const X = 1;
export {X as "default"}`
	out := compile(t, ts)
	assertNotContains(t, out, "default")
}

func TestObjectDestructuringTopLevel(t *testing.T) {
	ts := `const obj = { a: 1, b: 2 };
const { a, b } = obj;`
	out := compile(t, ts)
	assertContains(t, out, "obj.Get(\"a\")")
	assertContains(t, out, "obj.Get(\"b\")")
	assertNotContains(t, out, "_destructure_placeholder")
}

func TestArrayDestructuringTopLevel(t *testing.T) {
	ts := `const arr = [1, 2];
const [x, y] = arr;`
	out := compile(t, ts)
	// In all-JSValue mode, arrays use .Index() for destructuring
	assertContains(t, out, "var x = arr.Index(0)")
	assertContains(t, out, "var y = arr.Index(1)")
}

func TestObjectDestructuringInFunction(t *testing.T) {
	ts := `function test() {
	const obj = { a: 1 };
	const { a } = obj;
	console.log(a);
}`
	out := compile(t, ts)
	assertContains(t, out, "obj.Get(\"a\")")
	assertNotContains(t, out, "_destructure_placeholder")
}

func TestArrayDestructuringInFunction(t *testing.T) {
	ts := `function test() {
	const [first, second] = [3, 4];
}`
	out := compile(t, ts)
	assertContains(t, out, "var first =")
	assertContains(t, out, "var second =")
	assertNotContains(t, out, "_destructure_placeholder")
}

func TestArrayDestructuringRest(t *testing.T) {
	ts := `const arr = [1, 2, 3];
const [first, ...rest] = arr;`
	out := compile(t, ts)
	assertContains(t, out, "var first = arr.Index(0)")
	assertContains(t, out, "jsvalue.Slice(arr,")
}

func TestParamObjectDestructuring(t *testing.T) {
	ts := `function extract({ name, age }) { return name; }`
	out := compile(t, ts)
	assertContains(t, out, "_param0 *jsvalue.JSValue")
	assertContains(t, out, "_param0.Get(\"name\")")
	assertContains(t, out, "_param0.Get(\"age\")")
	assertNotContains(t, out, "{ name")
}

func TestDestructuredParamSubscript(t *testing.T) {
	ts := `function f({ key }) { return key[0]; }`
	out := compile(t, ts)
	assertNotContains(t, out, "cannot index _param0")
	assertNotContains(t, out, "[int(")
}

func TestArrayDestructuredParamUsesIndex(t *testing.T) {
	ts := `function f(arr) {
	arr.forEach(([key, value]) => {
		return key;
	});
}`
	out := compile(t, ts)
	assertContains(t, out, "_param0.Index(0)")
	assertContains(t, out, "_param0.Index(1)")
	assertNotContains(t, out, "_param0[0]")
	assertNotContains(t, out, "_param0[1]")
}

func TestEmptyInput(t *testing.T) {
	out, err := Compile([]byte(""), "main", "", false)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(out), "package main")
}

func TestObjectDestructuringWithDefaults(t *testing.T) {
	ts := `function f(options) {
		const { ambiguousIsNarrow = true, countAnsiEscapeCodes = false } = options;
		return ambiguousIsNarrow;
	}`
	out := compile(t, ts)
	// Destructured fields with defaults are now JSValue
	assertContains(t, out, "ambiguousIsNarrow = jsvalue.NewBool(true)")
	assertContains(t, out, "countAnsiEscapeCodes = jsvalue.NewBool(false)")
}

func TestGoBuiltinParamSanitized(t *testing.T) {
	ts := `function f(string) { return string; }`
	out := compile(t, ts)
	assertContains(t, out, "string_")
	assertNotContains(t, out, "string *jsvalue")
}

func TestDestructuringParamWithDefault(t *testing.T) {
	ts := `export default function ansiRegex({onlyFirst = false} = {}) {
		return onlyFirst;
	}`
	out := compile(t, ts)
	// Parameter should be variadic so callers can omit it
	assertContains(t, out, "...*jsvalue.JSValue")
	// Should extract from variadic args
	assertContains(t, out, "if len(_args0) > 0")
	// Destructuring default should be emitted with JSValue wrapper
	assertContains(t, out, "onlyFirst := jsvalue.NewBool(false)")
	// Synthetic param should be referenced to avoid unused error
	assertContains(t, out, "_ = _param0")
}

func TestDestructuringParamWithoutDefault(t *testing.T) {
	ts := `function f({name, age}) { return name; }`
	out := compile(t, ts)
	// Inner function receives destructured param, wrapper passes _args
	assertContains(t, out, `_param0.Get("name")`)
}

func TestClassMethodLocalsUseGet(t *testing.T) {
	// Variables declared inside class methods should be registered as locals,
	// so property access uses .Get() instead of capitalized field access.
	ts := `class Parser {
	parse(input) {
		const opts = Object.create(null);
		return opts.verbose;
	}
}`
	out := compile(t, ts)
	assertContains(t, out, `opts.Get("verbose")`)
	assertNotContains(t, out, "opts.Verbose")
}

func TestObjectAssignResultUsesGet(t *testing.T) {
	ts := `function f(options) {
	const config = Object.assign({key: true}, options);
	return config.key;
}`
	out := compile(t, ts)
	assertContains(t, out, `config.Get("key")`)
	assertNotContains(t, out, `config["key"]`)
}

func TestRestPatternParam(t *testing.T) {
	ts := `export function foo(...args) { return args; }`
	out := compile(t, ts)
	assertContains(t, out, "args ...*jsvalue.JSValue")
	assertNotContains(t, out, "...args")
}

func TestObjectAssignSubscriptUsesGet(t *testing.T) {
	ts := `function f(options) {
	const config = Object.assign({key: true}, options);
	return config['key'];
}`
	out := compile(t, ts)
	assertContains(t, out, `config.Get("key")`)
	assertNotContains(t, out, `config["key"]`)
}

func TestNestedSubscriptOnObjectAssign(t *testing.T) {
	ts := `function f(assignment, key) {
	const flags = Object.assign({}, {});
	flags[assignment][key] = true;
	const v = flags[assignment][key];
}`
	out := compile(t, ts)
	assertContains(t, out, `.Get(jsvalue.PropertyKey(assignment))`)
	assertContains(t, out, `.Get(jsvalue.PropertyKey(key))`)
	assertNotContains(t, out, `[int(key)]`)
}

func TestNestedMemberSubscriptOnMapLocal(t *testing.T) {
	ts := `function f(key) {
	const flags = {arrays: {}, bools: {}};
	flags.arrays[key] = true;
	const v = flags.arrays[key];
}`
	out := compile(t, ts)
	assertContains(t, out, `.Set(fmt.Sprint(key)`)
	assertContains(t, out, `.Get(jsvalue.PropertyKey(key))`)
	assertNotContains(t, out, `[int(key)]`)
}

func TestNestedFunctionDeclaration(t *testing.T) {
	ts := `function outer(x) {
	function inner(y) { return y; }
	return inner(x);
}`
	out := compile(t, ts)
	// Forward declaration as JSValue, assignment wraps in NewFunction
	assertContains(t, out, "var inner *jsvalue.JSValue")
	assertContains(t, out, "inner = jsvalue.NewFunction(")
	assertContains(t, out, "inner.Call(")
	assertNotContains(t, out, "func inner(")
}

func TestJSValueSubscriptWithJSValueIndex(t *testing.T) {
	ts := `function f(obj, key) { return obj[key]; }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get(jsvalue.PropertyKey(key))`)
	assertNotContains(t, out, "obj.Index(key)")
}

func TestPkgLevelJSValueUsesGet(t *testing.T) {
	ts := `let mixin;
function f() { return mixin.format; }`
	out := compile(t, ts)
	assertContains(t, out, `mixin.Get("format")`)
	assertNotContains(t, out, "mixin.Format")
}

func TestCodePointAtTyped(t *testing.T) {
	ts := `function f(s: string) {
	const cp = s.codePointAt(0);
	if (cp <= 0x1F) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "var cp =")
	assertContains(t, out, "jsvalue.LtE(")
}

func TestJSValueComparedWithBoolLit(t *testing.T) {
	ts := `function f(obj, key) {
	const check = (k, m) => m;
	if (check(key, obj) !== false) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NEq(")
	assertContains(t, out, "jsvalue.NewBool(false)).Bool()")
	assertNotContains(t, out, ".String() != false")
}

func TestTypedLocalAssignedFromUntypedCall(t *testing.T) {
	ts := `function outer(args) {
	function inner(i, key) { return i; }
	for (let i = 0; i < args.length; i++) {
		i = inner(i, args[i]);
	}
}`
	out := compile(t, ts)
	// Hoisted inner function called via .Call() in all-JSValue mode
	assertContains(t, out, "inner.Call(")
	assertContains(t, out, "i.Number() + 1")
}

func TestObjectAssignInBoolContext(t *testing.T) {
	ts := `function f(options) {
	const config = Object.assign({}, options);
	if (config["verbose"]) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, `config.Get("verbose").Bool()`)
}

func TestSubscriptOnJSValueCallResult(t *testing.T) {
	ts := `function f(arg) { return arg.slice(-1)[0]; }`
	out := compile(t, ts)
	assertContains(t, out, ".Index(0)")
	assertNotContains(t, out, "].Slice(-1)[0]")
}

func TestJSValueSliceLocalAssignedFromSlice(t *testing.T) {
	ts := `function f(args) {
	const notFlags = [];
	notFlags = args.slice(1);
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewArray()")
	assertContains(t, out, `MethodCall("slice"`)
}

func TestSplitOnJSValueReturnsJSValue(t *testing.T) {
	ts := `function f(key) { return key.split("."); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("split"`)
}

func TestLenInBoolContext(t *testing.T) {
	ts := `function f() {
	const arr = [1, 2];
	if (arr.length) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "> 0")
}

func TestJSValueLenInBoolContext(t *testing.T) {
	ts := `function f(arr) {
	if (arr.length) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, ".Len() > 0")
	assertNotContains(t, out, ".Len() != nil")
}

func TestJSValueSliceMethodCallWrapped(t *testing.T) {
	// Collection methods on JSValue arrays use prototype methods via MethodCall.
	ts := `function f() {
	const items = [];
	items.forEach((x) => { return x; });
}`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("forEach"`)
}

func TestTypedLocalIndexOnJSValueUsesGet(t *testing.T) {
	ts := `function f(obj) {
	const key = "hello";
	return obj[key];
}`
	out := compile(t, ts)
	assertContains(t, out, `.Get(jsvalue.PropertyKey(key))`)
	assertNotContains(t, out, ".Index(key)")
}

func TestHoistedFuncPaddedArgs(t *testing.T) {
	ts := `function outer(x) {
	function inner(a, b, c) { return a; }
	inner(x, x);
}`
	out := compile(t, ts)
	// Hoisted functions use .Call() — variadic, no nil padding needed
	assertContains(t, out, "inner.Call(")
}

func TestHoistedFuncLiteralArgsWrapped(t *testing.T) {
	ts := `function outer(arg) {
	function helper(a, b) { return a; }
	helper("_", arg);
}`
	out := compile(t, ts)
	// String literal "_" should be wrapped with jsvalue.From()
	assertContains(t, out, `jsvalue.From("_")`)
}

func TestLogicalAndWithComparisonsIsTyped(t *testing.T) {
	// All-JSValue: && with JSValue operands produces JSValue via jsvalue.And
	ts := `function f(s: string) {
	var check = s !== s.toLowerCase() && s !== s.toUpperCase();
	if (!check) { return s; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.And(")
	assertContains(t, out, "jsvalue.Not(check).Bool()")
}

func TestRegexLiteralIsJSValue(t *testing.T) {
	// Regex literals produce jsvalue.NewRegex wrapping jsvalue.CompileRegex.
	ts := `function f(s: string) {
	const re = /^hello/;
	return re.test(s);
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewRegex(jsvalue.CompileRegex(")
	assertContains(t, out, "jsvalue.MatchString(re,")
}

func TestMathCallResultIsJSValue(t *testing.T) {
	// Math.min/max returns *jsvalue.JSValue, so comparisons use jsvalue.Gt.
	ts := `function f(a: number, b: number) {
	var m = Math.min(a, b);
	if (m > 0) { return m; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsmath.Min(")
	assertContains(t, out, "jsvalue.Gt(")
}

func TestSplitOnJSValueUsesLenAndIndex(t *testing.T) {
	// split() on a JSValue param returns *jsvalue.JSValue (via FromStrings),
	// so len() and [0] must use .Len() and .Index().
	ts := `function f(key) {
	const parts = key.split(".");
	if (parts.length > 1) { return parts[0]; }
}`
	out := compile(t, ts)
	assertContains(t, out, ".Len()")
	assertNotContains(t, out, "len(parts)")
	assertContains(t, out, ".Index(0)")
	assertNotContains(t, out, "parts[0]")
}

func TestOrderingComparisonCoercesFloat64(t *testing.T) {
	// When one side is JSValue and the other is int, both should become float64.
	ts := `function f(arr, toEat) {
	var available = 5;
	if (available < toEat) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Lt(")
	assertContains(t, out, "jsvalue.From(available)")
	assertContains(t, out, "jsvalue.From(toEat)")
	assertContains(t, out, ".Bool()")
}

func TestDestructuringWithBooleanDefaults(t *testing.T) {
	// Destructured fields with boolean default values should be JSValue
	// and the ! operator should work correctly.
	ts := `function f(options) {
	const { flag = false } = options;
	if (!flag) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewBool(false)")
	assertContains(t, out, "jsvalue.Not(flag).Bool()")
	assertNotContains(t, out, "!flag)")
}

func TestDestructuringWithMultipleBooleanDefaults(t *testing.T) {
	// Multiple destructured boolean fields should all be JSValue.
	ts := `function f(options) {
	const { enabled = true, disabled = false } = options;
	if (!enabled || !disabled) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewBool(true)")
	assertContains(t, out, "jsvalue.NewBool(false)")
	assertContains(t, out, "jsvalue.Not(enabled)")
	assertContains(t, out, "jsvalue.Not(disabled)")
	assertContains(t, out, "jsvalue.Or(")
}

func TestDestructuringShorthandPattern(t *testing.T) {
	// Shorthand destructuring pattern should track variables as JSValue.
	ts := `function f(obj) {
	const { name } = obj;
	if (!name) { return "default"; }
}`
	out := compile(t, ts)
	assertContains(t, out, "obj.Get(\"name\")")
	assertContains(t, out, "jsvalue.Not(name).Bool()")
}

func TestDestructuringPairPattern(t *testing.T) {
	// Pair pattern destructuring should track variables as JSValue.
	ts := `function f(obj) {
	const { key: value } = obj;
	if (!value) { return "default"; }
}`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get("key")`)
	assertContains(t, out, "jsvalue.Not(value).Bool()")
}

func TestBooleanLiteralWithNotOperator(t *testing.T) {
	// Boolean literals are now JSValue — !flag becomes jsvalue.Not(flag).Bool()
	ts := `function test() {
	const flag = false;
	if (!flag) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "var flag = jsvalue.NewBool(false)")
	assertContains(t, out, "jsvalue.Not(flag).Bool()")
}

func TestBooleanLiteralInExpression(t *testing.T) {
	// Boolean literals are now JSValue — !enabled becomes jsvalue.Not(enabled)
	ts := `function test() {
	const enabled = true;
	const options = { active: !enabled };
}`
	out := compile(t, ts)
	assertContains(t, out, "var enabled = jsvalue.NewBool(true)")
	assertContains(t, out, "jsvalue.Not(enabled)")
}

func TestArrayDestructuringWithBooleans(t *testing.T) {
	// Array destructured elements should be JSValue and ! operator should work.
	ts := `function f(arr) {
	const [first, second] = arr;
	if (!first || !second) { return "default"; }
}`
	out := compile(t, ts)
	// When arr is JSValue param, uses .Index() instead of []
	assertContains(t, out, "var first = arr.Index(0)")
	assertContains(t, out, "var second = arr.Index(1)")
	assertContains(t, out, "jsvalue.Not(first)")
	assertContains(t, out, "jsvalue.Not(second)")
	assertContains(t, out, "jsvalue.Or(")
}

func TestArrayDestructuringWithRestPattern(t *testing.T) {
	// Array rest pattern should also track as JSValue.
	ts := `function f(arr) {
	const [first, ...rest] = arr;
	if (!first) { return rest; }
}`
	out := compile(t, ts)
	// When arr is JSValue param, uses .Index() instead of []
	assertContains(t, out, "var first = arr.Index(0)")
	assertContains(t, out, "jsvalue.Not(first).Bool()")
}

// --- All-JSValue regression tests ---

func TestTypedParamsAreJSValue(t *testing.T) {
	ts := `function f(x: number, s: string): boolean { return true; }`
	out := compile(t, ts)
	assertContains(t, out, "x *jsvalue.JSValue")
	assertContains(t, out, "s *jsvalue.JSValue")
	assertContains(t, out, ") *jsvalue.JSValue")
	assertNotContains(t, out, "float64")
	assertNotContains(t, out, ") bool")
}

func TestClassAsJSValueNewClass(t *testing.T) {
	ts := `class Foo {
	constructor(x) { this.x = x; }
	getX() { return this.x; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewClass(")
	assertContains(t, out, `this.Set("x",`)
	assertContains(t, out, `Foo.Get("prototype").Set("getX"`)
}

func TestClassExtendsParent(t *testing.T) {
	ts := `class Animal {}
class Dog extends Animal {
	bark() { return "woof"; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewClass(")
	assertContains(t, out, "Animal)")
	assertContains(t, out, `Dog.Get("prototype").Set("bark"`)
}

func TestNewExpressionUsesCall(t *testing.T) {
	ts := `function f() { return new Foo(1, 2); }`
	out := compile(t, ts)
	assertContains(t, out, "Foo.Call(")
}

func TestDestructuringUsesGet(t *testing.T) {
	ts := `function f(obj) { const { name, age } = obj; return name; }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get("name")`)
	assertContains(t, out, `obj.Get("age")`)
}

func TestDestructuringPairUsesGet(t *testing.T) {
	ts := `function f(obj) { const { key: value } = obj; return value; }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get("key")`)
}

func TestForOfDestructuringPreRegistersNames(t *testing.T) {
	// Destructured names from for-of should be recognized as JSValue in the loop body.
	ts := `function f(items) {
	for (const {segment: character} of items) {
		character.codePointAt(0);
	}
}`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("codePointAt"`)
	assertContains(t, out, `_item.Get("segment")`)
}

func TestClassMethodReceivesThis(t *testing.T) {
	// Class methods should extract 'this' from _args[0].
	ts := `class Foo {
	bar() { return this.name; }
}`
	out := compile(t, ts)
	assertContains(t, out, "var this *jsvalue.JSValue")
	assertContains(t, out, "this = _args[0]")
	assertContains(t, out, `this.Get("name")`)
}

func TestClassMethodRestParams(t *testing.T) {
	// Class methods with rest params wrapped as JSValue array from _args.
	ts := `class Foo {
	add(...items) { return items; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewArray(_args[1:]...)")
}

func TestClassMethodThisCallUsesGetCall(t *testing.T) {
	// Method calls on 'this' should use .Get().Call() for dynamic dispatch.
	ts := `class Foo {
	bar() { return this.baz(); }
}`
	out := compile(t, ts)
	assertContains(t, out, `this.MethodCall("baz")`)
}

func TestClassVoidMethodHasReturnType(t *testing.T) {
	// Void methods wrapped in NewFunction must still have *jsvalue.JSValue return type.
	ts := `class Foo {
	reset() { this.count = 0; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue")
	assertContains(t, out, "return nil")
}

func TestDestructuringParamDefaultCallsOriginal(t *testing.T) {
	// Destructured params with defaults: wrapper calls original function
	// preserving variadic destructuring param handling.
	ts := `export default function f({onlyFirst = false} = {}) {
		return onlyFirst;
	}`
	out := compile(t, ts)
	// Wrapper calls inner function with spread args
	assertContains(t, out, "_args[0:]...")
	// Inner function has variadic destructuring param
	assertContains(t, out, "_args0 ...*jsvalue.JSValue")
	// Original destructuring logic preserved
	assertContains(t, out, "if len(_args0) > 0")
	assertContains(t, out, "onlyFirst := jsvalue.NewBool(false)")
}

func TestMultiParamWithDestructuringDefault(t *testing.T) {
	// Mixed regular and destructuring params: wrapper passes regular params
	// individually and spreads the variadic destructuring param.
	ts := `export function eastAsianWidth(codePoint, {ambiguousAsWide = false} = {}) {
		return codePoint;
	}`
	out := compile(t, ts)
	// Regular param passed via IIFE extraction
	assertContains(t, out, "if len(_args) > 0")
	assertContains(t, out, "return _args[0]")
	// Variadic destructuring param spread
	assertContains(t, out, "_args[1:]...")
}

func TestFuncDeclBecomesJSValueVar(t *testing.T) {
	// Top-level function declarations become var = jsvalue.NewFunction(...)
	ts := `function helper(x) { return x; }`
	out := compile(t, ts)
	assertContains(t, out, "var helper = jsvalue.NewFunction(")
	assertNotContains(t, out, "func helper(")
}

func TestFuncDeclMainStaysGoFunc(t *testing.T) {
	// main() and init() remain as Go func declarations
	ts := `function main() { console.log("hi"); }`
	out := compile(t, ts)
	assertContains(t, out, "func main()")
	assertNotContains(t, out, "var main =")
}

func TestExportedFuncDeclBecomesJSValueVar(t *testing.T) {
	// Exported functions become capitalized var = jsvalue.NewFunction(...)
	ts := `export function validate(x) { return x; }`
	out := compile(t, ts)
	assertContains(t, out, "var Validate = jsvalue.NewFunction(")
}

func TestPkgLevelFuncCalledViaCall(t *testing.T) {
	// Same-file package-level function calls use .Call()
	ts := `function helper(x) { return x; }
function main() { helper(42); }`
	out := compile(t, ts)
	assertContains(t, out, "helper.Call(")
}

func TestExportedNameReferencedCapitalized(t *testing.T) {
	// References to exported vars within same file use capitalized name
	ts := `export const colors = ["red"];
const all = colors;`
	out := compile(t, ts)
	assertContains(t, out, "Colors")
}

func TestForwardReferenceToVarUsesJSValueOps(t *testing.T) {
	// Variables declared after their use site should still be recognized
	// as JSValue for binary expression dispatch (prescan handles this).
	ts := `function main() {
	if (count > 0) { return count; }
}
let count;`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Gt(")
	assertNotContains(t, out, "count > 0")
}

func TestForwardReferenceFuncCallUsesCall(t *testing.T) {
	// Functions declared after their call site should use .Call()
	ts := `function main() { helper(); }
function helper() { return 1; }`
	out := compile(t, ts)
	assertContains(t, out, "helper.Call()")
}

func TestRegexTestReturnsJSValue(t *testing.T) {
	// regex.test() should return *jsvalue.JSValue (wrapped in NewBool)
	// so it can be used in jsvalue.And/Or expressions.
	ts := `function f(s) {
	const ok = s.length > 0 && /hello/.test(s);
	return ok;
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewBool(jsvalue.MatchString(")
	assertContains(t, out, "jsvalue.And(")
}

func TestNestedFuncDeclIsJSValue(t *testing.T) {
	// Nested function declarations are JSValue vars, called via .Call()
	ts := `function outer() {
	function inner(x) { return x; }
	return inner(1);
}`
	out := compile(t, ts)
	assertContains(t, out, "var inner *jsvalue.JSValue")
	assertContains(t, out, "inner = jsvalue.NewFunction(")
	assertContains(t, out, "inner.Call(")
}

func TestBareReturnInJSValueFunc(t *testing.T) {
	// Bare return statements in functions with JSValue return type
	// should become return jsvalue.NewNull().
	ts := `function f(x) {
	if (x) { return; }
	return x;
}`
	out := compile(t, ts)
	assertContains(t, out, "return jsvalue.NewNull()")
}

func TestLiteralVarInitWrappedAsJSValue(t *testing.T) {
	// Variables initialized from literals are wrapped in JSValue constructors.
	ts := `function f() {
	let count = 0;
	let name = "hello";
	let flag = false;
}`
	out := compile(t, ts)
	assertContains(t, out, "var count = jsvalue.NewNumber(float64(0))")
	assertContains(t, out, `var name = jsvalue.NewString("hello")`)
	assertContains(t, out, "var flag = jsvalue.NewBool(false)")
}

func TestNewArrayBuiltin(t *testing.T) {
	// new Array(n) produces jsvalue.NewArray()
	ts := `function f() { return new Array(5); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewArray()")
}

func TestAugmentedSubAssignOnJSValue(t *testing.T) {
	// x -= y on JSValue produces x = jsvalue.Sub(x, y)
	ts := `function f(x, y) { x -= y; return x; }`
	out := compile(t, ts)
	assertContains(t, out, "x = jsvalue.Sub(x,")
}

func TestProcessStdoutColumnsUsesGet(t *testing.T) {
	// process.stdout.columns should use .Get("columns") since stdout is JSValue
	ts := `function f() { return process.stdout.columns; }`
	out := compile(t, ts)
	assertContains(t, out, `.Get("columns")`)
}

func TestStringGlobalReturnsJSValue(t *testing.T) {
	// String(x) → jsvalue.NewString(fmt.Sprint(x))
	ts := `function f(x) { return String(x); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewString(fmt.Sprint(")
}

func TestMethodOnCallResultUsesGetCall(t *testing.T) {
	// Method call on a call result (JSValue) uses .Get().Call()
	ts := `function f(s) { return String(s).normalize(); }`
	out := compile(t, ts)
	assertContains(t, out, `.MethodCall("normalize")`)
}

func TestObjectLiteralInOrUsesJSValueOr(t *testing.T) {
	// expr || {default: val} should use jsvalue.Or, not Go ||
	ts := `function f(x) { return x || {fallback: true}; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Or(")
	assertNotContains(t, out, " || ")
}

func TestImportedTranspiledSymbolIsJSValue(t *testing.T) {
	// Named imports from transpiled modules are JSValue
	ts := `import { helper } from "./utils";
const x = helper.name;`
	out, _ := Compile([]byte(ts), "mypkg", "mymod", true)
	assertContains(t, string(out), `.Get("name")`)
}

func TestArgumentsInClassMethod(t *testing.T) {
	// arguments keyword in class methods maps to _args[1:] (skip this)
	ts := `class Foo {
	bar() { return arguments[0]; }
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewArray(_args[1:]...)")
	assertContains(t, out, ".Index(0)")
}

func TestArgumentsInRegularFunction(t *testing.T) {
	// arguments keyword in regular functions maps to _args
	ts := `function f() { return arguments[0]; }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewArray(_args...)")
}

func TestArrayPrototypeAccess(t *testing.T) {
	// Array as standalone value resolves to jsvalue.ArrayPrototype
	ts := `function f() { return Array.isArray([]); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.IsArrayValue(")
}

func TestInitCycleSplitsToInit(t *testing.T) {
	// Self-referencing var declarations are split into forward decl + init()
	ts := `const f = (x) => f(x - 1);`
	out := compile(t, ts)
	assertContains(t, out, "var f *jsvalue.JSValue")
	assertContains(t, out, "func init()")
	assertContains(t, out, "f = jsvalue.NewFunction(")
}

func TestSuperCallInConstructor(t *testing.T) {
	// super(args) calls parent constructor on this
	ts := `class Child extends Error {
	constructor(msg) {
		super(msg);
		this.name = "Child";
	}
}`
	out := compile(t, ts)
	assertContains(t, out, `jserror.Error.CallSuper(this,`)
	assertContains(t, out, `this.Set("name"`)
}

func TestNamingCollisionClassAndFunction(t *testing.T) {
	// class Foo and function foo both capitalize to Foo — second gets numeric suffix
	ts := `export class Foo { }
export function foo() { return 1; }`
	out := compile(t, ts)
	assertContains(t, out, "var Foo = jsvalue.NewClass(")
	assertContains(t, out, "var Foo2 = jsvalue.NewFunction(")
	assertNotContains(t, out, "Foo redeclared")
}

func TestNamingCollisionReferencesUseRemap(t *testing.T) {
	// References to the renamed function use the suffixed name
	ts := `export class Foo { }
export function foo() { return 1; }
const x = foo();`
	out := compile(t, ts)
	assertContains(t, out, "Foo2.Call(")
}

func TestNoCollisionNoSuffix(t *testing.T) {
	// No collision — names stay as-is
	ts := `export class Dog { }
export function bark() { return "woof"; }`
	out := compile(t, ts)
	assertContains(t, out, "var Dog = jsvalue.NewClass(")
	assertContains(t, out, "var Bark = jsvalue.NewFunction(")
}

func TestSymbolGlobal(t *testing.T) {
	// Symbol() produces jsvalue.NewSymbol
	ts := `const s = Symbol("test");`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.NewSymbol(fmt.Sprint("test"))`)
}

func TestThisAtPackageLevel(t *testing.T) {
	// this at package level is undefined (ES module semantics)
	ts := `const x = this;`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewUndefined()")
}

func TestBareReturnInClassMethod(t *testing.T) {
	// Bare return in class method gets wrapped to return nil
	ts := `class Foo {
	bar(x) {
		if (x) { return; }
		return x;
	}
}`
	out := compile(t, ts)
	assertContains(t, out, "return nil")
	assertNotContains(t, out, "\treturn\n")
}

func TestBareReturnInArrowFunction(t *testing.T) {
	// Bare return in arrow function gets wrapped to return nil
	ts := `const f = (x) => { if (x) { return; } return x; };`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewNull()")
}

func TestDestructuringAssignment(t *testing.T) {
	// [a, ...b] = expr — assignment not declaration
	ts := `function f(cmd) {
	let aliases = [];
	[cmd, ...aliases] = cmd;
	return aliases;
}`
	out := compile(t, ts)
	// Should use = (assign), not := (define) for destructuring assignment
	assertContains(t, out, "cmd = ")
	assertNotContains(t, out, "nil = cmd")
}

func TestUnusedLocalVarSuppressed(t *testing.T) {
	// Local var declarations get _ = name suppression
	ts := `function f() {
	let x = foo();
	let y = bar();
	return y;
}`
	out := compile(t, ts)
	assertContains(t, out, "_ = x")
}

func TestForOfLoopVarInScope(t *testing.T) {
	// for-of loop variable is registered in scope so it doesn't get capitalized
	ts := `export function command() {}
function f(items) {
	for (const command of items) {
		console.log(command);
	}
}`
	out := compile(t, ts)
	// Loop variable should stay lowercase in the body
	assertContains(t, out, "console.Log(command)")
	assertNotContains(t, out, "console.Log(Command)")
}

func TestNegationOnLength(t *testing.T) {
	// !arr.length → arr.Len() == 0
	ts := `function f(arr) {
	if (!arr.length) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "arr.Len() == 0")
}

func TestStringAsCallback(t *testing.T) {
	// String passed as callback: arr.map(String)
	ts := `function f(arr) { return arr.map(String); }`
	out := compile(t, ts)
	assertContains(t, out, `MethodCall("map"`)
	assertContains(t, out, "jsvalue.NewFunction(")
}

func TestUnusedClassMethodParam(t *testing.T) {
	// Unused class method params get _ = pName suppression
	ts := `class Foo {
	bar(a, b, c) { return a; }
}`
	out := compile(t, ts)
	assertContains(t, out, "_ = b")
	assertContains(t, out, "_ = c")
}

func TestMultiVarForInit(t *testing.T) {
	// for (let i = 0, ii = arr.length; ...) → extra vars declared before loop
	ts := `function f(arr) {
	for (let i = 0, size = arr.length; i < size; i++) {
		console.log(arr[i]);
	}
}`
	out := compile(t, ts)
	assertContains(t, out, "size :=")
	assertContains(t, out, "for i :=")
}

func TestSamePackageNamespaceResolvesDirect(t *testing.T) {
	// import * as X from "./file" + X.foo → Foo (capitalized direct reference)
	ts := `import * as templates from "./completion-templates.js";
const s = templates.completionShTemplate;`
	out, _ := Compile([]byte(ts), "mypkg", "mymod", true)
	assertContains(t, string(out), "CompletionShTemplate")
	assertNotContains(t, string(out), "templates.Get(")
}

func TestParenthesizedDestructuringAssignment(t *testing.T) {
	// ({ a: this.a, b: this.b } = obj) — parenthesized object destructuring
	ts := `class Foo {
	unfreeze(obj) {
		({ a: this.a, b: this.b } = obj);
	}
}`
	out := compile(t, ts)
	assertContains(t, out, `.Set("a"`)
	assertContains(t, out, `.Set("b"`)
	assertNotContains(t, out, "nil =")
}

func TestConstStringUsesJSValue(t *testing.T) {
	// Consts should use JSValue wrapping, not Go const
	ts := `function f() {
		const prefix = "hello";
		return prefix.length;
	}`
	out := compile(t, ts)
	assertContains(t, out, `jsvalue.NewString("hello")`)
	assertNotContains(t, out, "const prefix")
	// .length on JSValue should use .Len()
	assertContains(t, out, ".Len()")
}

func TestConstNumberUsesJSValue(t *testing.T) {
	ts := `const MAX = 100;`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.NewNumber(float64(100))")
	assertNotContains(t, out, "const MAX")
}

func TestBareVarDeclHoisted(t *testing.T) {
	// Bare variable declarations should be hoisted so closures can reference them
	ts := `function factory() {
		const fn = function() { return cached; };
		let cached;
		cached = "value";
		return fn();
	}`
	out := compile(t, ts)
	// The hoisted var should appear before the function that references it
	assertContains(t, out, "var cached *jsvalue.JSValue")
}

func TestClassMethodDestructuredParam(t *testing.T) {
	// Class method with destructured param extracts fields
	ts := `class Foo {
	bar({ name, age }) { return name; }
}`
	out := compile(t, ts)
	assertContains(t, out, `_param0.Get("name")`)
	assertContains(t, out, `_param0.Get("age")`)
}
