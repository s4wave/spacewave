//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/pkg/errors"
)

// ViteTag is the comment tag used for vite.
const ViteTag = "bldr:vite"

// DefaultViteBundleID is the default ID to use for vite bundles.
const DefaultViteBundleID = "default"

// ViteBundleIDFlag is the flag for bundle-id.
const ViteBundleIDFlag = "--bundle-id="

// ViteConfigFlag is the flag for vite config paths.
const ViteConfigFlag = "--config="

// ViteDisableProjectConfigFlag is the flag to disable automatic project config detection.
const ViteDisableProjectConfigFlag = "--disable-project-config"

// ViteDirective are arguments parsed from a bldr:vite directive.
type ViteDirective struct {
	// BundleID is the bundle identifier to use for vite.
	// If unset, uses "default".
	BundleID string
	// ViteConfigPaths are the vite config paths options.
	// Note that all BuildOptions for the same BundleID are merged.
	ViteConfigPaths []string
	// EntrypointPath is the entrypoint path for vite.
	// This is the positional argument that doesn't start with a flag.
	EntrypointPath string
	// ViteVarType is the type of vite output variable.
	ViteVarType ViteVarType
	// DisableProjectConfig indicates whether to disable automatic project config detection.
	DisableProjectConfig bool
}

// TrimViteDirective trims the bldr:vite prefix from a string.
// Returns if the string had the prefix.
func TrimViteDirective(value string) (string, bool) {
	return TrimCommentArgs(ViteTag, value)
}

// ViteDirectiveArgs contains the parsed arguments from a vite directive.
type ViteDirectiveArgs struct {
	// BundleID is the bundle identifier to use for vite.
	BundleID string
	// ViteConfigPaths are the vite config paths options.
	ViteConfigPaths []string
	// EntrypointPath is the entrypoint path for vite.
	EntrypointPath string
	// DisableProjectConfig indicates whether to disable automatic project config detection.
	DisableProjectConfig bool
}

// ParseViteDirectiveArgs parses vite directive arguments to extract bundle ID, config paths, entrypoint path,
// and whether to disable project config detection.
// Only one positional argument is allowed as the entrypoint path.
func ParseViteDirectiveArgs(args []string) (ViteDirectiveArgs, error) {
	// Merge bundle flags while allowing only one entrypoint path.
	result := ViteDirectiveArgs{
		BundleID: DefaultViteBundleID,
	}
	var foundEntrypoint bool

	// Classify recognized flags before treating remaining arguments as entrypoints.
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, ViteBundleIDFlag):
			value := arg[len(ViteBundleIDFlag):]
			if len(value) != 0 {
				result.BundleID = value
			}
		case strings.HasPrefix(arg, ViteConfigFlag):
			value := arg[len(ViteConfigFlag):]
			if len(value) != 0 {
				result.ViteConfigPaths = append(result.ViteConfigPaths, value)
			}
		case arg == ViteDisableProjectConfigFlag:
			result.DisableProjectConfig = true
		default:
			// Preserve the positional entrypoint rule for unrecognized flags too.
			if foundEntrypoint {
				return ViteDirectiveArgs{}, errors.New("only one entrypoint path is allowed")
			}
			result.EntrypointPath = arg
			foundEntrypoint = true
		}
	}

	return result, nil
}

// determineViteVarType determines the variable type for a vite variable.
func (a *Analysis) determineViteVarType(obj types.Object) (ViteVarType, error) {
	return determineVarTypeWithReference(
		obj,
		a.webBundlerOutputType,
		ViteVarType_ViteVarType_ENTRYPOINT_PATH,
		ViteVarType_ViteVarType_WEB_BUNDLER_OUTPUT,
		"vite",
	)
}

// FindViteVariables searches for bldr:vite comments.
func (a *Analysis) FindViteVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*ViteDirective), error) {
	return FindTagCommentsWithTypes(
		ViteTag,
		a,
		codeFiles,
		func(values []string, obj types.Object) (*ViteDirective, bool, error) {
			// Parse the comments for vite directives.
			args, found, err := CombineShellComments(ViteTag, values)
			if err != nil || !found {
				return nil, found, err
			}

			// Parse the arguments into a structured result.
			argsResult, err := ParseViteDirectiveArgs(args)
			if err != nil {
				return nil, true, err
			}

			// Determine the variable type using the type system.
			varType, err := a.determineViteVarType(obj)
			if err != nil {
				return nil, true, err
			}

			return &ViteDirective{
				BundleID:             argsResult.BundleID,
				ViteConfigPaths:      argsResult.ViteConfigPaths,
				EntrypointPath:       argsResult.EntrypointPath,
				ViteVarType:          varType,
				DisableProjectConfig: argsResult.DisableProjectConfig,
			}, true, nil
		},
	)
}
