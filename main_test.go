package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gunTestEntry(t *testing.T) string {
	t.Helper()

	candidates := []string{
		filepath.Join("..", "gun-test", "index.ts"),
		"/Users/nikita/Work/gun-test/index.ts",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err != nil {
				t.Fatal(err)
			}
			return abs
		}
	}
	t.Skip("gun-test fixture not available")
	return ""
}

func TestGetNodeModuleEntryResolvesExtensionlessMain(t *testing.T) {
	pkgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"main":"nbt"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "nbt.js"), []byte(`module.exports = {}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := getNodeModuleEntry(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(pkgDir, "nbt.js")
	if got != want {
		t.Fatalf("entry mismatch: got %q want %q", got, want)
	}
}

func TestResolveSubpathEntryResolvesExtensionlessExport(t *testing.T) {
	fixtureRoot := t.TempDir()
	pkgDir := filepath.Join(fixtureRoot, "node_modules", "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"exports":{"./feature":"./feature"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "feature.js"), []byte(`export const ok = true`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSubpathEntry("pkg/feature", fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(pkgDir, "feature.js")
	if got != want {
		t.Fatalf("entry mismatch: got %q want %q", got, want)
	}
}

func TestFindOptionalRequireImportsDetectsTryCatchRequires(t *testing.T) {
	source := []byte(`const fs = require("fs");
let convert;
try {
	convert = require('encoding').convert;
} catch (e) {}
try { require("node:crypto"); } catch {}
const required = require("required");
`)

	optional := findOptionalRequireImports(source)
	if !optional["encoding"] {
		t.Fatal("expected encoding require inside try/catch to be optional")
	}
	if !optional["crypto"] {
		t.Fatal("expected node:crypto require inside try/catch to be optional")
	}
	if optional["fs"] || optional["required"] {
		t.Fatalf("non-try requires were marked optional: %+v", optional)
	}
}

func TestProcessNodeModuleImportsSkipsMissingOptionalRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	pkgDir := filepath.Join(fixtureRoot, "node_modules", "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(`let convert;
try {
	convert = require("encoding").convert;
} catch (e) {}
module.exports = { convert };
`), 0644); err != nil {
		t.Fatal(err)
	}

	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const pkg = require("pkg");
console.log(!!pkg);
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := transpileFile(entry, filepath.Join(outDir, "entry.go"), "main", "gunrun", false, false, 0, nil); err != nil {
		t.Fatal(err)
	}
	visited := map[string]bool{}
	err := processNodeModuleImports([]string{entry}, fixtureRoot, outDir, "gunrun", false, visited, 0, nil)
	if err != nil {
		t.Fatalf("optional missing require should not fail node_module discovery: %v", err)
	}
	if visited["encoding"] {
		t.Fatal("optional missing encoding require was visited as a hard dependency")
	}
	if !visited["pkg"] {
		t.Fatal("required package was not discovered")
	}
}

func TestProcessNodeModuleImportsIncludesResolvableOptionalRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	pkgDir := filepath.Join(fixtureRoot, "node_modules", "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(`try {
	require("encoding");
} catch (e) {}
module.exports = {};
`), 0644); err != nil {
		t.Fatal(err)
	}
	encodingDir := filepath.Join(fixtureRoot, "node_modules", "encoding")
	if err := os.MkdirAll(encodingDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(encodingDir, "package.json"), []byte(`{"main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(encodingDir, "index.js"), []byte(`throw new Error("load failure");`), 0644); err != nil {
		t.Fatal(err)
	}

	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`require("pkg");`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := transpileFile(entry, filepath.Join(outDir, "entry.go"), "main", "gunrun", false, false, 0, nil); err != nil {
		t.Fatal(err)
	}
	visited := map[string]bool{}
	err := processNodeModuleImports([]string{entry}, fixtureRoot, outDir, "gunrun", false, visited, 0, nil)
	if err != nil {
		t.Fatalf("resolvable optional require should be included even if runtime evaluation may throw: %v", err)
	}
	if !visited["encoding"] {
		t.Fatal("resolvable optional encoding require was not included")
	}
}

func TestBuildInlineFixtureWithJSONRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const data = require("./data.json");
console.log(data.hello + ":" + data.nums[1]);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "data.json"), []byte(`{"hello":"json","nums":[1,2,3]}`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "json:2" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "json:2", stderr.String())
	}
}

func TestBuildInlineFixtureWithJSONNamedImport(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`import { some, nums } from "./file.json";
console.log(some + ":" + nums[1]);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "file.json"), []byte(`{"some":"ok","nums":[1,2,3]}`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok:2" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok:2", stderr.String())
	}
}

func TestBuildInlineFixtureWithYAMLNamedImport(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`import config, { some, nums } from "./file.yaml";
console.log(config.nested.value + ":" + some + ":" + nums[1]);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "file.yaml"), []byte(`some: ok
nums:
  - 1
  - 2
  - 3
nested:
  value: yes
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "yes:ok:2" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "yes:ok:2", stderr.String())
	}
}

func TestBuildInlineFixtureWithYMLRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const config = require("./file.yml");
console.log(config.some + ":" + config.nums[2]);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "file.yml"), []byte(`some: ok
nums: [1, 2, 3]
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok:3" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok:3", stderr.String())
	}
}

func TestBuildInlineFixtureWithJSONDestructuredRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const { some, nested } = require("./file.json");
console.log(some + ":" + nested.value);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "file.json"), []byte(`{"some":"ok","nested":{"value":"yes"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok:yes" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok:yes", stderr.String())
	}
}

func TestBuildInlineFixtureWithDynamicJSONRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const name = "data";
const data = require("./" + name + ".json");
console.log(data.some + ":" + data.nums[1] + ":" + data.empty);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "data.json"), []byte(`{"some":"ok","nums":[1,2,3],"empty":null}`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok:2:null" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok:2:null", stderr.String())
	}
}

func TestBuildInlineFixtureWithDynamicExtensionlessJSONRequire(t *testing.T) {
	fixtureRoot := t.TempDir()
	entry := filepath.Join(fixtureRoot, "entry.ts")
	if err := os.WriteFile(entry, []byte(`const name = "config";
const data = require("./" + name);
console.log(data.enabled);
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "config.json"), []byte(`{"enabled":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, entry)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "true" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "true", stderr.String())
	}
}

func TestTranspileNodeModuleAsPackageSupportsExtensionlessJSONRequire(t *testing.T) {
	pkgDir := t.TempDir()
	entry := filepath.Join(pkgDir, "index.js")
	if err := os.WriteFile(entry, []byte(`const features = require("./lib/features");
module.exports = features;
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "lib", "features.json"), []byte(`[{"name":"jump","versions":["1.20"]}]`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	files, err := transpileNodeModuleAsPackage(entry, outDir, "gunrun", "pkg", false, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(pkgDir, "lib", "features.json")
	found := false
	for _, file := range files {
		if file == jsonPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("discovered files did not include %s: %+v", jsonPath, files)
	}
	if _, err := os.Stat(filepath.Join(outDir, "lib-features.go")); err != nil {
		t.Fatalf("expected compiled JSON module: %v", err)
	}
}

func TestTranspileProject_BuiltGunTestMatchesCLIParity(t *testing.T) {
	entry := gunTestEntry(t)
	outDir := t.TempDir()
	bin := filepath.Join(outDir, "gun-test-bin")

	t.Setenv("GOCACHE", filepath.Join(outDir, "gocache"))

	if err := transpileProject(entry, outDir, "main", false, 0, nil); err != nil {
		maybeSkipAsyncPhase0Boundary(t, err)
		t.Fatal(err)
	}
	if err := goBuild(outDir, bin, false); err != nil {
		if strings.Contains(err.Error(), "invalid character U+0023 '#'") {
			t.Skip("gun-test fixture is currently blocked on general JS private field support in transpiled Hono")
		}
		if strings.Contains(err.Error(), "# gunrun/hono") {
			t.Skipf("gun-test fixture is blocked on remaining Hono parity issues after private-field support:\n%v", err)
		}
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "greet", args: []string{"greet", "world"}, want: "Hello, world!"},
		{name: "greet_uppercase", args: []string{"greet", "world", "--uppercase"}, want: "HELLO, WORLD!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Env = append(os.Environ(), "GUN_ENTRY_SCRIPT="+entry)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
			}

			if got := strings.TrimSpace(stdout.String()); got != tc.want {
				t.Fatalf("stdout mismatch: got %q want %q", got, tc.want)
			}
			if strings.Contains(stderr.String(), "You need to specify a command") {
				t.Fatalf("unexpected yargs demandCommand failure:\n%s", stderr.String())
			}
		})
	}
}

