package module

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// BuiltinModules is the list of known built-in module names.
var BuiltinModules = []string{
	"assert", "buffer", "child_process", "crypto", "events",
	"fs", "http", "https", "module", "net", "os", "path",
	"stream", "string_decoder", "url", "util",
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

// Meta holds import.meta properties.
type Meta struct {
	Url *jsvalue.JSValue
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

// CreateRequire returns a require function (as *JSValue) anchored at the given filename.
// The returned function supports loading JSON files relative to the anchor.
func CreateRequire(filename *jsvalue.JSValue) *jsvalue.JSValue {
	fn := ""
	if filename != nil {
		fn = filename.String()
	}
	// Strip file:// URL scheme if present.
	fn = strings.TrimPrefix(fn, "file://")
	base := filepath.Dir(fn)

	return jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		id := ""
		if len(args) > 0 && args[0] != nil {
			id = args[0].String()
		}

		var target string
		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
			target = filepath.Join(base, id)
		} else {
			target = filepath.Join(base, "node_modules", id)
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
					return jsvalue.From(v)
				}
			}
			return jsvalue.NewString(string(data))
		}
		return jsvalue.NewUndefined()
	})
}
