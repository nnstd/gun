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

func TestConstantsBuiltinRegistered(t *testing.T) {
	if !IsBuiltin("constants") || !IsBuiltin("node:constants") {
		t.Fatal("expected constants and node:constants to be builtin")
	}
	mod, ok := lookupRegistry("constants")
	if !ok || mod == nil {
		t.Fatal("expected constants in module registry")
	}
	if got := mod.Get("RSA_PKCS1_PADDING").Number(); got != 1 {
		t.Fatalf("RSA_PKCS1_PADDING = %v", got)
	}
	if got := mod.Get("RSA_NO_PADDING").Number(); got != 3 {
		t.Fatalf("RSA_NO_PADDING = %v", got)
	}
	if mod.Get("RSA_SSLV23_PADDING").TypeString() != "undefined" {
		t.Fatal("RSA_SSLV23_PADDING should match Node as undefined")
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

func TestCreateRequireLoadsYAMLAsJSValue(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.js")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("some: ok\nnums: [1, null, true]\nnested:\n  value: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	require := CreateRequire(jsvalue.NewString(entry))
	data := require.Call(jsvalue.NewString("./config.yaml"))
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

func TestCreateRequireLoadsExtensionlessYAML(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.js")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("enabled: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	require := CreateRequire(jsvalue.NewString(entry))
	data := require.Call(jsvalue.NewString("./config"))
	if !data.Get("enabled").Bool() {
		t.Fatal("expected enabled=true")
	}
}

func TestModuleStateBeginInit(t *testing.T) {
	var ms ModuleState
	if !ms.BeginInit() {
		t.Fatal("first BeginInit should succeed")
	}
	if ms.BeginInit() {
		t.Fatal("second BeginInit should fail (state=running)")
	}
}

func TestModuleStateFinishInit(t *testing.T) {
	var ms ModuleState
	ms.BeginInit()
	ms.FinishInit()
	if ms.BeginInit() {
		t.Fatal("BeginInit after FinishInit should fail (state=done)")
	}
}

func TestModuleStateFailInit(t *testing.T) {
	var ms ModuleState
	ms.BeginInit()
	ms.FailInit()
	if !ms.BeginInit() {
		t.Fatal("BeginInit after FailInit should succeed (state reset to pending)")
	}
}

func TestModuleStateConcurrentBeginInit(t *testing.T) {
	var ms ModuleState
	winners := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			winners <- ms.BeginInit()
		}()
	}
	won := 0
	for i := 0; i < 10; i++ {
		if <-winners {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("expected exactly 1 BeginInit winner, got %d", won)
	}
}

func TestLookupRegistryNoMutexDeadlock(t *testing.T) {
	// Register a module whose loader calls lookupRegistry for another module
	// (simulating circular require). This should not deadlock.
	RegisterModuleLoader("test:inner", func() *jsvalue.JSValue {
		return jsvalue.NewString("inner-value")
	})
	RegisterModuleLoader("test:outer", func() *jsvalue.JSValue {
		inner, ok := lookupRegistry("test:inner")
		if !ok {
			return jsvalue.NewString("inner-missing")
		}
		return inner
	})
	done := make(chan struct{})
	go func() {
		v, ok := lookupRegistry("test:outer")
		if !ok || v == nil {
			t.Error("expected outer module to resolve")
		}
		if v.String() != "inner-value" {
			t.Errorf("outer module = %q, want inner-value", v.String())
		}
		close(done)
	}()
	<-done
}
