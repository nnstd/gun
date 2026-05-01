package jsonmodule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"sort"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/nnstd/gun/compiler/context"
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
	return &Module{Source: source, Value: value, Exports: topLevelExports(value)}, nil
}

func Compile(source []byte, pkgName, defaultName string, namedAliases map[string]string) ([]byte, error) {
	return CompileWithOptLevel(source, pkgName, defaultName, namedAliases, context.O0)
}

func CompileWithOptLevel(source []byte, pkgName, defaultName string, namedAliases map[string]string, optLevel context.OptLevel) ([]byte, error) {
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
	c.addJSValueVars(defaultName, mod.Exports, namedAliases)
	if len(source) <= inlineValueExprMaxBytes {
		c.addInit(c.smallInitStmts(defaultName, mod.Value, mod.Exports, namedAliases))
	} else if optLevel >= context.O2 {
		c.addImport("github.com/nnstd/gun/runtime/jsonx", "")
		if !c.addTypedJSON(mod.Value) {
			c.addImport("github.com/nnstd/gun/runtime/module", "")
			c.addInit(c.largeAnyInitStmts(defaultName, source, mod.Exports, namedAliases))
		} else {
			c.addInit(c.largeTypedInitStmts(defaultName, source, mod.Exports, namedAliases))
		}
	} else {
		c.addImport("github.com/nnstd/gun/runtime/jsonx", "")
		c.addImport("github.com/nnstd/gun/runtime/module", "")
		c.addInit(c.largeAnyInitStmts(defaultName, source, mod.Exports, namedAliases))
	}
	c.finalizeImports()
	return c.format()
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

type compiler struct {
	file    *ast.File
	imports map[string]string
	root    *schema
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
	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: specs}
	c.file.Decls = append([]ast.Decl{importDecl}, c.file.Decls...)
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

func (c *compiler) largeAnyInitStmts(defaultName string, source []byte, exports []Export, namedAliases map[string]string) []ast.Stmt {
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent("data")},
			Type:  ast.NewIdent("any"),
		}}}},
		assign(ident("_"), call(sel("jsonx", "Unmarshal"), call(byteSliceType(), stringLit(string(source))), unary(token.AND, ident("data")))),
		assign(ident(defaultName), call(sel("module", "DataToJSValue"), ident("data"))),
	}
	return append(stmts, exportStmts(defaultName, exports, namedAliases)...)
}

func (c *compiler) largeTypedInitStmts(defaultName string, source []byte, exports []Export, namedAliases map[string]string) []ast.Stmt {
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent("data")},
			Type:  typeExpr(c.root),
		}}}},
		assign(ident("_"), call(sel("jsonx", "Unmarshal"), call(byteSliceType(), stringLit(string(source))), unary(token.AND, ident("data")))),
		assign(ident(defaultName), converterExpr(c.root, ident("data"))),
	}
	return append(stmts, exportStmts(defaultName, exports, namedAliases)...)
}

