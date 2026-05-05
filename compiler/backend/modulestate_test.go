package backend

import (
	"go/ast"
	"strings"
	"testing"
)

func TestInitFuncNameFromSource(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"", ""},
		{"/app/node_modules/foo/index.js", "initFooIndex"},
		{"/app/node_modules/foo-bar/src/baz.js", "initFoobarSrcBaz"},
		{"/app/node_modules/@scope/pkg/sub/file.js", "initScopepkgSubFile"},
		{"/app/node_modules/@scope/pkg/index.js", "initScopepkgIndex"},
		{"/app/node_modules/minecraft-data/index.js", "initMinecraftdataIndex"},
		{"/app/node_modules/ecdsa-sig-formatter/src/ecdsa-sig-formatter.js", "initEcdsasigformatterSrcEcdsasigformatter"},
		// Extension stripped before sanitization — no _js suffix
		{"node_modules/foo/bar.js", "initFooBar"},
		// Non-node_modules path uses parent dir + filename
		{"/project/src/utils.ts", "initSrcUtils"},
	}
	for _, tt := range tests {
		got := initFuncNameFromSource(tt.source)
		if got != tt.want {
			t.Errorf("initFuncNameFromSource(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestInitStateVarName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"", "_fileInit"},
		{"/app/node_modules/foo/index.js", "_fileInitFooIndex"},
		{"/app/node_modules/foo-bar/src/baz.js", "_fileInitFoobarSrcBaz"},
		{"/app/node_modules/@scope/pkg/sub/file.js", "_fileInitScopepkgSubFile"},
	}
	for _, tt := range tests {
		got := initStateVarName(tt.source)
		if got != tt.want {
			t.Errorf("initStateVarName(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestSanitizeInitName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo", "foo"},
		{"foo-bar", "foobar"},
		{"foo.bar", "foo_bar"},
		{"@scope", "scope"},
		{"has space", "has_space"},
		{"a/b", "ab"},
		{"a-b.c@d", "ab_cd"},
	}
	for _, tt := range tests {
		got := sanitizeInitName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeInitName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractBarePkgName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"/app/node_modules/foo/index.js", "foo"},
		{"/app/node_modules/foo-bar/src/baz.js", "foo-bar"},
		{"/app/node_modules/@scope/pkg/sub/file.js", "@scope/pkg"},
		{"/app/node_modules/@scope/pkg/index.js", "@scope/pkg"},
		{"/no/node_modules/here.js", "here.js"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractBarePkgName(tt.source)
		if got != tt.want {
			t.Errorf("extractBarePkgName(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestInitFuncNameUniqueness(t *testing.T) {
	// Two files in the same package (same node_modules/pkg) must get different names
	a := initFuncNameFromSource("/app/node_modules/foo/index.js")
	b := initFuncNameFromSource("/app/node_modules/foo/lib-helper.js")
	if a == b {
		t.Errorf("same-package files got same initFuncName: %q", a)
	}

	// Scoped packages
	c := initFuncNameFromSource("/app/node_modules/@scope/pkg/index.js")
	d := initFuncNameFromSource("/app/node_modules/@scope/pkg/src/util.js")
	if c == d {
		t.Errorf("scoped-package files got same initFuncName: %q", c)
	}
}

func TestInitFuncNameNoExtensionLeak(t *testing.T) {
	name := initFuncNameFromSource("node_modules/foo/bar.js")
	if strings.HasSuffix(name, "_js") {
		t.Errorf("extension leaked into name: %q", name)
	}
}

func TestModuleStateInitStmts(t *testing.T) {
	stmts := moduleStateInitStmts("_fileInitTest", "initTestPkg")
	if len(stmts) == 0 {
		t.Fatal("moduleStateInitStmts returned no statements")
	}
	if _, ok := stmts[0].(*ast.IfStmt); !ok {
		t.Fatalf("expected IfStmt, got %T", stmts[0])
	}
}