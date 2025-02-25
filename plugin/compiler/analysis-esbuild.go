//go:build !js

package bldr_plugin_compiler

import (
	"go/ast"
	"go/types"
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	"github.com/pkg/errors"
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
	EsbuildVarType bldr_esbuild.EsbuildVarType
}

// TrimEsbuildDirective trims the bldr:esbuild prefix from a string.
// Returns if the string had the prefix.
func TrimEsbuildDirective(value string) (string, bool) {
	return TrimCommentArgs(EsbuildTag, value)
}

// EsbuildOutputPkgPath is the package path for EsbuildOutput type
const EsbuildOutputPkgPath = "github.com/aperturerobotics/bldr/web/esbuild"

// EsbuildOutputTypeName is the type name for EsbuildOutput
const EsbuildOutputTypeName = "EsbuildOutput"

// isEsbuildOutputType checks if a type is an EsbuildOutput type
func isEsbuildOutputType(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Pkg() != nil &&
			named.Obj().Pkg().Path() == EsbuildOutputPkgPath &&
			named.Obj().Name() == EsbuildOutputTypeName
	}
	return false
}

// determineEsbuildVarType determines the variable type for an esbuild variable
func determineEsbuildVarType(obj types.Object) (bldr_esbuild.EsbuildVarType, error) {
	// First check if it's directly an EsbuildOutput type
	if isEsbuildOutputType(obj.Type()) {
		return bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT, nil
	}

	// Check the underlying type
	switch t := obj.Type().Underlying().(type) {
	case *types.Basic:
		if t.Kind() == types.String {
			return bldr_esbuild.EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH, nil
		}
		return 0, errors.Errorf("unexpected basic type for bldr:esbuild variable: %v", t)
	case *types.Named, *types.Struct:
		// For named types and struct types, check if the original type is EsbuildOutput
		if isEsbuildOutputType(obj.Type()) {
			return bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT, nil
		}

		// Get a descriptive name for error reporting
		if named, ok := obj.Type().(*types.Named); ok && named.Obj().Pkg() != nil {
			return 0, errors.Errorf("unexpected type for bldr:esbuild variable: %v.%v",
				named.Obj().Pkg().Path(), named.Obj().Name())
		}
		return 0, errors.Errorf("unexpected type for bldr:esbuild variable")
	default:
		return 0, errors.Errorf("unexpected type for bldr:esbuild variable: %T", t)
	}
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
			varType, err := determineEsbuildVarType(obj)
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
