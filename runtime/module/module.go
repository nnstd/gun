package module

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// RequireFunc is the type returned by CreateRequire. It loads a module by path
// and returns its contents. JSON files are parsed into map[string]any;
// other modules return nil.
type RequireFunc func(id string) any

// CreateRequire returns a require function anchored at the given filename.
// The returned function supports loading JSON files relative to the anchor.
func CreateRequire(filename string) RequireFunc {
	// Strip file:// URL scheme if present.
	filename = strings.TrimPrefix(filename, "file://")
	base := filepath.Dir(filename)

	return func(id string) any {
		var target string
		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "/") {
			target = filepath.Join(base, id)
		} else {
			// Bare specifier — look in node_modules (best-effort).
			target = filepath.Join(base, "node_modules", id)
		}

		// Try .json extension if not already present.
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
					return v
				}
			}
			// Non-JSON file found — return contents as string.
			return string(data)
		}
		return nil
	}
}
