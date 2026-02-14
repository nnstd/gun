package compiler

import "testing"

func TestImportNamedFS(t *testing.T) {
	ts := `import { readFileSync } from "fs";
const data = readFileSync("hello.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"os"`)
	assertContains(t, out, "os.ReadFile")
}

func TestImportNamespacePath(t *testing.T) {
	ts := `import * as path from "path";
const p = path.join("a", "b");`
	out := compile(t, ts)
	assertContains(t, out, `"path/filepath"`)
	assertContains(t, out, "filepath.Join")
}

func TestImportDefaultModule(t *testing.T) {
	ts := `import fs from "fs";
const data = fs.readFileSync("test.txt");`
	out := compile(t, ts)
	assertContains(t, out, `"os"`)
}

func TestImportNamedMultiple(t *testing.T) {
	ts := `import { readFileSync, writeFileSync } from "fs";
const data = readFileSync("in.txt");
writeFileSync("out.txt", data);`
	out := compile(t, ts)
	assertContains(t, out, `"os"`)
	assertContains(t, out, "os.ReadFile")
	assertContains(t, out, "os.WriteFile")
}

func TestImportPathFunctions(t *testing.T) {
	ts := `import { join, basename, extname } from "path";
const p = join("a", "b");
const b = basename("/foo/bar.ts");
const e = extname("file.go");`
	out := compile(t, ts)
	assertContains(t, out, `"path/filepath"`)
	assertContains(t, out, "filepath.Join")
	assertContains(t, out, "filepath.Base")
	assertContains(t, out, "filepath.Ext")
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
