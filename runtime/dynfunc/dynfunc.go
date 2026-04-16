// Package dynfunc provides runtime compilation of JavaScript function bodies
// into callable Go functions. It supports the `new Function()` constructor
// and `eval()` by transpiling JS through the Gun pipeline, compiling the
// resulting Go source as a plugin, and loading it in-process.
package dynfunc

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
	"github.com/nnstd/gun/compiler/pipeline"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var (
	mu       sync.Mutex
	cache    map[string]*jsvalue.JSValue // body hash → compiled function
	gunRoot  string
	initOnce sync.Once
)

func initCache() {
	cache = make(map[string]*jsvalue.JSValue)
}

func getGunRoot() string {
	initOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if _, err := os.Stat(filepath.Join(dir, "compiler")); err == nil {
					gunRoot = dir
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	})
	return gunRoot
}

// CompileFunction compiles a JS function body into a callable JSValue function.
// Mirrors `new Function(paramNames..., body)` from the JS spec.
// The last argument is the function body; all preceding arguments are parameter names.
func CompileFunction(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		return jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		})
	}

	// Last arg is body, rest are param names
	body := args[len(args)-1].String()
	var paramNames []string
	for _, a := range args[:len(args)-1] {
		for _, p := range strings.Split(a.String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				paramNames = append(paramNames, p)
			}
		}
	}

	// Check cache
	cacheKey := strings.Join(paramNames, ",") + "\x00" + body
	mu.Lock()
	if cache == nil {
		initCache()
	}
	if cached, ok := cache[cacheKey]; ok {
		mu.Unlock()
		return cached
	}
	mu.Unlock()

	// Construct JS source: function DynFunc(p1, p2, ...) { body }
	params := strings.Join(paramNames, ", ")
	jsSource := fmt.Sprintf("function DynFunc(%s) { %s }", params, body)

	fn, err := compileJSFunction(jsSource, cacheKey)
	if err != nil {
		return jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
			panic(jsvalue.NewString(fmt.Sprintf("CompileFunction error: %v", err)))
		})
	}

	mu.Lock()
	cache[cacheKey] = fn
	mu.Unlock()
	return fn
}

// Eval evaluates a JS expression or statement and returns the result.
func Eval(code *jsvalue.JSValue) *jsvalue.JSValue {
	jsSource := fmt.Sprintf("function DynEval() { return (%s) }", code.String())

	fn, err := compileJSFunction(jsSource, "eval:"+code.String())
	if err != nil {
		return jsvalue.NewString(fmt.Sprintf("Eval error: %v", err))
	}

	return fn.Call()
}

// compileJSFunction transpiles a JS function declaration to Go, builds it as
// a plugin, and loads the resulting *jsvalue.JSValue.
// uniqueKey is used to generate a unique module name so each plugin loads separately.
func compileJSFunction(jsSource string, uniqueKey string) (*jsvalue.JSValue, error) {
	// 1. Parse with tree-sitter
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("set language: %w", err)
	}

	source := []byte(jsSource)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse JS: %s", jsSource)
	}
	defer tree.Close()

	// 2. Transpile through pipeline
	pipe := pipeline.New(pipeline.O0)
	goSource, err := pipe.CompileTree(tree.RootNode(), source, "main", "github.com/nnstd/gun", false)
	if err != nil {
		return nil, fmt.Errorf("transpile: %w", err)
	}

	// 3. Write to temp dir and build as plugin
	root := getGunRoot()
	if root == "" {
		return nil, fmt.Errorf("gun root not found")
	}

	// Unique module name from hash of the key — prevents "plugin already loaded"
	h := sha256.Sum256([]byte(uniqueKey))
	modName := fmt.Sprintf("dynfunc_%x", h[:8])
	tmpDir := filepath.Join(os.TempDir(), modName)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	gomod := fmt.Sprintf(`module %s

go 1.25.0

require github.com/nnstd/gun v0.0.0

replace github.com/nnstd/gun => %s
`, modName, root)

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), goSource, 0644); err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	soPath := filepath.Join(tmpDir, modName+".so")
	cmd = exec.Command("go", "build", "-buildmode=plugin", "-o", soPath, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("go build plugin: %w\n%s", err, out)
	}

	// 4. Load plugin
	plug, err := plugin.Open(soPath)
	if err != nil {
		return nil, fmt.Errorf("plugin.Open: %w", err)
	}

	// 5. Lookup DynFunc (the function declared in the JS source)
	sym, err := plug.Lookup("DynFunc")
	if err != nil {
		return nil, fmt.Errorf("Lookup DynFunc: %w", err)
	}

	dynFunc, ok := sym.(**jsvalue.JSValue)
	if !ok {
		return nil, fmt.Errorf("expected **jsvalue.JSValue, got %T", sym)
	}

	return *dynFunc, nil
}
