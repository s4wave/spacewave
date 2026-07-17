//go:build !js

package entrypoint_browser_bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

// bundleCacheFormatVersion is bumped when the cache schema or the compiler-owned
// output policy changes without changing a source file. A bump invalidates every
// persisted bundle provenance record.
const bundleCacheFormatVersion = 1

// bundleCacheDirName is the sidecar directory (under a build dir) that holds the
// per-bundle provenance records.
const bundleCacheDirName = ".bundle-cache"

// esbuildModulePath is the Go module path of the vendored esbuild compiler.
const esbuildModulePath = "github.com/aperturerobotics/esbuild"

// bundleCache reuses browser bundle outputs across builds when their content
// inputs are unchanged.
//
// Each bundle records a provenance file capturing the identities of every source
// file esbuild consumed (from its metafile), a config digest over the build
// options plus the compiler identity, and the produced output filenames. A later
// build reuses the recorded outputs when every input identity still matches, the
// config digest is unchanged, and the recorded outputs remain on disk; otherwise
// it rebuilds and records fresh provenance.
type bundleCache struct {
	// le is the logger.
	le *logrus.Entry
	// dir is the directory holding per-bundle provenance sidecar files.
	dir string
	// baseRoot is the esbuild working directory used to resolve the metafile's
	// input paths and the recorded output verification paths.
	baseRoot string
	// buildDir is the directory holding produced bundle outputs.
	buildDir string
	// compilerID augments the config digest with the esbuild module version when
	// the binary exposes module build info. It is empty for binaries without that
	// info (for example go test binaries); the manual bundleCacheFormatVersion is
	// the compiler-policy identity in that case.
	compilerID string
	// builds counts bundles that were compiled (cache misses).
	builds int
	// reuses counts bundles served from provenance (cache hits).
	reuses int
}

// newBundleCache constructs a bundle cache writing provenance beside buildDir.
//
// baseRoot is the esbuild AbsWorkingDir shared by every routed build; it resolves
// the relative input and output paths esbuild reports in its metafile.
func newBundleCache(le *logrus.Entry, buildDir, baseRoot string) *bundleCache {
	return &bundleCache{
		le:         le,
		dir:        filepath.Join(buildDir, bundleCacheDirName),
		baseRoot:   baseRoot,
		buildDir:   buildDir,
		compilerID: esbuildCompilerID(),
	}
}

// Builds returns the number of bundles compiled through this cache.
func (bc *bundleCache) Builds() int { return bc.builds }

// Reuses returns the number of bundles served from provenance by this cache.
func (bc *bundleCache) Reuses() int { return bc.reuses }

// bundleBuildOutput is the result of building or reusing a single bundle.
type bundleBuildOutput struct {
	// inputs are the source files esbuild consumed, relative to baseRoot.
	inputs []string
	// values are named scalar outputs (for example a worker output filename).
	values map[string]string
	// list is an ordered list output (for example renderer CSS paths).
	list []string
	// verify are output paths, relative to buildDir, that must exist for a reuse.
	verify []string
}

// value returns the named scalar output, or the empty string if absent.
func (o *bundleBuildOutput) value(key string) string {
	if o == nil {
		return ""
	}
	return o.values[key]
}

// build reuses the named bundle when its provenance is still valid, otherwise it
// runs doBuild, records fresh provenance, and returns the produced output.
//
// opts are the esbuild options for the bundle; their deterministic fields feed
// the config digest. extraDigest carries non-esbuild inputs that still affect the
// output (for example the web-package import map baked into index.html).
func (bc *bundleCache) build(
	name string,
	opts esbuild.BuildOptions,
	extraDigest []byte,
	doBuild func() (*bundleBuildOutput, error),
) (*bundleBuildOutput, error) {
	configDigest := bc.configDigest(opts, extraDigest)

	if cached := bc.load(name, configDigest); cached != nil {
		bc.reuses++
		bc.le.WithField("bundle", name).Debug("reusing cached browser bundle")
		return cached, nil
	}

	out, err := doBuild()
	if err != nil {
		return nil, err
	}
	bc.builds++

	if len(out.inputs) == 0 {
		// A build that reported no input files has incomplete provenance; leave it
		// rebuilding on every run rather than risk a stale reuse.
		bc.le.WithField("bundle", name).Debug("not caching bundle: no recorded inputs")
		return out, nil
	}
	if err := bc.store(name, configDigest, out); err != nil {
		// A provenance write failure must not fail an otherwise successful
		// build; the next run simply rebuilds.
		bc.le.WithField("bundle", name).WithError(err).Warn("failed to persist browser bundle provenance")
	}
	return out, nil
}

