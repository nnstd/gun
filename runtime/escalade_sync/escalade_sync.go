package escalade_sync

import (
	"os"
	"path/filepath"
)

// Default walks up the directory tree from start, calling callback with
// (directory, files) at each level. Returns the first truthy callback result.
func Default(start string, callback func(string, []string) string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}

	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		result := callback(dir, names)
		if result != "" {
			return filepath.Join(dir, result)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
