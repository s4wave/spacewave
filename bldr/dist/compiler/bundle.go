//go:build !js

package bldr_dist_compiler

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/enabled"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	spacewave "github.com/s4wave/spacewave"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	dist_compiler_bundle "github.com/s4wave/spacewave/bldr/dist/compiler/bundle"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	bldr_compress "github.com/s4wave/spacewave/bldr/util/compress"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	browser_build "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	web_runtime_goscript_build "github.com/s4wave/spacewave/bldr/web/runtime/goscript/build"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func browserIceServersForBundle(servers []*IceServer) []entrypoint_browser_bundle.BrowserIceServer {
	trusted := make([]entrypoint_browser_bundle.BrowserIceServer, 0, len(servers))
	for _, server := range servers {
		if server == nil || len(server.GetUrls()) == 0 {
			continue
		}
		trusted = append(trusted, entrypoint_browser_bundle.BrowserIceServer{
			URLs:       append([]string(nil), server.GetUrls()...),
			Username:   server.GetUsername(),
			Credential: server.GetCredential(),
		})
	}
	return trusted
}

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
	buildPolicy *bldr_manifest_build.BuildPolicy,
	buildPlatform bldr_platform.Platform,
	hostConfigSet map[string]*configset_proto.ControllerConfig,
	initEmbeddedWorld func(ctx context.Context, embedEngine world.Engine, embedOpPeerID peer.ID) error,
	cliImports map[string]string,
	enableCgoOpt enabled.Enabled,
	goCompilerOpt plugin_compiler_go.GoCompiler,
	enableCompressionOpt enabled.Enabled,
	browserIceServers []*IceServer,
	browserIceServersEndpoint string,
) error {
	isRelease := buildType.IsRelease()
	isWebPlatform := bldr_platform.IsWebPlatform(buildPlatform)
	jsMinify := buildPolicy.ResolveJsMinification(buildType)
	jsSourcemaps := buildPolicy.ResolveJsSourcemaps(buildType)
	goScriptCodeSplitting := buildPolicy.ResolveGoScriptCodeSplitting(buildType)

	// disable cgo on default
	enableCgo := enableCgoOpt.IsEnabled(false)
	// enable compression for release mode only on default
	enableCompression := enableCompressionOpt.IsEnabled(isRelease)
	goCompiler, err := resolveDistGoCompiler(buildPlatform, goCompilerOpt)
	if err != nil {
		return err
	}
	enableTinygo := goCompiler.IsTinyGo()
	useGoScript := goCompiler.IsGoScript()

	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Write the bldr license file.
	bldrLicense := spacewave.GetLicense()
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

	// construct a minimal bus with only the factories needed for dist builds
	le.Info("initializing embedded volume")
	workBus, workSr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	workSr.AddFactory(node_controller.NewFactory(workBus))
	workSr.AddFactory(bucket_setup.NewFactory(workBus))
	workSr.AddFactory(lookup_concurrent.NewFactory(workBus))
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
		// NewDistBucketConfig uses the static entrypoint block store id as the
		// fallback store. During build, before assets.kvfile exists, point that
		// lookup at the temporary working volume so embedded-world bootstrap can
		// read blocks from the same backing store it is populating.
		VolumeIdAlias:       []string{workingDbVolID, bldr_dist.StaticBlockStoreID},
		DisablePeer:         true,
		DisableEventBlockRm: true,
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
		embeddedVolumeHash = sha256.New()
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

	writeDistEntrypoint := func(embedAssets bool) error {
		le.Debug("writing dist entrypoint")
		entrypointSrc := FormatDistEntrypoint(meta, embedAssetsFS, cliImports, embedAssets)
		entrypointMainPath := filepath.Join(entrypointBuildDir, "main.go")
		return os.WriteFile(entrypointMainPath, []byte(entrypointSrc), 0o644)
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

		// TinyGo release bundles use the conservative MessagePort transport.
		// The SAB/OPFS worker transport can strand startup RPCs after the
		// quickstart frame is ready, leaving the browser stuck before content.
		forceMessagePortWorkerComms := enableTinygo

		runtimeWorkerName := "runtime-wasm.mjs"
		if useGoScript {
			runtimeWorkerName = "runtime-goscript.mjs"
		}
		runtimeWorkerPath := "/entrypoint/" + entrypointHash + "/" + runtimeWorkerName

		trustedIceServers := browserIceServersForBundle(browserIceServers)

		// Compile the bldr entrypoint (js bundle and index.html)
		le.Debug("building browser bundle")
		bundleResult, err := entrypoint_browser_bundle.BuildBrowserBundle(
			ctx,
			le,
			workingPath,
			srcPath,
			distSrcPath,
			outputPath,
			runtimeWorkerPath,
			entrypointToRootPrefix+"sw.mjs",
			entrypointToRootPrefix+"shw.mjs",
			webStartupSrcPath, // startupPath
			entrypointHash,
			jsMinify,                    // minify
			jsSourcemaps,                // sourcemaps
			false,                       // devMode
			false,                       // forceDedicatedWorkers
			forceMessagePortWorkerComms, // forceMessagePortWorkerComms
			trustedIceServers,           // browserIceServers
			browserIceServersEndpoint,   // browserIceServersEndpoint
		)
		if err != nil {
			return err
		}

		if err := writeDistEntrypoint(false); err != nil {
			return err
		}

		var wasmManifestPath string
		if useGoScript {
			le.Info("compiling dist TypeScript package tree")
			goScriptBuildFlags := newDistGoScriptBuildFlags(buildType, enableCgo)
			goScriptEnv, err := newDistGoScriptEnv(buildPlatform)
			if err != nil {
				return err
			}
			goScriptOverrideDirs := existingSourceDirs(srcPath, "gs")
			goScriptBindingRoots, err := gocompiler.GoScriptBindingRoots(ctx, entrypointBuildDir, goScriptEnv...)
			if err != nil {
				return err
			}
			mainPackagePath, err := gocompiler.GoListImportPath(ctx, entrypointBuildDir, goScriptBuildFlags, goScriptEnv...)
			if err != nil {
				return err
			}
			goScriptOutputPath := filepath.Join(workingPath, "dist-goscript")
			goScriptCacheRoot, err := gocompiler.GoScriptCompilerCacheRootFromEnv(workingPath)
			if err != nil {
				return err
			}
			if err := gocompiler.ExecGoScriptCompile(ctx, le, gocompiler.GoScriptCompileOptions{
				WorkDir:                   entrypointBuildDir,
				OutputPath:                goScriptOutputPath,
				CacheRoot:                 goScriptCacheRoot,
				Packages:                  []string{"."},
				BuildFlags:                goScriptBuildFlags,
				Env:                       goScriptEnv,
				OverrideDirs:              goScriptOverrideDirs,
				BindingRoots:              goScriptBindingRoots,
				AllDependencies:           true,
				ProtobufTypeScriptBinding: true,
			}); err != nil {
				return err
			}
			_, err = web_runtime_goscript_build.BuildWebGoScriptRuntimeScript(
				ctx,
				le,
				distSrcPath,
				entrypointBuildDir,
				goScriptOutputPath,
				filepath.Join(outEntryDir, runtimeWorkerName),
				mainPackagePath,
				jsMinify,
				jsSourcemaps,
				goScriptCodeSplitting,
			)
			if err != nil {
				return err
			}
		} else {
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
				jsMinify,
				jsSourcemaps,
				enableTinygo,
				outWasmRelPath,
			)
			if err != nil {
				return err
			}

			// store the wasm file where the entrypoint expects.
			outBinPath = filepath.Join(outEntryDir, "runtime.wasm")
			wasmManifestPath = "entrypoint/" + entrypointHash + "/runtime.wasm"
			if enableCompression {
				wasmManifestPath += ".gz"
			}
		}

		// write manifest.json for the prerender build script
		manifest := &entrypoint_browser_bundle.BuildManifest{
			Entrypoint:                 bundleResult.EntrypointPath,
			EntrypointDecompressedSize: bundleResult.EntrypointDecompressedSize,
			ServiceWorker:              bundleResult.ServiceWorkerFilename,
			SharedWorker:               bundleResult.SharedWorkerFilename,
			OpfsWorker:                 bundleResult.OpfsWorkerFilename,
			Wasm:                       wasmManifestPath,
			CSS:                        bundleResult.CSSPaths,
		}
		if err := entrypoint_browser_bundle.WriteBuildManifest(outputPath, manifest); err != nil {
			return err
		}
	} else {
		// otherwise we go:embed it
		embedAssetsFS = append(embedAssetsFS, embeddedVolumeFilename)
		outBinPath = filepath.Join(outputPath, outBinName)
		if err := writeDistEntrypoint(true); err != nil {
			return err
		}
	}

	if isWebPlatform && useGoScript {
		return nil
	}

	// compile runtime.wasm or the native entrypoint
	le.Debug("compiling dist entrypoint")
	err = gocompiler.ExecBuildEntrypoint(
		ctx,
		le,
		buildPlatform,
		buildType,
		entrypointBuildDir,
		outBinPath,
		enableCgo,
		enableTinygo,
		gocompiler.RuntimeStartupTraceBuildTagsForWebWasm(isWebPlatform, enableTinygo),
		distEntrypointLDFlags(buildPlatform, meta.GetEntrypointRole()),
	)
	if err != nil {
		return err
	}

	// Gzip compress the wasm binary for web distribution.
	// The browser decompresses via DecompressionStream('gzip').
	// Brotli is not supported by DecompressionStream.
	if isWebPlatform && enableCompression {
		if _, err := bldr_compress.CompressGzip(ctx, le, workingPath, outBinPath); err != nil {
			return err
		}
		if err := os.Remove(outBinPath); err != nil {
			return err
		}
	}

	return nil
}

