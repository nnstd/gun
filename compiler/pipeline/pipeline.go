// Package pipeline orchestrates the compiler stages from TypeScript source
// through HIR, MIR, SSA, optimization passes, and backend code generation.
//
// It provides configurable optimization levels (O0, O1, O2) and
// deterministic pass ordering. No business logic — only coordination.
package pipeline

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/nnstd/gun/compiler/backend"
	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/jsonmodule"
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/passes"
	"github.com/nnstd/gun/compiler/ssa"
	"github.com/nnstd/gun/compiler/symbol"
	"github.com/nnstd/gun/compiler/yamlmodule"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Pipeline orchestrates the compilation stages.
type Pipeline struct {
	OptLevel context.OptLevel
	Passes   []passes.Pass
	Ctx      *context.TranspilerContext

	// Hooks for observability (all optional)
	OnHIR func(*hir.Module) // called after HIR construction
	OnMIR func(*mir.Module) // called after MIR lowering
	OnSSA func(*ssa.Module) // called after SSA construction
}

type compileOptions struct {
	crossFileExports []backend.CrossFileExport
	reservedNames    []string
	importNameMap    map[string]string
	exportAliasMap   map[string]string
	localAliasMap    map[symbol.ID]string
	namespaceAlias   string
	namespaceEntries map[string]string
	cpuProfile       *backend.CPUProfileConfig
}

// New creates a pipeline with the given optimization level and default passes.
// The context is empty — call NewWithContext to use pre-registered builtins.
func New(level context.OptLevel) *Pipeline {
	return NewWithContext(level, context.New())
}

// NewWithContext creates a pipeline with a pre-configured TranspilerContext.
func NewWithContext(level context.OptLevel, ctx *context.TranspilerContext) *Pipeline {
	p := &Pipeline{
		OptLevel: level,
		Ctx:      ctx,
	}

	switch level {
	case context.O1:
		p.Passes = []passes.Pass{
			passes.ConstFold{},
		}
	case context.O2:
		p.Passes = []passes.Pass{
			passes.ConstFold{},
			passes.DCE{},
		}
	}

	return p
}

// CompileTree compiles a parsed tree-sitter CST into Go source code.
// This is the full pipeline: CST → HIR → MIR → SSA → Passes → De-SSA → Backend → Codegen.
// moduleName is the Go module name for resolving relative imports.
func (p *Pipeline) CompileTree(root *sitter.Node, source []byte, pkgName, moduleName string, samePackageImports bool) ([]byte, error) {
	return p.CompileTreeWithPath(root, source, pkgName, moduleName, "", samePackageImports)
}

func (p *Pipeline) CompileTreeWithPath(root *sitter.Node, source []byte, pkgName, moduleName, sourcePath string, samePackageImports bool) ([]byte, error) {
	return p.CompileTreeWithPathOptions(root, source, pkgName, moduleName, sourcePath, samePackageImports, nil)
}

func (p *Pipeline) CompileTreeWithPathOptions(root *sitter.Node, source []byte, pkgName, moduleName, sourcePath string, samePackageImports bool, cpuProfile *backend.CPUProfileConfig) ([]byte, error) {
	// Stage 1: Build HIR
	hirMod := hir.BuildModuleWithPath(root, source, pkgName, sourcePath)
	if p.OnHIR != nil {
		p.OnHIR(hirMod)
	}
	opts := compileOptions{}
	opts.cpuProfile = cpuProfile
	return p.compileHIRModule(hirMod, moduleName, samePackageImports, opts)
}

// CompileHIR compiles from an already-built HIR module.
func (p *Pipeline) CompileHIR(hirMod *hir.Module, moduleName string, samePackageImports bool) ([]byte, error) {
	return p.compileHIRModule(hirMod, moduleName, samePackageImports, compileOptions{})
}

