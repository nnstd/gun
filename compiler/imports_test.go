package compiler

import "testing"

func compilePipeline(t *testing.T, source string) string {
	t.Helper()
	out, err := CompileWithOptLevel([]byte(source), "main", "", false, 0)
	if err != nil {
		t.Fatalf("pipeline compile failed: %v", err)
	}
	return string(out)
}

func TestImportNamedFS(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync").Call(`)
}

func TestImportNamespacePath(t *testing.T) {
	ts := `import * as path from "path";
const p = path.join("a", "b");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/path"`)
	assertContains(t, out, `nodepath.AsJSValue.MethodCall("join"`)
}

func TestImportDefaultModule(t *testing.T) {
	ts := `import fs from "fs";
const data = fs.readFileSync("test.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
}

func TestImportNamedMultiple(t *testing.T) {
	ts := `import { readFileSync, writeFileSync } from "fs";
const data = readFileSync("in.txt");
writeFileSync("out.txt", data);`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync").Call(`)
	assertContains(t, out, `fs.AsJSValue.Get("writeFileSync").Call(`)
}

func TestImportPathFunctions(t *testing.T) {
	ts := `import { join, basename, extname } from "path";
const p = join("a", "b");
const b = basename("/foo/bar.ts");
const e = extname("file.go");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/path"`)
	assertContains(t, out, `nodepath.AsJSValue.Get("join").Call(`)
	assertContains(t, out, `nodepath.AsJSValue.Get("basename").Call(`)
	assertContains(t, out, `nodepath.AsJSValue.Get("extname").Call(`)
}

func TestImportChildProcess(t *testing.T) {
	ts := `import { execSync } from "child_process";
execSync("ls");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/child_process"`)
	assertContains(t, out, `child_process.AsJSValue.Get("execSync").Call(`)
}

func TestImportCryptoModule(t *testing.T) {
	ts := `import crypto from "crypto";
const h = crypto.createHash("sha256");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/crypto"`)
	assertContains(t, out, `crypto.AsJSValue.MethodCall("createHash"`)
}

func TestImportUtilModule(t *testing.T) {
	ts := `import { format } from "util";
const s = format("hello %s", "world");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/util"`)
	assertContains(t, out, `util.AsJSValue.Get("format").Call(`)
}

func TestImportEventsModule(t *testing.T) {
	ts := `import { EventEmitter } from "events";
const e = new EventEmitter();`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/events"`)
	assertContains(t, out, `events.AsJSValue.Get("EventEmitter")`)
}

func TestImportRelative(t *testing.T) {
	ts := `import { helper } from "./utils";
helper();`
	out := compileWithModule(t, ts, "myapp")
	assertContains(t, out, `"myapp/utils"`)
}

func TestImportTypeOnly(t *testing.T) {
	ts := `import type { User } from "./models";
function greet(u: User): string { return "hi"; }`
	out := compileWithModule(t, ts, "myapp")
	assertContains(t, out, "var greet = jsvalue.NewFunction(")
}

func TestImportOSModule(t *testing.T) {
	ts := `import { homedir, platform } from "os";
const h = homedir();
const p = platform();`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/os"`)
	assertContains(t, out, `nodeos.AsJSValue.Get("homedir").Call(`)
	assertContains(t, out, `nodeos.AsJSValue.Get("platform").Call(`)
}

