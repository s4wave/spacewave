//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	filehash "github.com/aperturerobotics/bldr/util/filehash"
	"github.com/aperturerobotics/bldr/util/node"
	"github.com/aperturerobotics/bldr/util/pipesock"
	singleton_muxed_conn "github.com/aperturerobotics/bldr/util/singleton-muxed-conn"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	bldr_web_bundler_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	esbuild "github.com/evanw/esbuild/pkg/api"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// viteBundlerTracker is a running Vite compiler instance.
type viteBundlerTracker struct {
	// c is the controller
	c *Controller
	// key is the vite compiler key
	key viteBundlerKey
	// le is the logger
	le *logrus.Entry
	// instancePromiseCtr contains the vite compiler rpc instance or any error running it
	instancePromiseCtr *promise.PromiseContainer[bldr_vite.SRPCViteBundlerClient]
}

// viteBundlerKey is a composite key for identifying a Vite bundler instance.
type viteBundlerKey struct {
	// distPath is the root path to the dist sources
	distPath string
	// sourcePath is the root path of the source code
	sourcePath string
	// workingPath is the path to the working directory
	workingPath string
	// bundleID is the ID of the Vite bundle
	bundleID string
}

// newViteBundlerKey creates a new viteBundlerKey with the given parameters.
func newViteBundlerKey(distPath, sourcePath, workingPath, bundleID string) viteBundlerKey {
	return viteBundlerKey{
		distPath:    distPath,
		sourcePath:  sourcePath,
		workingPath: workingPath,
		bundleID:    bundleID,
	}
}

// buildViteCompilerTracker returns a function that constructs a new Vite compiler tracker.
func (c *Controller) buildViteCompilerTracker(key viteBundlerKey) (keyed.Routine, *viteBundlerTracker) {
	le := c.GetLogger().WithField("vite-bundle", key.bundleID)
	tr := &viteBundlerTracker{
		c:                  c,
		key:                key,
		le:                 le,
		instancePromiseCtr: promise.NewPromiseContainer[bldr_vite.SRPCViteBundlerClient](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *viteBundlerTracker) execute(ctx context.Context) error {
	t.instancePromiseCtr.SetPromise(nil)

	t.le.Info("starting vite compiler process")
	defer t.le.Debug("exited vite compiler process")

	// Execute the Vite compiler with the necessary arguments
	sourcePath, distPath, bundleID := t.key.sourcePath, t.key.distPath, t.key.bundleID
	workingPath := t.key.workingPath

	// Set up the IPC making sure the pipe name is unique
	var pipeUuidBin [32]byte
	blake3.DeriveKey(
		"bldr vite-compiler pipe uuid",
		bytes.Join([][]byte{[]byte(sourcePath), []byte(workingPath), []byte(bundleID)},
			[]byte(" -- "),
		),
		pipeUuidBin[:],
	)
	pipeUuid := "vite-" + strings.ToLower(b58.Encode(pipeUuidBin[:]))[:4]

	// Compile the vite compiler host with esbuild to the working dir.
	viteScriptPath := filepath.Join(workingPath, "bldr-"+pipeUuid+".mjs")
	opts := esbuild.BuildOptions{
		AbsWorkingDir: distPath,
		// SourceRoot:    distPath,
		SourceRoot: workingPath,

		Outfile:     viteScriptPath,
		EntryPoints: []string{"web/bundler/vite/vite.ts"},

		Target:      esbuild.ES2022,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		LogLevel:    esbuild.LogLevelWarning,
		TreeShaking: esbuild.TreeShakingTrue,
		Sourcemap:   esbuild.SourceMapLinked,
		Drop:        esbuild.DropDebugger,

		Metafile:  false,
		Splitting: false,

		Define: map[string]string{
			"BLDR_IS_NODE": "true",
		},

		Plugins: []esbuild.Plugin{
			// Mark node_modules as external to prevent bundling dependencies unnecessarily.
			bldr_esbuild_build.ExternalNodeModulesPlugin(),
		},

		External: []string{"starpc", "vite"},

		Bundle: true,
		Write:  true,
	}
	result := esbuild.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return err
	}

	pipeListener, err := pipesock.BuildPipeListener(t.le, workingPath, pipeUuid)
	if err != nil {
		if ctx.Err() == nil {
			t.instancePromiseCtr.SetResult(nil, err)
		}
		return err
	}
	defer pipeListener.Close()

	// Start the listener
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)
	defer smc.Close()

	// Set up the node process
	cmd := node.NodeExec(ctx, viteScriptPath, "--bundle-id", bundleID, "--pipe-uuid", pipeUuid)
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = t.le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = t.le.WriterLevel(logrus.DebugLevel)

	// Check if canceled
	if ctx.Err() != nil {
		return context.Canceled
	}

	// Run the process
	err = cmd.Start()
	if err != nil {
		if ctx.Err() == nil {
			t.instancePromiseCtr.SetResult(nil, err)
		}
		return err
	}

	timeoutCtx, timeoutCtxCancel := context.WithTimeoutCause(ctx, time.Second*30, errors.New("timeout waiting for vite to connect"))
	defer timeoutCtxCancel()

	t.le.Debug("waiting for vite to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		return err
	}

	// Setup the client
	srpcClient := srpc.NewClientWithMuxedConn(smc)
	client := bldr_vite.NewSRPCViteBundlerClient(srpcClient)

	// Set the handle to the client
	t.le.Debug("vite compiler connected")
	t.instancePromiseCtr.SetResult(client, nil)

	// Wait for the process to exit
	err = cmd.Wait()
	if ctx.Err() != nil {
		t.instancePromiseCtr.SetPromise(nil)
		return context.Canceled
	}
	if err != nil {
		t.instancePromiseCtr.SetResult(nil, err)
	}
	return err
}

