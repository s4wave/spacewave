package plugin_compiler

import (
	"flag"
	"go/ast"
	gast "go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bldr/plugin"
	cf "github.com/aperturerobotics/bldr/util/copyfile"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AssetTag is the comment tag used for assets.
const AssetTag = "bldr:asset"

// AssetArgs are arguments parsed from a bldr:asset directive.
type AssetArgs struct {
	// FromPath is the relative path to the from file or dir.
	FromPath string
	// ToPath is the relative path to the location in the assets fs.
	ToPath string
}

// BuildAssetHref builds the path to an asset for a plugin id.
// assets path is available at /p/{plugin-id}/
func BuildAssetHref(pluginID string, assetPath string) string {
	return strings.Join([]string{
		plugin.PluginAssetsRoute,
		pluginID,
		"/",
		assetPath,
	}, "")
}

// BuildFlagSet builds the set of flags for the args.
func (a *AssetArgs) BuildFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(AssetTag, flag.ContinueOnError)
	// NOTE: currently we do not add any extra flags
	return fs
}

// TrimAssetArgs trims the bldr:asset prefix from a string.
// Returns if the string had the prefix.
func TrimAssetArgs(value string) (string, bool) {
	return TrimCommentArgs(AssetTag, value)
}

// ParseAssetComments parses the bldr:asset comments for a variable.
//
// If no bldr:asset prefix is found, returns nil, false, nil
func ParseAssetComments(values []string, spec *gast.ValueSpec) (*AssetArgs, bool, error) {
	args, found, err := CombineShellComments(AssetTag, values)
	if err != nil || !found {
		return nil, found, err
	}

	typeStr := types.ExprString(spec.Type)
	if typeStr != "string" {
		return nil, true, errors.Errorf("bldr:asset: expected string variable type but got %s", typeStr)
	}

	outArgs := &AssetArgs{}
	fs := outArgs.BuildFlagSet()
	if err := fs.Parse(args); err != nil {
		return nil, true, err
	}
	narg := fs.NArg()
	if narg != 2 {
		return nil, true, errors.Errorf("expected 2 args but got %d: expected %s from to", narg, AssetTag)
	}

	fromPath := fs.Arg(narg - 2)
	if path.IsAbs(fromPath) {
		return nil, true, errors.Errorf("from path must be relative: %s", fromPath)
	}

	toPath := fs.Arg(narg - 1)
	if path.IsAbs(toPath) {
		return nil, true, errors.Errorf("to path must be relative: %s", toPath)
	}

	return &AssetArgs{FromPath: fromPath, ToPath: toPath}, true, nil
}

// FindAssetVariables searches for bldr:asset comments.
func (a *Analysis) FindAssetVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*AssetArgs), error) {
	return FindTagComments(AssetTag, a.fset, codeFiles, ParseAssetComments)
}

// BuildDefAssets builds the list of go variable defs for the given code files.
func BuildDefAssets(
	le *logrus.Entry,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*AssetArgs),
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*GoVarDef, []string, error) {
	var defs []*GoVarDef
	var srcFilesPaths []string
	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, nil, errors.Errorf("failed to find corresponding ast.File for package: %s", pkgImportPath)
		}
		for pkgVar, assetArgs := range pkgVars {
			destPath := path.Join(outAssetsPath, assetArgs.ToPath)
			if !strings.HasPrefix(destPath, outAssetsPath) {
				return nil, nil, errors.Errorf("path must be child of current dir: %s", assetArgs.ToPath)
			}
			destPathRel, err := filepath.Rel(outAssetsPath, destPath)
			if err != nil {
				return nil, nil, err
			}

			pkgCodePath := path.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
			srcPath := path.Join(pkgCodePath, assetArgs.FromPath)
			if !strings.HasPrefix(srcPath, pkgCodePath) {
				return nil, nil, errors.Errorf("path must be child of current dir: %s", assetArgs.FromPath)
			}

			st, err := os.Stat(srcPath)
			if err == nil {
				if !st.IsDir() && !st.Mode().IsRegular() {
					err = errors.Errorf("path must be a dir or regular file: %s", assetArgs.FromPath)
				}
			}
			if err != nil {
				return nil, nil, err
			}

			le.Debugf("copying asset file(s) or dir(s): %s", assetArgs.FromPath)
			err = cf.CopyRecursive(destPath, srcPath, func(srcPath string, fi fs.DirEntry, err error) error {
				if !fi.IsDir() {
					srcFilesPaths = append(srcFilesPaths, srcPath)
				}
				return nil
			})
			if err != nil {
				return nil, nil, err
			}

			defs = append(defs, &GoVarDef{
				PackagePath:  pkgImportPath,
				VariableName: pkgVar,
				Value: &gast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(BuildAssetHref(pluginID, destPathRel)),
				},
			})
		}
	}
	return defs, srcFilesPaths, nil
}
