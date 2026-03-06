package jsvalue

import (
	"fmt"
	"regexp"
	"strconv"
)

// GoRegex is a regexp-compatible interface satisfied by both *regexp.Regexp
// and *regexp2Wrapper. This allows CompileRegex to transparently fall back to
// the regexp2 engine for JS regex features (lookaheads, lookbehinds) that
// Go's RE2 engine does not support.
type GoRegex interface {
	MatchString(s string) bool
	String() string
	ReplaceAllString(src, repl string) string
	FindStringSubmatch(s string) []string
	Split(s string, n int) []string
}

// jsUnicodeEscapeRe matches JS-style \uNNNN Unicode escapes in regex patterns.
var jsUnicodeEscapeRe = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// jsUnicodePropMap maps JS Unicode property names to Go equivalents.
var jsUnicodePropMap = map[string]string{
	"Default_Ignorable_Code_Point": "Cf",
	"Emoji_Presentation":           "So",
	"Extended_Pictographic":        "So",
	"Emoji_Modifier_Base":          "So",
	"Emoji_Modifier":               "Sk",
	"Emoji_Component":              "So",
	"Regional_Indicator":           "So",
}

// jsUnicodePropRe matches \p{PropertyName} or \P{PropertyName} in regex patterns.
var jsUnicodePropRe = regexp.MustCompile(`\\[pP]\{([^}]+)\}`)

// CompileRegex compiles a regex pattern, converting JS-style \uNNNN Unicode
// escapes and unsupported Unicode property names to Go-compatible equivalents.
// Uses Go's stdlib regexp (RE2) when possible, falls back to regexp2 (.NET
// engine) for patterns with lookaheads, lookbehinds, and other JS features.
func CompileRegex(pattern string) GoRegex {
	// Convert \uNNNN escapes to literal characters
	converted := jsUnicodeEscapeRe.ReplaceAllStringFunc(pattern, func(match string) string {
		hex := match[2:] // strip \u prefix
		code, err := strconv.ParseInt(hex, 16, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})
	// Convert unsupported Unicode property names to Go equivalents
	converted = jsUnicodePropRe.ReplaceAllStringFunc(converted, func(match string) string {
		prefix := match[:2]  // \p or \P
		name := match[3 : len(match)-1] // extract property name
		if goName, ok := jsUnicodePropMap[name]; ok {
			return prefix + "{" + goName + "}"
		}
		return match
	})
	// Tier 1: try Go stdlib regexp (RE2)
	re, err := regexp.Compile(converted)
	if err == nil {
		return re
	}
	// Tier 2: fall back to regexp2 for lookaheads, lookbehinds, etc.
	if r2, err2 := newRegexp2(converted); err2 == nil {
		return r2
	}
	// Last resort: return a regex that matches nothing
	return regexp.MustCompile(`\A\z(?:never)`)
}

// NewRegex creates a regex JSValue from a compiled regexp.
// The regex parameter should satisfy GoRegex (or *regexp.Regexp) but is typed
// as interface{} to avoid import cycles. The returned JSValue has test() and
// exec() methods.
func NewRegex(regex interface{}) *JSValue {
	v := &JSValue{
		typ:        TypeRegex,
		properties: make(map[string]*PropertyDescriptor),
		regexVal:   regex,
	}
	// Add test() method: regex.test(str) → boolean
	v.Set("test", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 {
			return NewBool(false)
		}
		return NewBool(v.MatchString(args[0]))
	}))
	// Add exec() method: regex.exec(str) → array of matches or null
	v.Set("exec", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 || v.regexVal == nil {
			return NewNull()
		}
		str := fmt.Sprint(args[0])
		if re, ok := v.regexVal.(interface {
			FindStringSubmatch(string) []string
		}); ok {
			matches := re.FindStringSubmatch(str)
			if matches == nil {
				return NewNull()
			}
			return FromStrings(matches)
		}
		return NewNull()
	}))
	return v
}

// MatchString tests whether a JSValue regex matches a given string.
// This is a core bridge method for regex.test() when the regex is a JSValue.
// The argument is coerced to string if needed.
// Returns false if the JSValue is not a regex.
func (v *JSValue) MatchString(s any) bool {
	if v == nil || v.typ != TypeRegex || v.regexVal == nil {
		return false
	}

	// Coerce argument to string
	var str string
	switch val := s.(type) {
	case string:
		str = val
	case *JSValue:
		str = val.String()
	default:
		str = fmt.Sprint(val)
	}

	if re, ok := v.regexVal.(interface{ MatchString(string) bool }); ok {
		return re.MatchString(str)
	}
	return false
}

// MatchString tests whether a regex JSValue matches a string.
// Package-level function version for use in transpiled code:
//
//	jsvalue.MatchString(pattern, value)
//
// The value argument is coerced to string if it's a JSValue.
// Returns false if the regex is not a valid regex JSValue.
func MatchString(regex *JSValue, value any) bool {
	if regex == nil || regex.typ != TypeRegex || regex.regexVal == nil {
		return false
	}

	// Coerce argument to string; nil values don't match
	var str string
	switch val := value.(type) {
	case string:
		str = val
	case *JSValue:
		if val == nil {
			return false
		}
		str = val.String()
	default:
		if value == nil {
			return false
		}
		str = fmt.Sprint(val)
	}

	if re, ok := regex.regexVal.(interface{ MatchString(string) bool }); ok {
		return re.MatchString(str)
	}
	return false
}

// RegexExec executes a regex match and returns an array of matches or null.
// This implements JavaScript's regex.exec(s) method.
func RegexExec(regex *JSValue, value any) *JSValue {
	if regex == nil || regex.typ != TypeRegex || regex.regexVal == nil {
		return NewNull()
	}
	var str string
	switch val := value.(type) {
	case string:
		str = val
	case *JSValue:
		str = val.String()
	default:
		str = fmt.Sprint(val)
	}
	if re, ok := regex.regexVal.(interface {
		FindStringSubmatch(string) []string
	}); ok {
		matches := re.FindStringSubmatch(str)
		if matches == nil {
			return NewNull()
		}
		return FromStrings(matches)
	}
	return NewNull()
}
