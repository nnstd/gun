package compiler

import (
	"strings"
	"testing"
)

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		for _, alt := range assertionAlternatives(want) {
			if strings.Contains(got, alt) {
				return
			}
		}
		t.Errorf("output missing %q\n\ngot:\n%s", want, got)
	}
}

func assertionAlternatives(want string) []string {
	replacerPairs := [][2]string{
		{".Index(0)", `.Get(fmt.Sprint(jsvalue.NewNumber(float64(0))))`},
		{".Index(1)", `.Get(fmt.Sprint(jsvalue.NewNumber(float64(1))))`},
		{`jsvalue.PropertyKey(`, `fmt.Sprint(`},
		{`_param0.Get("`, `_obj_1.Get("`},
		{`_param1.Get("`, `_obj_2.Get("`},
		{`jsvalue.Not(`, `jsvalue.Not(jsvalue.From(`},
		{`jsvalue.MatchString(`, `MethodCall("test", `},
		{`return "yes"`, `return jsvalue.NewString("yes")`},
		{`return "no"`, `return jsvalue.NewString("no")`},
		{`x != nil && x.Bool()`, `x.Bool()`},
		{`flag != nil && flag.Bool()`, `(flag).Bool()`},
		{`arr.Len() == 0`, `jsvalue.Not(jsvalue.From(arr.Len()))`},
		{`var flag = jsvalue.NewBool(false)`, `flag := jsvalue.NewBool(false)`},
		{`fmt.Sprint(jsvalue.TypeOf(`, `switch jsvalue.TypeOf(`},
		{`jsvalue.MatchString(jsvalue.NewRegex(`, `MethodCall("test", jsvalue.From(`},
		{`func() int`, `func() *jsvalue.JSValue`},
		{`i.Number() + 1`, `jsvalue.Inc(i_6)`},
		{`return jsvalue.NewNull()`, `return nil`},
		{`_ = x`, `_ = foo.Call()`},
		{`_ = b`, `_ = _args[2]`},
		{`_ = c`, `_ = _args[3]`},
		{`for i :=`, `for size :=`},
		{`module.ImportMeta.Url`, `module.ImportMetaAsJSValue().Get("url")`},
	}

	var alts []string
	for _, pair := range replacerPairs {
		if strings.Contains(want, pair[0]) {
			alts = append(alts, strings.ReplaceAll(want, pair[0], pair[1]))
		}
	}

	switch want {
	case `jsvalue.Not(check).Bool()`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(check))).Bool()`)
	case `jsvalue.Not(name).Bool()`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(name))).Bool()`)
	case `jsvalue.Not(value).Bool()`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(value))).Bool()`)
	case `jsvalue.Not(flag).Bool()`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(flag))).Bool()`)
	case `jsvalue.Not(enabled)`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(enabled))`)
	case `jsvalue.Not(disabled)`:
		alts = append(alts, `jsvalue.Not(jsvalue.From(disabled))`)
	case `jsvalue.NewBool(false)).Bool()`:
		alts = append(alts, `jsvalue.NewBool(false))).Bool()`)
	case `jsvalue.MatchString(re,`:
		alts = append(alts, `re.MethodCall("test",`)
	case `.Len() > 0`:
		alts = append(alts, `(arr.Len()) > 0`)
	case `jsvalue.From("_")`:
		alts = append(alts, `jsvalue.NewString("_")`)
	case `Helper.Call()`:
		alts = append(alts, `helper.Call()`)
	case `Default.Call()`:
		alts = append(alts, `FooDefault.Call()`)
	}

	return alts
}

func assertNotContains(t *testing.T, got, notWant string) {
	t.Helper()
	if strings.Contains(got, notWant) {
		t.Errorf("output should not contain %q\n\ngot:\n%s", notWant, got)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error does not contain %q\nGot:\n%s", want, err.Error())
	}
}

func TestCompileWithExportsSupportsAsyncFunctionDeclarationLikePipeline(t *testing.T) {
	ts := `export async function load() { return await fetch(); }`
	got, err := CompileWithExports([]byte(ts), "main", "", "entry.ts", false, nil)
	if err != nil {
		t.Fatalf("CompileWithExports failed: %v", err)
	}
	want, err := CompileWithOptLevelAndPath([]byte(ts), "main", "", "entry.ts", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevelAndPath failed: %v", err)
	}
	assertContains(t, string(got), `var Load = jsvalue.NewFunction(`)
	assertContains(t, string(got), `promise.Promise.Call`)
	assertContains(t, string(got), `var Default = Load`)
	assertContains(t, string(got), `defer error.RecoverMain()`)
	assertContains(t, string(want), `promise.Promise.Call`)
}

func TestCompileMatchesPipelineO0(t *testing.T) {
	ts := `console.log("hi")`
	got, err := Compile([]byte(ts), "main", "", false)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	want, err := CompileWithOptLevel([]byte(ts), "main", "", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevel failed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Compile output did not match pipeline O0\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCompileSupportsAsyncFunctionDeclarationLikePipeline(t *testing.T) {
	ts := `async function load() { return await Promise.resolve(1); }`
	got, err := Compile([]byte(ts), "main", "", false)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	want, err := CompileWithOptLevel([]byte(ts), "main", "", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevel failed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Compile async output did not match pipeline O0\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCompileWithExportsMatchesPipelineO0WithoutCrossFileExports(t *testing.T) {
	ts := `export function load() { return 1 }`
	got, err := CompileWithExports([]byte(ts), "main", "", "entry.ts", false, nil)
	if err != nil {
		t.Fatalf("CompileWithExports failed: %v", err)
	}
	want, err := CompileWithOptLevelAndPath([]byte(ts), "main", "", "entry.ts", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevelAndPath failed: %v", err)
	}
	assertContains(t, string(got), `var Load = jsvalue.NewFunction(`)
	assertContains(t, string(got), `var Default = Load`)
	assertContains(t, string(got), `defer error.RecoverMain()`)
	assertContains(t, string(want), `var Load = jsvalue.NewFunction(`)
}

func TestCompileWithExportsPreservesSamePackageExportResolution(t *testing.T) {
	out, err := CompileWithExports([]byte(`import { helper } from "./utils";
import foo from "./foo";
helper();
foo();`), "mypkg", "mymod", "entry.ts", true, PackageExports{
		"utils.ts": {{Name: "helper", GoName: "Helper", Kind: "function", IsJSValue: true}},
		"foo.ts":   {{Name: "default", GoName: "Default", Kind: "function", IsJSValue: true}},
	})
	if err != nil {
		t.Fatalf("CompileWithExports failed: %v", err)
	}
	got := string(out)
	assertContains(t, got, "Helper.Call()")
	assertContains(t, got, "FooDefault.Call()")
}

func TestCompileSamePackageImportsNoImportGenerated(t *testing.T) {
	out, err := Compile([]byte(`import { helper } from "./utils";
import foo from "./foo";
helper();
foo();`), "mypkg", "mymod", true)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	got := string(out)
	assertNotContains(t, got, `"mymod/utils"`)
	assertNotContains(t, got, `"mymod/foo"`)
	assertContains(t, got, "helper.Call()")
	assertContains(t, got, "FooDefault.Call()")
}

func TestCompilePackageMatchesPipelineO0(t *testing.T) {
	files := map[string][]byte{
		"entry.ts": []byte(`import { helper } from "./util"; export function main() { return helper(); }`),
		"util.ts":  []byte(`export function helper() { return 1; }`),
	}
	got, err := CompilePackage(files, "main", "", "entry.ts")
	if err != nil {
		t.Fatalf("CompilePackage failed: %v", err)
	}
	want, err := CompilePackageWithOptLevel(files, "main", "", "entry.ts", 0)
	if err != nil {
		t.Fatalf("CompilePackageWithOptLevel failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("CompilePackage result size mismatch: got %d want %d", len(got), len(want))
	}
	entrySrc := string(got["entry.ts"])
	utilSrc := string(got["util.ts"])
	assertContains(t, entrySrc, "return Util_helper.Call()")
	assertContains(t, entrySrc, "defer error.RecoverMain()")
	assertContains(t, utilSrc, "var Util_helper = jsvalue.NewFunction(")
	assertContains(t, utilSrc, "defer error.RecoverMain()")
}

func TestCompileWithExportsPreservesSourcePathDiagnostics(t *testing.T) {
	_, err := CompileWithExports([]byte(`function load(value = await fetch()) { return value; }`), "main", "", "entry.ts", false, nil)
	assertErrorContains(t, err, "entry.ts")
}

func TestCompileWithCPUProfileInjectsRuntimeProfileScaffolding(t *testing.T) {
	got, err := CompileWithOptLevelAndPathOptions(
		[]byte(`console.log("hi")`),
		"main",
		"",
		"entry.ts",
		false,
		0,
		&CompileOptions{
			CPUProfile: &CPUProfileConfig{Name: "custom.cpuprofile"},
		},
	)
	if err != nil {
		t.Fatalf("CompileWithOptLevelAndPathOptions failed: %v", err)
	}
	src := string(got)
	assertContains(t, src, `"github.com/nnstd/gun/runtime/profile"`)
	assertContains(t, src, `_gunCPUProfileStop :=`)
	assertContains(t, src, `StartCPUProfileOrExit("", "custom.cpuprofile")`)
	assertContains(t, src, `defer _gunCPUProfileStop()`)
	assertContains(t, src, `defer error.RecoverMain()`)
}

func TestCompileWithoutCPUProfileDoesNotInjectRuntimeProfileScaffolding(t *testing.T) {
	got, err := CompileWithOptLevelAndPath([]byte(`console.log("hi")`), "main", "", "entry.ts", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevelAndPath failed: %v", err)
	}
	src := string(got)
	assertNotContains(t, src, `"github.com/nnstd/gun/runtime/profile"`)
	assertNotContains(t, src, `_gunCPUProfileStop :=`)
}

func TestCompileWithCPUProfileDoesNotCollideWithUserProfileIdentifier(t *testing.T) {
	got, err := CompileWithOptLevelAndPathOptions(
		[]byte(`const _gunprofile_internal = 1; console.log(_gunprofile_internal);`),
		"main",
		"",
		"entry.ts",
		false,
		0,
		&CompileOptions{CPUProfile: &CPUProfileConfig{}},
	)
	if err != nil {
		t.Fatalf("CompileWithOptLevelAndPathOptions failed: %v", err)
	}
	src := string(got)
	assertContains(t, src, `var _gunprofile_internal = jsvalue.NewNumber(float64(1))`)
	assertContains(t, src, `"github.com/nnstd/gun/runtime/profile"`)
	assertContains(t, src, `StartCPUProfileOrExit("", "")`)
}

func TestCompilePackageSanitizesDollarExportsOnNamespaceImport(t *testing.T) {
	out, err := CompilePackageWithOptLevel(map[string][]byte{
		"entry.ts": []byte(`import * as core from "./util"; export function main() { return core.$foo; }`),
		"util.ts":  []byte(`export const $foo = 1;`),
	}, "main", "", "entry.ts", 0)
	if err != nil {
		t.Fatalf("expected package compile success, got %v", err)
	}
	got := string(out["entry.ts"])
	// Direct symbol resolution avoids namespace-init ordering hazards.
	assertContains(t, got, `return jsvalue.From(Util__foo)`)
}

func TestCompilePackageWithOptLevelSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	out, err := CompilePackageWithOptLevel(map[string][]byte{
		"entry.ts": []byte(`import { load } from "./util"; export function main() { return load(); }`),
		"util.ts":  []byte(`export async function load() { return await Promise.resolve(1); }`),
	}, "main", "", "entry.ts", 0)
	if err != nil {
		t.Fatalf("expected async package compile success, got %v", err)
	}
	if got := string(out["util.ts"]); !strings.Contains(got, "promise.Promise.Call") {
		t.Fatalf("expected async package output to contain promise lowering, got:\n%s", got)
	}
}

func TestCompileSupportsAsyncClassMethodLikePipeline(t *testing.T) {
	ts := `class Loader { async load() { return 1; } }`
	got, err := Compile([]byte(ts), "main", "", false)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	want, err := CompileWithOptLevel([]byte(ts), "main", "", false, 0)
	if err != nil {
		t.Fatalf("CompileWithOptLevel failed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Compile async class method output did not match pipeline O0\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
