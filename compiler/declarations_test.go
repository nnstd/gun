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
	assertContains(t, out, "func add(a float64, b float64) float64")
	assertContains(t, out, "return a + b")
}

func TestArrowFunction(t *testing.T) {
	out := compile(t, `const double = (x: number): number => x * 2;`)
	assertContains(t, out, "func(x float64) float64")
	assertContains(t, out, "return x * 2")
}

func TestExportCapitalizesName(t *testing.T) {
	out := compile(t, `export function greet(name: string): string { return name; }`)
	assertContains(t, out, "func Greet(name string) string")
}

func TestInterfaceWithMethods(t *testing.T) {
	out := compile(t, `interface Reader { read(buf: string): number; }`)
	assertContains(t, out, "type Reader interface")
	assertContains(t, out, "Read(buf string) float64")
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
	assertContains(t, out, "type Dog struct")
	assertContains(t, out, "func NewDog(name string) *Dog")
	assertContains(t, out, "func (d *Dog) Bark() string")
}

func TestClassExtends(t *testing.T) {
	ts := `class Animal { name: string; }
	class Dog extends Animal { bark(): string { return this.name; } }`
	out := compile(t, ts)
	assertContains(t, out, "type Dog struct")
	assertContains(t, out, "Animal") // embedded parent
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
	assertContains(t, out, "type_ string")
	assertNotContains(t, out, "type string")
}

func TestRestParameter(t *testing.T) {
	ts := `function sum(...nums: number[]): number { return 0; }`
	out := compile(t, ts)
	assertContains(t, out, "nums ...float64")
}

func TestOptionalParameter(t *testing.T) {
	ts := `function greet(name?: string): void {}`
	out := compile(t, ts)
	assertContains(t, out, "*string")
}

func TestNullableUnionType(t *testing.T) {
	ts := `function maybe(x: string | null): void {}`
	out := compile(t, ts)
	assertContains(t, out, "*string")
}

func TestBooleanType(t *testing.T) {
	ts := `function check(b: boolean): boolean { return b; }`
	out := compile(t, ts)
	assertContains(t, out, "func check(b bool) bool")
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
	assertContains(t, out, "map_ string")
}

func TestParamGoKeywordRange(t *testing.T) {
	ts := `function f(range: number): void {}`
	out := compile(t, ts)
	assertContains(t, out, "range_ float64")
}

func TestClassComputedMethodSkipped(t *testing.T) {
	ts := `const sym = Symbol('test');
class Foo {
	name: string;
	[sym]() { return 1; }
	greet(): string { return this.name; }
}`
	out := compile(t, ts)
	assertContains(t, out, "type Foo struct")
	assertContains(t, out, "func (f *Foo) Greet() string")
	assertNotContains(t, out, "[sym]")
	assertNotContains(t, out, "func (f *Foo) Sym")
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
	assertContains(t, out, "var a = obj.A")
	assertContains(t, out, "var b = obj.B")
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
	assertContains(t, out, "var a = obj.A")
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
	assertContains(t, out, "name := _param0.Name")
	assertContains(t, out, "age := _param0.Age")
	assertNotContains(t, out, "{ name")
}

func TestDestructuredParamSubscript(t *testing.T) {
	ts := `function f({ key }) { return key[0]; }`
	out := compile(t, ts)
	assertNotContains(t, out, "cannot index _param0")
	assertNotContains(t, out, "[int(")
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
	assertContains(t, out, "ambiguousIsNarrow = true")
	assertContains(t, out, "countAnsiEscapeCodes = false")
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
	// Destructuring default should still be emitted
	assertContains(t, out, "onlyFirst := false")
	// Synthetic param should be referenced to avoid unused error
	assertContains(t, out, "_ = _param0")
}

func TestDestructuringParamWithoutDefault(t *testing.T) {
	ts := `function f({name, age}) { return name; }`
	out := compile(t, ts)
	// No default value — should NOT be variadic
	assertNotContains(t, out, "...")
	assertContains(t, out, "_param0")
	assertContains(t, out, "name := _param0.Name")
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

func TestMapLocalFromObjectAssign(t *testing.T) {
	ts := `function f(options) {
	const config = Object.assign({key: true}, options);
	return config.key;
}`
	out := compile(t, ts)
	assertContains(t, out, `config["key"]`)
	assertNotContains(t, out, `config.Get("key")`)
}

func TestRestPatternParam(t *testing.T) {
	ts := `export function foo(...args) { return args; }`
	out := compile(t, ts)
	assertContains(t, out, "args ...*jsvalue.JSValue")
	assertNotContains(t, out, "...args")
}

func TestMapLocalSubscriptAccess(t *testing.T) {
	ts := `function f(options) {
	const config = Object.assign({key: true}, options);
	return config['key'];
}`
	out := compile(t, ts)
	assertContains(t, out, `config["key"]`)
	assertNotContains(t, out, `config.Get("key")`)
}

func TestNestedSubscriptOnMapLocal(t *testing.T) {
	ts := `function f(assignment, key) {
	const flags = Object.assign({}, {});
	flags[assignment][key] = true;
	const v = flags[assignment][key];
}`
	out := compile(t, ts)
	assertContains(t, out, `.Set(fmt.Sprint(key)`)
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

func TestPkgLevelJSValueUsesGet(t *testing.T) {
	ts := `let mixin;
function f() { return mixin.format; }`
	out := compile(t, ts)
	assertContains(t, out, `mixin.Get("format")`)
	assertNotContains(t, out, "mixin.Format")
}
