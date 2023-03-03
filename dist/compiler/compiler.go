package dist_compiler

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/aperturerobotics/bldr"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/bldr/util/fsutil"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// BuildDistBundle builds the distribution bundle for an application.
//
// baseGoMod, baseGoSum should be the go.mod go.sum files from the .bldr distribution src dir.
// appID is used to control the app storage path and/or web storage db name.
func BuildDistBundle(
	ctx context.Context,
	le *logrus.Entry,
	baseGoMod, baseGoSum []byte,
	workingPath, outputPath string,
	worldState world.WorldState,
	distPlatformID string,
	embedPluginManifests []*bucket.ObjectRef,
	startPlugins []string,
	appID string,
) error {
	// Write the bldr license file.
	bldrLicense := bldr.GetLicense()
	if err := os.WriteFile(path.Join(workingPath, "LICENSE"), []byte(bldrLicense), 0644); err != nil {
		return err
	}

	// Adjust the go.mod module name to "entrypoint"
	moduleName := LabelToPackageName("app", appID)
	outGoModPath := path.Join(workingPath, "go.mod")
	outModFile, err := modfile.ParseLax(outGoModPath, baseGoMod, func(path, version string) (string, error) { return version, nil })
	if err != nil {
		return err
	}
	outModFile.AddModuleStmt(moduleName)
	outModFile.Cleanup()

	// Write the go.mod.
	goModOut, err := outModFile.Format()
	if err != nil {
		return err
	}
	if err := os.WriteFile(outGoModPath, goModOut, 0644); err != nil {
		return err
	}

	// Write the go.sum
	if err := os.WriteFile(path.Join(workingPath, "go.sum"), baseGoSum, 0644); err != nil {
		return err
	}

	// Extract the static plugins to disk so "go/embed" can use them.
	pluginRoot := "plugin"
	pluginWorkingRoot := path.Join(workingPath, pluginRoot)
	pluginPackageNames := make([]string, len(embedPluginManifests))
	pluginPackagePaths := make([]string, len(embedPluginManifests))
	for embedPluginIdx, embedPluginRef := range embedPluginManifests {
		if err := plugin_host.AccessPluginManifest(
			ctx,
			le,
			worldState.AccessWorldState,
			embedPluginRef,
			func(
				ctx context.Context,
				bls *bucket_lookup.Cursor,
				bcs *block.Cursor,
				manifest *plugin.PluginManifest,
				distFS *unixfs.FS,
				assetsFS *unixfs.FS,
			) error {
				embedPluginID := manifest.GetMeta().GetPluginId()
				embedPluginWorkingRoot := path.Join(pluginWorkingRoot, embedPluginID)
				embedPluginPackageName := LabelToPackageName("plugin", embedPluginID)
				pluginPackageNames[embedPluginIdx] = embedPluginPackageName
				pluginPackagePaths[embedPluginIdx] = path.Join(moduleName, pluginRoot, embedPluginID)

				// sync the plugin dist fs to dist/
				embedPluginDistRoot := path.Join(embedPluginWorkingRoot, "dist")
				if err := os.MkdirAll(embedPluginDistRoot, 0755); err != nil {
					return err
				}
				distFSHandle, err := distFS.AddRootReference(ctx)
				if err != nil {
					return err
				}
				err = unixfs_sync.Sync(
					ctx,
					embedPluginDistRoot,
					distFSHandle,
					unixfs_sync.DeleteMode_DeleteMode_BEFORE,
					nil,
				)
				distFSHandle.Release()
				if err != nil {
					return err
				}

				// sync the plugin asset fs to assets/
				embedPluginAssetsRoot := path.Join(embedPluginWorkingRoot, "assets")
				if err := os.MkdirAll(embedPluginAssetsRoot, 0755); err != nil {
					return err
				}
				assetsFSHandle, err := assetsFS.AddRootReference(ctx)
				if err != nil {
					return err
				}
				err = unixfs_sync.Sync(
					ctx,
					embedPluginAssetsRoot,
					assetsFSHandle,
					unixfs_sync.DeleteMode_DeleteMode_BEFORE,
					nil,
				)
				distFSHandle.Release()
				if err != nil {
					return err
				}
				assetsEmpty, err := fsutil.CheckDirEmpty(embedPluginAssetsRoot)
				if err != nil {
					return err
				}
				if assetsEmpty {
					le.Debug("assets fs is empty, touching placeholder file")
					err = os.WriteFile(path.Join(embedPluginAssetsRoot, "empty"), nil, 0644)
					if err != nil {
						return err
					}
				}

				// write the plugin definition file static-plugin.go
				staticPluginFile := FormatStaticPluginFile(
					embedPluginPackageName,
					embedPluginID,
					manifest.Entrypoint,
					manifest.GetMeta().GetBuildType(),
				)
				staticPluginFilePath := path.Join(embedPluginWorkingRoot, "static-plugin.go")
				if err := os.WriteFile(staticPluginFilePath, []byte(staticPluginFile), 0644); err != nil {
					return err
				}

				// done
				return nil
			},
		); err != nil {
			return err
		}
	}

	// Format and write the main.go file.
	entrypointSrc := FormatEntrypoint(appID, pluginPackageNames, pluginPackagePaths, startPlugins)
	outEntrypointPath := path.Join(workingPath, "main.go")
	if err := os.WriteFile(outEntrypointPath, entrypointSrc, 0644); err != nil {
		return err
	}

	// TODO: compiler
	return errors.New("TODO")
}