func (c *compiler) addTypedJSON(value any) bool {
	s := newSchemaBuilder()
	root, ok := s.infer(value, "jsonRoot")
	if !ok || root.kind == schemaAny {
		return false
	}
	c.root = root
	if schemaContainsAny(root) {
		c.addImport("github.com/nnstd/gun/runtime/module", "")
	}
	for _, decl := range s.decls {
		c.file.Decls = append(c.file.Decls, decl)
	}
	if schemaContainsArray(root) {
		c.file.Decls = append(c.file.Decls, jsonArrayConverterDecl())
	}
	for _, decl := range s.converterDecls() {
		c.file.Decls = append(c.file.Decls, decl)
	}
	return true
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

type schemaKind int

const (
	schemaAny schemaKind = iota
	schemaBool
	schemaString
	schemaNumber
	schemaArray
	schemaObject
)

type schema struct {
	kind   schemaKind
	name   string
	elem   *schema
	fields []schemaField
}

type schemaField struct {
	key  string
	name string
	sch  *schema
}

type schemaBuilder struct {
	decls     []*ast.GenDecl
	objects   []*schema
	usedTypes map[string]int
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{usedTypes: map[string]int{}}
}

func (b *schemaBuilder) infer(value any, name string) (*schema, bool) {
	switch v := value.(type) {
	case nil:
		return &schema{kind: schemaAny}, true
	case bool:
		return &schema{kind: schemaBool}, true
	case string:
		return &schema{kind: schemaString}, true
	case json.Number:
		return &schema{kind: schemaNumber}, true
	case []any:
		if len(v) == 0 {
			return &schema{kind: schemaArray, elem: &schema{kind: schemaAny}}, true
		}
		elem, ok := b.infer(v[0], name+"Item")
		if !ok {
			return &schema{kind: schemaAny}, true
		}
		for _, item := range v[1:] {
			if !schemaMatchesValue(elem, item) {
				return &schema{kind: schemaArray, elem: &schema{kind: schemaAny}}, true
			}
		}
		return &schema{kind: schemaArray, elem: elem}, true
	case map[string]any:
		s := &schema{kind: schemaObject, name: b.typeName(name)}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		used := map[string]int{}
		fields := make([]schemaField, 0, len(keys))
		for _, key := range keys {
			fieldSchema, ok := b.infer(v[key], s.name+capitalize(sanitize(key)))
			if !ok {
				fieldSchema = &schema{kind: schemaAny}
			}
			fields = append(fields, schemaField{
				key:  key,
				name: makeFieldName(key, used),
				sch:  fieldSchema,
			})
		}
		s.fields = fields
		b.objects = append(b.objects, s)
		b.decls = append(b.decls, b.objectDecl(s))
		return s, true
	default:
		return &schema{kind: schemaAny}, true
	}
}

func (b *schemaBuilder) typeName(base string) string {
	if base == "" {
		base = "jsonValue"
	}
	return makeUnique(base, b.usedTypes)
}

func schemaMatchesValue(s *schema, value any) bool {
	switch s.kind {
	case schemaAny:
		return true
	case schemaBool:
		_, ok := value.(bool)
		return ok
	case schemaString:
		_, ok := value.(string)
		return ok
	case schemaNumber:
		_, ok := value.(json.Number)
		return ok
	case schemaArray:
		items, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if !schemaMatchesValue(s.elem, item) {
				return false
			}
		}
		return true
	case schemaObject:
		obj, ok := value.(map[string]any)
		if !ok || len(obj) != len(s.fields) {
			return false
		}
		for _, field := range s.fields {
			item, ok := obj[field.key]
			if !ok || !schemaMatchesValue(field.sch, item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func schemaContainsArray(s *schema) bool {
	if s == nil {
		return false
	}
	switch s.kind {
	case schemaArray:
		return true
	case schemaObject:
		for _, field := range s.fields {
			if schemaContainsArray(field.sch) {
				return true
			}
		}
	}
	return false
}

func schemaContainsAny(s *schema) bool {
	if s == nil {
		return false
	}
	switch s.kind {
	case schemaAny:
		return true
	case schemaArray:
		return schemaContainsAny(s.elem)
	case schemaObject:
		for _, field := range s.fields {
			if schemaContainsAny(field.sch) {
				return true
			}
		}
	}
	return false
}

func (b *schemaBuilder) objectDecl(s *schema) *ast.GenDecl {
	fields := make([]*ast.Field, 0, len(s.fields))
	for _, field := range s.fields {
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(field.name)},
			Type:  typeExpr(field.sch),
			Tag:   stringLit("json:" + strconv.Quote(field.key)),
		})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(s.name),
		Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
	}}}
}

func (b *schemaBuilder) converterDecls() []*ast.FuncDecl {
	objects := append([]*schema(nil), b.objects...)
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].name < objects[j].name
	})
	decls := make([]*ast.FuncDecl, 0, len(objects))
	for _, obj := range objects {
		decls = append(decls, b.objectConverterDecl(obj))
	}
	return decls
}

func (b *schemaBuilder) objectConverterDecl(s *schema) *ast.FuncDecl {
	args := make([]ast.Expr, 0, len(s.fields)*2)
	for _, field := range s.fields {
		args = append(args, stringLit(field.key), converterExpr(field.sch, sel("v", field.name)))
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(s.name + "ToJSValue"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("v")},
				Type:  ast.NewIdent(s.name),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(sel("jsvalue", "JSValue"))}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{call(sel("jsvalue", "ObjectFrom"), args...)}},
		}},
	}
}

