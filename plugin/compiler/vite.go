//go:build !js

package bldr_plugin_compiler

import (
	"errors"
	"go/ast"
	"go/types"
	"strings"

	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	"golang.org/x/exp/slices"
	"golang.org/x/tools/go/packages"
)

// ViteTag is the comment tag used for vite.
const ViteTag = "bldr:vite"

// DefaultViteBundleID is the default ID to use for vite bundles.
const DefaultViteBundleID = "default"

// ViteBundleIDFlag is the flag for bundle-id.
const ViteBundleIDFlag = "--bundle-id="

// ViteConfigFlag is the flag for vite config paths.
const ViteConfigFlag = "--config="

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
	// ViteVarType is the type of vite output variable we are using.
	ViteVarType bldr_vite.ViteVarType
}

// TrimViteDirective trims the bldr:vite prefix from a string.
// Returns if the string had the prefix.
func TrimViteDirective(value string) (string, bool) {
	return TrimCommentArgs(ViteTag, value)
}

// ViteOutputPkgPath is the package path for ViteOutput type
const ViteOutputPkgPath = "github.com/aperturerobotics/bldr/web/bundler"

// ViteOutputTypeName is the type name for ViteOutput
const ViteOutputTypeName = "WebBundlerOutput"

// determineViteVarType determines the variable type for a vite variable
func (a *Analysis) determineViteVarType(obj types.Object) (bldr_vite.ViteVarType, error) {
	result, err := a.determineVarTypeWithReference(
		obj,
		a.webBundlerOutputType, // Reuse the same type as esbuild
		bldr_vite.ViteVarType_ViteVarType_ENTRYPOINT_PATH,
		bldr_vite.ViteVarType_ViteVarType_WEB_BUNDLER_OUTPUT,
		"vite",
	)
	if err != nil {
		return 0, err
	}
	return result.(bldr_vite.ViteVarType), nil
}

// parseViteArgs parses vite directive arguments to extract bundle ID, config paths, and entrypoint path
// Only one positional argument is allowed as the entrypoint path
func parseViteArgs(args []string) (bundleID string, viteConfigPaths []string, entrypointPath string, err error) {
	bundleID = DefaultViteBundleID
	var configPaths []string
	var foundEntrypoint bool

	for _, arg := range args {
		if strings.HasPrefix(arg, ViteBundleIDFlag) {
			value := arg[len(ViteBundleIDFlag):]
			if len(value) != 0 {
				bundleID = value
			}
		} else if strings.HasPrefix(arg, ViteConfigFlag) {
			value := arg[len(ViteConfigFlag):]
			if len(value) != 0 {
				configPaths = append(configPaths, value)
			}
		} else {
			// Any argument that doesn't start with a flag is considered an entrypoint path
			if foundEntrypoint {
				return "", nil, "", errors.New("only one entrypoint path is allowed")
			}
			entrypointPath = arg
			foundEntrypoint = true
		}
	}

	return bundleID, configPaths, entrypointPath, nil
}

// FindViteVariables searches for bldr:vite comments.
func (a *Analysis) FindViteVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*ViteDirective), error) {
	return FindTagCommentsWithTypes(
		ViteTag,
		a,
		codeFiles,
		func(values []string, varName string, pkg *packages.Package, obj types.Object) (*ViteDirective, bool, error) {
			// Parse the comments for vite directives
			args, found, err := CombineShellComments(ViteTag, values)
			if err != nil || !found {
				return nil, found, err
			}

			// Determine bundle ID, config paths, and entrypoint path from the args
			bundleID, viteConfigPaths, entrypointPath, err := parseViteArgs(args)
			if err != nil {
				return nil, true, err
			}

			// Determine the variable type using the type system
			varType, err := a.determineViteVarType(obj)
			if err != nil {
				return nil, true, err
			}

			return &ViteDirective{
				BundleID:        bundleID,
				ViteConfigPaths: viteConfigPaths,
				EntrypointPath:  entrypointPath,
				ViteVarType:     varType,
			}, true, nil
		},
	)
}

// SortViteOutputMetas sorts and compacts a list of esbuild output meta.
func SortViteOutputMetas(metas []*ViteOutputMeta) []*ViteOutputMeta {
	slices.SortFunc(metas, func(a, b *ViteOutputMeta) int {
		return strings.Compare(a.GetPath(), b.GetPath())
	})
	return slices.CompactFunc(metas, func(a, b *ViteOutputMeta) bool {
		return a.GetPath() == b.GetPath()
	})
}