func TestImportFSExistsSync(t *testing.T) {
	ts := `import { existsSync } from "fs";
const ok = existsSync("/tmp/test");`
	out := compile(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("existsSync").Call(`)
}

func TestImportFSReadFileAlias(t *testing.T) {
	ts := `import { readFile } from "fs";
const data = readFile("x.txt");`
	out := compile(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("readFile").Call(`)
	assertNotContains(t, out, `fs.ReadFileSync`)
}

func TestImportFsPromisesNamed(t *testing.T) {
	ts := `import { stat } from "node:fs/promises";
const s = stat("x");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
	assertContains(t, out, `fs.PromisesAsJSValue.Get("stat").Call(`)
}

func TestImportFsPromisesDefault(t *testing.T) {
	ts := `import fs from "fs/promises";
const s = fs.readFile("x");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
	assertContains(t, out, `fs.PromisesAsJSValue.MethodCall("readFile"`)
}

func TestImportNamespaceOS(t *testing.T) {
	ts := `import * as os from "os";
const h = os.homedir();`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/os"`)
	assertContains(t, out, `nodeos.AsJSValue.MethodCall("homedir"`)
}

func TestPipelineImportNamedFS(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compilePipeline(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync").Call(`)
}

func TestPipelineImportNamespacePath(t *testing.T) {
	ts := `import * as path from "path";
const p = path.join("a", "b");`
	out := compilePipeline(t, ts)
	assertContains(t, out, `nodepath.AsJSValue.MethodCall("join"`)
}

func TestImportAssertStrictNamespace(t *testing.T) {
	ts := `import { strict } from "assert";
strict.strictEqual(1, 1);`
	out := compile(t, ts)
	assertContains(t, out, `assert.AsJSValue.Get("strict")`)
	assertContains(t, out, `MethodCall("strictEqual", jsvalue.From(1), jsvalue.From(1))`)
}

func TestImportBufferModule(t *testing.T) {
	ts := `import { Buffer } from "buffer";
const buf = Buffer.from("hi");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/buffer"`)
	assertContains(t, out, `buffer.AsJSValue.Get("Buffer")`)
	assertContains(t, out, `MethodCall("from", jsvalue.From("hi"))`)
}

func TestImportStreamModule(t *testing.T) {
	ts := `import * as stream from "stream";
const out = stream.pipeline(a, b);`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/stream"`)
	assertContains(t, out, `stream.AsJSValue.MethodCall("pipeline"`)
}

func TestSamePackageImportsNoImportGenerated(t *testing.T) {
	ts := `import { helper } from "./utils";
import foo from "./foo";
helper();
foo();`
	out, err := Compile([]byte(ts), "mypkg", "mymod", true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Same-package: no import path should be generated for relative imports
	assertNotContains(t, s, `"mymod/utils"`)
	assertNotContains(t, s, `"mymod/foo"`)
	// Same-package transpiled functions use .Call() (all JSValue)
	assertContains(t, s, "Helper.Call()")
	assertContains(t, s, "Default.Call()")
}

func TestSamePackageNamespaceImport(t *testing.T) {
	ts := `import * as templates from "./completion-templates.js";
const s = templates.completionShTemplate;`
	out, err := Compile([]byte(ts), "mypkg", "mymod", true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Same-package namespace: templates.foo → Foo (capitalized direct reference)
	assertNotContains(t, s, `"mymod/completion-templates"`)
	assertContains(t, s, "CompletionShTemplate")
}

func TestShorthandPropertyResolvesImport(t *testing.T) {
	ts := `import { readFileSync } from "fs";
export default { readFileSync };`
	out := compile(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync")`)
}

func TestTranspiledImportArgsWrappedWithJSValue(t *testing.T) {
	ts := `import { eastAsianWidth } from "get-east-asian-width";
const w = eastAsianWidth(42, {wide: true});`
	out := compileWithModule(t, ts, "myapp")
	assertContains(t, out, "jsvalue.From(42)")
	assertContains(t, out, "jsvalue.ObjectFrom(")
}

func TestKnownModuleArgsWrapped(t *testing.T) {
	// All-JSValue: runtime package args are wrapped with jsvalue constructors
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync").Call(jsvalue.From("hello.txt"))`)
}

func TestConsoleErrorUsesRuntimePackage(t *testing.T) {
	ts := `console.error("fail");`
	out := compile(t, ts)
	assertContains(t, out, `runtime/builtin/console`)
	assertContains(t, out, `console.Error(jsvalue.NewString("fail"))`)
	assertNotContains(t, out, "os.Stderr")
}


// --- All-JSValue regression tests ---

func TestFsReadFileSyncArgsWrapped(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `fs.AsJSValue.Get("readFileSync").Call(jsvalue.From("hello.txt"))`)
}
