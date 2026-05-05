package backend

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler/symbol"
)

// initFuncNameFromSource derives a unique Go-friendly init function name from
// the source file path. It includes the full relative path after node_modules/
// to ensure uniqueness within a Go package that contains multiple JS files.
// For "node_modules/foo-bar/src/baz.js" it returns "initFoobarSrcBaz".
// For empty sourcePath it returns "".
func initFuncNameFromSource(sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	if idx := strings.Index(sourcePath, "/node_modules/"); idx >= 0 {
		after := sourcePath[idx+len("/node_modules/"):]
		// Split: pkg/subpath/file.ext
		// For scoped packages: @scope/pkg/subpath/file.ext
		var pkg, rest string
		if strings.HasPrefix(after, "@") {
			// @scope/pkg/rest
			parts := strings.SplitN(after, "/", 3)
			if len(parts) < 2 {
				return ""
			}
			pkg = parts[0] + "/" + parts[1]
			if len(parts) >= 3 {
				rest = parts[2]
			}
		} else {
			parts := strings.SplitN(after, "/", 2)
			pkg = parts[0]
			if len(parts) >= 2 {
				rest = parts[1]
			}
		}
		pkg = sanitizeInitName(pkg)
		// Build name from all remaining path segments
		var name string
		if rest != "" {
			segments := strings.Split(rest, "/")
			for i, seg := range segments {
				// Strip extension from last segment BEFORE sanitization
				if i == len(segments)-1 {
					if dotIdx := strings.LastIndex(seg, "."); dotIdx > 0 {
						seg = seg[:dotIdx]
					}
				}
				seg = sanitizeInitName(seg)
				name += symbol.Capitalize(seg)
			}
		}
		return "init" + symbol.Capitalize(pkg) + name
	}
	// Non-node_modules path: use parent dir + filename
	base := filepath.Base(sourcePath)
	// Strip extension before sanitization
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	dir := filepath.Dir(sourcePath)
	parent := filepath.Base(dir)
	parent = sanitizeInitName(parent)
	base = sanitizeInitName(base)
	return "init" + symbol.Capitalize(parent) + symbol.Capitalize(base)
}

// initStateVarName derives a unique variable name for the ModuleState var
// from the source file path. E.g. "_fileInitEcdsasigformatterSrcBaz".
// Falls back to "_fileInit" for empty sourcePath (shouldn't happen in practice).
func initStateVarName(sourcePath string) string {
	if sourcePath == "" {
		return "_fileInit"
	}
	name := initFuncNameFromSource(sourcePath)
	// Remove "init" prefix and capitalize first letter
	if strings.HasPrefix(name, "init") && len(name) > 4 {
		return "_fileInit" + name[4:]
	}
	return "_fileInit"
}

// sanitizeInitName converts a path segment to a valid Go identifier part.
func sanitizeInitName(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "@", "")
	return s
}

// extractBarePkgName extracts the bare npm package name from a source path
// like "/path/to/node_modules/minecraft-data/index.js" -> "minecraft-data".
// Returns "" if the path doesn't contain node_modules.
func extractBarePkgName(sourcePath string) string {
	idx := strings.Index(sourcePath, "/node_modules/")
	if idx < 0 {
		return ""
	}
	after := sourcePath[idx+len("/node_modules/"):]
	if strings.HasPrefix(after, "@") {
		parts := strings.SplitN(after, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return ""
	}
	parts := strings.SplitN(after, "/", 2)
	return parts[0]
}

// moduleStateInitStmts generates the AST statements for the ModuleState-guarded
// init pattern:
//
//	if initStateName.BeginInit() {
//		defer func() {
//			if r := recover(); r != nil {
//				initStateName.FailInit()
//				panic(r)
//			}
//		}()
//		initFuncName()
//		initStateName.FinishInit()
//	}
func moduleStateInitStmts(initStateName, initFuncName string) []ast.Stmt {
	return []ast.Stmt{
		&ast.IfStmt{
			Cond: callExpr(selectorExpr(goIdent(initStateName), "BeginInit")),
			Body: blockStmt(
				&ast.DeferStmt{
					Call: callExpr(&ast.FuncLit{
						Type: &ast.FuncType{
							Params: &ast.FieldList{},
						},
						Body: blockStmt(
							&ast.IfStmt{
								Init: &ast.AssignStmt{
									Lhs: []ast.Expr{goIdent("r")},
									Tok: token.DEFINE,
									Rhs: []ast.Expr{callExpr(goIdent("recover"))},
								},
								Cond: &ast.BinaryExpr{
									X:  goIdent("r"),
									Op: token.NEQ,
									Y:  goIdent("nil"),
								},
								Body: blockStmt(
									exprStmt(callExpr(selectorExpr(goIdent(initStateName), "FailInit"))),
									exprStmt(callExpr(goIdent("panic"), goIdent("r"))),
								),
							},
						),
					}),
				},
				exprStmt(callExpr(goIdent(initFuncName))),
				exprStmt(callExpr(selectorExpr(goIdent(initStateName), "FinishInit"))),
			),
		},
	}
}