package bldr_dist_compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	hydra_core "github.com/aperturerobotics/hydra/core"
	kvtx_kvfile "github.com/aperturerobotics/hydra/kvtx/kvfile"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildDistBundle builds the distribution bundle for an application.
//
// initEmbeddedWorld should initialize the embedded manifest world.
func BuildDistBundle(
	rctx context.Context,
	le *logrus.Entry,
	distSrcPath string,
	workingPath string,
	outputPath string,
	outBinName string,
	meta *bldr_dist.DistMeta,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
	hostConfigSet map[string]*configset_proto.ControllerConfig,
	initEmbeddedWorld func(ctx context.Context, embedEngine world.Engine, embedOpPeerID peer.ID) error,
	enableCgo bool,
) error {
	isRelease := buildType.IsRelease()
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Write the bldr license file.
	bldrLicense := bldr.GetLicense()
	if err := os.WriteFile(filepath.Join(outputPath, "LICENSE.bldr"), []byte(bldrLicense), 0o644); err != nil {
		return err
	}

	// NOTE: we use the go.mod from the parent program.
	// we compile under ${parent_program}/.bldr/build/...
	// the Go compiler will find the go.mod with reference to bldr in a parent dir

	// encode config set for embedded config set binary
	var hostConfigSetBin []byte
	if len(hostConfigSet) != 0 {
		configSetObj := &configset_proto.ConfigSet{
			Configurations: hostConfigSet,
		}
		var err error
		hostConfigSetBin, err = configSetObj.MarshalVT()
		if err != nil {
			return err
		}
	}

	// EntrypointBuildDir is the directory we will run "go build"
	entrypointBuildDir := filepath.Join(workingPath, "entrypoint")
	if err := os.MkdirAll(entrypointBuildDir, 0o755); err != nil {
		return err
	}

	// Write the configset bin file.
	outConfigSetFilename := "config-set.bin"
	if len(hostConfigSetBin) != 0 {
		outConfigSetPath := filepath.Join(entrypointBuildDir, outConfigSetFilename)
		if err := os.WriteFile(outConfigSetPath, hostConfigSetBin, 0o644); err != nil {
			return err
		}
	}

	// construct a new bus to hold our working volume
	le.Info("initializing embedded volume")
	workBus, workSr, err := hydra_core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	workSr.AddFactory(world_block_engine.NewFactory(workBus))

	workingDbDir := filepath.Join(workingPath, "dist-vol")
	if err := os.MkdirAll(workingDbDir, 0o755); err != nil {
		return err
	}

	storageOpts := default_storage.BuildStorage(workBus, workingDbDir)
	if len(storageOpts) == 0 {
		return errors.New("no available storage types for build system")
	}
	storage := storageOpts[0]
	storage.AddFactories(workBus, workSr)

	// run the node controller
	_, _, nref, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(
			&node_controller.Config{},
		),
		nil,
	)
	if err != nil {
		return err
	}
	defer nref.Release()

	// workingID is a unique working id to use
	// used to derive some at-rest crypto keys
	// may be replaced with something w/ more randomness later
	workingID := strings.Join([]string{ControllerID, meta.GetProjectId(), buildPlatform.GetPlatformID()}, "/")

	// start with a working db on-disk in the working dir
	workingDbVolID := "dist-working-vol"
	workingDbVolConf, err := storage.BuildVolumeConfig("dist-working-vol", &volume_controller.Config{
		VolumeIdAlias:           []string{workingDbVolID},
		DisablePeer:             true,
		DisableEventBlockRm:     true,
		DisableReconcilerQueues: true,
	})
	if err != nil {
		return err
	}

	workingVolCtrli, _, workingVolRef, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(workingDbVolConf),
		nil,
	)
	if err != nil {
		return err
	}
	defer workingVolRef.Release()
	_ = workingVolCtrli
	workingVolCtrl, ok := workingVolCtrli.(*volume_controller.Controller)
	if !ok {
		return errors.New("unexpected type for volume controller")
	}
	workingVol, err := workingVolCtrl.GetVolume(ctx)
	if err != nil {
		return err
	}
	boltVol, ok := workingVol.(common_kvtx.KvtxVolume)
	if !ok {
		return errors.New("unexpected type for volume")
	}

	// workingVol will be embedded in the dist binary & available to application.
	// it will contain the embedded manifests.

	// create the embedded manifests world
	embedWorldID := strings.Join([]string{"dist", meta.GetProjectId()}, "/")
	embedBucketID := embedWorldID
	embedObjStoreID := embedWorldID
	// TODO: do not replicate flag?
	embedXfrmConf, err := block_transform.NewConfig(buildEmbedTransformConf(workingID))
	if err != nil {
		return err
	}
	bucketConf, err := bucket.NewConfig(embedBucketID, 1, nil, &bucket.LookupConfig{
		// Disable: true,
	})
	if err != nil {
		return err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, workBus, bucket.NewApplyBucketConfig(bucketConf, nil, []string{workingDbVolID}))
	if err != nil {
		return err
	}
	embedEngineConf := world_block_engine.NewConfig(
		embedWorldID,
		workingDbVolID,
		embedBucketID,
		embedObjStoreID,
		&bucket.ObjectRef{TransformConf: embedXfrmConf.CloneVT()},
		nil,
	)
	embedEngineCtrli, _, embedEngineCtrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		workBus,
		resolver.NewLoadControllerWithConfig(embedEngineConf),
		nil,
	)
	if err != nil {
		return err
	}
	defer embedEngineCtrlRef.Release()
	embedEngineCtrl, ok := embedEngineCtrli.(*world_block_engine.Controller)
	if !ok {
		return errors.New("unexpected type for world block engine controller")
	}
	embedEngine, err := embedEngineCtrl.GetWorldEngine(ctx)
	if err != nil {
		return err
	}

	// Write contents to the embedded world.
	le.Debug("copying contents to embedded volume")
	if err := initEmbeddedWorld(ctx, embedEngine, workingVol.GetPeerID()); err != nil {
		return err
	}

	// Close the embedded world controller, no longer needed.
	embedEngineCtrlRef.Release()

	le.Debug("packing embedded volume to assets.kvfile")
	embeddedVolumeFilename := "assets.kvfile"
	embeddedVolumePath := filepath.Join(entrypointBuildDir, embeddedVolumeFilename)
	embeddedVolFile, err := os.OpenFile(embeddedVolumePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	// Access the workingVol kvtx
	workingVolKvtx := boltVol.GetKvtxStore()

	// Write the kvfile
	// NOTE: We don't use compression here since the content is already compressed / not compressable.
	// In testing, the zstd compression had NO reduction in file-size here.
	err = kvtx_kvfile.KvfileFromStore(ctx, embeddedVolFile, workingVolKvtx, nil)
	if err != nil {
		_ = embeddedVolFile.Close()
		return err
	}
	if err := embeddedVolFile.Close(); err != nil {
		return err
	}

	// build list of files to embed in the assets fs
	var embedAssetsFS []string
	if len(hostConfigSetBin) != 0 {
		embedAssetsFS = append(embedAssetsFS, outConfigSetFilename)
	}

	// on the Web platform we distribute the kvfile separately
	// we also name the entrypoint file differently
	var outBinPath string
	if buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_WEB {
		// output directory for the entrypoint
		outEntryDir := filepath.Join(outputPath, "entrypoint")
		if err := os.MkdirAll(outEntryDir, 0o755); err != nil {
			return err
		}

		// store the wasm file where the entrypoint expects.
		// TODO: name it based on outBinName and add a hash to the name
		outBinPath = filepath.Join(outEntryDir, "runtime.wasm")

		le.Debugf("copying %v to output directory", embeddedVolumeFilename)
		if err := fsutil.CopyFile(
			filepath.Join(outputPath, embeddedVolumeFilename),
			embeddedVolumePath,
			0o644,
		); err != nil {
			return err
		}

		// Write the URL to the kvfile
		embeddedVolumeURL := "../" + embeddedVolumeFilename
		outVolumeURLFilename := "assets.url"
		outVolumeURLPath := filepath.Join(entrypointBuildDir, outVolumeURLFilename)
		if err := os.WriteFile(outVolumeURLPath, []byte(embeddedVolumeURL), 0o644); err != nil {
			return err
		}
		embedAssetsFS = append(embedAssetsFS, outVolumeURLFilename)

		// Compile the bldr entrypoint (js bundle and index.html)
		le.Debug("building browser bundle")
		entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
		err := entrypoint_browser_bundle.BuildBrowserBundle(
			ctx,
			le,
			distSrcPath,
			outputPath,
			// web-document is located under /pkgs/@aptre/bldr
			"./entrypoint/runtime-wasm.mjs",
			isRelease,
			false,
		)
		if err != nil {
			return err
		}

		le.Info("building web wasm entrypoint")
		err = browser_build.BuildWasmRuntimeEntrypoint(
			ctx,
			le,
			distSrcPath,
			outEntryDir,
			buildType,
			buildPlatform,
		)
		if err != nil {
			return err
		}
	} else {
		// otherwise we go:embed it
		embedAssetsFS = append(embedAssetsFS, embeddedVolumeFilename)
		outBinPath = filepath.Join(outputPath, outBinName)
	}

	// Format and write the main.go file.
	le.Debug("compiling dist entrypoint")
	entrypointSrc := FormatDistEntrypoint(meta, embedAssetsFS)
	entrypointMainPath := filepath.Join(entrypointBuildDir, "main.go")
	if err := os.WriteFile(entrypointMainPath, []byte(entrypointSrc), 0o644); err != nil {
		return err
	}

	err = gocompiler.ExecBuildEntrypoint(
		le,
		buildPlatform,
		buildType,
		entrypointBuildDir,
		outBinPath,
		enableCgo,
		nil,
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}
