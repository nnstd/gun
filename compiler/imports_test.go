package compiler

import "testing"

func TestImportNamedFS(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/fs"`)
	assertContains(t, out, "fs.ReadFileSync")
}

func TestImportNamespacePath(t *testing.T) {
	ts := `import * as path from "path";
const p = path.join("a", "b");`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/path"`)
	assertContains(t, out, "nodepath.Join")
}

func TestImportDefaultModule(t *testing.T) {
	ts := `import fs from "fs";
const data = fs.readFileSync("test.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/fs"`)
}

func TestImportNamedMultiple(t *testing.T) {
	ts := `import { readFileSync, writeFileSync } from "fs";
const data = readFileSync("in.txt");
writeFileSync("out.txt", data);`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/fs"`)
	assertContains(t, out, "fs.ReadFileSync")
	assertContains(t, out, "fs.WriteFileSync")
}

func TestImportPathFunctions(t *testing.T) {
	ts := `import { join, basename, extname } from "path";
const p = join("a", "b");
const b = basename("/foo/bar.ts");
const e = extname("file.go");`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/path"`)
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
	assertContains(t, out, "func greet")
}

func TestImportOSModule(t *testing.T) {
	ts := `import { homedir, platform } from "os";
const h = homedir();
const p = platform();`
	out := compile(t, ts)
	assertContains(t, out, `"gun/runtime/os"`)
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
	assertContains(t, out, `"gun/runtime/os"`)
	assertContains(t, out, "nodeos.Homedir")
}

func TestConsoleErrorStillUsesStdlibOS(t *testing.T) {
	ts := `console.error("fail");`
	out := compile(t, ts)
	assertContains(t, out, `"os"`)
	assertContains(t, out, "os.Stderr")
	assertNotContains(t, out, "nodeos")
}
