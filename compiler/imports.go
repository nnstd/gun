package compiler

import (
	"go/ast"
	"path"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// resolvedImport describes how a TS imported name maps to Go.
type resolvedImport struct {
	goImportPath string // Go import path (e.g. "os", "path/filepath")
	goPkgName    string // Go package identifier (e.g. "os", "filepath")
	goSymbol     string // Go symbol name (e.g. "ReadFile"); empty for namespace imports
}

// moduleMapping maps a TS module to a Go package.
type moduleMapping struct {
	goPath string // Go import path
	goName string // Go package name used in code
}

// knownModules maps Node.js / TS built-in module names to Go packages.
var knownModules = map[string]moduleMapping{
	"fs":            {goPath: "github.com/nnstd/gun/runtime/fs", goName: "fs"},
	"path":          {goPath: "github.com/nnstd/gun/runtime/path", goName: "nodepath"},
	"os":            {goPath: "github.com/nnstd/gun/runtime/os", goName: "nodeos"},
	"http":          {goPath: "net/http", goName: "http"},
	"https":         {goPath: "net/http", goName: "http"},
	"url":           {goPath: "net/url", goName: "url"},
	"util":          {goPath: "fmt", goName: "fmt"},
	"events":        {goPath: "sync", goName: "sync"},
	"stream":        {goPath: "io", goName: "io"},
	"buffer":        {goPath: "bytes", goName: "bytes"},
	"crypto":        {goPath: "crypto", goName: "crypto"},
	"child_process": {goPath: "os/exec", goName: "exec"},
	"assert":        {goPath: "testing", goName: "testing"},
	"hono":          {goPath: "github.com/nnstd/gun/runtime/hono", goName: "hono"},
}

// knownSymbols maps (module, symbol) pairs to specific Go translations.
// If a symbol isn't listed here, it gets capitalized and called on the package.
var knownSymbols = map[string]map[string]resolvedImport{
	"fs": {
		"readFile":       {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "ReadFileSync"},
		"readFileSync":   {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "ReadFileSync"},
		"writeFile":      {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "WriteFileSync"},
		"writeFileSync":  {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "WriteFileSync"},
		"existsSync":     {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "ExistsSync"},
		"mkdirSync":      {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "MkdirSync"},
		"readdirSync":    {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "ReaddirSync"},
		"unlinkSync":     {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "UnlinkSync"},
		"statSync":       {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "StatSync"},
		"rmdirSync":      {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "RmdirSync"},
		"appendFileSync": {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "AppendFileSync"},
		"copyFileSync":   {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "CopyFileSync"},
		"renameSync":     {goImportPath: "github.com/nnstd/gun/runtime/fs", goPkgName: "fs", goSymbol: "RenameSync"},
	},
	"path": {
		"join":       {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Join"},
		"resolve":    {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Resolve"},
		"basename":   {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Basename"},
		"dirname":    {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Dirname"},
		"extname":    {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Extname"},
		"relative":   {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Relative"},
		"isAbsolute": {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "IsAbsolute"},
		"normalize":  {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Normalize"},
		"parse":      {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Parse"},
		"sep":        {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Sep"},
		"delimiter":  {goImportPath: "github.com/nnstd/gun/runtime/path", goPkgName: "nodepath", goSymbol: "Delimiter"},
	},
	"os": {
		"homedir":  {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Homedir"},
		"tmpdir":   {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Tmpdir"},
		"hostname": {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Hostname"},
		"platform": {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Platform"},
		"arch":     {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Arch"},
		"cpus":     {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "Cpus"},
		"EOL":      {goImportPath: "github.com/nnstd/gun/runtime/os", goPkgName: "nodeos", goSymbol: "EOL"},
	},
	"url": {
		"parse":  {goImportPath: "net/url", goPkgName: "url", goSymbol: "Parse"},
		"format": {goImportPath: "net/url", goPkgName: "url", goSymbol: "String"},
	},
	"child_process": {
		"exec":     {goImportPath: "os/exec", goPkgName: "exec", goSymbol: "Command"},
		"execSync": {goImportPath: "os/exec", goPkgName: "exec", goSymbol: "Command"},
		"spawn":    {goImportPath: "os/exec", goPkgName: "exec", goSymbol: "Command"},
	},
	"hono": {
		"Hono": {goImportPath: "github.com/nnstd/gun/runtime/hono", goPkgName: "hono", goSymbol: "Hono"},
	},
}

func (t *Transformer) transformImport(node *sitter.Node) {
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return
	}

	// Extract module path from the string literal
	modulePath := sourceNode.Utf8Text(t.source)
	modulePath = strings.Trim(modulePath, "'\"")

	// Check for type-only import
	typeOnly := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "type" {
			typeOnly = true
			break
		}
	}

	// Find the import_clause
	var clauseNode *sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "import_clause" {
			clauseNode = child
			break
		}
	}
	if clauseNode == nil {
		return
	}

	// Resolve the Go module mapping
	mod, isKnown := knownModules[modulePath]
	if !isKnown {
		mod = t.resolveModulePath(modulePath)
	}

	// Walk the import clause to extract names
	for i := uint(0); i < clauseNode.NamedChildCount(); i++ {
		child := clauseNode.NamedChild(i)
		switch child.Kind() {
		case "named_imports":
			t.processNamedImports(child, modulePath, mod, typeOnly)

		case "namespace_import":
			// import * as X from "mod" → X becomes a package alias
			var alias string
			for j := uint(0); j < child.NamedChildCount(); j++ {
				if child.NamedChild(j).Kind() == "identifier" {
					alias = child.NamedChild(j).Utf8Text(t.source)
				}
			}
			if alias != "" {
				t.importedNames[alias] = resolvedImport{
					goImportPath: mod.goPath,
					goPkgName:    mod.goName,
				}
			}

		case "identifier":
			// import X from "mod" → default import
			localName := child.Utf8Text(t.source)
			t.importedNames[localName] = resolvedImport{
				goImportPath: mod.goPath,
				goPkgName:    mod.goName,
			}
		}
	}
}

func (t *Transformer) processNamedImports(node *sitter.Node, modulePath string, mod moduleMapping, typeOnly bool) {
	symTable := knownSymbols[modulePath]

	for i := uint(0); i < node.NamedChildCount(); i++ {
		spec := node.NamedChild(i)
		if spec.Kind() != "import_specifier" {
			continue
		}

		nameNode := spec.ChildByFieldName("name")
		aliasNode := spec.ChildByFieldName("alias")
		if nameNode == nil {
			continue
		}

		origName := nameNode.Utf8Text(t.source)
		localName := origName
		if aliasNode != nil {
			localName = aliasNode.Utf8Text(t.source)
		}

		// Check for a specific symbol mapping
		if symTable != nil {
			if sym, ok := symTable[origName]; ok {
				t.importedNames[localName] = sym
				continue
			}
		}

		// Default: capitalize the symbol and use the module's Go package
		t.importedNames[localName] = resolvedImport{
			goImportPath: mod.goPath,
			goPkgName:    mod.goName,
			goSymbol:     capitalize(origName),
		}

		// For type-only imports, also register the capitalized form
		if typeOnly {
			t.importedNames[capitalize(localName)] = resolvedImport{
				goImportPath: mod.goPath,
				goPkgName:    mod.goName,
				goSymbol:     capitalize(origName),
			}
		}
	}
}

// resolveModulePath converts a TS module path to a Go import path.
func (t *Transformer) resolveModulePath(modulePath string) moduleMapping {
	// Relative import: ./foo or ../bar
	if strings.HasPrefix(modulePath, ".") {
		// Strip file extension
		clean := strings.TrimSuffix(modulePath, ".ts")
		clean = strings.TrimSuffix(clean, ".js")
		// Use the last path segment as the Go package name
		pkgName := path.Base(clean)
		// Build Go import path: module/relative/path
		modName := t.moduleName
		if modName == "" {
			modName = "main"
		}
		goPath := modName + "/" + strings.TrimPrefix(clean, "./")
		goPath = strings.TrimPrefix(goPath, "../")
		return moduleMapping{goPath: goPath, goName: pkgName}
	}

	// Scoped package: @scope/pkg → scope_pkg
	if strings.HasPrefix(modulePath, "@") {
		parts := strings.SplitN(modulePath, "/", 2)
		if len(parts) == 2 {
			pkgName := strings.TrimPrefix(parts[0], "@") + "_" + parts[1]
			return moduleMapping{goPath: modulePath, goName: pkgName}
		}
	}

	// Third-party: use module name as package name
	// Strip any subpath for the package name
	pkgName := path.Base(modulePath)
	return moduleMapping{goPath: modulePath, goName: pkgName}
}

// resolveIdentifier checks if a name was imported from a TS module and returns
// the appropriate Go expression. Falls back to builtin identifier mapping.
func (t *Transformer) resolveIdentifier(name string) ast.Expr {
	if imp, ok := t.importedNames[name]; ok {
		if imp.goImportPath != "" {
			// Use aliased import when package name differs from import path's last segment
			if imp.goPkgName != "" && imp.goPkgName != filepath.Base(imp.goImportPath) {
				t.addAliasedImport(imp.goImportPath, imp.goPkgName)
			} else {
				t.addImport(imp.goImportPath)
			}
		}
		// Namespace import (import * as X) — return the package ident
		if imp.goSymbol == "" {
			return ident(imp.goPkgName)
		}
		// Named import — return pkg.Symbol
		return selectorExpr(ident(imp.goPkgName), imp.goSymbol)
	}
	return mapIdentifier(name, t.addImport)
}
