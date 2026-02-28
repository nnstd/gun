package url

import (
	"strings"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// FileURLToPath converts a file:// URL to a local file path.
func FileURLToPath(url *jsvalue.JSValue) *jsvalue.JSValue {
	s := ""
	if url != nil {
		s = url.String()
	}
	s = strings.TrimPrefix(s, "file://")
	return jsvalue.NewString(s)
}
