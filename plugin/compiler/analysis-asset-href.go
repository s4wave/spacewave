//go:build !js

package bldr_plugin_compiler

import (
	"flag"
	"go/ast"
	gast "go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AssetHrefTag is the comment tag used for getting paths to assets.
const AssetHrefTag = "bldr:asset:href"

// AssetHrefArgs are arguments parsed from a bldr:asset:href directive.
type AssetHrefArgs struct {
	// AssetPath is the relative path to the location in the assets fs.
	AssetPath string
}

// BuildFlagSet builds the set of flags for the args.
func (a *AssetHrefArgs) BuildFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(AssetHrefTag, flag.ContinueOnError)
	// NOTE: currently we do not add any extra flags
	return fs
}

// TrimAssetHrefArgs trims the bldr:asset prefix from a string.
// Returns if the string had the prefix.
func TrimAssetHrefArgs(value string) (string, bool) {
	return TrimCommentArgs(AssetHrefTag, value)
}

// ParseAssetHrefComments parses the bldr:asset:ref comments for a variable.
//
// If no bldr:asset:href prefix is found, returns nil, false, nil
func ParseAssetHrefComments(values []string, spec *gast.ValueSpec) (*AssetHrefArgs, bool, error) {
	args, found, err := CombineShellComments(AssetHrefTag, values)
	if err != nil || !found {
		return nil, found, err
	}

	typeStr := types.ExprString(spec.Type)
	if typeStr != "string" {
		return nil, true, errors.Errorf("bldr:asset: expected string variable type but got %s", typeStr)
	}

	outArgs := &AssetHrefArgs{}
	fs := outArgs.BuildFlagSet()
	if err := fs.Parse(args); err != nil {
		return nil, true, err
	}
	narg := fs.NArg()
	if narg != 1 {
		return nil, true, errors.Errorf("expected 1 args but got %d: expected %s asset-path", narg, AssetHrefTag)
	}

	assetPath := fs.Arg(narg - 1)
	if filepath.IsAbs(assetPath) {
		return nil, true, errors.Errorf("to path must be relative: %s", assetPath)
	}

	return &AssetHrefArgs{AssetPath: assetPath}, true, nil
}

// FindAssetHrefVariables searches for bldr:asset:href comments.
func (a *Analysis) FindAssetHrefVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*AssetHrefArgs), error) {
	return FindTagComments(AssetHrefTag, a.fset, codeFiles, ParseAssetHrefComments)
}

// BuildDefAssetHrefs builds the list of go variable defs for the given code files.
func BuildDefAssetHrefs(
	le *logrus.Entry,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*AssetHrefArgs),
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*vardef.PluginVar, error) {
	var defs []*vardef.PluginVar
	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, errors.Errorf("failed to find corresponding ast.File for package: %s", pkgImportPath)
		}
		for pkgVar, assetArgs := range pkgVars {
			destPath := filepath.Join(outAssetsPath, assetArgs.AssetPath)
			if !strings.HasPrefix(destPath, outAssetsPath) {
				return nil, errors.Errorf("path must be child of current dir: %s", assetArgs.AssetPath)
			}
			destPathRel, err := filepath.Rel(outAssetsPath, destPath)
			if err != nil {
				return nil, err
			}
			destPathRel = filepath.ToSlash(destPathRel)

			defs = append(defs, vardef.NewPluginVar(
				pkgImportPath,
				pkgVar,
				&vardef.PluginVar_StringValue{
					StringValue: BuildAssetHref(pluginID, destPathRel),
				},
			))
		}
	}

	return defs, nil
}