// CompileHIRWithExports compiles an already-built HIR module while preserving
// cross-file export knowledge for same-package compatibility entrypoints.
func (p *Pipeline) CompileHIRWithExports(hirMod *hir.Module, moduleName string, samePackageImports bool, crossFileExports []backend.CrossFileExport, reservedNames []string, importNameMap map[string]string, exportAliasMap map[string]string, localAliasMap map[symbol.ID]string, namespaceAlias string, namespaceEntries map[string]string) ([]byte, error) {
	return p.compileHIRModule(hirMod, moduleName, samePackageImports, compileOptions{
		crossFileExports: crossFileExports,
		reservedNames:    reservedNames,
		importNameMap:    importNameMap,
		exportAliasMap:   exportAliasMap,
		localAliasMap:    localAliasMap,
		namespaceAlias:   namespaceAlias,
		namespaceEntries: namespaceEntries,
	})
}

func (p *Pipeline) compileHIRModule(hirMod *hir.Module, moduleName string, samePackageImports bool, opts compileOptions) ([]byte, error) {
	if err := hir.AsyncPipelinePhase1Error(hirMod); err != nil {
		return nil, err
	}

	mirMod := mir.Lower(hirMod)
	if p.OnMIR != nil {
		p.OnMIR(mirMod)
	}

	if p.OptLevel > context.O0 && len(p.Passes) > 0 {
		ssaMod := ssa.Build(mirMod)
		if p.OnSSA != nil {
			p.OnSSA(ssaMod)
		}
		for _, pass := range p.Passes {
			if err := pass.Run(ssaMod); err != nil {
				return nil, fmt.Errorf("pass %s: %w", pass.Name(), err)
			}
		}
		_ = ssa.DeSSA(ssaMod)
	}

	if hirMod.SynthesizeDefault == "" && !samePackageImports {
		synthesizeDefaultIfNeeded(hirMod)
	}

	var goFile *ast.File
	if len(opts.crossFileExports) == 0 && len(opts.reservedNames) == 0 && len(opts.importNameMap) == 0 && len(opts.exportAliasMap) == 0 && len(opts.localAliasMap) == 0 && opts.namespaceAlias == "" && len(opts.namespaceEntries) == 0 {
		goFile = backend.LowerWithCPUProfile(hirMod, p.Ctx, moduleName, samePackageImports, opts.cpuProfile, p.OptLevel)
	} else {
		goFile = backend.LowerWithExportsAndCPUProfile(hirMod, p.Ctx, moduleName, samePackageImports, opts.crossFileExports, opts.reservedNames, opts.importNameMap, opts.exportAliasMap, opts.localAliasMap, opts.namespaceAlias, opts.namespaceEntries, opts.cpuProfile, p.OptLevel, true)
	}
	return backend.GenerateWithSource(goFile, hirMod.SourcePath, hirMod.SourceSize)
}

// CompilePackage compiles multiple TypeScript files that belong to the same Go package.
// It scans all files for exports first, then compiles each with cross-file knowledge.
func (p *Pipeline) CompilePackage(files map[string][]byte, pkgName, moduleName, entryFile string) (map[string][]byte, error) {
	return p.CompilePackageWithOptions(files, pkgName, moduleName, entryFile, nil)
}

