package plugin_compiler

import (
	gast "go/ast"
)

// GoVarDef defines the value of a variable at build time in the init() function.
type GoVarDef struct {
	// PackagePath is the Go package path.
	PackagePath string
	// VariableName is the Go variable name.
	// Must be exported (first character is uppercase).
	VariableName string
	// Value is the value to set.
	Value gast.Expr
}
