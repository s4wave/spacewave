//go:build !js

package bldr_plugin_compiler

import (
	"go/ast"
	"go/types"
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	"github.com/pkg/errors"
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

// ParseEsbuildComments parses the bldr:esbuild directive comments.
//
// If no bldr:esbuild prefix is found, returns nil, false, nil
func ParseEsbuildComments(values []string, spec *ast.ValueSpec) (*EsbuildDirective, bool, error) {
	args, found, err := CombineShellComments(EsbuildTag, values)
	if err != nil || !found {
		return nil, found, err
	}

	// determine bundle id from the args
	bundleID := DefaultBundleID
	for _, arg := range args {
		if strings.HasPrefix(arg, BundleIDFlag) {
			value := arg[len(BundleIDFlag):]
			if len(value) != 0 {
				bundleID = value
			}
		}
	}

	// parse esbuild cli args
	/*
		buildOpts, err := esbuild_cli.ParseBuildOptions(args)
		if err != nil {
			return nil, true, err
		}
	*/

	// determine the variable type for the Esbuild variable
	var varType bldr_esbuild.EsbuildVarType
	typeStr := types.ExprString(spec.Type)
	switch typeStr {
	case "string":
		varType = bldr_esbuild.EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH
	case "bldr_esbuild.EsbuildOutput":
		varType = bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT
	default:
		return nil, true, errors.Errorf("unexpected type for bldr:esbuild variable: %s", typeStr)
	}

	return &EsbuildDirective{
		BundleID:       bundleID,
		EsbuildFlags:   args,
		EsbuildVarType: varType,
	}, true, nil
}

// FindEsbuildVariables searches for bldr:esbuild comments.
func (a *Analysis) FindEsbuildVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*EsbuildDirective), error) {
	return FindTagComments(EsbuildTag, a.fset, codeFiles, ParseEsbuildComments)
}
