//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildViteBundlerConfig builds the vite bundler controller config.
func BuildViteBundlerConfig(
	bundleVars []*ViteBundleVarMeta,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	viteConfigPaths []string,
	disableProjectConfig bool,
) (*bldr_web_bundler_vite_compiler.Config, error) {
	// build list of ViteBundleMeta from bundleVar list
	var viteBundleMeta []*bldr_web_bundler_vite_compiler.ViteBundleMeta
	for _, bundleVar := range bundleVars {
		// build the entrypoints for this bundle
		var entrypoints []*bldr_web_bundler_vite_compiler.ViteBundleEntrypoint
		bundleConfigPaths := []string{}
		bundleDisableProjectConfig := disableProjectConfig

		for _, bundleVarEntrypoint := range bundleVar.GetEntrypointVars() {
			// validate entrypoint path
			if bundleVarEntrypoint.EntrypointPath == "" {
				return nil, errors.Errorf("entrypoint path is required for %s.%s",
					bundleVarEntrypoint.PkgImportPath, bundleVarEntrypoint.PkgVar)
			}

			// add to list
			entrypoints = append(entrypoints, &bldr_web_bundler_vite_compiler.ViteBundleEntrypoint{
				InputPath: filepath.Join(bundleVarEntrypoint.PkgCodePath, bundleVarEntrypoint.EntrypointPath),
			})

			// collect config paths and settings from entrypoints
			if len(bundleVarEntrypoint.ViteConfigPaths) > 0 {
				bundleConfigPaths = append(bundleConfigPaths, bundleVarEntrypoint.ViteConfigPaths...)
			}
			if bundleVarEntrypoint.DisableProjectConfig {
				bundleDisableProjectConfig = true
			}
		}

		// build the bundle metadata
		bundleMeta := &bldr_web_bundler_vite_compiler.ViteBundleMeta{
			Id:                   bundleVar.Id,
			Entrypoints:          entrypoints,
			ViteConfigPaths:      bundleConfigPaths,
			DisableProjectConfig: bundleDisableProjectConfig,
		}

		// add to the bundle meta list
		viteBundleMeta = append(viteBundleMeta, bundleMeta)
	}

	return &bldr_web_bundler_vite_compiler.Config{
		Bundles:              viteBundleMeta,
		WebPkgs:              webPkgs,
		ViteConfigPaths:      viteConfigPaths,
		DisableProjectConfig: disableProjectConfig,
	}, nil
}

// BuildViteBundleVarMeta builds the bundle metadata from the list of go variable defs.
func BuildViteBundleVarMeta(
	le *logrus.Entry,
	codeRootPath string,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*ViteDirective),
) ([]*ViteBundleVarMeta, error) {
	// bundles is the map of bundle-id to bundle-def
	bundles := make(map[string]*ViteBundleVarMeta)
	getBundle := func(bundleID string) *ViteBundleVarMeta {
		bundleDef := bundles[bundleID]
		if bundleDef != nil {
			return bundleDef
		}

		bundleDef = &ViteBundleVarMeta{Id: bundleID}
		bundles[bundleID] = bundleDef
		return bundleDef
	}

	// for each package variable, build a bundle definition + variable
	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, errors.Errorf("failed to find ast.File for package: %s", pkgImportPath)
		}

		pkgCodePath := filepath.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
		relPkgCodePath, err := filepath.Rel(codeRootPath, pkgCodePath)
		if err != nil {
			return nil, errors.Wrap(err, "unable to determine relative path")
		}

		for pkgVar, pkgViteDirective := range pkgVars {
			// validate entrypoint path
			if pkgViteDirective.EntrypointPath == "" {
				return nil, errors.Errorf("%s.%s: entrypoint path is required", pkgImportPath, pkgVar)
			}

			bundleID := pkgViteDirective.BundleID
			bundleDef := getBundle(bundleID)
			bundleDef.EntrypointVars = append(bundleDef.EntrypointVars, &ViteEntrypointVar{
				PkgImportPath:        pkgImportPath,
				PkgVar:               pkgVar,
				PkgVarType:           pkgViteDirective.ViteVarType,
				PkgCodePath:          relPkgCodePath,
				ViteConfigPaths:      pkgViteDirective.ViteConfigPaths,
				EntrypointPath:       pkgViteDirective.EntrypointPath,
				DisableProjectConfig: pkgViteDirective.DisableProjectConfig,
			})
		}
	}

	// sort entrypoint variables
	bundleVals := slices.Collect(maps.Values(bundles))
	for _, bundle := range bundleVals {
		bundle.SortEntrypointVars()
	}

	// sort by bundle id
	slices.SortFunc(bundleVals, func(a, b *ViteBundleVarMeta) int {
		return strings.Compare(a.GetId(), b.GetId())
	})

	return bundleVals, nil
}

// SortEntrypointVars sorts the entrypoint variables field.
func (m *ViteBundleVarMeta) SortEntrypointVars() {
	slices.SortFunc(m.EntrypointVars, func(a, b *ViteEntrypointVar) int {
		sa := a.PkgImportPath + "." + a.PkgVar
		sb := b.PkgImportPath + "." + b.PkgVar
		return strings.Compare(sa, sb)
	})
}
