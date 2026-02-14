package get_east_asian_width

import "unicode"

// EastAsianWidth returns the display width of a Unicode code point (1 or 2).
func EastAsianWidth(codePoint int) int {
	if isWide(codePoint) || isFullWidth(codePoint) {
		return 2
	}
	return 1
}

// EastAsianWidthType returns the East Asian Width category string.
func EastAsianWidthType(codePoint int) string {
	if isWide(codePoint) {
		return "W"
	}
	if isFullWidth(codePoint) {
		return "F"
	}
	if isAmbiguous(codePoint) {
		return "A"
	}
	if isHalfWidth(codePoint) {
		return "H"
	}
	if isNarrow(codePoint) {
		return "Na"
	}
	return "N"
}

func isWide(cp int) bool {
	r := rune(cp)
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Yi, r) ||
		(cp >= 0x1100 && cp <= 0x115F) ||
		(cp >= 0x2E80 && cp <= 0x303E) ||
		(cp >= 0x3041 && cp <= 0x33BF) ||
		(cp >= 0xA000 && cp <= 0xA4CF) ||
		(cp >= 0xAC00 && cp <= 0xD7AF) ||
		(cp >= 0xF900 && cp <= 0xFAFF) ||
		(cp >= 0xFE30 && cp <= 0xFE6F) ||
		(cp >= 0x1F000 && cp <= 0x1FAFF) ||
		(cp >= 0x20000 && cp <= 0x2FFFF) ||
		(cp >= 0x30000 && cp <= 0x3FFFF)
}

func isFullWidth(cp int) bool {
	return cp == 0x3000 ||
		(cp >= 0xFF01 && cp <= 0xFF60) ||
		(cp >= 0xFFE0 && cp <= 0xFFE6)
}

func isAmbiguous(cp int) bool {
	return cp == 0x00A1 || cp == 0x00A4 ||
		(cp >= 0x00A7 && cp <= 0x00A8) ||
		cp == 0x00AA || cp == 0x00AD || cp == 0x00AE ||
		(cp >= 0x00B0 && cp <= 0x00B4) ||
		(cp >= 0x00B6 && cp <= 0x00BA) ||
		(cp >= 0x00BC && cp <= 0x00BF) ||
		cp == 0x00C6 || cp == 0x00D0 ||
		(cp >= 0x00D7 && cp <= 0x00D8) ||
		(cp >= 0x00DE && cp <= 0x00E1) ||
		cp == 0x00E6 ||
		(cp >= 0x00E8 && cp <= 0x00EA) ||
		(cp >= 0x00EC && cp <= 0x00ED) ||
		cp == 0x00F0 ||
		(cp >= 0x00F2 && cp <= 0x00F3) ||
		(cp >= 0x00F7 && cp <= 0x00FA) ||
		cp == 0x00FC || cp == 0x00FE
}

func isHalfWidth(cp int) bool {
	return (cp >= 0xFF61 && cp <= 0xFFDC) ||
		(cp >= 0xFFE8 && cp <= 0xFFEE)
}

func isNarrow(cp int) bool {
	return (cp >= 0x0020 && cp <= 0x007E) ||
		(cp >= 0x00A2 && cp <= 0x00A3) ||
		cp == 0x00A5 || cp == 0x00A6 ||
		cp == 0x00AC || cp == 0x00AF ||
		(cp >= 0x27E6 && cp <= 0x27ED) ||
		(cp >= 0x2985 && cp <= 0x2986)
}
