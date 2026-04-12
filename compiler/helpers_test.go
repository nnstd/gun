package compiler

import (
	"strings"
	"testing"
)

func compile(t *testing.T, ts string) string {
	t.Helper()
	out, err := Compile([]byte(ts), "main", "", false)
	if err != nil {
		t.Fatalf("Compile error: %v\nInput:\n%s", err, ts)
	}
	return string(out)
}

func compileWithModule(t *testing.T, ts, moduleName string) string {
	t.Helper()
	out, err := Compile([]byte(ts), "main", moduleName, false)
	if err != nil {
		t.Fatalf("Compile error: %v\nInput:\n%s", err, ts)
	}
	return string(out)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output missing %q\n\ngot:\n%s", want, got)
	}
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

func TestCompileWithExportsRejectsAsyncPhase0(t *testing.T) {
	_, err := CompileWithExports(
		[]byte(`export async function load() { return await fetch(); }`),
		"main",
		"",
		"entry.ts",
		false,
		nil,
	)
	assertErrorContains(t, err, "async function declarations are not implemented yet")
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

func TestCompileRejectsAsyncClassMethodPhase0(t *testing.T) {
	_, err := Compile([]byte(`class Loader { async load() { return 1; } }`), "main", "", false)
	assertErrorContains(t, err, "async class methods are not implemented yet")
}
