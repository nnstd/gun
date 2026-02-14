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
