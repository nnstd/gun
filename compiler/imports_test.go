package compiler

import "testing"

func TestImportNamedFS(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/fs"`)
	assertContains(t, out, "fs.ReadFileSync")
}

func TestImportNamespacePath(t *testing.T) {
	ts := `import * as path from "path";
const p = path.join("a", "b");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/path"`)
	assertContains(t, out, "nodepath.Join")
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
	assertContains(t, out, "fs.ReadFileSync")
	assertContains(t, out, "fs.WriteFileSync")
}

func TestImportPathFunctions(t *testing.T) {
	ts := `import { join, basename, extname } from "path";
const p = join("a", "b");
const b = basename("/foo/bar.ts");
const e = extname("file.go");`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/path"`)
	assertContains(t, out, "nodepath.Join")
	assertContains(t, out, "nodepath.Basename")
	assertContains(t, out, "nodepath.Extname")
}

func TestImportChildProcess(t *testing.T) {
	ts := `import { execSync } from "child_process";
execSync("ls");`
	out := compile(t, ts)
	assertContains(t, out, `"os/exec"`)
	assertContains(t, out, "exec.Command")
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
	assertContains(t, out, "nodeos.Homedir")
	assertContains(t, out, "nodeos.Platform")
}

func TestImportFSExistsSync(t *testing.T) {
	ts := `import { existsSync } from "fs";
const ok = existsSync("/tmp/test");`
	out := compile(t, ts)
	assertContains(t, out, "fs.ExistsSync")
}

func TestImportNamespaceOS(t *testing.T) {
	ts := `import * as os from "os";
const h = os.homedir();`
	out := compile(t, ts)
	assertContains(t, out, `"github.com/nnstd/gun/runtime/os"`)
	assertContains(t, out, "nodeos.Homedir")
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
	// All-JSValue: namespace imports use .Get() for property access
	assertNotContains(t, s, `"mymod/completion-templates"`)
	assertContains(t, s, `templates.Get("completionShTemplate")`)
}

func TestShorthandPropertyResolvesImport(t *testing.T) {
	ts := `import { readFileSync } from "fs";
export default { readFileSync };`
	out := compile(t, ts)
	// Value should be resolved to fs.ReadFileSync, not bare readFileSync
	assertContains(t, out, "fs.ReadFileSync")
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
	assertContains(t, out, `fs.ReadFileSync(jsvalue.NewString("hello.txt"))`)
}

func TestConsoleErrorUsesRuntimePackage(t *testing.T) {
	ts := `console.error("fail");`
	out := compile(t, ts)
	assertContains(t, out, `runtime/console`)
	assertContains(t, out, `console.Error("fail")`)
	assertNotContains(t, out, "os.Stderr")
}


// --- All-JSValue regression tests ---

func TestFsReadFileSyncArgsWrapped(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `fs.ReadFileSync(jsvalue.NewString("hello.txt"))`)
}
