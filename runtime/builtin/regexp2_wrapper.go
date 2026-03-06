package jsvalue

import (
	"strings"

	"github.com/dlclark/regexp2"
)

// regexp2Wrapper wraps *regexp2.Regexp to provide the same interface methods
// as *regexp.Regexp. This enables transparent fallback from Go's RE2 to the
// .NET-compatible regexp2 engine for JS regex features like lookaheads.
type regexp2Wrapper struct {
	re      *regexp2.Regexp
	pattern string
}

func newRegexp2(pattern string) (*regexp2Wrapper, error) {
	re, err := regexp2.Compile(pattern, regexp2.RE2)
	if err != nil {
		// Try without RE2 flag for full .NET compat (lookaheads etc.)
		re, err = regexp2.Compile(pattern, 0)
		if err != nil {
			return nil, err
		}
	}
	return &regexp2Wrapper{re: re, pattern: pattern}, nil
}

func (w *regexp2Wrapper) MatchString(s string) bool {
	m, _ := w.re.MatchString(s)
	return m
}

func (w *regexp2Wrapper) String() string {
	return w.pattern
}

func (w *regexp2Wrapper) ReplaceAllString(src, repl string) string {
	result, _ := w.re.Replace(src, repl, -1, -1)
	return result
}

func (w *regexp2Wrapper) FindStringSubmatch(s string) []string {
	m, _ := w.re.FindStringMatch(s)
	if m == nil {
		return nil
	}
	groups := m.Groups()
	result := make([]string, len(groups))
	for i, g := range groups {
		result[i] = g.String()
	}
	return result
}

func (w *regexp2Wrapper) Split(s string, n int) []string {
	// Implement split using FindStringMatch
	var parts []string
	lastIndex := 0
	count := 0

	m, _ := w.re.FindStringMatch(s)
	for m != nil {
		if n > 0 && count >= n-1 {
			break
		}
		parts = append(parts, s[lastIndex:m.Index])
		lastIndex = m.Index + m.Length
		count++
		m, _ = w.re.FindNextMatch(m)
	}
	parts = append(parts, s[lastIndex:])
	return parts
}

// FindAllString returns all non-overlapping matches, like regexp.FindAllString.
func (w *regexp2Wrapper) FindAllString(s string, n int) []string {
	var results []string
	m, _ := w.re.FindStringMatch(s)
	for m != nil {
		if n >= 0 && len(results) >= n {
			break
		}
		results = append(results, m.String())
		m, _ = w.re.FindNextMatch(m)
	}
	return results
}

// FindStringSubmatchIndex returns the index pairs for the first match.
func (w *regexp2Wrapper) FindStringSubmatchIndex(s string) []int {
	m, _ := w.re.FindStringMatch(s)
	if m == nil {
		return nil
	}
	groups := m.Groups()
	result := make([]int, 0, len(groups)*2)
	for _, g := range groups {
		if g.Length == 0 && g.Index == 0 && !strings.Contains(s, g.String()) {
			result = append(result, -1, -1)
		} else {
			result = append(result, g.Index, g.Index+g.Length)
		}
	}
	return result
}
