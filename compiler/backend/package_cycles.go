package backend

import (
	"go/ast"
	"go/token"
)

// BreakPackageInitCycles detects package-level var initializer cycles across
// multiple generated files in the same Go package and splits participating vars
// into forward declarations plus init() assignments.
func BreakPackageInitCycles(files map[string]*ast.File) {
	type varInfo struct {
		file  *ast.File
		name  string
		typ   ast.Expr
		value ast.Expr
	}

	varInfos := make(map[string]varInfo)
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					continue
				}
				info := varInfo{file: file, name: vs.Names[0].Name, typ: vs.Type}
				if len(vs.Values) > 0 {
					info.value = vs.Values[0]
				}
				varInfos[info.name] = info
			}
		}
	}

	deps := make(map[string]map[string]bool)
	for name, info := range varInfos {
		if info.value == nil {
			continue
		}
		for ref := range exprReferencedIdents(info.value) {
			if _, ok := varInfos[ref]; ok {
				if deps[name] == nil {
					deps[name] = make(map[string]bool)
				}
				deps[name][ref] = true
			}
		}
	}

	cyclic := make(map[string]bool)
	visiting := make(map[string]bool)
	memo := make(map[string]bool)
	var inCycle func(string) bool
	inCycle = func(name string) bool {
		if v, ok := memo[name]; ok {
			return v
		}
		if visiting[name] {
			return true
		}
		visiting[name] = true
		defer delete(visiting, name)
		for dep := range deps[name] {
			if dep == name || inCycle(dep) {
				memo[name] = true
				return true
			}
		}
		memo[name] = false
		return false
	}
	for name := range varInfos {
		if inCycle(name) {
			cyclic[name] = true
		}
	}
	if len(cyclic) == 0 {
		return
	}

	for _, file := range files {
		var newDecls []ast.Decl
		var initStmts []ast.Stmt
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				newDecls = append(newDecls, decl)
				continue
			}

			var keptSpecs []ast.Spec
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					continue
				}
				name := vs.Names[0].Name
				if !cyclic[name] {
					keptSpecs = append(keptSpecs, spec)
					continue
				}
				typ := vs.Type
				if typ == nil {
					typ = jsValuePtrType()
				}
				newDecls = append(newDecls, varDecl(name, typ, nil))
				if len(vs.Values) > 0 {
					initStmts = append(initStmts, assignStmt(
						[]ast.Expr{goIdent(name)},
						[]ast.Expr{vs.Values[0]},
					))
				}
			}
			if len(keptSpecs) > 0 {
				newDecls = append(newDecls, &ast.GenDecl{Tok: token.VAR, Specs: keptSpecs})
			}
		}
		if len(initStmts) > 0 {
			newDecls = append(newDecls, funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: initStmts}))
		}
		file.Decls = newDecls
		if len(initStmts) > 0 {
			ensureAliasedImport(file, "github.com/nnstd/gun/runtime/builtin", "jsvalue")
		}
	}
}

func ensureAliasedImport(file *ast.File, pkg, alias string) {
	if file == nil {
		return
	}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for _, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil {
				continue
			}
			if is.Path.Value == `"`+pkg+`"` {
				if alias != "" && is.Name == nil {
					is.Name = goIdent(alias)
				}
				return
			}
		}
		gd.Specs = append(gd.Specs, importSpecAlias(pkg, alias))
		if len(gd.Specs) > 1 && gd.Lparen == token.NoPos {
			gd.Lparen = 1
		}
		return
	}
	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{importSpecAlias(pkg, alias)}}
	file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
}
