package yargs_parser

import (
	"os"
	"strings"
)

// YargsParser provides command-line argument parsing compatible with yargs-parser.
type YargsParser struct {
	cwd       func() string
	env       func() map[string]string
	normalize func(string) string
	resolve   func(...string) string
}

// Options configures the parser behavior.
type Options struct {
	Alias         map[string][]string
	Array         []string
	Boolean       []string
	Count         []string
	Default       map[string]any
	Number        []string
	String        []string
	Configuration map[string]bool
}

// ParseResult holds the parsed arguments.
type ParseResult struct {
	Argv   map[string]any
	Error  error
	Aliases map[string][]string
}

// Default creates a new YargsParser with default settings.
func Default(opts ...map[string]any) *YargsParser {
	return &YargsParser{
		cwd: func() string {
			d, _ := os.Getwd()
			return d
		},
		env: func() map[string]string {
			result := make(map[string]string)
			for _, e := range os.Environ() {
				if i := strings.IndexByte(e, '='); i >= 0 {
					result[e[:i]] = e[i+1:]
				}
			}
			return result
		},
	}
}

// Parse parses command-line arguments.
func (p *YargsParser) Parse(args []string, opts ...any) *ParseResult {
	result := &ParseResult{
		Argv:    make(map[string]any),
		Aliases: make(map[string][]string),
	}
	result.Argv["_"] = []string{}

	positional := []string{}
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			key := arg[2:]
			if eqIdx := strings.IndexByte(key, '='); eqIdx >= 0 {
				result.Argv[key[:eqIdx]] = key[eqIdx+1:]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.Argv[key] = args[i+1]
				i++
			} else {
				result.Argv[key] = true
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			flags := arg[1:]
			for j, ch := range flags {
				key := string(ch)
				if j == len(flags)-1 && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					result.Argv[key] = args[i+1]
					i++
				} else {
					result.Argv[key] = true
				}
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}
	result.Argv["_"] = positional
	return result
}

// CamelCase converts a-b-c to aBC.
func CamelCase(str string) string {
	parts := strings.Split(str, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// Decamelize converts aBC to a-b-c.
func Decamelize(str string, sep ...string) string {
	s := "-"
	if len(sep) > 0 {
		s = sep[0]
	}
	var result strings.Builder
	for i, ch := range str {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				result.WriteString(s)
			}
			result.WriteRune(ch + 32)
		} else {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// LooksLikeNumber returns true if the string looks like a number.
func LooksLikeNumber(x string) bool {
	if x == "" {
		return false
	}
	for i, ch := range x {
		if ch == '-' && i == 0 {
			continue
		}
		if ch == '.' {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
