package yamlmodule

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"math"
	"sort"
	"strconv"
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

const inlineValueExprMaxBytes = 32 * 1024

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
	c := &compiler{
		file: &ast.File{
			Name:  ast.NewIdent(pkgName),
			Decls: []ast.Decl{},
		},
		imports: map[string]string{},
	}
	c.addImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	if len(source) <= inlineValueExprMaxBytes {
		c.addJSValueVars(defaultName, mod.Exports, namedAliases)
		c.addInit(c.smallInitStmts(defaultName, mod.Value, mod.Exports, namedAliases))
	} else {
		c.addImport("github.com/goccy/go-yaml", "")
		c.addImport("github.com/nnstd/gun/runtime/module", "")
		c.addJSValueVars(defaultName, mod.Exports, namedAliases)
		c.addInit(c.largeInitStmts(defaultName, source, mod.Exports, namedAliases))
	}
	c.finalizeImports()
	return c.format()
}

type compiler struct {
	file    *ast.File
	imports map[string]string
}

func (c *compiler) addImport(path, alias string) {
	c.imports[path] = alias
}

func (c *compiler) finalizeImports() {
	paths := make([]string, 0, len(c.imports))
	for path := range c.imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	specs := make([]ast.Spec, 0, len(paths))
	for _, path := range paths {
		spec := &ast.ImportSpec{Path: stringLit(path)}
		if alias := c.imports[path]; alias != "" {
			spec.Name = ast.NewIdent(alias)
		}
		specs = append(specs, spec)
	}
	c.file.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: specs}}, c.file.Decls...)
}

func (c *compiler) format() ([]byte, error) {
	var b bytes.Buffer
	if err := format.Node(&b, token.NewFileSet(), c.file); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (c *compiler) addJSValueVars(defaultName string, exports []Export, namedAliases map[string]string) {
	names := []string{defaultName}
	for _, exp := range exports {
		name := exportName(exp, namedAliases)
		if name != defaultName {
			names = append(names, name)
		}
	}
	for _, name := range names {
		c.file.Decls = append(c.file.Decls, &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ast.NewIdent(name)},
				Type:  star(sel("jsvalue", "JSValue")),
			}},
		})
	}
}

func (c *compiler) addInit(stmts []ast.Stmt) {
	c.file.Decls = append(c.file.Decls, &ast.FuncDecl{
		Name: ast.NewIdent("init"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	})
}

func (c *compiler) smallInitStmts(defaultName string, value any, exports []Export, namedAliases map[string]string) []ast.Stmt {
	stmts := []ast.Stmt{assign(ident(defaultName), valueExpr(value))}
	return append(stmts, exportStmts(defaultName, exports, namedAliases)...)
}

func (c *compiler) largeInitStmts(defaultName string, source []byte, exports []Export, namedAliases map[string]string) []ast.Stmt {
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent("data")},
			Type:  ast.NewIdent("any"),
		}}}},
		assign(ident("_"), call(sel("yaml", "Unmarshal"), call(byteSliceType(), stringLit(string(source))), unary(token.AND, ident("data")))),
		assign(ident(defaultName), call(sel("module", "DataToJSValue"), call(sel("module", "NormalizeYAMLValue"), ident("data")))),
	}
	return append(stmts, exportStmts(defaultName, exports, namedAliases)...)
}

func exportStmts(defaultName string, exports []Export, namedAliases map[string]string) []ast.Stmt {
	stmts := make([]ast.Stmt, 0, len(exports))
	for _, exp := range exports {
		name := exportName(exp, namedAliases)
		if name == defaultName {
			continue
		}
		stmts = append(stmts, assign(ident(name), call(sel(defaultName, "Get"), stringLit(exp.OriginalName))))
	}
	return stmts
}

func exportName(exp Export, namedAliases map[string]string) string {
	if name := namedAliases[exp.OriginalName]; name != "" {
		return name
	}
	return exp.GoName
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

func valueExpr(value any) ast.Expr {
	switch v := value.(type) {
	case nil:
		return call(sel("jsvalue", "NewNull"))
	case bool:
		if v {
			return call(sel("jsvalue", "NewBool"), ident("true"))
		}
		return call(sel("jsvalue", "NewBool"), ident("false"))
	case string:
		return call(sel("jsvalue", "NewString"), stringLit(v))
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
		args := make([]ast.Expr, len(v))
		for i, elem := range v {
			args[i] = valueExpr(elem)
		}
		return call(sel("jsvalue", "NewArray"), args...)
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		args := make([]ast.Expr, 0, len(keys)*2)
		for _, key := range keys {
			args = append(args, stringLit(key), valueExpr(v[key]))
		}
		return call(sel("jsvalue", "ObjectFrom"), args...)
	default:
		return call(sel("jsvalue", "NewNull"))
	}
}

func numberExpr(v float64) ast.Expr {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return call(sel("jsvalue", "NewNull"))
	}
	return call(sel("jsvalue", "NewNumber"), rawLit(strconv.FormatFloat(v, 'g', -1, 64)))
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
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "Value"
	}
	return string(out)
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

func ident(name string) ast.Expr {
	return ast.NewIdent(name)
}

func sel(pkg, name string) ast.Expr {
	return &ast.SelectorExpr{X: ast.NewIdent(pkg), Sel: ast.NewIdent(name)}
}

func star(expr ast.Expr) ast.Expr {
	return &ast.StarExpr{X: expr}
}

func call(fn ast.Expr, args ...ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: fn, Args: args}
}

func unary(op token.Token, expr ast.Expr) ast.Expr {
	return &ast.UnaryExpr{Op: op, X: expr}
}

func assign(lhs, rhs ast.Expr) ast.Stmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}
}

func stringLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

func rawLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.FLOAT, Value: s}
}

func byteSliceType() ast.Expr {
	return &ast.ArrayType{Elt: ast.NewIdent("byte")}
}