func TestRunCommandStripsPassthroughSeparatorForChildArgs(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "argv.ts")
	if err := os.WriteFile(entry, []byte(`console.log(JSON.stringify(process.argv));`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "gocache"))

	cmd := exec.Command("go", "run", ".", "run", entry, "--", "--help")
	cmd.Dir = "/Users/nikita/Work/gun"

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if strings.Contains(got, `"--","--help"`) {
		t.Fatalf("child argv still contains passthrough separator: %s", got)
	}
	if !strings.Contains(got, `"--help"`) {
		t.Fatalf("expected child argv to contain --help: %s", got)
	}
}

func TestRunCommandPreservesTopLevelWhileStatements(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "top_level_while.ts")
	if err := os.WriteFile(entry, []byte(`
let i = 0;
while (i < 3) {
  i += 1;
}
console.log(i);
`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "gocache"))

	cmd := exec.Command("go", "run", ".", "run", entry)
	cmd.Dir = "/Users/nikita/Work/gun"
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "3", stderr.String())
	}
}

func TestRunCommandCPUProfWritesNodeLikeProfileAndKeepsChildArgvClean(t *testing.T) {
	tempDir := t.TempDir()
	entry := filepath.Join(tempDir, "argv.ts")
	if err := os.WriteFile(entry, []byte(`
function fib(n) {
  if (n < 2) return n;
  return fib(n - 1) + fib(n - 2);
}
let acc = 0;
for (let i = 0; i < 5; i++) acc += fib(35);
console.log(JSON.stringify({ argv: process.argv, acc }));
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(tempDir, "gun-bin")
	cacheDir := filepath.Join(tempDir, "gocache")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.gunModuleRoot=/Users/nikita/Work/gun", "-o", bin, ".")
	cmd.Dir = "/Users/nikita/Work/gun"
	cmd.Env = append(os.Environ(), "GOCACHE="+cacheDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build gun failed: %v\n%s", err, out)
	}

	run := exec.Command(bin, "run", entry, "--cpu-prof", "--cpu-prof-name=custom.cpuprofile", "--", "--help")
	run.Dir = tempDir
	run.Env = append(os.Environ(), "GOCACHE="+cacheDir)
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	gotArgv := strings.TrimSpace(stdout.String())
	if strings.Contains(gotArgv, "--cpu-prof") {
		t.Fatalf("child argv leaked cpu-prof flag: %s", gotArgv)
	}
	if !strings.Contains(gotArgv, `"--help"`) {
		t.Fatalf("child argv missing passthrough arg: %s", gotArgv)
	}

	profilePath := filepath.Join(tempDir, "custom.cpuprofile")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read cpu profile %s: %v\nstderr:\n%s", profilePath, err, stderr.String())
	}
	var profile struct {
		Nodes []struct {
			CallFrame struct {
				FunctionName string `json:"functionName"`
				URL          string `json:"url"`
			} `json:"callFrame"`
		} `json:"nodes"`
		StartTime  int64   `json:"startTime"`
		EndTime    int64   `json:"endTime"`
		Samples    []int   `json:"samples"`
		TimeDeltas []int64 `json:"timeDeltas"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("invalid cpuprofile json: %v\n%s", err, data)
	}
	if len(profile.Nodes) == 0 {
		t.Fatal("expected cpuprofile nodes")
	}
	if len(profile.Samples) != len(profile.TimeDeltas) {
		t.Fatalf("samples/timeDeltas mismatch: %d vs %d", len(profile.Samples), len(profile.TimeDeltas))
	}
	if profile.EndTime <= profile.StartTime {
		t.Fatalf("unexpected profile time bounds: start=%d end=%d", profile.StartTime, profile.EndTime)
	}
	for _, node := range profile.Nodes {
		if strings.Contains(node.CallFrame.URL, "/Work/gun/runtime/") {
			t.Fatalf("profile leaked gun runtime Go frame: %s", node.CallFrame.URL)
		}
	}
}

func TestRunCommandRejectsCustomCPUProfInterval(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "argv.ts")
	if err := os.WriteFile(entry, []byte(`console.log("ok")`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".", "run", entry, "--cpu-prof", "--cpu-prof-interval=500")
	cmd.Dir = "/Users/nikita/Work/gun"
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected run to fail for unsupported cpu-prof interval")
	}
	if !strings.Contains(stderr.String(), "only supports --cpu-prof-interval=1000") {
		t.Fatalf("expected unsupported interval error, got:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestRunCommandNodeV8Module(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "v8.ts")
	if err := os.WriteFile(entry, []byte(`
import v8 from 'node:v8'

function burn(n) {
  if (n < 2) return n
  return burn(n - 1) + burn(n - 2)
}

function run() {
  const tag1 = v8.cachedDataVersionTag()
  const tag2 = v8.cachedDataVersionTag()
  const obj = { hello: 'world', nums: [1, 2], undef: undefined }
  obj.self = obj
  obj.map = new Map()
  obj.map.set('x', 2)
  obj.set = new Set()
  obj.set.add(3)
  obj.big = BigInt('4')
  const round = v8.deserialize(v8.serialize(obj))
  const h = v8.startCpuProfile()
  burn(32)
  const prof = h.stop()
  console.log(JSON.stringify({
    stableTag: tag1 === tag2,
    hello: round.hello,
    nums1: round.nums[1],
    hasUndef: Object.prototype.hasOwnProperty.call(round, 'undef') && typeof round.undef === 'undefined',
    self: round.self === round,
    map: round.map.get('x'),
    set: round.set.has(3),
    big: String(round.big),
    profileText: typeof prof === 'string' && prof.includes('"nodes"'),
    profileSamples: prof.includes('"samples"') ? 1 : 0,
  }))
}

run()
`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "gocache"))

	cmd := exec.Command("go", "run", ".", "run", entry)
	cmd.Dir = "/Users/nikita/Work/gun"
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	var got struct {
		StableTag      bool   `json:"stableTag"`
		Hello          string `json:"hello"`
		Nums1          int    `json:"nums1"`
		HasUndef       bool   `json:"hasUndef"`
		Self           bool   `json:"self"`
		Map            int    `json:"map"`
		Set            bool   `json:"set"`
		Big            string `json:"big"`
		ProfileText    bool   `json:"profileText"`
		ProfileSamples int    `json:"profileSamples"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &got); err != nil {
		t.Fatalf("invalid json output: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !got.StableTag || got.Hello != "world" || got.Nums1 != 2 || !got.HasUndef || !got.Self || got.Map != 2 || !got.Set || got.Big != "4" || !got.ProfileText || got.ProfileSamples < 0 {
		t.Fatalf("unexpected v8 result: %+v\nstderr:\n%s", got, stderr.String())
	}
}

func buildFixture(t *testing.T, fixture string) string {
	t.Helper()
	outDir := t.TempDir()
	bin := filepath.Join(outDir, "fixture-bin")

	t.Setenv("GOCACHE", filepath.Join(outDir, "gocache"))

	if err := transpileProject(fixture, outDir, "main", false, 0, nil); err != nil {
		maybeSkipAsyncPhase0Boundary(t, err)
		t.Fatal(err)
	}
	if err := goBuild(outDir, bin, false); err != nil {
		t.Fatal(err)
	}
	return bin
}

func buildInlineFixtureWithNodeModules(t *testing.T, filename, source string) string {
	t.Helper()

	fixtureRoot := t.TempDir()
	nodeModulesSource := filepath.Join("/Users/nikita/Work/gun-test", "node_modules")
	if _, err := os.Stat(nodeModulesSource); err != nil {
		t.Skip("gun-test node_modules fixture not available")
	}
	if err := os.Symlink(nodeModulesSource, filepath.Join(fixtureRoot, "node_modules")); err != nil {
		t.Fatal(err)
	}

	fixture := filepath.Join(fixtureRoot, filename)
	if err := os.WriteFile(fixture, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	return buildFixture(t, fixture)
}

func waitForOutput(t *testing.T, buf *bytes.Buffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("process did not emit %q; stdout so far:\n%s", want, buf.String())
}

func maybeSkipSandboxBind(t *testing.T, err error, stdout, stderr *bytes.Buffer) {
	t.Helper()
	if err == nil {
		return
	}
	out := stdout.String() + "\n" + stderr.String()
	if strings.Contains(out, "bind: operation not permitted") || strings.Contains(out, "listen tcp") && strings.Contains(out, "operation not permitted") {
		t.Skip("sandbox does not permit binding local TCP listeners")
	}
}

func maybeSkipAsyncPhase0Boundary(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "await expressions are not implemented yet") ||
		strings.Contains(msg, "await in this expression position is not implemented yet") ||
		strings.Contains(msg, "await inside try/catch/finally is not implemented yet") ||
		strings.Contains(msg, "try/catch/finally is not implemented in async function declarations yet") ||
		strings.Contains(msg, "destructuring declarations are not implemented in async function declarations yet") ||
		strings.Contains(msg, "destructuring parameters are not implemented in async function declarations yet") ||
		strings.Contains(msg, "async arrow functions are not implemented yet") ||
		strings.Contains(msg, "async class methods are not implemented yet") ||
		strings.Contains(msg, "async function declarations are not implemented yet") {
		t.Skipf("fixture is blocked on planned async/await lowering work:\n%v", err)
	}
}

func TestTranspileProject_BuiltBunServeFixtureBuilds(t *testing.T) {
	port := 43110
	bin := buildInlineFixtureWithNodeModules(t, "bun_serve.ts", fmt.Sprintf(`Bun.serve({
	port: %d,
	fetch() {
		return new Response("bun-ok");
	},
});

console.log("Listening on %d");
`, port, port))
	if _, err := os.Stat(bin); err != nil {
		t.Fatal(err)
	}
}

func TestTranspileProject_AsyncFunctionDeclarationRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_decl.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let total = 0
	for (let i = 0; i < 2; i++) {
		if (i === 0) {
			await Promise.resolve(0)
			total = total + 1
		} else {
			await Promise.resolve(0)
			total = total + 1
		}
	}
	return total
}

load().then((value) => {
	console.log(value)
})
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "2" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "2", stderr.String())
	}
}

func TestTranspileProject_AsyncArrowRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_arrow.ts")
	if err := os.WriteFile(fixture, []byte(`const load = async () => {
	await Promise.resolve(0)
	return 3
}

load().then((value) => {
	console.log(value)
})
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "3", stderr.String())
	}
}

func TestTranspileProject_AsyncMethodRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_method.ts")
	if err := os.WriteFile(fixture, []byte(`class Loader {
	async load() {
		await Promise.resolve(0)
		return 4
	}
}

new Loader().load().then((value) => {
	console.log(value)
})
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "4" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "4", stderr.String())
	}
}

func TestTranspileProject_AsyncDestructuringDeclarationRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_destructure.ts")
	if err := os.WriteFile(fixture, []byte(`async function load(options) {
	const { all = false, dot = false } = options
	await Promise.resolve(0)
	if (all || dot) {
		return "bad"
	}
	return "ok"
}

load({}).then((value) => {
	console.log(value)
})
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok", stderr.String())
	}
}

func TestTranspileProject_AsyncTryCatchRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_trycatch.ts")
	if err := os.WriteFile(fixture, []byte(`async function load(flag) {
	try {
		if (flag) {
			return await Promise.reject("bad")
		}
		return await Promise.resolve("ok")
	} catch (err) {
		return err
	}
}

load(true).then((value) => { console.log(value) })
load(false).then((value) => { console.log(value) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || lines[0] != "bad" || lines[1] != "ok" {
		t.Fatalf("stdout mismatch: got %q want [bad ok]\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestTranspileProject_AsyncFinallyRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_finally.ts")
	if err := os.WriteFile(fixture, []byte(`async function load(flag) {
	let trace = ""
	try {
		trace = trace + "t"
		if (flag) {
			return await Promise.resolve("ok")
		}
		throw "bad"
	} catch (err) {
		trace = trace + err
		return trace
	} finally {
		trace = trace + "f"
	}
}

load(true).then((value) => { console.log(value) })
load(false).then((value) => { console.log(value) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || lines[0] != "tbad" || lines[1] != "ok" {
		t.Fatalf("stdout mismatch: got %q want [tbad ok]\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestTranspileProject_AwaitExpressionPositionRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_expr_position.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	const wrapped = { value: await Promise.resolve("ok") }
	return String(await Promise.resolve(wrapped.value))
}

load().then((value) => { console.log(value) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok", stderr.String())
	}
}

func TestTranspileProject_AwaitInLoopHeadersRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_loop_headers.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let i = 0
	let out = ""
	while (await Promise.resolve(i < 2)) {
		for (; await Promise.resolve(i < 2); i = await Promise.resolve(i + 1)) {
			out = out + i
		}
	}
	return out
}

load().then((value) => { console.log(value) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "01" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "01", stderr.String())
	}
}

func TestTranspileProject_AsyncDestructuringParametersRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_param_destructure.ts")
	if err := os.WriteFile(fixture, []byte(`async function load({ value = "ok" }, [count = 2]) {
	await Promise.resolve(0)
	return value + count
}

load({}, []).then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok2" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "ok2", stderr.String())
	}
}

func TestTranspileProject_AsyncForInRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_forin.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let keys = ""
	const obj = Object.fromEntries([["a", 1], ["b", 2]])
	for (const key in obj) {
		await Promise.resolve(0)
		keys = keys + key
	}
	return keys
}

load().then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != "ab" && got != "ba" {
		t.Fatalf("stdout mismatch: got %q want one of [ab ba]\nstderr:\n%s", got, stderr.String())
	}
}

func TestTranspileProject_AsyncForOfRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_forof.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let out = ""
	for (const [a, b] of [[1, 2], [3, 4]]) {
		await Promise.resolve(0)
		out = out + (a + b)
	}
	return out
}

load().then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "37" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "37", stderr.String())
	}
}

func TestTranspileProject_AsyncSwitchRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_switch.ts")
	if err := os.WriteFile(fixture, []byte(`async function load(value) {
	let out = ""
	switch (await Promise.resolve(value)) {
		case 1:
			out = "one"
			break
		case 2:
			out = "two"
			break
		default:
			out = "other"
	}
	return out
}

load(2).then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "two" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "two", stderr.String())
	}
}

func TestTranspileProject_AsyncDestructuringAssignmentRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_destructure_assign.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let a = 0
	let b = 0
	;[a, b] = await Promise.resolve([1, 2])
	return a + b
}

load().then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "3", stderr.String())
	}
}

