//go:build !js

package bldr_cli_compiler

import plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"

// CliImport describes a discovered NewCliCommands in a Go package.
type CliImport struct {
	// Alias is the import alias for the package.
	Alias string
}

// ResolveComposePackage resolves a compose package path relative to the
// project module. Empty stays empty.
func ResolveComposePackage(pkg, rootModule string) string {
	resolved, _ := plugin_compiler_go.UpdateRelativeGoPackagePaths([]string{pkg}, rootModule)
	return resolved[0]
}
