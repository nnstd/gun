package cliui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nnstd/gun/runtime/jsvalue"
)

// UI formats text into aligned columns for CLI output.
type UI struct {
	width int
	wrap  bool
	rows  []row
}

type row struct {
	cols []col
	span bool
}

type col struct {
	text    string
	width   int
	align   string
	padding [4]int
}

// Default is the default export matching the npm cliui package.
// It accepts an optional *jsvalue.JSValue or map with "width" and "wrap" keys.
func Default(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	width := 80
	wrap := true

	if len(args) > 0 && args[0] != nil && args[0].Type() == jsvalue.TypeObject {
		opts := args[0]
		if w := opts.Get("width"); w != nil && w.Type() == jsvalue.TypeNumber {
			width = w.Int()
		}
		if wr := opts.Get("wrap"); wr != nil && wr.Type() == jsvalue.TypeBoolean {
			wrap = wr.Bool()
		}
	}

	ui := &UI{width: width, wrap: wrap}

	obj := jsvalue.NewObject()
	obj.Set("div", jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
		ui.div(a)
		return jsvalue.NewUndefined()
	}))
	obj.Set("span", jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
		ui.span(a)
		return jsvalue.NewUndefined()
	}))
	obj.Set("resetOutput", jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
		ui.rows = nil
		return jsvalue.NewUndefined()
	}))
	obj.Set("toString", jsvalue.NewFunction(func(a ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(ui.String())
	}))
	return obj
}

func (u *UI) div(args []*jsvalue.JSValue) {
	if len(args) == 0 {
		u.rows = append(u.rows, row{cols: []col{{text: ""}}})
		return
	}
	var cols []col
	for _, arg := range args {
		cols = append(cols, parseCol(arg))
	}
	u.rows = append(u.rows, row{cols: cols})
}

func (u *UI) span(args []*jsvalue.JSValue) {
	if len(args) == 0 {
		u.rows = append(u.rows, row{cols: []col{{text: ""}}, span: true})
		return
	}
	var cols []col
	for _, arg := range args {
		cols = append(cols, parseCol(arg))
	}
	u.rows = append(u.rows, row{cols: cols, span: true})
}

func parseCol(v *jsvalue.JSValue) col {
	if v == nil {
		return col{}
	}
	if v.Type() == jsvalue.TypeString {
		return col{text: v.String()}
	}
	if v.Type() != jsvalue.TypeObject {
		return col{text: fmt.Sprint(v)}
	}

	c := col{}
	if t := v.Get("text"); t != nil && t.Type() != jsvalue.TypeUndefined {
		c.text = t.String()
	}
	if w := v.Get("width"); w != nil && w.Type() == jsvalue.TypeNumber {
		c.width = w.Int()
	}
	if a := v.Get("align"); a != nil && a.Type() == jsvalue.TypeString {
		c.align = a.String()
	}
	if p := v.Get("padding"); p != nil && p.Type() == jsvalue.TypeObject {
		arr := p.Array()
		for i := 0; i < 4 && i < len(arr); i++ {
			if arr[i] != nil && arr[i].Type() == jsvalue.TypeNumber {
				c.padding[i] = arr[i].Int()
			}
		}
	}
	return c
}

// String renders the UI output.
func (u *UI) String() string {
	var lines []string
	for _, r := range u.rows {
		line := u.renderRow(r)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (u *UI) renderRow(r row) string {
	if len(r.cols) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(r.cols))
	remaining := u.width
	unset := 0
	for i, c := range r.cols {
		if c.width > 0 {
			widths[i] = c.width
			remaining -= c.width
		} else {
			unset++
		}
	}
	if unset > 0 {
		each := remaining / unset
		if each < 1 {
			each = 1
		}
		for i := range widths {
			if widths[i] == 0 {
				widths[i] = each
			}
		}
	}

	var parts []string
	for i, c := range r.cols {
		text := c.text
		padLeft := c.padding[3]
		padRight := c.padding[1]

		colWidth := widths[i] - padLeft - padRight
		if colWidth < 0 {
			colWidth = 0
		}

		// Truncate or pad text to fit column width
		textWidth := utf8.RuneCountInString(text)
		if u.wrap && colWidth > 0 && textWidth > colWidth {
			text = string([]rune(text)[:colWidth])
			textWidth = colWidth
		}

		switch c.align {
		case "right":
			if textWidth < colWidth {
				text = strings.Repeat(" ", colWidth-textWidth) + text
			}
		case "center":
			if textWidth < colWidth {
				left := (colWidth - textWidth) / 2
				text = strings.Repeat(" ", left) + text
			}
		default:
			if textWidth < colWidth {
				text = text + strings.Repeat(" ", colWidth-textWidth)
			}
		}

		parts = append(parts, strings.Repeat(" ", padLeft)+text+strings.Repeat(" ", padRight))
	}

	return strings.Join(parts, "")
}
