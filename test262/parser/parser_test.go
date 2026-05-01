package parser

import (
	"testing"
)

func TestParseFrontmatter_Basic(t *testing.T) {
	source := []byte(`/*---
description: Basic addition test
esid: sec-addition-operator
features: [Symbol]
flags: [noStrict]
includes: [sta.js]
---*/

var x = 1 + 2;
assert.sameValue(x, 3);
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Description != "Basic addition test" {
		t.Errorf("description = %q, want %q", info.Description, "Basic addition test")
	}
	if info.Esid != "sec-addition-operator" {
		t.Errorf("esid = %q, want %q", info.Esid, "sec-addition-operator")
	}
	if len(info.Features) != 1 || info.Features[0] != "Symbol" {
		t.Errorf("features = %v, want [Symbol]", info.Features)
	}
	if len(info.Flags) != 1 || info.Flags[0] != "noStrict" {
		t.Errorf("flags = %v, want [noStrict]", info.Flags)
	}
	if len(info.Includes) != 1 || info.Includes[0] != "sta.js" {
		t.Errorf("includes = %v, want [sta.js]", info.Includes)
	}
}

func TestParseFrontmatter_Negative(t *testing.T) {
	source := []byte(`/*---
description: Throws TypeError
negative:
  type: TypeError
  phase: runtime
---*/

throw new TypeError();
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.IsNegative() {
		t.Error("expected IsNegative() = true")
	}
	if info.Negative.Type != "TypeError" {
		t.Errorf("negative.type = %q, want %q", info.Negative.Type, "TypeError")
	}
	if info.Negative.Phase != "runtime" {
		t.Errorf("negative.phase = %q, want %q", info.Negative.Phase, "runtime")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	source := []byte(`var x = 1;
assert(x);
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Description != "" {
		t.Errorf("description = %q, want empty", info.Description)
	}
	if len(info.Features) != 0 {
		t.Errorf("features = %v, want empty", info.Features)
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	source := []byte(`/*---
---*/

assert(true);
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Description != "" {
		t.Errorf("description = %q, want empty", info.Description)
	}
}

func TestParseFrontmatter_Es6id(t *testing.T) {
	source := []byte(`/*---
es6id: 12.5.1
description: old-style test
---*/
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Es6id != "12.5.1" {
		t.Errorf("es6id = %q, want %q", info.Es6id, "12.5.1")
	}
}

func TestParseFrontmatter_MultipleFeatures(t *testing.T) {
	source := []byte(`/*---
description: Multiple features
features: [async-iteration, generators, Symbol.species]
---*/
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Features) != 3 {
		t.Fatalf("features count = %d, want 3", len(info.Features))
	}
	if !info.HasFeature("generators") {
		t.Error("expected HasFeature(generators) = true")
	}
	if info.HasFeature("nonexistent") {
		t.Error("expected HasFeature(nonexistent) = false")
	}
}

func TestParseFrontmatter_AsyncFlags(t *testing.T) {
	source := []byte(`/*---
description: Async test
flags: [async]
includes: [asyncHelpers.js]
---*/
`)

	info, err := ParseFrontmatter(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.HasFlag("async") {
		t.Error("expected HasFlag(async) = true")
	}
	if !info.HasInclude("asyncHelpers.js") {
		t.Error("expected HasInclude(asyncHelpers.js) = true")
	}
}

func TestStripFrontmatter(t *testing.T) {
	source := []byte(`/*---
description: test
---*/

var x = 1;
`)

	stripped := StripFrontmatter(source)
	want := "/\n\nvar x = 1;\n"
	if string(stripped) != want {
		t.Errorf("stripped = %q, want %q", string(stripped), want)
	}
}

func TestStripFrontmatter_NoFrontmatter(t *testing.T) {
	source := []byte(`var x = 1;`)
	stripped := StripFrontmatter(source)
	if string(stripped) != "var x = 1;" {
		t.Errorf("should return original source unchanged")
	}
}

func TestHasFlag(t *testing.T) {
	info := &TestInfo{Flags: []string{"onlyStrict", "module"}}
	if !info.HasFlag("onlyStrict") {
		t.Error("expected HasFlag(onlyStrict) = true")
	}
	if info.HasFlag("noStrict") {
		t.Error("expected HasFlag(noStrict) = false")
	}
}

func TestKey(t *testing.T) {
	info := &TestInfo{}
	got := info.Key("test/language/expressions/addition/S9.3_A1.js")
	want := "language/expressions/addition/S9.3_A1.js"
	if got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
}
