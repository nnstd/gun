package jsvalue

import (
	"fmt"
	"strings"
)

// _emptyString is the interned "" singleton. Prototype is patched in
// prototype.init() to avoid init-order dependency.
var _emptyString = &JSValue{typ: TypeString, strVal: "", frozen: true}

// NewString creates a string JSValue. Returns the singleton for "".
func NewString(s string) *JSValue {
	if s == "" {
		return _emptyString
	}
	return &JSValue{typ: TypeString, strVal: s, prototype: StringPrototype}
}

// ToLowerCase converts a JSValue to lowercase string and wraps it.
func ToLowerCase(val *JSValue) *JSValue {
	return NewString(strings.ToLower(fmt.Sprint(val)))
}

// ToUpperCase converts a JSValue to uppercase string and wraps it.
func ToUpperCase(val *JSValue) *JSValue {
	return NewString(strings.ToUpper(fmt.Sprint(val)))
}

// Trim trims whitespace from a JSValue string.
func Trim(val *JSValue) *JSValue {
	return NewString(strings.TrimSpace(fmt.Sprint(val)))
}

// Split splits a JSValue string by separator.
func Split(val *JSValue, sep *JSValue) *JSValue {
	s := fmt.Sprint(val)
	// Regex separator: use GoRegex.Split
	if sep != nil && sep.typ == TypeRegex && sep.regexVal != nil {
		if re, ok := sep.regexVal.(GoRegex); ok {
			parts := re.Split(s, -1)
			return FromStrings(parts)
		}
	}
	// String separator
	sepStr := ","
	if sep != nil {
		sepStr = sep.String()
	}
	parts := strings.Split(s, sepStr)
	return FromStrings(parts)
}

// Replace replaces occurrences of pattern with replacement in a JSValue string.
// pattern can be a string JSValue or a regex JSValue.
func Replace(val *JSValue, pattern, replacement *JSValue) *JSValue {
	s := fmt.Sprint(val)
	repl := ""
	if replacement != nil {
		repl = replacement.String()
	}
	if pattern != nil && pattern.typ == TypeRegex && pattern.regexVal != nil {
		if re, ok := pattern.regexVal.(interface {
			ReplaceAllString(string, string) string
		}); ok {
			return NewString(re.ReplaceAllString(s, repl))
		}
	}
	old := ""
	if pattern != nil {
		old = pattern.String()
	}
	return NewString(strings.Replace(s, old, repl, -1))
}

// CharAt returns the character at the given index.
func CharAt(val *JSValue, index *JSValue) *JSValue {
	s := fmt.Sprint(val)
	runes := []rune(s)
	idx := 0
	if index != nil {
		idx = int(index.Number())
	}
	if idx < 0 || idx >= len(runes) {
		return NewString("")
	}
	return NewString(string(runes[idx]))
}

// StartsWith checks if a JSValue string starts with prefix.
func StartsWith(val *JSValue, prefix *JSValue) *JSValue {
	p := ""
	if prefix != nil {
		p = prefix.String()
	}
	return NewBool(strings.HasPrefix(fmt.Sprint(val), p))
}

// EndsWith checks if a JSValue string ends with suffix.
func EndsWith(val *JSValue, suffix *JSValue) *JSValue {
	s := ""
	if suffix != nil {
		s = suffix.String()
	}
	return NewBool(strings.HasSuffix(fmt.Sprint(val), s))
}

// Repeat repeats a JSValue string count times.
func Repeat(val *JSValue, count *JSValue) *JSValue {
	n := 0
	if count != nil {
		n = int(count.Number())
	}
	return NewString(strings.Repeat(fmt.Sprint(val), n))
}

// LastIndexOf returns the last index of search in str, starting from position.
func LastIndexOf(str *JSValue, search *JSValue, position ...*JSValue) *JSValue {
	s := fmt.Sprint(str)
	sub := fmt.Sprint(search)
	if len(position) > 0 && position[0] != nil {
		pos := int(position[0].Number())
		if pos < len(s) {
			s = s[:pos+1]
		}
	}
	return NewNumber(float64(strings.LastIndex(s, sub)))
}

// Substring returns the part of the string between start and end indices.
func Substring(str *JSValue, start *JSValue, end ...*JSValue) *JSValue {
	s := []rune(fmt.Sprint(str))
	st := 0
	if start != nil {
		st = int(start.Number())
	}
	if st < 0 {
		st = 0
	}
	if st > len(s) {
		st = len(s)
	}
	e := len(s)
	if len(end) > 0 && end[0] != nil {
		e = int(end[0].Number())
	}
	if e < 0 {
		e = 0
	}
	if e > len(s) {
		e = len(s)
	}
	if st > e {
		st, e = e, st
	}
	return NewString(string(s[st:e]))
}

// StringSlice implements JavaScript String.prototype.slice semantics.
func StringSlice(str *JSValue, start *JSValue, end ...*JSValue) *JSValue {
	s := []rune(fmt.Sprint(str))
	length := len(s)
	st := 0
	if start != nil {
		st = int(start.Number())
	}
	if st < 0 {
		st = length + st
	}
	if st < 0 {
		st = 0
	}
	if st > length {
		st = length
	}
	e := length
	if len(end) > 0 && end[0] != nil {
		e = int(end[0].Number())
		if e < 0 {
			e = length + e
		}
	}
	if e < 0 {
		e = 0
	}
	if e > length {
		e = length
	}
	if e < st {
		e = st
	}
	return NewString(string(s[st:e]))
}