func (p *Pipeline) CompilePackageWithOptions(files map[string][]byte, pkgName, moduleName, entryFile string, cpuProfile *backend.CPUProfileConfig) (map[string][]byte, error) {
	// Phase 1: Parse all files into HIR and scan exports
	hirModules := make(map[string]*hir.Module)
	allExports := make(map[string][]backend.CrossFileExport)
	allNames := make(map[string][]string)
	exportAliases := make(map[string]map[string]string)
	localAliases := make(map[string]map[symbol.ID]string)
	namespaceAliases := make(map[string]string)

	for name, source := range files {
		if isDataModule(name) {
			mod, err := parseDataModule(name, source)
			if err != nil {
				return nil, err
			}
			exports := []backend.CrossFileExport{{OriginalName: "default", GoName: "Default", IsJSValue: true}}
			names := []string{"Default"}
			for _, exp := range mod {
				exports = append(exports, backend.CrossFileExport{OriginalName: exp.OriginalName, GoName: exp.GoName, IsJSValue: true})
				names = append(names, exp.GoName)
			}
			allExports[name] = exports
			allNames[name] = names
			continue
		}
		tree, err := parseTypeScript(source)
		if err != nil {
			continue
		}
		hirMod := hir.BuildModuleWithPath(tree.RootNode(), source, pkgName, name)
		tree.Close()
		if err := hir.AsyncPipelinePhase1Error(hirMod); err != nil {
			return nil, err
		}
		hirModules[name] = hirMod
		allExports[name] = backend.ScanHIRExports(hirMod)
		allExports[name] = append(allExports[name], backend.ScanHIRCJSExports(hirMod)...)
		allNames[name] = backend.ScanHIRTopLevelNames(hirMod)
	}

	expandWildcardReexports(hirModules, allExports, files)

	// Phase 1b: Synthesize Default export for modules with named exports
	// but no explicit default (SWC interop). Only add to the entry file.
	hasDefault := false
	for _, exps := range allExports {
		for _, exp := range exps {
			if exp.GoName == "Default" {
				hasDefault = true
			}
		}
	}
	if !hasDefault && entryFile != "" {
		if entryExps := allExports[entryFile]; entryExps != nil {
			// Find the primary named export (first exported class or function)
			for _, exp := range entryExps {
				if exp.GoName != "" && exp.GoName != "Default" {
					allExports[entryFile] = append(allExports[entryFile],
						backend.CrossFileExport{GoName: "Default", IsJSValue: true})
					// Mark the entry HIR module to synthesize var Default = PrimaryExport
					if entryMod, ok := hirModules[entryFile]; ok {
						entryMod.SynthesizeDefault = exp.GoName
					}
					break
				}
			}
		}
	}

	// Phase 2: Detect conflicting Default exports
	var defaultFiles []string
	for name, exps := range allExports {
		for _, exp := range exps {
			if exp.GoName == "Default" {
				defaultFiles = append(defaultFiles, name)
			}
		}
	}
	renameDefault := make(map[string]string) // file → renamed name
	if len(defaultFiles) > 1 {
		for _, name := range defaultFiles {
			if name != entryFile {
				renameDefault[name] = fileDefaultName(name)
			}
		}
	}

	// Update allExports for renamed files so cross-file references use the new name
	for fileName, newName := range renameDefault {
		exps := allExports[fileName]
		for i, exp := range exps {
			if exp.GoName == "Default" {
				exps[i].GoName = newName
			}
		}
	}

	// Phase 2b: assign package-global alias names for exported symbols.
	// Entry-file exports keep their public names. Non-entry exports get
	// file-specific aliases so flattened packages preserve module boundaries.
	globalUsedAliases := make(map[string]int)
	for fileName, exps := range allExports {
		aliases := make(map[string]string)
		for _, exp := range exps {
			exportName := exp.GoName
			originalName := exp.OriginalName
			if exportName == "Default" || originalName == "default" {
				alias := exportName
				if fileName != entryFile {
					alias = fileDefaultName(fileName)
				}
				aliases["default"] = makeUniqueAlias(alias, globalUsedAliases)
				continue
			}
			alias := exportName
			if fileName != entryFile {
				alias = fileSpecificExportName(fileName, originalName)
			}
			aliases[originalName] = makeUniqueAlias(alias, globalUsedAliases)
		}
		exportAliases[fileName] = aliases
		namespaceAliases[fileName] = makeUniqueAlias(fileSpecificExportName(fileName, "namespace"), globalUsedAliases)
	}

	for _, name := range allNames[entryFile] {
		if globalUsedAliases[name] == 0 {
			globalUsedAliases[name] = 1
		}
	}
	// Phase 2c: Resolve re-export aliases transitively.
	// Barrel files re-export from content files. Their aliases (e.g.
	// V4_core_index__constructor) are set in init(), creating ordering issues.
	// Resolve each barrel's alias to the ultimate source file's alias so
	// importers reference the original package-level variable directly.
	hir.ResolveReexportAliases(hirModules, exportAliases, files, entryFile)

	for fileName, hirMod := range hirModules {
		localAliases[fileName] = collectTopLevelAliases(hirMod, fileName, entryFile, exportAliases[fileName], globalUsedAliases)
	}

	// Phase 3: Compile each file with cross-file export knowledge
	results := make(map[string][]byte)
	goFiles := make(map[string]*ast.File)
	for name, hirMod := range hirModules {
		// Collect exports from OTHER files (not this one)
		var crossExports []backend.CrossFileExport
		var reservedNames []string
		importNameMap := make(map[string]string)
		for otherFile, exps := range allExports {
			if otherFile == name {
				continue
			}
			crossExports = append(crossExports, exps...)
		}
		for otherFile, names := range allNames {
			if otherFile == name {
				continue
			}
			reservedNames = append(reservedNames, names...)
			for _, alias := range localAliases[otherFile] {
				reservedNames = append(reservedNames, alias)
			}
			for _, alias := range exportAliases[otherFile] {
				reservedNames = append(reservedNames, alias)
			}
		}
		for _, imp := range hirMod.Imports {
			if !strings.HasPrefix(imp.ModulePath, ".") {
				continue
			}
			target := hir.ResolvePackageImportFile(name, imp.ModulePath, files)
			if target == "" {
				continue
			}
			targetAliases := exportAliases[target]
			if imp.Default != nil {
				if alias := targetAliases["default"]; alias != "" {
					importNameMap[imp.ModulePath+"\x00default"] = alias
				} else if nsAlias := namespaceAliases[target]; nsAlias != "" {
					// CJS require("./module") with no explicit default -> use namespace object
					importNameMap[imp.ModulePath+"\x00default"] = nsAlias
				}
			}
			if imp.Namespace != nil {
				if nsAlias := namespaceAliases[target]; nsAlias != "" {
					importNameMap[imp.ModulePath+"\x00*"] = nsAlias
				}
				for originalName, alias := range targetAliases {
					importNameMap[imp.ModulePath+"\x00"+originalName] = alias
				}
			}
			for _, n := range imp.Named {
				if alias := targetAliases[n.OriginalName]; alias != "" {
					importNameMap[imp.ModulePath+"\x00"+n.OriginalName] = alias
				}
			}
		}
		for _, decl := range hirMod.Declarations {
			ex, ok := decl.(*hir.ExportDecl)
			if !ok || ex.FromModule == "" || !strings.HasPrefix(ex.FromModule, ".") {
				continue
			}
			target := hir.ResolvePackageImportFile(name, ex.FromModule, files)
			if target == "" {
				continue
			}
			targetAliases := exportAliases[target]
			if nsAlias := namespaceAliases[target]; nsAlias != "" {
				importNameMap[ex.FromModule+"\x00*"] = nsAlias
			}
			for _, n := range ex.Names {
				if n.LocalName == "*" {
					continue
				}
				if alias := targetAliases[n.LocalName]; alias != "" {
					importNameMap[ex.FromModule+"\x00"+n.LocalName] = alias
				}
			}
		}

		goFiles[name] = backend.LowerWithExportsAndCPUProfile(hirMod, p.Ctx, moduleName, true, crossExports, reservedNames, importNameMap, exportAliases[name], localAliases[name], namespaceAliases[name], exportAliases[name], cpuProfile, p.OptLevel, name == entryFile)
	}
	for name, source := range files {
		if !isDataModule(name) {
			continue
		}
		defaultName := exportAliases[name]["default"]
		if defaultName == "" {
			defaultName = "Default"
		}
		namedAliases := map[string]string{}
		for originalName, alias := range exportAliases[name] {
			if originalName != "default" {
				namedAliases[originalName] = alias
			}
		}
		out, err := compileDataModuleWithOptLevel(name, source, pkgName, defaultName, namedAliases, p.OptLevel)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		results[name] = out
	}

	backend.BreakPackageInitCycles(goFiles)

	// Detect barrel files (files that are predominantly re-exports from other
	// same-package files). Their init() functions depend on source files' init()
	// having run first. Go runs init() in alphabetical file order, so we prefix
	// barrel file output names with "zz_" to ensure they sort last.
	barrelFiles := make(map[string]bool)
	for name, hirMod := range hirModules {
		if hir.IsBarrelFile(hirMod, files) {
			barrelFiles[name] = true
		}
	}

	for name, goFile := range goFiles {
		hirMod := hirModules[name]
		out, err := backend.GenerateWithSource(goFile, hirMod.SourcePath, hirMod.SourceSize)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}

		// Rename Default in non-entry files to avoid conflicts
		if _, shouldRename := renameDefault[name]; shouldRename {
			out = renameDefaultExport(out, name)
		}

		// Rename barrel files so their init() runs after content files
		outName := name
		if barrelFiles[name] {
			dir := filepath.Dir(name)
			base := filepath.Base(name)
			outName = filepath.Join(dir, "zz_"+base)
		}
		results[outName] = out
	}
	for name, out := range results {
		if !isDataModule(name) {
			continue
		}
		delete(results, name)
		results[strings.TrimSuffix(name, filepath.Ext(name))] = out
	}

	return results, nil
}

