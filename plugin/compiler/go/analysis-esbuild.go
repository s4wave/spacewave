//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// EsbuildTag is the comment tag used for esbuild.
const EsbuildTag = "bldr:esbuild"

// DefaultBundleID is the default ID to use for esbuild bundles.
const DefaultBundleID = "default"

// BundleIDFlag is the flag for bundle-id.
const BundleIDFlag = "--bundle-id="

// EsbuildDirective are arguments parsed from a bldr:esbuild directive.
type EsbuildDirective struct {
	// BundleID is the bundle identifier to use for esbuild.
	// If unset, uses "default".
	BundleID string
	// EsbuildFlags are the esbuild build options.
	// Note that all BuildOptions for the same BundleID are merged.
	EsbuildFlags []string
	// EsbuildVarType is the type of esbuild output variable we are using.
	EsbuildVarType EsbuildVarType
}

// TrimEsbuildDirective trims the bldr:esbuild prefix from a string.
// Returns if the string had the prefix.
func TrimEsbuildDirective(value string) (string, bool) {
	return TrimCommentArgs(EsbuildTag, value)
}

// EsbuildOutputPkgPath is the package path for EsbuildOutput type
const EsbuildOutputPkgPath = "github.com/aperturerobotics/bldr/web/bundler"

// EsbuildOutputTypeName is the type name for EsbuildOutput
const EsbuildOutputTypeName = "WebBundlerOutput"

// determineEsbuildVarType determines the variable type for an esbuild variable
func (a *Analysis) determineEsbuildVarType(obj types.Object) (EsbuildVarType, error) {
	return determineVarTypeWithReference[EsbuildVarType](
		a,
		obj,
		a.webBundlerOutputType,
		EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH,
		EsbuildVarType_EsbuildVarType_WEB_BUNDLER_OUTPUT,
		"esbuild",
	)
}

// parseEsbuildArgs parses esbuild directive arguments to extract bundle ID and other flags
func parseEsbuildArgs(args []string) (string, []string) {
	bundleID := DefaultBundleID
	for _, arg := range args {
		if strings.HasPrefix(arg, BundleIDFlag) {
			value := arg[len(BundleIDFlag):]
			if len(value) != 0 {
				bundleID = value
			}
		}
	}
	return bundleID, args
}

// FindEsbuildVariables searches for bldr:esbuild comments.
func (a *Analysis) FindEsbuildVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*EsbuildDirective), error) {
	return FindTagCommentsWithTypes(
		EsbuildTag,
		a,
		codeFiles,
		func(values []string, varName string, pkg *packages.Package, obj types.Object) (*EsbuildDirective, bool, error) {
			// Parse the comments for esbuild directives
			args, found, err := CombineShellComments(EsbuildTag, values)
			if err != nil || !found {
				return nil, found, err
			}

			// Determine bundle ID from the args
			bundleID, esbuildFlags := parseEsbuildArgs(args)

			// Determine the variable type using the type system
			varType, err := a.determineEsbuildVarType(obj)
			if err != nil {
				return nil, true, err
			}

			return &EsbuildDirective{
				BundleID:       bundleID,
				EsbuildFlags:   esbuildFlags,
				EsbuildVarType: varType,
			}, true, nil
		},
	)
}
