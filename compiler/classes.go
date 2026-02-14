package compiler

import (
	"go/ast"
	"go/token"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func (t *Transformer) transformClassDecl(node *sitter.Node) []ast.Decl {
	nameNode := node.ChildByFieldName("name")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil {
		return nil
	}

	className := capitalize(nameNode.Utf8Text(t.source))
	recv := receiverName(className)

	var decls []ast.Decl
	var structFields []*ast.Field
	var ctorParams *ast.FieldList
	var ctorBody *ast.BlockStmt
	var methods []*ast.FuncDecl

	// Check for extends
	var parentName string
	superclassNode := node.ChildByFieldName("superclass")
	if superclassNode != nil {
		parentName = capitalize(superclassNode.Utf8Text(t.source))
	}

	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			member := bodyNode.NamedChild(i)
			switch member.Kind() {
			case "public_field_definition", "property_definition":
				f := t.transformClassField(member)
				if f != nil {
					structFields = append(structFields, f)
				}

			case "method_definition":
				nameN := member.ChildByFieldName("name")
				if nameN == nil {
					continue
				}
				mName := nameN.Utf8Text(t.source)

				if mName == "constructor" {
					ctorParams, ctorBody = t.transformConstructor(member, className, recv)
				} else {
					m := t.transformMethod(member, className, recv)
					if m != nil {
						methods = append(methods, m)
					}
				}
			}
		}
	}

	// Add parent embed if extends
	if parentName != "" {
		embedField := &ast.Field{Type: ident(parentName)}
		structFields = append([]*ast.Field{embedField}, structFields...)
	}

	// Struct type declaration
	structDecl := typeDecl(className, &ast.StructType{
		Fields: fieldList(structFields...),
	})
	decls = append(decls, structDecl)

	// Constructor → NewClassName function
	if ctorBody != nil {
		if ctorParams == nil {
			ctorParams = fieldList()
		}
		ctorFn := funcDecl(
			"New"+className,
			ctorParams,
			fieldList(field("", ptrType(ident(className)))),
			ctorBody,
		)
		decls = append(decls, ctorFn)
	}

	// Methods
	for _, m := range methods {
		decls = append(decls, m)
	}

	return decls
}

func (t *Transformer) transformClassField(node *sitter.Node) *ast.Field {
	nameNode := node.ChildByFieldName("name")
	typeNode := node.ChildByFieldName("type")

	if nameNode == nil {
		return nil
	}

	name := capitalize(nameNode.Utf8Text(t.source))

	var typ ast.Expr = ident("any")
	if typeNode != nil {
		mapped := t.getTypeAnnotation(typeNode)
		if mapped != nil {
			typ = mapped
		}
	}

	origName := nameNode.Utf8Text(t.source)
	tag := "`json:\"" + origName + "\"`"

	return &ast.Field{
		Names: []*ast.Ident{ident(name)},
		Type:  typ,
		Tag:   basicLit(token.STRING, tag),
	}
}

func (t *Transformer) transformConstructor(node *sitter.Node, className, recv string) (*ast.FieldList, *ast.BlockStmt) {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")

	params := t.transformParams(paramsNode)

	// Build constructor body:
	// recv := &ClassName{}
	// <translated body with this.x → recv.X>
	// return recv
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(recv)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{addrOf(compositeLit(ident(className)))},
		},
	}

	if bodyNode != nil {
		block := t.transformBlock(bodyNode)
		for _, stmt := range block.List {
			stmts = append(stmts, t.rewriteThis(stmt, recv))
		}
	}

	stmts = append(stmts, returnStmt(ident(recv)))

	return params, &ast.BlockStmt{List: stmts}
}

func (t *Transformer) transformMethod(node *sitter.Node, className, recv string) *ast.FuncDecl {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	returnTypeNode := node.ChildByFieldName("return_type")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil {
		return nil
	}

	mName := capitalize(nameNode.Utf8Text(t.source))
	params := t.transformParams(paramsNode)

	var results *ast.FieldList
	if returnTypeNode != nil {
		retType := t.getTypeAnnotation(returnTypeNode)
		if retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
		// Rewrite this.x → recv.X
		rewritten := make([]ast.Stmt, len(body.List))
		for i, stmt := range body.List {
			rewritten[i] = t.rewriteThis(stmt, recv)
		}
		body.List = rewritten
	} else {
		body = blockStmt()
	}

	return methodDecl(recv, mName, ptrType(ident(className)), params, results, body)
}

// rewriteThis replaces `this` identifiers with the receiver variable name.
// This is a simplified rewrite that handles common patterns.
func (t *Transformer) rewriteThis(stmt ast.Stmt, recv string) ast.Stmt {
	ast.Inspect(stmt, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name == "this" {
				x.Name = recv
			}
		}
		return true
	})
	return stmt
}
