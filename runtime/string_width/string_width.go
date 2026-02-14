package string_width

import (
	"unicode/utf8"

	"github.com/nnstd/gun/runtime/get_east_asian_width"
	"github.com/nnstd/gun/runtime/strip_ansi"
)

// Default returns the visual width of a string, accounting for
// ANSI escape codes, fullwidth characters, and emoji.
func Default(s string) int {
	s = strip_ansi.Default(s)
	width := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			i++
			continue
		}
		// Control characters have zero width
		if r < 0x20 || (r >= 0x7F && r < 0xA0) {
			i += size
			continue
		}
		// Zero-width characters
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			i += size
			continue
		}
		// Combining marks have zero width
		if r >= 0x0300 && r <= 0x036F {
			i += size
			continue
		}
		// Variation selectors
		if r >= 0xFE00 && r <= 0xFE0F {
			i += size
			continue
		}
		width += get_east_asian_width.EastAsianWidth(int(r))
		i += size
	}
	return width
}
