package jsonmodule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Export struct {
	OriginalName string
	GoName       string
}

type Module struct {
	Source  []byte
	Exports []Export
}

func Parse(source []byte) (*Module, error) {
	dec := json.NewDecoder(bytes.NewReader(source))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("multiple JSON values")
	}
	return &Module{Source: source, Exports: topLevelExports(value)}, nil
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
	fmt.Fprintf(&b, "\t%s = %s\n", defaultName, valueExpr(modValue(source)))
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

func modValue(source []byte) any {
	dec := json.NewDecoder(bytes.NewReader(source))
	dec.UseNumber()
	var value any
	_ = dec.Decode(&value)
	return value
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
	case json.Number:
		return "jsvalue.NewNumber(" + string(v) + ")"
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