func initStringPrototype() {
	// toString returns the string value.
	StringPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].String())
			}
			return NewString("")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})

	// normalize returns the string as-is (basic NFC is a no-op for ASCII).
	StringPrototype.DefineProperty("normalize", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return args[0]
			}
			return NewString("")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})

	defMethod(StringPrototype, "toLowerCase", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.ToLower(args[0].String()))
	})
	defMethod(StringPrototype, "toUpperCase", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.ToUpper(args[0].String()))
	})
	defMethod(StringPrototype, "trim", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.TrimSpace(args[0].String()))
	})
	defMethod(StringPrototype, "trimStart", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.TrimLeft(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "trimLeft", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.TrimLeft(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "trimEnd", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.TrimRight(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "trimRight", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return NewString(strings.TrimRight(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "slice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return StringSlice(args[0], safeArg(args, 1), args[2:]...)
	})
	defMethod(StringPrototype, "split", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewArray()
		}
		return Split(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "replace", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return Replace(args[0], safeArg(args, 1), safeArg(args, 2))
	})
	defMethod(StringPrototype, "replaceAll", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return Replace(args[0], safeArg(args, 1), safeArg(args, 2))
	})
	defMethod(StringPrototype, "charAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return CharAt(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(-1)
		}
		s := args[0].String()
		sub := args[1].String()
		start := 0
		if len(args) > 2 && args[2] != nil {
			start = int(args[2].Number())
		}
		runes := []rune(s)
		if start < 0 {
			start = 0
		}
		if start >= len(runes) {
			return NewNumber(-1)
		}
		idx := strings.Index(string(runes[start:]), sub)
		if idx < 0 {
			return NewNumber(-1)
		}
		return NewNumber(float64(start + idx))
	})
	defMethod(StringPrototype, "lastIndexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(-1)
		}
		return LastIndexOf(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "substring", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		extras := args[2:]
		return Substring(args[0], safeArg(args, 1), extras...)
	})
	defMethod(StringPrototype, "startsWith", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewBool(false)
		}
		return StartsWith(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "endsWith", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewBool(false)
		}
		return EndsWith(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "includes", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewBool(false)
		}
		return Includes(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "repeat", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("")
		}
		return Repeat(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "match", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewUndefined()
		}
		return RegexExec(safeArg(args, 1), args[0])
	})
	defMethod(StringPrototype, "search", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return NewNumber(-1)
		}
		pattern := safeArg(args, 1)
		if pattern != nil && pattern.typ == TypeRegex && pattern.regexVal != nil {
			if re, ok := pattern.regexVal.(interface{ FindStringIndex(string) []int }); ok {
				loc := re.FindStringIndex(args[0].String())
				if loc != nil {
					return NewNumber(float64(loc[0]))
				}
			}
		}
		if pattern != nil {
			return NewNumber(float64(strings.Index(args[0].String(), pattern.String())))
		}
		return NewNumber(-1)
	})
	defMethod(StringPrototype, "padStart", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return args[0]
		}
		s := args[0].String()
		targetLen := int(args[1].Number())
		pad := " "
		if len(args) > 2 && args[2] != nil {
			pad = args[2].String()
		}
		for len([]rune(s)) < targetLen {
			s = pad + s
		}
		return NewString(string([]rune(s)[:targetLen]))
	})
	defMethod(StringPrototype, "padEnd", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil {
			return args[0]
		}
		s := args[0].String()
		targetLen := int(args[1].Number())
		pad := " "
		if len(args) > 2 && args[2] != nil {
			pad = args[2].String()
		}
		for len([]rune(s)) < targetLen {
			s = s + pad
		}
		return NewString(string([]rune(s)[:targetLen]))
	})
	defMethod(StringPrototype, "codePointAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		runes := []rune(args[0].String())
		idx := 0
		if len(args) > 1 && args[1] != nil {
			idx = int(args[1].Number())
		}
		if idx < 0 || idx >= len(runes) {
			return NewUndefined()
		}
		return NewNumber(float64(runes[idx]))
	})
	defMethod(StringPrototype, "charCodeAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		runes := []rune(args[0].String())
		idx := 0
		if len(args) > 1 && args[1] != nil {
			idx = int(args[1].Number())
		}
		if idx < 0 || idx >= len(runes) {
			return NewNumber(0)
		}
		return NewNumber(float64(runes[idx]))
	})
	defMethod(StringPrototype, "at", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewUndefined()
		}
		s := []rune(args[0].String())
		idx := 0
		if len(args) > 1 && args[1] != nil {
			idx = int(args[1].Number())
		}
		if idx < 0 {
			idx = len(s) + idx
		}
		if idx < 0 || idx >= len(s) {
			return NewUndefined()
		}
		return NewString(string(s[idx]))
	})
}