func isJSONModule(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".json")
}

func isYAMLModule(name string) bool {
	ext := filepath.Ext(name)
	return strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml")
}

func isDataModule(name string) bool {
	return isJSONModule(name) || isYAMLModule(name)
}

type dataExport struct {
	OriginalName string
	GoName       string
}

func parseDataModule(name string, source []byte) ([]dataExport, error) {
	if isYAMLModule(name) {
		mod, err := yamlmodule.Parse(source)
		if err != nil {
			return nil, fmt.Errorf("parse yaml %s: %w", name, err)
		}
		out := make([]dataExport, len(mod.Exports))
		for i, exp := range mod.Exports {
			out[i] = dataExport{OriginalName: exp.OriginalName, GoName: exp.GoName}
		}
		return out, nil
	}
	mod, err := jsonmodule.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse json %s: %w", name, err)
	}
	out := make([]dataExport, len(mod.Exports))
	for i, exp := range mod.Exports {
		out[i] = dataExport{OriginalName: exp.OriginalName, GoName: exp.GoName}
	}
	return out, nil
}

func compileDataModuleWithOptLevel(name string, source []byte, pkgName, defaultName string, namedAliases map[string]string, optLevel context.OptLevel) ([]byte, error) {
	if isYAMLModule(name) {
		return yamlmodule.Compile(source, pkgName, defaultName, namedAliases)
	}
	return jsonmodule.CompileWithOptLevel(source, pkgName, defaultName, namedAliases, optLevel)
}

