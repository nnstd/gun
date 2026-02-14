package wrap_ansi

import (
	"strings"

	"github.com/nnstd/gun/runtime/string_width"
	"github.com/nnstd/gun/runtime/strip_ansi"
)

// Default wraps a string to the specified column width, preserving ANSI codes.
func Default(s string, columns int, opts ...map[string]bool) string {
	hard := true
	trim := true
	wordWrap := true
	if len(opts) > 0 {
		if v, ok := opts[0]["hard"]; ok {
			hard = v
		}
		if v, ok := opts[0]["trim"]; ok {
			trim = v
		}
		if v, ok := opts[0]["wordWrap"]; ok {
			wordWrap = v
		}
	}

	if columns < 1 {
		if wordWrap {
			columns = 1
		} else {
			return s
		}
	}

	lines := strings.Split(s, "\n")
	var result []string

	for _, line := range lines {
		stripped := strip_ansi.Default(line)
		if string_width.Default(stripped) <= columns {
			result = append(result, line)
			continue
		}

		if hard {
			result = append(result, hardWrap(line, columns)...)
		} else if wordWrap {
			result = append(result, softWrap(line, columns)...)
		} else {
			result = append(result, line)
		}
	}

	if trim {
		for i, line := range result {
			result[i] = strings.TrimRight(line, " ")
		}
	}

	return strings.Join(result, "\n")
}

func hardWrap(s string, columns int) []string {
	var lines []string
	for len(s) > 0 {
		stripped := strip_ansi.Default(s)
		if string_width.Default(stripped) <= columns {
			lines = append(lines, s)
			break
		}
		// Find break point
		w := 0
		breakIdx := 0
		for i, r := range stripped {
			rw := 1
			if r > 0x7F {
				rw = string_width.Default(string(r))
			}
			if w+rw > columns {
				break
			}
			w += rw
			breakIdx = i + len(string(r))
		}
		if breakIdx == 0 {
			breakIdx = 1
		}
		lines = append(lines, s[:breakIdx])
		s = s[breakIdx:]
	}
	return lines
}

func softWrap(s string, columns int) []string {
	words := strings.Split(s, " ")
	var lines []string
	current := ""

	for _, word := range words {
		test := current
		if test != "" {
			test += " "
		}
		test += word

		if string_width.Default(strip_ansi.Default(test)) > columns {
			if current != "" {
				lines = append(lines, current)
			}
			current = word
		} else {
			current = test
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
