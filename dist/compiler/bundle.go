//go:build !js

package bldr_dist_compiler

import (
	"context"
	"encoding/base32"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	dist_compiler_bundle "github.com/aperturerobotics/bldr/dist/compiler/bundle"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	bldr_compress "github.com/aperturerobotics/bldr/util/compress"
	"github.com/aperturerobotics/bldr/util/gocompiler"
	browser_build "github.com/aperturerobotics/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/go-kvfile"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	hydra_core "github.com/aperturerobotics/hydra/core"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/util/enabled"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// BuildDistBundle builds the distribution bundle for an application.
//
// initEmbeddedWorld should initialize the embedded manifest world.
func BuildDistBundle(
	rctx context.Context,
	le *logrus.Entry,
	srcPath string,
	distSrcPath string,
	webStartupSrcPath string,
	workingPath string,
	outputPath string,
	outBinName string,
	meta *bldr_dist.DistMeta,
	buildType bldr_manifest.BuildType,
	buildPlatform bldr_platform.Platform,
	hostConfigSet map[string]*configset_proto.ControllerConfig,
	initEmbeddedWorld func(ctx context.Context, embedEngine world.Engine, embedOpPeerID peer.ID) error,
	enableCgoOpt enabled.Enabled,
	enableTinygoOpt enabled.Enabled,
	enableCompressionOpt enabled.Enabled,
) error {
	isRelease := buildType.IsRelease()
	isWebPlatform := buildPlatform.GetExecutableExt() == ".wasm"

	// disable cgo on default
	enableCgo := enableCgoOpt.IsEnabled(false)
	// enable compression for release mode only on default
	enableCompression := enableCompressionOpt.IsEnabled(isRelease)
	// enable tinygo on the web platform in release mode on default
	tinygoSupported := false // TODO: TinyGo cannot yet build Bldr successfully.
	enableTinygo := isWebPlatform && enableTinygoOpt.IsEnabled(isRelease && tinygoSupported)

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
			Configs: hostConfigSet,
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
	embedWorldID := bldr_dist.DistWorldEngineID
	embedObjStoreID := embedWorldID
	bucketConf, err := bldr_dist.NewDistBucketConfig(meta.GetProjectId())
	if err != nil {
		return err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, workBus, bucket.NewApplyBucketConfig(bucketConf, nil, []string{workingDbVolID}))
	if err != nil {
		return err
	}
	embedXfrmConf, err := block_transform.NewConfig(buildEmbedTransformConf(workingID))
	if err != nil {
		return err
	}

	embedEngineConf := world_block_engine.NewConfig(
		embedWorldID,
		workingDbVolID,
		bucketConf.GetId(),
		embedObjStoreID,
		&bucket.ObjectRef{TransformConf: embedXfrmConf.CloneVT()},
		nil,
		false,
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
	embedBlockEngine, ok := embedEngine.(*world_block.Engine)
	if !ok {
		return errors.New("unexpected type for world block engine")
	}

	// Write contents to the embedded world.
	le.Debug("copying contents to embedded volume")
	if err := initEmbeddedWorld(ctx, embedEngine, workingVol.GetPeerID()); err != nil {
		return err
	}

	// Update the initial root ref
	meta.DistWorldRef = embedBlockEngine.GetRootRef().Clone()

	// Close the embedded world controller, no longer needed.
	embedEngineCtrlRef.Release()

	// Validate the metadata
	if err := meta.Validate(); err != nil {
		return err
	}

	le.Debug("packing embedded volume to assets.kvfile")
	embeddedVolumeFilename := "assets.kvfile"
	embeddedVolumePath := filepath.Join(entrypointBuildDir, embeddedVolumeFilename)
	embeddedVolFile, err := os.OpenFile(embeddedVolumePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	var embeddedVolumeWrite io.Writer = embeddedVolFile
	var embeddedVolumeHash hash.Hash
	if isWebPlatform {
		// on the web platform add a hash to the filename to cache miss when the file changes
		embeddedVolumeHash = blake3.New()
		_, _ = embeddedVolumeHash.Write([]byte("bldr hash " + embeddedVolumeFilename + " Fri May  3 21:35:53 PDT 2024 embedded volume"))
		embeddedVolumeWrite = io.MultiWriter(embeddedVolFile, embeddedVolumeHash)
	}

	// build kvfile writer
	kvfileWriter := kvfile.NewWriter(embeddedVolumeWrite)
	kvfileKvkey := store_kvkey.NewDefaultKVKey()
	kvfileBlockPrefix := kvfileKvkey.GetBlockFullPrefix()

	// Access the workingVol kvtx
	kvtxVolStore := boltVol.GetKvtxStore()
	kvtxVolBlockPrefix := boltVol.GetKvKey().GetBlockFullPrefix()

	// Write the kvfile
	// NOTE: We don't use compression here since the content is already compressed / not compressable.
	err = dist_compiler_bundle.BundleManifestsKvfile(
		ctx,
		le,
		kvfileWriter,
		kvfileBlockPrefix,
		embedBlockEngine,
		kvtxVolStore,
		kvtxVolBlockPrefix,
	)
	if err != nil {
		_ = kvfileWriter.Close()
		_ = embeddedVolFile.Close()
		return err
	}
	if err := kvfileWriter.Close(); err != nil {
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
	if isWebPlatform {
		// compute the hash for the path
		entrypointHash := strings.ToLower(base32.StdEncoding.EncodeToString(embeddedVolumeHash.Sum(nil))[:8])

		// output directory for the entrypoint with hash
		outEntryDir := filepath.Join(outputPath, "entrypoint", entrypointHash)
		if err := os.MkdirAll(outEntryDir, 0o755); err != nil {
			return err
		}

		embeddedVolumeOutputPath := filepath.Join(outEntryDir, "assets.kvfile")
		le.Debugf("copying %v to output as %v", embeddedVolumeFilename, embeddedVolumeOutputPath)
		if err := fsutil.CopyFile(
			embeddedVolumeOutputPath,
			embeddedVolumePath,
			0o644,
		); err != nil {
			return err
		}

		// Write the URL to the kvfile - adjust path to include hash
		embeddedVolumeURL := "../" + entrypointHash + "/assets.kvfile"
		outVolumeURLFilename := "assets.url"
		outVolumeURLPath := filepath.Join(entrypointBuildDir, outVolumeURLFilename)
		if err := os.WriteFile(outVolumeURLPath, []byte(embeddedVolumeURL), 0o644); err != nil {
			return err
		}
		embedAssetsFS = append(embedAssetsFS, outVolumeURLFilename)

		// entrypoint is located under /entrypoint/{hash}/pkgs/@aptre/bldr
		entrypointToRootPrefix := "../../../../../"

		// Compile the bldr entrypoint (js bundle and index.html)
		le.Debug("building browser bundle")
		entrypoint_browser_bundle.EsbuildLogLevel = esbuild.LogLevelError
		err := entrypoint_browser_bundle.BuildBrowserBundle(
			ctx,
			le,
			srcPath,
			distSrcPath,
			outputPath,
			"./entrypoint/"+entrypointHash+"/runtime-wasm.mjs",
			entrypointToRootPrefix+"sw.mjs",
			entrypointToRootPrefix+"shw.mjs",
			webStartupSrcPath, // startupPath
			entrypointHash,
			isRelease, // minify
			false,     // devMode
		)
		if err != nil {
			return err
		}

		outWasmRelPath := "./runtime.wasm"
		if enableCompression {
			outWasmRelPath += ".gz"
		}

		le.Info("building web wasm entrypoint script")
		err = browser_build.BuildWasmRuntimeEntrypoint(
			ctx,
			le,
			distSrcPath,
			outEntryDir,
			buildType,
			enableTinygo,
			outWasmRelPath,
		)
		if err != nil {
			return err
		}

		// store the wasm file where the entrypoint expects.
		outBinPath = filepath.Join(outEntryDir, "runtime.wasm")
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

	// compile runtime.wasm or the native entrypoint
	err = gocompiler.ExecBuildEntrypoint(
		ctx,
		le,
		buildPlatform,
		buildType,
		entrypointBuildDir,
		outBinPath,
		enableCgo,
		enableTinygo,
		nil,
		nil,
	)
	if err != nil {
		return err
	}

	// We must use gzip compression here since DecompressStream does not support brotli.
	if isWebPlatform && enableCompression {
		gzPath, err := bldr_compress.CompressGzip(ctx, le, workingPath, outBinPath)
		if err != nil {
			return err
		}
		err = os.Remove(outBinPath)
		if err != nil {
			return err
		}

		// TODO: these values should be used?
		outBinName = filepath.Base(gzPath)
		outBinPath = gzPath
		_, _ = outBinName, outBinPath
	}

	return nil
}
