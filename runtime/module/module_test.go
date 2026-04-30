package module

import (
	"os"
	"path/filepath"
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestDgramIsBuiltin(t *testing.T) {
	if !IsBuiltin("dgram") {
		t.Fatal("expected dgram to be builtin")
	}
	if !IsBuiltin("node:dgram") {
		t.Fatal("expected node:dgram to normalize to builtin dgram")
	}
}

func TestDgramRegisteredInModuleRegistry(t *testing.T) {
	if mod, ok := lookupRegistry("dgram"); !ok || mod == nil {
		t.Fatal("expected dgram in module registry")
	}
}

func TestURLRegisteredInModuleRegistry(t *testing.T) {
	if !IsBuiltin("url") || !IsBuiltin("node:url") {
		t.Fatal("expected url and node:url to be builtin")
	}
	if mod, ok := lookupRegistry("url"); !ok || mod == nil || mod.Get("URL") == nil {
		t.Fatal("expected url in module registry")
	}
	if mod, ok := lookupRegistry("node:url"); !ok || mod == nil || mod.Get("URL") == nil {
		t.Fatal("expected node:url in module registry")
	}
}

func TestCreateRequireLoadsJSONAsJSValue(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.js")
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"some":"ok","nums":[1,null,true],"nested":{"value":2}}`), 0644); err != nil {
		t.Fatal(err)
	}

	require := CreateRequire(jsvalue.NewString(entry))
	data := require.Call(jsvalue.NewString("./data.json"))
	if got := data.Get("some").String(); got != "ok" {
		t.Fatalf("some = %q", got)
	}
	if data.Get("nums").Index(1).Type() != jsvalue.TypeNull {
		t.Fatalf("nums[1] should be null")
	}
	if got := data.Get("nested").Get("value").Number(); got != 2 {
		t.Fatalf("nested.value = %v", got)
	}
}

func TestCreateRequireLoadsExtensionlessJSON(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.js")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"enabled":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	require := CreateRequire(jsvalue.NewString(entry))
	data := require.Call(jsvalue.NewString("./config"))
	if !data.Get("enabled").Bool() {
		t.Fatal("expected enabled=true")
	}
}