// configDigest hashes the compiler identity, cache format, esbuild options, and
// any extra inputs into a single provenance key.
func (bc *bundleCache) configDigest(opts esbuild.BuildOptions, extraDigest []byte) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte("bldr browser bundle cache v" + strconv.Itoa(bundleCacheFormatVersion)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(bc.compilerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(esbuildOptionsDigest(opts))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(extraDigest)
	return h.Sum(nil)
}

// load returns the reusable output for name, or nil if provenance is missing,
// stale, or its recorded outputs are gone.
func (bc *bundleCache) load(name string, configDigest []byte) *bundleBuildOutput {
	data, err := os.ReadFile(bc.recordPath(name))
	if err != nil {
		return nil
	}
	record, err := parseBundleRecord(data)
	if err != nil {
		bc.le.WithField("bundle", name).WithError(err).Debug("ignoring unreadable bundle provenance")
		return nil
	}
	if record.formatVersion != bundleCacheFormatVersion {
		return nil
	}
	if record.configDigest != hex.EncodeToString(configDigest) {
		return nil
	}
	for _, input := range record.inputs {
		match, err := input.identity.MatchesFile(filepath.Join(bc.baseRoot, input.path))
		if err != nil || !match {
			return nil
		}
	}
	for _, verifyPath := range record.verify {
		if _, err := os.Stat(filepath.Join(bc.buildDir, verifyPath)); err != nil {
			return nil
		}
	}
	return &bundleBuildOutput{
		values: record.values,
		list:   record.list,
		verify: record.verify,
	}
}

// store captures the input identities for out and writes its provenance record.
func (bc *bundleCache) store(name string, configDigest []byte, out *bundleBuildOutput) error {
	if err := os.MkdirAll(bc.dir, 0o755); err != nil {
		return err
	}
	record := &bundleRecord{
		formatVersion: bundleCacheFormatVersion,
		compilerID:    bc.compilerID,
		configDigest:  hex.EncodeToString(configDigest),
		values:        out.values,
		list:          out.list,
		verify:        out.verify,
	}
	for _, inputPath := range out.inputs {
		identity, err := bldr_manifest_builder.CaptureFileIdentity(filepath.Join(bc.baseRoot, inputPath))
		if err != nil {
			return errors.Wrapf(err, "capture identity for bundle input %q", inputPath)
		}
		record.inputs = append(record.inputs, bundleInput{path: inputPath, identity: identity})
	}
	return os.WriteFile(bc.recordPath(name), record.marshal(), 0o644)
}

// recordPath returns the provenance sidecar path for a bundle name.
func (bc *bundleCache) recordPath(name string) string {
	return filepath.Join(bc.dir, name+".json")
}

// bundleInput is one recorded source file identity.
type bundleInput struct {
	path     string
	identity *bldr_manifest_builder.InputManifest_FileIdentity
}

// bundleRecord is the persisted per-bundle provenance record.
type bundleRecord struct {
	formatVersion int
	compilerID    string
	configDigest  string
	inputs        []bundleInput
	values        map[string]string
	list          []string
	verify        []string
}

// marshal encodes the record as JSON using typed fields.
func (r *bundleRecord) marshal() []byte {
	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("formatVersion", a.NewNumberInt(r.formatVersion))
	obj.Set("compilerId", a.NewString(r.compilerID))
	obj.Set("configDigest", a.NewString(r.configDigest))

	inputs := a.NewArray()
	for i, input := range r.inputs {
		item := a.NewObject()
		item.Set("path", a.NewString(input.path))
		// Size and modtime are stored as strings so the full int64/uint64 range
		// survives JSON round-tripping without float precision loss.
		item.Set("size", a.NewString(strconv.FormatUint(input.identity.GetSizeBytes(), 10)))
		item.Set("modTimeUnixNano", a.NewString(strconv.FormatInt(input.identity.GetModTimeUnixNano(), 10)))
		item.Set("sha256", a.NewString(hex.EncodeToString(input.identity.GetSha256())))
		inputs.SetArrayItem(i, item)
	}
	obj.Set("inputs", inputs)

	values := a.NewObject()
	for _, key := range sortedKeys(r.values) {
		values.Set(key, a.NewString(r.values[key]))
	}
	obj.Set("values", values)

	list := a.NewArray()
	for i, item := range r.list {
		list.SetArrayItem(i, a.NewString(item))
	}
	obj.Set("list", list)

	verify := a.NewArray()
	for i, item := range r.verify {
		verify.SetArrayItem(i, a.NewString(item))
	}
	obj.Set("verify", verify)

	return obj.MarshalTo(nil)
}

// parseBundleRecord decodes a persisted provenance record.
func parseBundleRecord(data []byte) (*bundleRecord, error) {
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	record := &bundleRecord{
		formatVersion: v.GetInt("formatVersion"),
		compilerID:    string(v.GetStringBytes("compilerId")),
		configDigest:  string(v.GetStringBytes("configDigest")),
		values:        map[string]string{},
	}
	for _, item := range v.GetArray("inputs") {
		sha, err := hex.DecodeString(string(item.GetStringBytes("sha256")))
		if err != nil {
			return nil, errors.Wrap(err, "decode input sha256")
		}
		size, err := strconv.ParseUint(string(item.GetStringBytes("size")), 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "decode input size")
		}
		modTime, err := strconv.ParseInt(string(item.GetStringBytes("modTimeUnixNano")), 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "decode input mod time")
		}
		record.inputs = append(record.inputs, bundleInput{
			path: string(item.GetStringBytes("path")),
			identity: &bldr_manifest_builder.InputManifest_FileIdentity{
				SizeBytes:       size,
				ModTimeUnixNano: modTime,
				Sha256:          sha,
			},
		})
	}
	if values := v.GetObject("values"); values != nil {
		values.Visit(func(k []byte, item *fastjson.Value) {
			record.values[string(k)] = string(item.GetStringBytes())
		})
	}
	for _, item := range v.GetArray("list") {
		record.list = append(record.list, string(item.GetStringBytes()))
	}
	for _, item := range v.GetArray("verify") {
		record.verify = append(record.verify, string(item.GetStringBytes()))
	}
	return record, nil
}

