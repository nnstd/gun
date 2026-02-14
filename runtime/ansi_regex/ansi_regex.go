package ansi_regex

import "regexp"

var pattern = `[\x1B\x9B][[\]()#;?]*(?:(?:(?:(?:;[-a-zA-Z\d/#&.:=?%@~_]+)*|[a-zA-Z\d]+(?:;[-a-zA-Z\d/#&.:=?%@~_]*)*)?(?:\x07|\x1B\x5C|\x9C))|(?:(?:\d{1,4}(?:;\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]))`

// Default returns a compiled regexp matching ANSI escape codes.
func Default() *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
