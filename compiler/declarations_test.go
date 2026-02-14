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
	assertContains(t, out, "...nums")
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
	out, err := Compile([]byte(`console.log("hi");`), "lib", "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	assertContains(t, s, "package lib")
	assertContains(t, s, "func init()")
	assertNotContains(t, s, "func main()")
}

func TestEmptyInput(t *testing.T) {
	out, err := Compile([]byte(""), "main", "")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(out), "package main")
}
