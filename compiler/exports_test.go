package compiler

import "testing"

func TestExportDefaultNamedFunction(t *testing.T) {
	ts := `export default function handler() { return 1; }`
	out := compile(t, ts)
	assertContains(t, out, "func Handler()")
}

func TestExportDefaultAnonFunction(t *testing.T) {
	ts := `export default function() { return 1; }`
	out := compile(t, ts)
	assertContains(t, out, "func Default()")
}

func TestExportDefaultClass(t *testing.T) {
	ts := `export default class Foo { }`
	out := compile(t, ts)
	assertContains(t, out, "type Foo struct")
}

func TestExportDefaultIdentifier(t *testing.T) {
	ts := `const app = 1;
export default app`
	out := compile(t, ts)
	assertContains(t, out, "var Default = app")
}

func TestExportNamedList(t *testing.T) {
	ts := `const foo = 1;
const bar = 2;
export { foo, bar }`
	out := compile(t, ts)
	assertContains(t, out, "var Foo = foo")
	assertContains(t, out, "var Bar = bar")
}

func TestExportNamedAlias(t *testing.T) {
	ts := `const foo = 1;
export { foo as bar }`
	out := compile(t, ts)
	assertContains(t, out, "var Bar = foo")
}

func TestExportReexportFromModule(t *testing.T) {
	ts := `export { readFileSync } from "fs"`
	out := compile(t, ts)
	assertContains(t, out, "var ReadFileSync = fs.ReadFileSync")
	assertContains(t, out, `"gun/runtime/fs"`)
}

func TestExportWildcard(t *testing.T) {
	// Should compile without error; wildcard re-exports are silently skipped
	ts := `export * from "fs"`
	_ = compile(t, ts)
}

func TestExportDefaultServerPattern(t *testing.T) {
	ts := `export default { port: 3000, fetch: app.fetch }`
	out := compile(t, ts)
	assertContains(t, out, "fmt.Println")
	assertContains(t, out, "http.ListenAndServe")
}

func TestExportDefaultNonServerObject(t *testing.T) {
	ts := `export default { name: "test" }`
	out := compile(t, ts)
	assertContains(t, out, "var Default")
	assertContains(t, out, "map[string]any")
}