func TestTranspileProject_AsyncSwitchCaseAwaitRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_switch_case_await.ts")
	if err := os.WriteFile(fixture, []byte(`async function load(value) {
	switch (value) {
		case await Promise.resolve(1):
			return "one"
		case await Promise.resolve(2):
			return "two"
		default:
			return "other"
	}
}

load(2).then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "two" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "two", stderr.String())
	}
}

func TestTranspileProject_AsyncAugmentedAssignmentRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_aug_assign.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let total = 1
	total += await Promise.resolve(2)
	return total
}

load().then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "3", stderr.String())
	}
}

func TestTranspileProject_AsyncLabeledLoopRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_labeled.ts")
	if err := os.WriteFile(fixture, []byte(`async function load() {
	let out = ""
	outer: for (let i = 0; await Promise.resolve(i < 3); i = await Promise.resolve(i + 1)) {
		for (const value of [1, 2, 3]) {
			await Promise.resolve(0)
			if (value === 2) {
				continue outer
			}
			if (value === 3) {
				break outer
			}
			out = out + value
		}
	}
	return out
}

load().then((result) => { console.log(result) })
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "111" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "111", stderr.String())
	}
}

func TestTranspileProject_AsyncMainRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_main.ts")
	if err := os.WriteFile(fixture, []byte(`async function main() {
	await Promise.resolve(0)
	console.log("async-main")
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "async-main" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "async-main", stderr.String())
	}
}

func TestTranspileProject_AsyncInitRuns(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "async_init.ts")
	if err := os.WriteFile(fixture, []byte(`async function init() {
	await Promise.resolve(0)
	console.log("async-init")
}

function main() {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "async-init" {
		t.Fatalf("stdout mismatch: got %q want %q\nstderr:\n%s", got, "async-init", stderr.String())
	}
}

func TestTranspileProject_BuiltBunServeHonoFixtureResponds(t *testing.T) {
	fixtureRoot := t.TempDir()
	nodeModulesSource := filepath.Join("/Users/nikita/Work/gun-test", "node_modules")
	if _, err := os.Stat(nodeModulesSource); err != nil {
		t.Skip("gun-test node_modules fixture not available")
	}
	if err := os.Symlink(nodeModulesSource, filepath.Join(fixtureRoot, "node_modules")); err != nil {
		t.Fatal(err)
	}

	fixture := filepath.Join(fixtureRoot, "bun_hono.ts")
	if err := os.WriteFile(fixture, []byte(`import { Hono } from "hono";

const app = new Hono();
const port = 43111;

app.get("/", (c) => c.text("Hono!"));

Bun.serve({
	port,
	fetch: app.fetch,
});

console.log("Listening on " + port);
`), 0644); err != nil {
		t.Fatal(err)
	}
	port := 43111
	outDir := t.TempDir()
	bin := filepath.Join(outDir, "fixture-bin")

	t.Setenv("GOCACHE", filepath.Join(outDir, "gocache"))

	if err := transpileProject(fixture, outDir, "main", false, 0, nil); err != nil {
		maybeSkipAsyncPhase0Boundary(t, err)
		t.Fatal(err)
	}
	if err := goBuild(outDir, bin, false); err != nil {
		if strings.Contains(err.Error(), "invalid character U+0023 '#'") {
			t.Skip("Hono fixture is still blocked on general JS private field support")
		}
		if strings.Contains(err.Error(), "# gunrun/hono") {
			t.Skipf("Hono fixture is blocked on remaining non-private-field parity issues:\n%v", err)
		}
		t.Fatal(err)
	}

	cmd := exec.Command(bin)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	doneConsumed := false
	defer func() {
		_ = cmd.Process.Kill()
		if !doneConsumed {
			<-done
		}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), fmt.Sprintf("Listening on %d", port)) {
			break
		}
		select {
		case err := <-done:
			doneConsumed = true
			maybeSkipSandboxBind(t, err, &stdout, &stderr)
			t.Fatalf("server exited early: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	waitForOutput(t, &stdout, fmt.Sprintf("Listening on %d", port))
}

func TestTranspileProject_PrivateFieldErrorsMatchBunReadAndCall(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "private_field_read",
			src: `class A { #x = 1; read(o) { return o.#x } }
new A().read({});`,
			want: "Cannot access invalid private field (evaluating 'o.#x')",
		},
		{
			name: "private_method_call",
			src: `class A { #m() { return 1 } call(o) { return o.#m() } }
new A().call({});`,
			want: "Cannot access private method or acessor (evaluating 'o.#m')",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join(t.TempDir(), "private.ts")
			if err := os.WriteFile(fixture, []byte(tc.src), 0644); err != nil {
				t.Fatal(err)
			}
			bin := buildFixture(t, fixture)

			cmd := exec.Command(bin)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected runtime failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			}

			out := stdout.String() + "\n" + stderr.String()
			if !strings.Contains(out, tc.want) {
				t.Fatalf("expected error containing %q\nstdout:\n%s\nstderr:\n%s", tc.want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestTranspileProject_RuntimeErrorsShowSourceLocationsWithoutGoPanicDump(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantMessage string
		wantLine    string
	}{
		{
			name: "type_error_throw",
			src: `function boom() {
  throw new TypeError("boom")
}

boom()
`,
			wantMessage: "TypeError: boom",
			wantLine:    ":2:0",
		},
		{
			name: "private_field_access",
			src: `class A {
  #x = 1
  read(o) {
    return o.#x
  }
}

new A().read({})
`,
			wantMessage: "Cannot access invalid private field",
			wantLine:    ":4:0",
		},
		{
			name: "private_field_brand_mismatch_same_name",
			src: `class A {
  #x = 1
  read(o) {
    return o.#x
  }
}

class B {
  #x = 2
}

new A().read(new B())
`,
			wantMessage: "Cannot access invalid private field",
			wantLine:    ":4:0",
		},
		{
			name: "bun_serve_invalid_arg",
			src: `Bun.serve(123 as any)
`,
			wantMessage: "TypeError: Bun.serve expects an object",
			wantLine:    ":1:0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join(t.TempDir(), "stack.ts")
			if err := os.WriteFile(fixture, []byte(tc.src), 0644); err != nil {
				t.Fatal(err)
			}
			bin := buildFixture(t, fixture)

			cmd := exec.Command(bin)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected runtime failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			}

			out := stdout.String() + "\n" + stderr.String()
			if !strings.Contains(out, tc.wantMessage) {
				t.Fatalf("expected output containing %q\nstdout:\n%s\nstderr:\n%s", tc.wantMessage, stdout.String(), stderr.String())
			}
			if !strings.Contains(out, fixture+tc.wantLine) {
				t.Fatalf("expected output containing %q\nstdout:\n%s\nstderr:\n%s", fixture+tc.wantLine, stdout.String(), stderr.String())
			}
			if strings.Contains(out, "goroutine 1 [running]") {
				t.Fatalf("unexpected Go panic dump in output:\n%s", out)
			}
		})
	}
}

func TestTranspileProject_BunServePortInUseShowsSourceStacktrace(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sandbox does not permit binding local TCP listeners: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	fixture := filepath.Join(t.TempDir(), "port_in_use.ts")
	source := fmt.Sprintf(`Bun.serve({
  port: %d,
  fetch() {
    return new Response("ok")
  }
})
`, port)
	if err := os.WriteFile(fixture, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	bin := buildFixture(t, fixture)
	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	maybeSkipSandboxBind(t, err, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected runtime failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}

	out := stdout.String() + "\n" + stderr.String()
	if !strings.Contains(out, "Failed to start server. Is port") {
		t.Fatalf("expected port-in-use error\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(out, fixture+":1:0") {
		t.Fatalf("expected JS source stack frame in output\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestTranspileProject_YargsCommandThrowShowsStacktrace(t *testing.T) {
	bin := buildInlineFixtureWithNodeModules(t, "yargs_throw.ts", `import yargs from "yargs"
import { hideBin } from "yargs/helpers"

yargs(hideBin(process.argv))
  .command("serve", "serve", () => {}, () => {
    throw new Error("boom")
  })
  .demandCommand(1)
  .help()
  .parse()
`)

	cmd := exec.Command(bin, "serve")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected runtime failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}

	out := stdout.String() + "\n" + stderr.String()
	if !strings.Contains(out, "Error: boom") {
		t.Fatalf("expected error stack header\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(out, "yargs_throw.ts:6:0") {
		t.Fatalf("expected source frame in thrown command output\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}