// fileDefaultName generates a file-specific name for renamed default exports.
func fileDefaultName(fileName string) string {
	base := filepath.Base(fileName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	name := ""
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			name += string(r)
		} else {
			name += "_"
		}
	}
	if name == "" {
		name = "FileDefault"
	}
	return strings.ToUpper(name[:1]) + name[1:] + "Default"
}

func fileSpecificExportName(fileName, exportName string) string {
	clean := strings.TrimSuffix(filepath.Clean(fileName), filepath.Ext(fileName))
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) > 3 {
		parts = parts[len(parts)-3:]
	}
	name := ""
	for _, r := range strings.Join(parts, "_") + "_" + exportName {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			name += string(r)
		} else {
			name += "_"
		}
	}
	if name == "" {
		name = "File"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func makeUniqueAlias(alias string, used map[string]int) string {
	if used[alias] == 0 {
		used[alias] = 1
		return alias
	}
	used[alias]++
	return fmt.Sprintf("%s_%d", alias, used[alias])
}

func expandWildcardReexports(mods map[string]*hir.Module, allExports map[string][]backend.CrossFileExport, files map[string][]byte) {
	changed := true
	for changed {
		changed = false
		for fileName, mod := range mods {
			existing := allExports[fileName]
			seen := make(map[string]bool, len(existing))
			for _, exp := range existing {
				seen[exp.OriginalName+"=>"+exp.GoName] = true
			}
			for _, decl := range mod.Declarations {
				ex, ok := decl.(*hir.ExportDecl)
				if !ok || ex.FromModule == "" || !ex.IsWildcard || !strings.HasPrefix(ex.FromModule, ".") {
					continue
				}
				target := hir.ResolvePackageImportFile(fileName, ex.FromModule, files)
				if target == "" {
					continue
				}
				for _, targetExp := range allExports[target] {
					if targetExp.OriginalName == "default" {
						continue
					}
					foundName := false
					for _, n := range ex.Names {
						if n.LocalName == targetExp.OriginalName && n.ExportedName == targetExp.OriginalName {
							foundName = true
							break
						}
					}
					if !foundName {
						ex.Names = append(ex.Names, hir.ExportName{
							LocalName:    targetExp.OriginalName,
							ExportedName: targetExp.OriginalName,
						})
					}
					key := targetExp.OriginalName + "=>" + targetExp.GoName
					if seen[key] {
						continue
					}
					existing = append(existing, targetExp)
					seen[key] = true
					changed = true
				}
			}
			allExports[fileName] = existing
		}
	}
}

func collectTopLevelAliases(mod *hir.Module, fileName, entryFile string, exportAliases map[string]string, used map[string]int) map[symbol.ID]string {
	aliases := make(map[symbol.ID]string)
	isEntry := fileName == entryFile

	assign := func(sym *symbol.Symbol, exported bool, exportName string) {
		if sym == nil {
			return
		}
		if _, exists := aliases[sym.ID]; exists {
			return
		}
		// Prefer export alias for any symbol whose name matches,
		// even non-exported locals that back CJS named exports.
		if alias := exportAliases[sym.OriginalName]; alias != "" {
			aliases[sym.ID] = alias
			return
		}
		if exported {
			if alias := exportAliases[exportName]; alias != "" {
				aliases[sym.ID] = alias
				return
			}
		}
		if !isEntry {
			aliases[sym.ID] = makeUniqueAlias(fileSpecificExportName(fileName, sym.OriginalName), used)
		}
	}

	var walkDecl func(hir.Decl, bool)
	walkDecl = func(d hir.Decl, forceExported bool) {
		switch d := d.(type) {
		case *hir.FuncDecl:
			assign(d.Symbol, forceExported || d.Exported, d.Symbol.OriginalName)
		case *hir.VarDecl:
			exported := forceExported || d.Exported
			for _, decl := range d.Declarators {
				if decl.Symbol != nil {
					assign(decl.Symbol, exported, decl.Symbol.OriginalName)
				}
			}
		case *hir.ClassDecl:
			assign(d.Symbol, forceExported || d.Exported, d.Symbol.OriginalName)
		case *hir.EnumDecl:
			assign(d.Symbol, forceExported || d.Exported, d.Symbol.OriginalName)
		case *hir.InterfaceDecl:
			assign(d.Symbol, forceExported || d.Exported, d.Symbol.OriginalName)
		case *hir.TypeAliasDecl:
			assign(d.Symbol, forceExported || d.Exported, d.Symbol.OriginalName)
		case *hir.ExportDecl:
			if d.Decl != nil {
				walkDecl(d.Decl, true)
			}
		}
	}

	for _, d := range mod.Declarations {
		walkDecl(d, false)
	}
	return aliases
}

// renameDefaultExport replaces "Default" in compiled output with a file-specific name.
func renameDefaultExport(source []byte, fileName string) []byte {
	name := fileDefaultName(fileName)

	s := string(source)
	s = strings.ReplaceAll(s, "var Default ", "var "+name+" ")
	s = strings.ReplaceAll(s, "var Default\n", "var "+name+"\n")
	// Also rename bare assignments from fixInitCycles (e.g. "\tDefault = " in init())
	s = strings.ReplaceAll(s, "\tDefault = ", "\t"+name+" = ")
	s = strings.ReplaceAll(s, "\nDefault = ", "\n"+name+" = ")
	return []byte(s)
}

// synthesizeDefaultIfNeeded checks if a module has named exports but no default,
// and sets SynthesizeDefault to the primary named export (SWC interop).
func synthesizeDefaultIfNeeded(mod *hir.Module) {
	hasDefault := false
	var primaryExport string
	for _, d := range mod.Declarations {
		if ed, ok := d.(*hir.ExportDecl); ok {
			if ed.IsDefault {
				hasDefault = true
				return
			}
			if ed.Decl != nil {
				switch inner := ed.Decl.(type) {
				case *hir.ClassDecl:
					if inner.Symbol != nil && primaryExport == "" {
						primaryExport = symbol.Capitalize(inner.Symbol.OriginalName)
					}
				case *hir.FuncDecl:
					if inner.Symbol != nil && primaryExport == "" {
						primaryExport = symbol.Capitalize(inner.Symbol.OriginalName)
					}
				}
			}
		}
	}
	if !hasDefault && primaryExport != "" {
		mod.SynthesizeDefault = primaryExport
	}
}

// parseTypeScript is a helper to parse TypeScript source into a tree-sitter tree.
func parseTypeScript(source []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse TypeScript source")
	}
	return tree, nil
}
