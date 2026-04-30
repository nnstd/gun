package yamlmodule

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

type Export struct {
	OriginalName string
	GoName       string
}

type Module struct {
	Source  []byte
	Value   any
	Exports []Export
}

func Parse(source []byte) (*Module, error) {
	var value any
	if err := yaml.Unmarshal(source, &value); err != nil {
		return nil, err
	}
	value = normalize(value)
	return &Module{Source: source, Value: value, Exports: topLevelExports(value)}, nil
}

func Compile(source []byte, pkgName, defaultName string, namedAliases map[string]string) ([]byte, error) {
	mod, err := Parse(source)
	if err != nil {
		return nil, err
	}
	if defaultName == "" {
		defaultName = "Default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\n", pkgName)
	b.WriteString("import jsvalue \"github.com/nnstd/gun/runtime/builtin\"\n\n")
	fmt.Fprintf(&b, "var %s *jsvalue.JSValue\n", defaultName)
	for _, exp := range mod.Exports {
		name := namedAliases[exp.OriginalName]
		if name == "" {
			name = exp.GoName
		}
		if name == defaultName {
			continue
		}
		fmt.Fprintf(&b, "var %s *jsvalue.JSValue\n", name)
	}
	b.WriteString("\nfunc init() {\n")
	fmt.Fprintf(&b, "\t%s = %s\n", defaultName, valueExpr(mod.Value))
	for _, exp := range mod.Exports {
		name := namedAliases[exp.OriginalName]
		if name == "" {
			name = exp.GoName
		}
		if name == defaultName {
			continue
		}
		fmt.Fprintf(&b, "\t%s = %s.Get(%s)\n", name, defaultName, strconv.Quote(exp.OriginalName))
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func normalize(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, elem := range v {
			out[key] = normalize(elem)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, elem := range v {
			out[fmt.Sprint(key)] = normalize(elem)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = normalize(elem)
		}
		return out
	default:
		return value
	}
}

func topLevelExports(value any) []Export {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		if key != "default" && isExportableIdentifier(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	used := map[string]int{"Default": 1}
	exports := make([]Export, 0, len(keys))
	for _, key := range keys {
		name := makeUnique(capitalize(sanitize(key)), used)
		exports = append(exports, Export{OriginalName: key, GoName: name})
	}
	return exports
}

func valueExpr(value any) string {
	switch v := value.(type) {
	case nil:
		return "jsvalue.NewNull()"
	case bool:
		if v {
			return "jsvalue.NewBool(true)"
		}
		return "jsvalue.NewBool(false)"
	case string:
		return "jsvalue.NewString(" + strconv.Quote(v) + ")"
	case int:
		return numberExpr(float64(v))
	case int8:
		return numberExpr(float64(v))
	case int16:
		return numberExpr(float64(v))
	case int32:
		return numberExpr(float64(v))
	case int64:
		return numberExpr(float64(v))
	case uint:
		return numberExpr(float64(v))
	case uint8:
		return numberExpr(float64(v))
	case uint16:
		return numberExpr(float64(v))
	case uint32:
		return numberExpr(float64(v))
	case uint64:
		return numberExpr(float64(v))
	case float32:
		return numberExpr(float64(v))
	case float64:
		return numberExpr(v)
	case []any:
		if len(v) == 0 {
			return "jsvalue.NewArray()"
		}
		parts := make([]string, len(v))
		for i, elem := range v {
			parts[i] = valueExpr(elem)
		}
		return "jsvalue.NewArray(" + strings.Join(parts, ", ") + ")"
	case map[string]any:
		if len(v) == 0 {
			return "jsvalue.ObjectFrom()"
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys)*2)
		for _, key := range keys {
			parts = append(parts, strconv.Quote(key), valueExpr(v[key]))
		}
		return "jsvalue.ObjectFrom(" + strings.Join(parts, ", ") + ")"
	default:
		return "jsvalue.NewNull()"
	}
}

func numberExpr(v float64) string {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "jsvalue.NewNull()"
	}
	return "jsvalue.NewNumber(" + strconv.FormatFloat(v, 'g', -1, 64) + ")"
}

func isExportableIdentifier(s string) bool {
	if s == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 0 {
		return false
	}
	if r != '_' && r != '$' && !unicode.IsLetter(r) {
		return false
	}
	for _, r := range s[size:] {
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "Value"
	}
	return b.String()
}

func capitalize(s string) string {
	if s == "" {
		return "Value"
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 0 {
		return "Value"
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func makeUnique(name string, used map[string]int) string {
	if name == "" {
		name = "Value"
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	return fmt.Sprintf("%s_%d", name, used[name])
}