func distEntrypointLDFlags(buildPlatform bldr_platform.Platform, entrypointRole string) []string {
	native, ok := buildPlatform.(*bldr_platform.NativePlatform)
	if !ok || native.GetGOOS() != "windows" || entrypointRole != bldr_dist.EntrypointRoleDesktop {
		return nil
	}
	return []string{"-H=windowsgui"}
}

func resolveDistGoCompiler(
	buildPlatform bldr_platform.Platform,
	goCompilerOpt plugin_compiler_go.GoCompiler,
) (gocompiler.GoCompiler, error) {
	resolvedGoCompilerOpt, err := goCompilerOpt.GoCompiler()
	if err != nil {
		return "", err
	}
	goCompiler, err := gocompiler.ResolveGoCompiler(
		buildPlatform,
		resolvedGoCompilerOpt,
		false,
	)
	if err != nil {
		return "", err
	}
	return goCompiler, nil
}

func newDistGoScriptBuildFlags(buildType bldr_manifest.BuildType, enableCgo bool) []string {
	buildTags := gocompiler.NewBuildTags(buildType, enableCgo)
	buildTags = append(buildTags, gocompiler.GoScriptBuildTag, gocompiler.SQLLiteBuildTag)
	buildTags = append(buildTags, gocompiler.RuntimeStartupTraceBuildTags()...)
	return []string{"-tags=" + strings.Join(buildTags, ",")}
}

func newDistGoScriptEnv(platform bldr_platform.Platform) ([]string, error) {
	return bldr_platform_go.PlatformToGoEnv(platform)
}

func existingSourceDirs(root string, names ...string) []string {
	var dirs []string
	for _, name := range names {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}