// TODO: publicPath?

// BuildViteBundle builds a Vite bundle with the given bundle args.
func BuildViteBundle(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath string,
	codeRootPath string,
	workingPath string,
	baseViteConfigPaths []string,
	viteBundleMeta *ViteBundleMeta,
	viteBundler bldr_vite.SRPCViteBundlerClient,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*web_pkg.WebPkgRef, []*bldr_web_bundler_vite.ViteOutputMeta, []string, error) {
	// outputs
	var sourceFilesList []string
	var webPkgRefs []*web_pkg.WebPkgRef
	var outputMetas []*bldr_web_bundler_vite.ViteOutputMeta

	// Create a temporary output directory for Vite
	viteBundleMetaID := viteBundleMeta.GetId()

	outAssetsBundleDir := "./"
	if viteBundleMetaID != "default" {
		outAssetsBundleDir = "./b/" + viteBundleMetaID
	}
	viteOutDir := filepath.Join(outAssetsPath, outAssetsBundleDir)

	// Determine the mode based on isRelease
	mode := "development"
	if isRelease {
		mode = "production"
	}

	// Build the vite config paths
	viteConfigPaths := slices.Clone(baseViteConfigPaths)
	for _, configPath := range viteConfigPaths {
		var err error
		configPath, err = filepath.Rel(codeRootPath, filepath.Join(codeRootPath, configPath))
		if err == nil && strings.HasPrefix(configPath, "../") {
			err = errors.New("config path must be within code dir")
		}
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "invalid vite config path: %v", configPath)
		}
	}

	// If project config is not disabled, look for vite.config.{js,ts,cjs,mjs} in the code root
	disableProjectConfig := viteBundleMeta.GetDisableProjectConfig()
	if !disableProjectConfig {
		possibleConfigExtensions := []string{".ts", ".js", ".cjs", ".mjs"}
		for _, ext := range possibleConfigExtensions {
			configPath := filepath.Join(codeRootPath, "vite.config"+ext)
			if _, err := os.Stat(configPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, nil, nil, errors.Wrapf(err, "error reading vite config at: %v", configPath)
			}

			// Found a config file, add it to the list
			relConfigPath, err := filepath.Rel(codeRootPath, configPath)
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "failed to get relative path for project config: %v", configPath)
			}
			le.Debugf("found project vite config: %s", relConfigPath)
			viteConfigPaths = append(viteConfigPaths, relConfigPath)
		}
	}

	// Add the base config path
	baseConfigRelPath, err := filepath.Rel(codeRootPath, filepath.Join(distSourcePath, "web/bundler/vite/vite-base.config.ts"))
	if err != nil {
		return nil, nil, nil, err
	}
	viteConfigPaths = append(viteConfigPaths, baseConfigRelPath)

	// Build entrypoint configs
	entrypoints := make([]*bldr_vite.ViteBuildRequestEntrypoint, 0)
	usedNames := make(map[string]bool)

	for _, entrypointConf := range viteBundleMeta.GetEntrypoints() {
		// Validate entrypoint path
		entrypointPath := entrypointConf.GetInputPath()
		if entrypointPath == "" {
			return nil, nil, nil, errors.New("entrypoint path is required for vite bundle")
		}

		// Skip if we already have this entrypoint
		found := slices.IndexFunc(entrypoints, func(e *bldr_vite.ViteBuildRequestEntrypoint) bool { return e.GetInputPath() == entrypointPath }) != -1
		if !found {
			// Use filename (without extension) as entrypoint name
			baseName := filepath.Base(entrypointPath)
			baseEntryName := strings.TrimSuffix(baseName, filepath.Ext(baseName))

			// Deconflict names by adding incremental numbers if necessary
			entryName := baseEntryName
			for i := 1; ; i++ {
				if _, ok := usedNames[entryName]; !ok {
					break
				}

				entryName = baseEntryName + strconv.Itoa(i)
			}
			usedNames[entryName] = true

			entrypoints = append(entrypoints, &bldr_vite.ViteBuildRequestEntrypoint{
				Name:      entryName,
				InputPath: entrypointPath,
			})
		}
	}

	// Store vite cache in cache dir
	cacheDir := filepath.Join(workingPath, "cache")

	// Run these through the web pkg plugin to remap to /p/...
	extWebPkgs := slices.Clone(webPkgs)

	// Exclude BldrExternal since that's already passed as external.
	extWebPkgs = slices.DeleteFunc(extWebPkgs, func(conf *bldr_web_bundler.WebPkgRefConfig) bool {
		return slices.Contains(web_pkg_esbuild.BldrExternal, conf.GetId())
	})

	// Sort and compact
	extWebPkgs = bldr_web_bundler.CompactWebPkgRefConfigs(extWebPkgs)

	// Run the build rpc
	buildResp, err := viteBundler.Build(ctx, &bldr_vite.BuildRequest{
		ConfigPaths:  viteConfigPaths,
		Mode:         mode,
		RootDir:      codeRootPath,
		DistDir:      distSourcePath,
		OutDir:       viteOutDir,
		CacheDir:     cacheDir,
		Entrypoints:  entrypoints,
		ExternalPkgs: web_pkg_esbuild.BldrExternal,
		WebPkgs:      extWebPkgs,
	})
	if ctx.Err() != nil {
		return nil, nil, nil, context.Canceled
	}
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "vite build failed")
	}

	// Add source files to the list
	// This is also set even if success=false for watching for changes
	sourceFilesList = append(sourceFilesList, buildResp.GetInputFiles()...)

	// le.Debugf("vite result: %v", buildResp.String())
	if !buildResp.GetSuccess() {
		return nil, nil, sourceFilesList, errors.New("vite build failed: " + buildResp.GetError())
	}

	// Process web package references
	for _, ref := range buildResp.GetWebPkgRefs() {
		pkgID := ref.GetPkgId()
		pkgRoot := ref.GetPkgRoot()

		// Add each subpath to webPkgRefs
		for _, subPath := range ref.GetSubPaths() {
			webPkgRefs, _ = web_pkg.
				WebPkgRefSlice(webPkgRefs).
				AppendWebPkgRef(pkgID, pkgRoot, subPath)
		}
	}

	// Helper function to hash a file and update its source map reference
	processAssetFile := func(
		filename string,
		entrypointPath string,
		fileHashMap map[string]string,
	) (string, error) {
		originalPath := filepath.Join(outAssetsPath, outAssetsBundleDir, filename)

		// Skip if the file doesn't exist or was already processed
		if _, err := os.Stat(originalPath); err != nil {
			return "", nil
		}
		if hashedPath, exists := fileHashMap[originalPath]; exists {
			return filepath.Base(hashedPath), nil
		}

		// Hash the file
		hash, err := filehash.HashFileWithBlake3(originalPath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash file: %s", filename)
		}

		// Create new filename with hash
		hashedFilename := filehash.AddHashToFilename(filename, hash)
		hashedPath := filepath.Join(outAssetsPath, outAssetsBundleDir, hashedFilename)

		// Store in map
		fileHashMap[originalPath] = hashedPath

		// Check for source map
		mapFilename := filename + ".map"
		mapPath := filepath.Join(outAssetsPath, outAssetsBundleDir, mapFilename)
		if _, err := os.Stat(mapPath); err == nil {
			hashedMapFilename := hashedFilename + ".map"
			hashedMapPath := filepath.Join(outAssetsPath, outAssetsBundleDir, hashedMapFilename)
			fileHashMap[mapPath] = hashedMapPath

			// Update source map reference in the file
			if err := filehash.UpdateSourceMapReference(originalPath, hashedMapFilename); err != nil {
				return "", errors.Wrap(err, "failed to update source map reference")
			}
		}

		// Add to output metadata
		outputPath := filepath.Join(outAssetsBundleDir, hashedFilename)
		outputMetas = append(outputMetas, &bldr_vite.ViteOutputMeta{
			Path:           outputPath,
			EntrypointPath: entrypointPath,
		})

		return hashedFilename, nil
	}

	// Map to store original to hashed filenames
	fileHashMap := make(map[string]string)

	// Process entrypoint outputs
	for _, entrypoint := range buildResp.GetEntrypointOutputs() {
		// Process JS output
		if entrypoint.JsOutput != "" {
			hashedJsFilename, err := processAssetFile(entrypoint.JsOutput, entrypoint.Entrypoint, fileHashMap)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "failed to process JS file")
			}
			if hashedJsFilename != "" {
				entrypoint.JsOutput = hashedJsFilename
			}
		}

		// Process CSS outputs
		for i, cssOutput := range entrypoint.CssOutputs {
			hashedCssFilename, err := processAssetFile(cssOutput, entrypoint.Entrypoint, fileHashMap)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "failed to process CSS file")
			}
			if hashedCssFilename != "" {
				entrypoint.CssOutputs[i] = hashedCssFilename
			}
		}
	}

	// Process global CSS files
	for _, cssFile := range buildResp.GetGlobalCssFiles() {
		_, err := processAssetFile(cssFile, "", fileHashMap)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to process global CSS file")
		}
	}

	// Rename all the files after processing
	for originalPath, hashedPath := range fileHashMap {
		if err := os.Rename(originalPath, hashedPath); err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to rename file from %s to %s", originalPath, hashedPath)
		}
	}

	return webPkgRefs, outputMetas, sourceFilesList, nil
}
