package compiler

import (
	"strings"

	tcontext "github.com/nnstd/gun/compiler/context"
)

// IsKnownModule reports whether the given module name is backed by a registered
// runtime/module mapping in the pipeline context.
func IsKnownModule(name string) bool {
	ctx := tcontext.New()
	RegisterDefaultBuiltins(ctx)
	name = strings.TrimPrefix(name, "node:")
	return ctx.LookupModule(name) != nil
}

// SanitizeGoPkgName converts an npm package name to a valid Go package name.
func SanitizeGoPkgName(npmName string) string {
	name := strings.TrimPrefix(npmName, "@")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}
