package module

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// BuiltinModules is the list of known built-in module names.
var BuiltinModules = []string{
	"assert", "buffer", "child_process", "crypto", "dgram", "dns", "events",
	"fs", "http", "https", "module", "os", "path", "process",
	"stream", "string_decoder", "timers", "url", "util", "v8", "zlib",
}

// IsBuiltin reports whether the given module name is a Node.js built-in.
func IsBuiltin(name string) bool {
	name = strings.TrimPrefix(name, "node:")
	for _, b := range BuiltinModules {
		if b == name {
			return true
		}
	}
	return false
}

// ModuleRegistry is the global runtime module registry. Builtins are loaded
// from an init() in builtins.go; transpiled packages self-register from
// their own init() via RegisterModule.
var (
	ModuleRegistry = map[string]*jsvalue.JSValue{}
	registryMu     sync.Mutex
)

// RegisterModule adds a module's exports object to the global registry.
// Safe to call from any goroutine and from package init().
func RegisterModule(name string, exports *jsvalue.JSValue) {
	registryMu.Lock()
	defer registryMu.Unlock()
	ModuleRegistry[name] = exports
}

// lookupRegistry reads the registry under the mutex.
func lookupRegistry(name string) (*jsvalue.JSValue, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	v, ok := ModuleRegistry[name]
	return v, ok
}

// Meta holds import.meta properties.
type Meta struct {
	Url     *jsvalue.JSValue
	Resolve func(*jsvalue.JSValue) *jsvalue.JSValue
}

func init() {
	// import.meta.resolve(specifier) — resolves a module specifier relative to the current module
	ImportMeta.Resolve = func(specifier *jsvalue.JSValue) *jsvalue.JSValue {
		if specifier == nil {
			return jsvalue.NewUndefined()
		}
		spec := specifier.String()
		if filepath.IsAbs(spec) {
			return jsvalue.NewString("file://" + spec)
		}
		exe, err := os.Executable()
		if err != nil {
			return jsvalue.NewString(spec)
		}
		resolved := filepath.Join(filepath.Dir(exe), spec)
		return jsvalue.NewString("file://" + resolved)
	}
}

// ImportMeta returns the import.meta object for the current process.
var ImportMeta = &Meta{
	Url: jsvalue.NewString(func() string {
		exe, err := os.Executable()
		if err != nil {
			return ""
		}
		return "file://" + exe
	}()),
}

// normalizeRequirePath normalizes a module specifier to canonical form for
// registry lookup. Relative/absolute paths become cleaned absolute-ish paths;
// bare names are returned untouched.
func normalizeRequirePath(id, base string) string {
	id = strings.TrimSuffix(id, "/")
	if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
		return filepath.Clean(filepath.Join(base, id))
	}
	return id
}

// CreateRequire returns a require function (as *JSValue) anchored at the
// given filename. The returned function:
//  1. Strips node: prefix.
//  2. Consults ModuleRegistry directly (builtins + transpiled locals).
//  3. For relative/absolute paths, also tries the path normalized against the anchor.
//  4. Falls back to reading JSON/raw files from disk.
//
// The returned function additionally carries require.resolve, require.cache,
// and require.main as properties, matching Node.js semantics.
func CreateRequire(filename *jsvalue.JSValue) *jsvalue.JSValue {
	fn := ""
	if filename != nil {
		fn = filename.String()
	}
	fn = strings.TrimPrefix(fn, "file://")
	base := filepath.Dir(fn)

	requireFn := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		id := ""
		if len(args) > 0 && args[0] != nil {
			id = args[0].String()
		}
		id = strings.TrimPrefix(id, "node:")

		if mod, ok := lookupRegistry(id); ok {
			return mod
		}

		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
			resolved := normalizeRequirePath(id, base)
			if mod, ok := lookupRegistry(resolved); ok {
				return mod
			}
		}

		target := id
		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
			target = filepath.Join(base, id)
		}
		candidates := []string{target}
		if filepath.Ext(target) == "" {
			candidates = append(candidates, target+".json", filepath.Join(target, "index.json"))
		}
		for _, c := range candidates {
			data, err := os.ReadFile(c)
			if err != nil {
				continue
			}
			if strings.HasSuffix(c, ".json") {
				var v any
				if json.Unmarshal(data, &v) == nil {
					return jsonToJSValue(v)
				}
			}
			return jsvalue.NewString(string(data))
		}
		return jsvalue.NewUndefined()
	})

	requireFn.Set("resolve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		id := ""
		if len(args) > 0 && args[0] != nil {
			id = args[0].String()
		}
		id = strings.TrimPrefix(id, "node:")

		if IsBuiltin(id) {
			return jsvalue.NewString("node:" + id)
		}
		resolved := id
		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
			resolved = filepath.Join(base, id)
		} else {
			resolved = filepath.Join(base, "node_modules", id)
		}
		if abs, err := filepath.Abs(resolved); err == nil {
			return jsvalue.NewString(abs)
		}
		return jsvalue.NewString(resolved)
	}))
	requireFn.Set("cache", jsvalue.NewObject())
	requireFn.Set("main", jsvalue.NewUndefined())

	return requireFn
}

func jsonToJSValue(v any) *jsvalue.JSValue {
	switch val := v.(type) {
	case nil:
		return jsvalue.NewNull()
	case bool:
		return jsvalue.NewBool(val)
	case float64:
		return jsvalue.NewNumber(val)
	case string:
		return jsvalue.NewString(val)
	case []any:
		elems := make([]*jsvalue.JSValue, len(val))
		for i, elem := range val {
			elems[i] = jsonToJSValue(elem)
		}
		return jsvalue.NewArray(elems...)
	case map[string]any:
		pairs := make([]any, 0, len(val)*2)
		for key, elem := range val {
			pairs = append(pairs, key, jsonToJSValue(elem))
		}
		return jsvalue.ObjectFrom(pairs...)
	default:
		return jsvalue.NewUndefined()
	}
}

// AsJSValue returns a JSValue object representing the 'module' module.
// Properties:
//   - createRequire: NewFunction wrapping CreateRequire
//   - importMeta: the import.meta object
var AsJSValue = func() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("createRequire", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return CreateRequire(args[0])
		}
		return CreateRequire(nil)
	}))
	obj.Set("importMeta", ImportMetaAsJSValue())
	return obj
}()

// ImportMetaAsJSValue returns the import.meta object as a *JSValue.
func ImportMetaAsJSValue() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("url", ImportMeta.Url)
	obj.Set("resolve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 && ImportMeta.Resolve != nil {
			return ImportMeta.Resolve(args[0])
		}
		return jsvalue.NewUndefined()
	}))
	return obj
}
