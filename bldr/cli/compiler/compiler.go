//go:build !js

package bldr_cli_compiler

import (
	"context"
	"encoding/binary"
	"go/types"
	"os"
	"path/filepath"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/s4wave/spacewave/db/world"
	"golang.org/x/mod/modfile"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

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

// marshalConfigSetDeterministic encodes the ConfigSet map in key order so the
// generated configset.bin is stable across builds.
func marshalConfigSetDeterministic(
	configs map[string]*configset_proto.ControllerConfig,
) ([]byte, error) {
	keys := make([]string, 0, len(configs))
	for key := range configs {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var result []byte
	for _, key := range keys {
		value, err := configs[key].MarshalVT()
		if err != nil {
			return nil, err
		}
		var entry []byte
		entry = append(entry, 0x0a)
		entry = binary.AppendUvarint(entry, uint64(len(key)))
		entry = append(entry, key...)
		entry = append(entry, 0x12)
		entry = binary.AppendUvarint(entry, uint64(len(value)))
		entry = append(entry, value...)
		result = append(result, 0x0a)
		result = binary.AppendUvarint(result, uint64(len(entry)))
		result = append(result, entry...)
	}
	return result, nil
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
	// Match analysis GOOS/GOARCH to the target so factories gated on
	// platform-specific build tags are excluded from the generated factory
	// list when targeting js/wasm or another non-host platform.
	var analyzeGOOS, analyzeGOARCH string
	if native, ok := buildPlatform.(*bldr_platform.NativePlatform); ok {
		analyzeGOOS = native.GetGOOS()
		analyzeGOARCH = native.GetGOARCH()
	}
	analysis, err := plugin_compiler_go.AnalyzePackages(
		ctx, le, sourcePath, conf.GetGoPkgs(), nil, analyzeGOOS, analyzeGOARCH, true,
	)
	if err != nil {
		return nil, err
	}

	// build factory imports from analyzed packages
	factoryImports := make(map[string]FactoryImport)
	for _, pkg := range analysis.GetLoadedPackages() {
		newFactoryObj := pkg.Types.Scope().Lookup("NewFactory")
		if newFactoryObj == nil {
			continue
		}
		sig, ok := newFactoryObj.Type().(*types.Signature)
		if !ok {
			return nil, errors.Errorf("package %s NewFactory is not a function", pkg.PkgPath)
		}
		passBus, err := plugin_compiler_go.FactoryNeedsBus(pkg.PkgPath, sig)
		if err != nil {
			return nil, err
		}
		factoryImports[pkg.PkgPath] = FactoryImport{
			Path:    pkg.PkgPath,
			Alias:   plugin_compiler_go.BuildPackageName(pkg.Types),
			PassBus: passBus,
		}
	}

	// resolve cli package paths
	cliPkgs, _ := plugin_compiler_go.UpdateRelativeGoPackagePaths(
		conf.GetCliPkgs(), rootModule,
	)
	cliImports, err := AnalyzeCliImports(ctx, le, sourcePath, cliPkgs, analyzeGOOS, analyzeGOARCH)
	if err != nil {
		return nil, err
	}

	// serialize config set
	configSetPath := filepath.Join(entrypointBuildDir, "configset.bin")
	configSet := conf.GetConfigSet()
	if len(configSet) != 0 {
		data, err := marshalConfigSetDeterministic(configSet)
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

	// determine app name and storage project id
	appName := manifestID
	projectID := conf.GetProjectId()

	// generate entrypoint main.go
	entrypointSrc, err := FormatCliEntrypoint(appName, projectID, factoryImports, cliImports)
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
var _ bldr_manifest_builder.Controller = (*Controller)(nil)
