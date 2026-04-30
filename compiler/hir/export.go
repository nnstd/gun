package hir

import (
	"path/filepath"
	"strings"
)

// IsBarrelFile returns true if a module is predominantly re-exports from other
// same-package files (e.g. an index.ts that does `export * from "./schemas"`).
// These files' init() must run AFTER the files they re-export from.
func IsBarrelFile(mod *Module, allFiles map[string][]byte) bool {
	var reexportCount, contentCount int
	for _, decl := range mod.Declarations {
		switch d := decl.(type) {
		case *ExportDecl:
			if d.FromModule != "" && strings.HasPrefix(d.FromModule, ".") {
				// Re-export from same-package file
				target := ResolvePackageImportFile(mod.SourcePath, d.FromModule, allFiles)
				if target != "" {
					reexportCount++
					continue
				}
			}
			if d.Decl != nil {
				contentCount++
			}
		case *ImportDecl:
			// Imports don't count as content
		default:
			contentCount++
		}
	}
	// A barrel file has re-exports and very little original content
	return reexportCount > 0 && reexportCount >= contentCount
}

// ResolveReexportAliases rewrites barrel file export aliases to point directly
// to the original source file's aliases. This avoids init-ordering problems
// where a barrel file's init() sets re-export variables that other files need.
//
// For example, if core/index.js re-exports $constructor from core/core.js,
// instead of V4_core_index__constructor (set in barrel's init()),
// importers get V4_core_core__constructor (set at package level).
func ResolveReexportAliases(hirModules map[string]*Module, exportAliases map[string]map[string]string, files map[string][]byte, entryFile string) {
	// Build a map of which exports are re-exports and where they come from.
	// reexportSource[file][exportName] = sourceFile
	type source struct {
		file, name string
	}
	reexportSource := make(map[string]map[string]source)

	for fileName, mod := range hirModules {
		for _, decl := range mod.Declarations {
			ex, ok := decl.(*ExportDecl)
			if !ok || ex.FromModule == "" || !strings.HasPrefix(ex.FromModule, ".") {
				continue
			}
			target := ResolvePackageImportFile(fileName, ex.FromModule, files)
			if target == "" {
				continue
			}
			if reexportSource[fileName] == nil {
				reexportSource[fileName] = make(map[string]source)
			}
			for _, n := range ex.Names {
				if n.LocalName == "*" {
					continue
				}
				reexportSource[fileName][n.ExportedName] = source{target, n.LocalName}
			}
		}
	}

	// Resolve transitively: chase re-export chains to find ultimate source.
	resolve := func(file, exportName string) (string, string) {
		visited := make(map[string]bool)
		for {
			key := file + "\x00" + exportName
			if visited[key] {
				break
			}
			visited[key] = true
			src, ok := reexportSource[file]
			if !ok {
				break
			}
			s, ok := src[exportName]
			if !ok {
				break
			}
			file = s.file
			exportName = s.name
		}
		return file, exportName
	}

	// Rewrite barrel file aliases to point to source aliases.
	// Skip the entry file — its aliases are the package's public API.
	for fileName, aliases := range exportAliases {
		if fileName == entryFile {
			continue
		}
		if reexportSource[fileName] == nil {
			continue
		}
		for exportName := range aliases {
			srcFile, srcExportName := resolve(fileName, exportName)
			if srcFile == fileName {
				continue
			}
			if srcAliases := exportAliases[srcFile]; srcAliases != nil {
				if srcAlias := srcAliases[srcExportName]; srcAlias != "" {
					aliases[exportName] = srcAlias
				}
			}
		}
	}
}

// ResolvePackageImportFile resolves a relative import path within a package
// to a concrete file name present in the files map.
func ResolvePackageImportFile(currentFile, importPath string, files map[string][]byte) string {
	fromDir := filepath.Dir(currentFile)
	base := filepath.Join(fromDir, importPath)
	candidates := []string{
		base,
		base + ".ts",
		base + ".js",
		base + ".mjs",
		base + ".json",
		filepath.Join(base, "index.ts"),
		filepath.Join(base, "index.js"),
		filepath.Join(base, "index.json"),
	}
	for _, candidate := range candidates {
		if _, ok := files[candidate]; ok {
			return candidate
		}
		clean := filepath.Clean(candidate)
		if _, ok := files[clean]; ok {
			return clean
		}
		abs, _ := filepath.Abs(candidate)
		if _, ok := files[abs]; ok {
			return abs
		}
	}
	return ""
}