// esbuildCompilerID returns the vendored esbuild module version, or the empty
// string when the running binary lacks module build information (for example a
// go test binary). Callers treat an empty value as "no extra compiler identity";
// bundleCacheFormatVersion still guards compiler-policy changes.
func esbuildCompilerID() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == esbuildModulePath {
			return "esbuild@" + dep.Version
		}
	}
	return ""
}

// runEsbuildBundle runs opts with a metafile and returns the esbuild result plus
// the consumed input file paths (relative to opts.AbsWorkingDir).
func runEsbuildBundle(opts esbuild.BuildOptions) (esbuild.BuildResult, []string, error) {
	opts.Metafile = true
	result := esbuild.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return result, nil, err
	}
	metafile, err := bldr_esbuild_build.ParseEsbuildMetafile([]byte(result.Metafile))
	if err != nil {
		return result, nil, errors.Wrap(err, "parse esbuild metafile")
	}
	inputs := make([]string, 0, len(metafile.Inputs))
	for inputPath := range metafile.Inputs {
		inputs = append(inputs, inputPath)
	}
	slices.Sort(inputs)
	return result, inputs, nil
}

// esbuildOptionsDigest hashes the deterministic esbuild options that affect a
// bundle's output. Plugin behaviour is covered by the resolved input identities,
// so only plugin names are hashed here.
func esbuildOptionsDigest(opts esbuild.BuildOptions) []byte {
	h := sha256.New()
	writeInt := func(label string, value int) {
		_, _ = h.Write([]byte(label))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(strconv.Itoa(value)))
		_, _ = h.Write([]byte{'\n'})
	}
	writeBool := func(label string, value bool) {
		writeInt(label, boolToInt(value))
	}
	writeStr := func(label, value string) {
		_, _ = h.Write([]byte(label))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{'\n'})
	}
	writeList := func(label string, values []string) {
		sorted := slices.Clone(values)
		slices.Sort(sorted)
		for _, value := range sorted {
			writeStr(label, value)
		}
	}
	writeMap := func(label string, values map[string]string) {
		for _, key := range sortedKeys(values) {
			writeStr(label+":"+key, values[key])
		}
	}

	writeInt("target", int(opts.Target))
	writeInt("format", int(opts.Format))
	writeInt("platform", int(opts.Platform))
	writeInt("sourcemap", int(opts.Sourcemap))
	writeInt("drop", int(opts.Drop))
	writeInt("treeShaking", int(opts.TreeShaking))
	writeBool("bundle", opts.Bundle)
	writeBool("splitting", opts.Splitting)
	writeBool("minifyWhitespace", opts.MinifyWhitespace)
	writeBool("minifyIdentifiers", opts.MinifyIdentifiers)
	writeBool("minifySyntax", opts.MinifySyntax)
	writeStr("entryNames", opts.EntryNames)
	writeStr("chunkNames", opts.ChunkNames)
	writeStr("assetNames", opts.AssetNames)
	writeStr("publicPath", opts.PublicPath)
	writeStr("jsx", strconv.Itoa(int(opts.JSX)))
	writeList("entryPoints", opts.EntryPoints)
	for _, entry := range opts.EntryPointsAdvanced {
		writeStr("entryPointAdvanced", entry.InputPath+"=>"+entry.OutputPath)
	}
	writeList("external", opts.External)
	writeList("inject", opts.Inject)
	writeMap("define", opts.Define)
	writeMap("banner", opts.Banner)
	writeMap("footer", opts.Footer)
	loaders := make(map[string]string, len(opts.Loader))
	for ext, loader := range opts.Loader {
		loaders[ext] = strconv.Itoa(int(loader))
	}
	writeMap("loader", loaders)
	writeMap("outExtension", opts.OutExtension)
	var pluginNames []string
	for _, plugin := range opts.Plugins {
		pluginNames = append(pluginNames, plugin.Name)
	}
	writeList("plugin", pluginNames)
	return h.Sum(nil)
}

