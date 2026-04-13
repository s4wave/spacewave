//go:build !js

package bldr_cli_compiler

import (
	"context"
	"os"
	"path"
	"path/filepath"

	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler_go "github.com/aperturerobotics/bldr/plugin/compiler/go"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	"golang.org/x/mod/modfile"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "cli compiler controller"

// Controller is the CLI compiler controller.
type Controller struct {
	*bus.BusController[*Config]
}

// Factory is the factory for the CLI compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewFactory constructs a new CLI compiler controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		NewConfig,
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// SupportsStartupManifestCache returns true if startup cache reuse is safe.
func (c *Controller) SupportsStartupManifestCache() bool {
	return false
}

// BuildManifest compiles the CLI manifest once with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	platformID := meta.GetPlatformId()
	manifestID := meta.GetManifestId()
	sourcePath := builderConf.GetSourcePath()
	workingPath := builderConf.GetWorkingPath()

	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("platform-id", platformID)
	le.Debug("building CLI manifest")

	// clean / create dist dir
	outDistPath := filepath.Join(workingPath, "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	// clean / create assets dir (empty for CLI)
	outAssetsPath := filepath.Join(workingPath, "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// entrypoint build dir
	entrypointBuildDir := filepath.Join(workingPath, "entrypoint")
	if err := os.MkdirAll(entrypointBuildDir, 0o755); err != nil {
		return nil, err
	}

	// read go.mod to resolve relative package paths
	goModPath := filepath.Join(sourcePath, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}
	rootModule := modfile.ModulePath(goModData)

	// analyze go packages for factory discovery
	// AnalyzePackages handles ./ relative path resolution internally
	le.Debug("analyzing packages for factory discovery")
	analysis, err := plugin_compiler_go.AnalyzePackages(
		ctx, le, sourcePath, conf.GetGoPkgs(), nil,
	)
	if err != nil {
		return nil, err
	}

	// build factory imports from analyzed packages
	factoryImports := make(map[string]string)
	for _, pkg := range analysis.GetLoadedPackages() {
		if pkg.Types.Scope().Lookup("NewFactory") == nil {
			continue
		}
		factoryImports[pkg.PkgPath] = plugin_compiler_go.BuildPackageName(pkg.Types)
	}

	// resolve cli package paths
	cliPkgs, _ := plugin_compiler_go.UpdateRelativeGoPackagePaths(
		conf.GetCliPkgs(), rootModule,
	)
	cliImports := make(map[string]string)
	for _, pkg := range cliPkgs {
		cliImports[pkg] = path.Base(pkg)
	}

	// serialize config set
	configSetPath := filepath.Join(entrypointBuildDir, "configset.bin")
	configSet := conf.GetConfigSet()
	if len(configSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{Configs: configSet}
		data, err := configSetObj.MarshalVT()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(configSetPath, data, 0o644); err != nil {
			return nil, err
		}
	} else {
		// write empty file for the go:embed directive
		if err := os.WriteFile(configSetPath, nil, 0o644); err != nil {
			return nil, err
		}
	}

	// determine app name
	appName := manifestID
	if projID := conf.GetProjectId(); projID != "" {
		appName = projID
	}

	// generate entrypoint main.go
	entrypointSrc, err := FormatCliEntrypoint(appName, factoryImports, cliImports)
	if err != nil {
		return nil, err
	}
	entrypointMainPath := filepath.Join(entrypointBuildDir, "main.go")
	if err := os.WriteFile(entrypointMainPath, entrypointSrc, 0o644); err != nil {
		return nil, err
	}

	// compile the binary
	outBinName := manifestID + buildPlatform.GetExecutableExt()
	outBinPath := filepath.Join(outDistPath, outBinName)
	le.Debug("compiling CLI entrypoint")
	err = gocompiler.ExecBuildEntrypoint(
		ctx,
		le,
		buildPlatform,
		"dev",
		entrypointBuildDir,
		outBinPath,
		false, // enableCgo
		false, // useTinygo
		nil,   // buildTags
		nil,   // ldFlags
	)
	if err != nil {
		return nil, err
	}

	// commit the manifest
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())
	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("committing CLI manifest")
	committedManifest, committedManifestRef, err := builderConf.CommitManifestWithPaths(
		ctx,
		le,
		tx,
		meta,
		outBinName,
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	le.Debug("CLI build complete")
	return bldr_manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		bldr_manifest_builder.NewInputManifest(nil, nil),
	), nil
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
func (c *Controller) GetSupportedPlatforms() []string {
	return []string{bldr_platform.PlatformID_DESKTOP}
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*Controller)(nil))
