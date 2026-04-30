package jsonmodule

import (
	"strings"
	"testing"
)

func TestCompileUsesInlineConstructorsForSmallJSON(t *testing.T) {
	out, err := Compile([]byte(`{"name":"stone","drops":[1,2,3]}`), "fixture", "Default", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	if !strings.Contains(source, "jsvalue.ObjectFrom(") || !strings.Contains(source, "jsvalue.NewArray(") {
		t.Fatalf("small JSON should compile to inline constructors:\n%s", source)
	}
	if strings.Contains(source, "json.Unmarshal") || strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("small JSON should not compile to runtime parsing:\n%s", source)
	}
}

func TestCompileUsesRuntimeParseForLargeJSON(t *testing.T) {
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
	if !strings.Contains(source, "json.Unmarshal") || !strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("large JSON should compile to runtime parsing")
	}
}
