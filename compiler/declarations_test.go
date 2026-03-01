package compiler

import "testing"

func TestConstDeclaration(t *testing.T) {
	out := compile(t, `const x: number = 42;`)
	assertContains(t, out, "const x float64 = 42")
}

func TestLetDeclaration(t *testing.T) {
	out := compile(t, `let name: string = "hello";`)
	assertContains(t, out, `var name string = "hello"`)
}

func TestVarDeclarationInferred(t *testing.T) {
	out := compile(t, `var flag = true;`)
	assertContains(t, out, "var flag = true")
}

func TestConstStringDeclaration(t *testing.T) {
	out := compile(t, `const msg = "hi";`)
	assertContains(t, out, `const msg = "hi"`)
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
	assertContains(t, out, "var x = arr[0]")
	assertContains(t, out, "var y = arr[1]")
	assertNotContains(t, out, "_destructure_placeholder")
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
	assertContains(t, out, "var first = arr[0]")
	assertContains(t, out, "var rest = arr[1:]")
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
	assertContains(t, out, `.Get(fmt.Sprint(assignment))`)
	assertContains(t, out, `.Get(fmt.Sprint(key))`)
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
	assertContains(t, out, `.Get(fmt.Sprint(key))`)
	assertNotContains(t, out, `[int(key)]`)
}

func TestNestedFunctionDeclaration(t *testing.T) {
	ts := `function outer(x) {
	function inner(y) { return y; }
	return inner(x);
}`
	out := compile(t, ts)
	// Forward declaration at top, assignment at original position
	assertContains(t, out, "var inner func(")
	assertContains(t, out, "inner = func(")
	assertNotContains(t, out, "func inner(")
}

func TestJSValueSubscriptWithJSValueIndex(t *testing.T) {
	ts := `function f(obj, key) { return obj[key]; }`
	out := compile(t, ts)
	assertContains(t, out, `obj.Get(fmt.Sprint(key))`)
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
	assertContains(t, out, ".Number())")
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
	assertContains(t, out, ".Array()")
	assertNotContains(t, out, "int(")
	assertNotContains(t, out, ".Number()")
}

func TestSplitOnJSValueReturnsJSValue(t *testing.T) {
	ts := `function f(key) { return key.split("."); }`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.Split(key,")
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
	// Collection methods on []*jsvalue.JSValue slices work directly
	// because runtime functions accept any (handles []*JSValue internally).
	ts := `function f() {
	const items = [];
	items.forEach((x) => { return x; });
}`
	out := compile(t, ts)
	assertContains(t, out, "jsvalue.ForEach(")
}

func TestTypedLocalIndexOnJSValueUsesGet(t *testing.T) {
	ts := `function f(obj) {
	const key = "hello";
	return obj[key];
}`
	out := compile(t, ts)
	assertContains(t, out, `.Get(fmt.Sprint(key))`)
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

func TestRegexLiteralIsTyped(t *testing.T) {
	// Variables initialized from regex literals should be *regexp.Regexp, not JSValue.
	ts := `function f(s: string) {
	const re = /^hello/;
	return re.test(s);
}`
	out := compile(t, ts)
	assertContains(t, out, "regexp.MustCompile")
	assertContains(t, out, "re.MatchString")
	assertNotContains(t, out, "fmt.Sprint(re)")
}

func TestMathCallResultIsTyped(t *testing.T) {
	// Variables initialized from Math.min/max should be typed (float64),
	// not treated as JSValue.
	ts := `function f(a: number, b: number) {
	var m = Math.min(a, b);
	if (m > 0) { return m; }
}`
	out := compile(t, ts)
	assertContains(t, out, "m > 0")
	assertNotContains(t, out, "m.Number()")
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
	// Boolean literals in regular declarations should be plain Go booleans.
	ts := `function test() {
	const flag = false;
	if (!flag) { return true; }
}`
	out := compile(t, ts)
	assertContains(t, out, "var flag = false")
	assertContains(t, out, "if !flag")
	// The return statement will wrap in JSValue (function returns *jsvalue.JSValue),
	// but the variable declaration and condition should use plain boolean
	assertNotContains(t, out, "var flag = jsvalue.NewBool")
}

func TestBooleanLiteralInExpression(t *testing.T) {
	// Boolean literals in regular declarations should be plain Go booleans.
	ts := `function test() {
	const enabled = true;
	const options = { active: !enabled };
}`
	out := compile(t, ts)
	assertContains(t, out, "var enabled = true")
	assertContains(t, out, "!enabled")
	assertNotContains(t, out, "jsvalue.NewBool(true)")
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
	assertContains(t, out, "fmt.Sprint(character)")
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
	// Class methods with rest params should extract them as a slice from _args.
	ts := `class Foo {
	add(...items) { return items; }
}`
	out := compile(t, ts)
	assertContains(t, out, "items := _args[1:]")
}

func TestClassMethodThisCallUsesGetCall(t *testing.T) {
	// Method calls on 'this' should use .Get().Call() for dynamic dispatch.
	ts := `class Foo {
	bar() { return this.baz(); }
}`
	out := compile(t, ts)
	assertContains(t, out, `this.Get("baz").Call(this)`)
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
	assertContains(t, out, "var all = Colors")
}
