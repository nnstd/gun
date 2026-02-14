package ansi_styles

import "fmt"

// Style represents an ANSI style with open/close escape sequences.
type Style struct {
	Open  string
	Close string
}

func esc(open, close int) Style {
	return Style{
		Open:  fmt.Sprintf("\x1b[%dm", open),
		Close: fmt.Sprintf("\x1b[%dm", close),
	}
}

// Modifier styles
var (
	Reset         = esc(0, 0)
	Bold          = esc(1, 22)
	Dim           = esc(2, 22)
	Italic        = esc(3, 23)
	Underline     = esc(4, 24)
	Overline      = esc(53, 55)
	Inverse       = esc(7, 27)
	Hidden        = esc(8, 28)
	Strikethrough = esc(9, 29)
)

// Foreground color styles
var (
	Black   = esc(30, 39)
	Red     = esc(31, 39)
	Green   = esc(32, 39)
	Yellow  = esc(33, 39)
	Blue    = esc(34, 39)
	Magenta = esc(35, 39)
	Cyan    = esc(36, 39)
	White   = esc(37, 39)
	Gray    = esc(90, 39)
)

// Background color styles
var (
	BgBlack   = esc(40, 49)
	BgRed     = esc(41, 49)
	BgGreen   = esc(42, 49)
	BgYellow  = esc(43, 49)
	BgBlue    = esc(44, 49)
	BgMagenta = esc(45, 49)
	BgCyan    = esc(46, 49)
	BgWhite   = esc(47, 49)
	BgGray    = esc(100, 49)
)

// Codes maps open codes to their close codes.
var Codes = map[int]int{
	0: 0, 1: 22, 2: 22, 3: 23, 4: 24, 7: 27, 8: 28, 9: 29, 53: 55,
	30: 39, 31: 39, 32: 39, 33: 39, 34: 39, 35: 39, 36: 39, 37: 39, 90: 39,
	40: 49, 41: 49, 42: 49, 43: 49, 44: 49, 45: 49, 46: 49, 47: 49, 100: 49,
}
