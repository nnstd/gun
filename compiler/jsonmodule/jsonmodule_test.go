package jsonmodule

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nnstd/gun/compiler/context"
)

func TestCompileUsesInlineConstructorsForSmallJSON(t *testing.T) {
	out, err := Compile([]byte(`{"name":"stone","drops":[1,2,3]}`), "fixture", "Default", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "jsvalue.ObjectFrom(") || !strings.Contains(source, "jsvalue.NewArray(") {
		t.Fatalf("small JSON should compile to inline constructors:\n%s", source)
	}
	if strings.Contains(source, "jsonx.Unmarshal") || strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("small JSON should not compile to runtime parsing:\n%s", source)
	}
}

func TestCompileUsesRuntimeParseForLargeJSONBelowO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"stone","id":1}`)
	}
	b.WriteString(`]}`)

	out, err := Compile([]byte(b.String()), "fixture", "Default", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "jsonx.Unmarshal") || !strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("large JSON should compile to runtime parsing")
	}
	if strings.Contains(source, "type jsonRoot struct") {
		t.Fatalf("large JSON below O2 should not emit typed schema:\n%s", source)
	}
}

func TestCompileUsesTypedSchemaForLargeJSONAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"stone","id":1}`)
	}
	b.WriteString(`]}`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "type jsonRoot struct") || !strings.Contains(source, "jsonRootToJSValue") {
		t.Fatalf("large JSON at O2 should emit typed schema and converter:\n%s", source)
	}
	if strings.Contains(source, "module.DataToJSValue(data)") {
		t.Fatalf("large JSON at O2 should not route typed root through DataToJSValue:\n%s", source)
	}
}

func TestCompileFallsBackForAmbiguousLargeJSONAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if i%2 == 0 {
			b.WriteString(`{"name":"stone","id":1}`)
		} else {
			b.WriteString(`{"name":"stone","id":null,"extra":true}`)
		}
	}
	b.WriteString(`]}`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("ambiguous O2 JSON should fall back at ambiguous node:\n%s", source)
	}
	if strings.Contains(source, "module.DataToJSValue(data)") {
		t.Fatalf("ambiguous O2 JSON should not route typed root through DataToJSValue:\n%s", source)
	}
}

func TestCompileDeduplicatesTypedSchemaFieldNamesAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"1-id":1,"1_id":2}`)
	}
	b.WriteString(`]}`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "Field1_id") || !strings.Contains(source, "Field1_id_2") {
		t.Fatalf("typed schema should produce valid deterministic field names:\n%s", source)
	}
}

func TestCompileUsesTypedSchemaForLargeTopLevelArrayAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"stone","id":1}`)
	}
	b.WriteString(`]`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	assertGeneratedGoCompiles(t, source)
	if !strings.Contains(source, "[]jsonRootItem") || !strings.Contains(source, "jsonArrayToJSValue") {
		t.Fatalf("large top-level array at O2 should use typed array conversion:\n%s", source)
	}
	if strings.Contains(source, "module.DataToJSValue(data)") {
		t.Fatalf("large top-level array at O2 should not route typed root through DataToJSValue:\n%s", source)
	}
}

func TestCompileUsesTypedSchemaForLargeNestedArraysAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"matrix":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`[[1,2],[3,4]]`)
	}
	b.WriteString(`]}`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	assertGeneratedGoCompiles(t, source)
	if !strings.Contains(source, "[][][]float64") || !strings.Contains(source, "jsonArrayToJSValue") {
		t.Fatalf("large nested arrays at O2 should use typed nested array conversion:\n%s", source)
	}
}

func TestCompileAvoidsNestedTypeNameCollisionAtO2(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"a-b":{"left":1},"a_b":{"right":2},"items":[`)
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"stone","id":1}`)
	}
	b.WriteString(`]}`)

	out, err := CompileWithOptLevel([]byte(b.String()), "fixture", "Default", nil, context.O2)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	assertGeneratedGoParses(t, source)
	if !strings.Contains(source, "type jsonRootA_b struct") || !strings.Contains(source, "type jsonRootA_b_2 struct") {
		t.Fatalf("typed schema should disambiguate colliding nested type names:\n%s", source)
	}
	if !strings.Contains(source, "jsonRootA_bToJSValue") || !strings.Contains(source, "jsonRootA_b_2ToJSValue") {
		t.Fatalf("typed schema should generate converters for both colliding nested types:\n%s", source)
	}
}

func assertGeneratedGoParses(t *testing.T, source string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.SkipObjectResolution); err != nil {
		t.Fatalf("generated source should parse: %v\n%s", err, source)
	}
}

func assertGeneratedGoCompiles(t *testing.T, source string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "tmp-jsonmodule-compile-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "./"+filepath.Base(dir))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/gun-go-build")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated source should compile: %v\n%s", err, out)
	}
}
