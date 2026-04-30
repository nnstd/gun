package yamlmodule

import (
	"strings"
	"testing"
)

func TestCompileUsesInlineConstructorsForSmallYAML(t *testing.T) {
	out, err := Compile([]byte("name: stone\ndrops:\n  - 1\n  - 2\n"), "fixture", "Default", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	if !strings.Contains(source, "jsvalue.ObjectFrom(") || !strings.Contains(source, "jsvalue.NewArray(") {
		t.Fatalf("small YAML should compile to inline constructors:\n%s", source)
	}
	if strings.Contains(source, "yaml.Unmarshal") || strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("small YAML should not compile to runtime parsing:\n%s", source)
	}
}

func TestCompileUsesRuntimeParseForLargeYAML(t *testing.T) {
	var b strings.Builder
	b.WriteString("items:\n")
	for i := 0; i < inlineValueExprMaxBytes; i++ {
		b.WriteString("  - name: stone\n    id: 1\n")
	}

	out, err := Compile([]byte(b.String()), "fixture", "Default", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := string(out)
	if !strings.Contains(source, "yaml.Unmarshal") || !strings.Contains(source, "module.DataToJSValue") {
		t.Fatalf("large YAML should compile to runtime parsing")
	}
}