func jsonArrayConverterDecl() *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent("jsonArrayToJSValue"),
		Type: &ast.FuncType{
			TypeParams: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("T")},
				Type:  ast.NewIdent("any"),
			}}},
			Params: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("items")},
					Type:  &ast.ArrayType{Elt: ast.NewIdent("T")},
				},
				{
					Names: []*ast.Ident{ast.NewIdent("convert")},
					Type: &ast.FuncType{
						Params:  &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("T")}}},
						Results: &ast.FieldList{List: []*ast.Field{{Type: star(sel("jsvalue", "JSValue"))}}},
					},
				},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(sel("jsvalue", "JSValue"))}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			define(ident("values"), call(ident("make"), &ast.ArrayType{Elt: star(sel("jsvalue", "JSValue"))}, call(ident("len"), ident("items")))),
			&ast.RangeStmt{
				Key:   ast.NewIdent("i"),
				Value: ast.NewIdent("item"),
				Tok:   token.DEFINE,
				X:     ast.NewIdent("items"),
				Body: &ast.BlockStmt{List: []ast.Stmt{
					assign(index(ident("values"), ident("i")), call(ident("convert"), ident("item"))),
				}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{callEllipsis(sel("jsvalue", "NewArray"), ident("values"))}},
		}},
	}
}

func typeExpr(s *schema) ast.Expr {
	switch s.kind {
	case schemaBool:
		return ast.NewIdent("bool")
	case schemaString:
		return ast.NewIdent("string")
	case schemaNumber:
		return ast.NewIdent("float64")
	case schemaArray:
		return &ast.ArrayType{Elt: typeExpr(s.elem)}
	case schemaObject:
		return ast.NewIdent(s.name)
	default:
		return ast.NewIdent("any")
	}
}

func converterExpr(s *schema, expr ast.Expr) ast.Expr {
	switch s.kind {
	case schemaBool:
		return call(sel("jsvalue", "NewBool"), expr)
	case schemaString:
		return call(sel("jsvalue", "NewString"), expr)
	case schemaNumber:
		return call(sel("jsvalue", "NewNumber"), expr)
	case schemaArray:
		return call(ident("jsonArrayToJSValue"), expr, converterFunc(s.elem))
	case schemaObject:
		return call(ident(s.name+"ToJSValue"), expr)
	default:
		return call(sel("module", "DataToJSValue"), expr)
	}
}

func converterFunc(s *schema) ast.Expr {
	switch s.kind {
	case schemaBool:
		return funcLit(ast.NewIdent("bool"), call(sel("jsvalue", "NewBool"), ident("v")))
	case schemaString:
		return funcLit(ast.NewIdent("string"), call(sel("jsvalue", "NewString"), ident("v")))
	case schemaNumber:
		return funcLit(ast.NewIdent("float64"), call(sel("jsvalue", "NewNumber"), ident("v")))
	case schemaArray:
		return funcLit(typeExpr(s), converterExpr(s, ident("v")))
	case schemaObject:
		return ast.NewIdent(s.name + "ToJSValue")
	default:
		return funcLit(ast.NewIdent("any"), call(sel("module", "DataToJSValue"), ident("v")))
	}
}

func funcLit(paramType ast.Expr, result ast.Expr) ast.Expr {
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("v")}, Type: paramType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(sel("jsvalue", "JSValue"))}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{result}}}},
	}
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
	case json.Number:
		return call(sel("jsvalue", "NewNumber"), rawLit(string(v)))
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

func makeFieldName(key string, used map[string]int) string {
	name := capitalize(sanitize(key))
	r, _ := utf8.DecodeRuneInString(name)
	if r != '_' && !unicode.IsLetter(r) {
		name = "Field" + name
	}
	return makeUnique(name, used)
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

func define(lhs, rhs ast.Expr) ast.Stmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.DEFINE, Rhs: []ast.Expr{rhs}}
}

func callEllipsis(fn ast.Expr, arg ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: fn, Args: []ast.Expr{arg}, Ellipsis: token.Pos(1)}
}

func index(x, idx ast.Expr) ast.Expr {
	return &ast.IndexExpr{X: x, Index: idx}
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