// buildServiceWorkerCached builds the service worker bundle through the cache.
func buildServiceWorkerCached(cache *bundleCache, bldrDistRoot, buildDir, buildPkgsDir string, minify, sourcemaps, devMode bool) (string, error) {
	opts := serviceWorkerBundleOpts(bldrDistRoot, buildDir, buildPkgsDir, minify, sourcemaps, devMode)
	return buildSingleFileWorkerCached(cache, "service-worker", opts, singleWorkerOutputName)
}

// buildSharedWorkerCached builds the shared worker bundle through the cache.
func buildSharedWorkerCached(cache *bundleCache, bldrDistRoot, buildDir, buildPkgsDir string, minify, sourcemaps, devMode bool) (string, error) {
	opts := sharedWorkerBundleOpts(bldrDistRoot, buildDir, buildPkgsDir, minify, sourcemaps, devMode)
	return buildSingleFileWorkerCached(cache, "shared-worker", opts, func(result esbuild.BuildResult) (string, error) {
		return mjsWorkerOutputName(result, "shared worker")
	})
}

// buildOpfsWorkerCached builds the OPFS worker bundle through the cache.
func buildOpfsWorkerCached(cache *bundleCache, bldrDistRoot, buildDir, buildPkgsDir string, minify, sourcemaps, devMode bool) (string, error) {
	opts := opfsWorkerBundleOpts(bldrDistRoot, buildDir, buildPkgsDir, minify, sourcemaps, devMode)
	return buildSingleFileWorkerCached(cache, "opfs-worker", opts, func(result esbuild.BuildResult) (string, error) {
		return mjsWorkerOutputName(result, "OPFS worker")
	})
}

// buildSingleFileWorkerCached routes a single-output worker build through the
// cache, extracting its output filename with extractName.
func buildSingleFileWorkerCached(
	cache *bundleCache,
	name string,
	opts esbuild.BuildOptions,
	extractName func(esbuild.BuildResult) (string, error),
) (string, error) {
	out, err := cache.build(name, opts, nil, func() (*bundleBuildOutput, error) {
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		filename, err := extractName(result)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{
			inputs: inputs,
			values: map[string]string{"filename": filename},
			verify: []string{filename},
		}, nil
	})
	if err != nil {
		return "", err
	}
	return out.value("filename"), nil
}

// buildRendererCached builds the web renderer bundle (index.html plus the
// entrypoint esbuild bundle) through the cache. The rendered index.html bytes
// feed the config digest so an import-map or template change invalidates reuse.
func buildRendererCached(
	cache *bundleCache,
	sourcesRoot, bldrDistRoot, buildDir,
	runtimeJsPath, runtimeSwPath, runtimeShwPath, runtimeOpfsWorkerPath,
	webStartupSrcPath, entrypointHash string,
	minify, sourcemaps, forceDedicatedWorkers, forceMessagePortWorkerComms, devMode bool,
	webPkgImportMap web_entrypoint_index.ImportMap,
) ([]string, error) {
	indexHTML, err := renderIndexHTML("./"+stableBootFilename, webPkgImportMap)
	if err != nil {
		return nil, err
	}
	opts, err := rendererBundleOpts(
		sourcesRoot, bldrDistRoot, buildDir,
		runtimeJsPath, runtimeSwPath, runtimeShwPath, runtimeOpfsWorkerPath,
		webStartupSrcPath, entrypointHash,
		minify, sourcemaps, forceDedicatedWorkers, forceMessagePortWorkerComms, devMode,
	)
	if err != nil {
		return nil, err
	}

	indexDigest := sha256.Sum256(indexHTML)
	entrypointRel := rendererEntrypointRelPath(entrypointHash)
	out, err := cache.build("renderer", opts, indexDigest[:], func() (*bundleBuildOutput, error) {
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), indexHTML, 0o644); err != nil {
			return nil, err
		}
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		cssPaths := collectRendererCSSPaths(result, buildDir)
		verify := append([]string{"index.html", entrypointRel}, cssPaths...)
		return &bundleBuildOutput{
			inputs: inputs,
			list:   cssPaths,
			verify: verify,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out.list, nil
}

// boolToInt maps a boolean to 0 or 1 for digest stability.
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// sortedKeys returns the map keys in sorted order.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
