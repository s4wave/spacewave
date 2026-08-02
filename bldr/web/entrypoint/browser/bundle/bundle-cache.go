//go:build !js

package entrypoint_browser_bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

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
const bundleCacheFormatVersion = 2

// bundleCacheDirName is the sidecar directory (under a build dir) that holds the
// per-bundle provenance records.
const bundleCacheDirName = ".bundle-cache"

// esbuildModulePath is the Go module path of the vendored esbuild compiler.
const esbuildModulePath = "github.com/aperturerobotics/esbuild"

// esbuildPinnedVersion is the module version compiled into this checkout. It
// must be updated with the dependency and cache format when build info is absent.
const esbuildPinnedVersion = "v0.24.1-0.20260219011422-6d4b923e2023"

// bundleCache reuses browser bundle outputs only when their complete deterministic
// provenance and output content are still valid.
type bundleCache struct {
	// le is the logger.
	le *logrus.Entry
	// dir is the directory holding per-bundle provenance sidecar files.
	dir string
	// baseRoot is the esbuild working directory used to resolve metafile paths.
	baseRoot string
	// buildDir is the directory holding produced bundle outputs.
	buildDir string
	// compilerID identifies the esbuild module used by this process. An empty ID
	// disables reuse because a cache key cannot safely identify the compiler.
	compilerID string
	// builds counts bundles that were compiled (cache misses).
	builds atomic.Int64
	// reuses counts bundles served from provenance (cache hits).
	reuses atomic.Int64
}

// newBundleCache constructs a bundle cache writing provenance beside buildDir.
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
func (bc *bundleCache) Builds() int { return int(bc.builds.Load()) }

// Reuses returns the number of bundles served from provenance by this cache.
func (bc *bundleCache) Reuses() int { return int(bc.reuses.Load()) }

// bundleBuildOutput is the result of building or reusing a single bundle.
type bundleBuildOutput struct {
	// inputs are the source files esbuild consumed, relative to baseRoot.
	inputs []string
	// values are named scalar outputs (for example a worker output filename).
	values map[string]string
	// list is an ordered list output (for example renderer CSS paths).
	list []string
	// verify are output paths, relative to buildDir, whose content is recorded.
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
func (bc *bundleCache) build(
	name string,
	opts esbuild.BuildOptions,
	extraDigest []byte,
	doBuild func() (*bundleBuildOutput, error),
) (*bundleBuildOutput, error) {
	configDigest, cacheable := bc.configDigest(opts, extraDigest)
	if !cacheable {
		out, err := doBuild()
		if err == nil {
			bc.builds.Add(1)
		}
		return out, err
	}

	lock, err := acquireBundleCacheLock(filepath.Join(bc.dir, name+".lock"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()

	if cached := bc.load(name, configDigest); cached != nil {
		bc.reuses.Add(1)
		bc.le.WithField("bundle", name).Debug("reusing cached browser bundle")
		return cached, nil
	}

	out, err := doBuild()
	if err != nil {
		return nil, err
	}
	bc.builds.Add(1)

	if len(out.inputs) == 0 || len(out.verify) == 0 {
		// Missing graph or output provenance is incomplete; leave this bundle
		// rebuilding rather than risk a stale reuse.
		bc.le.WithField("bundle", name).Debug("not caching browser bundle: incomplete provenance")
		return out, nil
	}
	if err := bc.store(name, opts, configDigest, out); err != nil {
		// A provenance write failure must not fail an otherwise successful build;
		// the next run simply rebuilds.
		bc.le.WithField("bundle", name).WithError(err).Warn("failed to persist browser bundle provenance")
	}
	return out, nil
}

// configDigest hashes the compiler identity, cache format, esbuild options, and
// any extra inputs into a single provenance key.
func (bc *bundleCache) configDigest(opts esbuild.BuildOptions, extraDigest []byte) ([]byte, bool) {
	optionsDigest, ok := esbuildOptionsDigest(opts)
	if !ok || bc.compilerID == "" {
		return nil, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("bldr browser bundle cache v" + strconv.Itoa(bundleCacheFormatVersion)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(bc.compilerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(optionsDigest)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(extraDigest)
	return h.Sum(nil), true
}

// load returns the reusable output for name, or nil if provenance is missing,
// stale, incomplete, or its recorded outputs are gone.
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
	if record.formatVersion != bundleCacheFormatVersion || record.compilerID != bc.compilerID {
		return nil
	}
	if record.configDigest != hex.EncodeToString(configDigest) {
		return nil
	}
	if len(record.inputs) == 0 || len(record.outputs) == 0 {
		return nil
	}
	for _, input := range record.inputs {
		if input.identity == nil {
			return nil
		}
		match, err := input.identity.MatchesFile(filepath.Join(bc.baseRoot, input.path))
		if err != nil || !match {
			return nil
		}
	}
	for _, input := range record.configInputs {
		path := filepath.Join(bc.baseRoot, input.path)
		if input.identity == nil {
			if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
				return nil
			}
			continue
		}
		match, err := input.identity.MatchesFile(path)
		if err != nil || !match {
			return nil
		}
	}
	for _, output := range record.outputs {
		if output.identity == nil {
			return nil
		}
		match, err := output.identity.MatchesFile(filepath.Join(bc.buildDir, output.path))
		if err != nil || !match {
			return nil
		}
	}
	verify := make([]string, 0, len(record.outputs))
	for _, output := range record.outputs {
		verify = append(verify, output.path)
	}
	return &bundleBuildOutput{
		values: record.values,
		list:   record.list,
		verify: verify,
	}
}

// store captures source/config/output identities and atomically writes provenance.
func (bc *bundleCache) store(name string, opts esbuild.BuildOptions, configDigest []byte, out *bundleBuildOutput) error {
	if err := os.MkdirAll(bc.dir, 0o755); err != nil {
		return err
	}
	record := &bundleRecord{
		formatVersion: bundleCacheFormatVersion,
		compilerID:    bc.compilerID,
		configDigest:  hex.EncodeToString(configDigest),
		values:        out.values,
		list:          out.list,
	}
	for _, inputPath := range out.inputs {
		identity, err := bldr_manifest_builder.CaptureFileIdentity(filepath.Join(bc.baseRoot, inputPath))
		if err != nil {
			return errors.Wrapf(err, "capture identity for bundle input %q", inputPath)
		}
		record.inputs = append(record.inputs, bundleInput{path: inputPath, identity: identity})
	}
	configInputs, err := captureBundleConfigInputs(opts, out.inputs, bc.baseRoot)
	if err != nil {
		return err
	}
	record.configInputs = configInputs
	for _, outputPath := range out.verify {
		identity, err := bldr_manifest_builder.CaptureFileIdentity(filepath.Join(bc.buildDir, outputPath))
		if err != nil {
			return errors.Wrapf(err, "capture identity for bundle output %q", outputPath)
		}
		record.outputs = append(record.outputs, bundleOutput{path: outputPath, identity: identity})
	}
	return writeBundleRecordAtomic(bc.recordPath(name), record.marshal())
}

// recordPath returns the provenance sidecar path for a bundle name.
func (bc *bundleCache) recordPath(name string) string {
	return filepath.Join(bc.dir, name+".json")
}

// bundleInput is one recorded source or configuration file identity.
type bundleInput struct {
	path     string
	identity *bldr_manifest_builder.InputManifest_FileIdentity
}

// bundleOutput is one recorded generated output identity.
type bundleOutput struct {
	path     string
	identity *bldr_manifest_builder.InputManifest_FileIdentity
}

// bundleRecord is the persisted per-bundle provenance record.
type bundleRecord struct {
	formatVersion int
	compilerID    string
	configDigest  string
	inputs        []bundleInput
	configInputs  []bundleInput
	outputs       []bundleOutput
	values        map[string]string
	list          []string
}

// marshal encodes the record as JSON using typed fields.
func (r *bundleRecord) marshal() []byte {
	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("formatVersion", a.NewNumberInt(r.formatVersion))
	obj.Set("compilerId", a.NewString(r.compilerID))
	obj.Set("configDigest", a.NewString(r.configDigest))
	obj.Set("inputs", marshalBundleInputs(&a, r.inputs))
	obj.Set("configInputs", marshalBundleInputs(&a, r.configInputs))

	outputs := a.NewArray()
	for i, output := range r.outputs {
		item := a.NewObject()
		item.Set("path", a.NewString(output.path))
		marshalIdentity(&a, item, output.identity)
		outputs.SetArrayItem(i, item)
	}
	obj.Set("outputs", outputs)

	values := a.NewObject()
	for _, key := range sortedStringKeys(r.values) {
		values.Set(key, a.NewString(r.values[key]))
	}
	obj.Set("values", values)

	list := a.NewArray()
	for i, item := range r.list {
		list.SetArrayItem(i, a.NewString(item))
	}
	obj.Set("list", list)
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
	record.inputs, err = parseBundleInputs(v.GetArray("inputs"))
	if err != nil {
		return nil, errors.Wrap(err, "parse bundle inputs")
	}
	record.configInputs, err = parseBundleInputs(v.GetArray("configInputs"))
	if err != nil {
		return nil, errors.Wrap(err, "parse bundle config inputs")
	}
	for _, item := range v.GetArray("outputs") {
		identity, err := parseIdentity(item)
		if err != nil {
			return nil, errors.Wrap(err, "parse bundle output identity")
		}
		record.outputs = append(record.outputs, bundleOutput{
			path:     string(item.GetStringBytes("path")),
			identity: identity,
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
	return record, nil
}

func marshalBundleInputs(a *fastjson.Arena, inputs []bundleInput) *fastjson.Value {
	items := a.NewArray()
	for i, input := range inputs {
		item := a.NewObject()
		item.Set("path", a.NewString(input.path))
		marshalIdentity(a, item, input.identity)
		items.SetArrayItem(i, item)
	}
	return items
}

func marshalIdentity(a *fastjson.Arena, item *fastjson.Value, identity *bldr_manifest_builder.InputManifest_FileIdentity) {
	if identity == nil {
		item.Set("exists", a.NewFalse())
		return
	}
	item.Set("exists", a.NewTrue())
	item.Set("size", a.NewString(strconv.FormatUint(identity.GetSizeBytes(), 10)))
	item.Set("modTimeUnixNano", a.NewString(strconv.FormatInt(identity.GetModTimeUnixNano(), 10)))
	item.Set("sha256", a.NewString(hex.EncodeToString(identity.GetSha256())))
}

func parseBundleInputs(values []*fastjson.Value) ([]bundleInput, error) {
	inputs := make([]bundleInput, 0, len(values))
	for _, item := range values {
		identity, err := parseIdentity(item)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, bundleInput{
			path:     string(item.GetStringBytes("path")),
			identity: identity,
		})
	}
	return inputs, nil
}

func parseIdentity(item *fastjson.Value) (*bldr_manifest_builder.InputManifest_FileIdentity, error) {
	if !item.GetBool("exists") {
		return nil, nil
	}
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
	return &bldr_manifest_builder.InputManifest_FileIdentity{
		SizeBytes:       size,
		ModTimeUnixNano: modTime,
		Sha256:          sha,
	}, nil
}

// esbuildCompilerID returns the vendored esbuild module version. The pinned
// fallback keeps cache identity stable for test binaries without build info.
func esbuildCompilerID() string {
	fallback := "esbuild@" + esbuildPinnedVersion
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fallback
	}
	for _, dep := range info.Deps {
		if dep.Path != esbuildModulePath {
			continue
		}
		version := dep.Version
		if dep.Replace != nil {
			version = dep.Replace.Path + "@" + dep.Replace.Version
		}
		if version == "" || version == "(devel)" {
			return fallback
		}
		return "esbuild@" + version
	}
	return fallback
}

// runEsbuildBundle runs opts with a metafile and returns the consumed input file paths.
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

// esbuildOptionsDigest hashes every output-affecting BuildOptions field the
// browser bundle build sets. Unknown typed plugin state is not cacheable.
func esbuildOptionsDigest(opts esbuild.BuildOptions) ([]byte, bool) {
	if len(opts.MangleCache) != 0 {
		return nil, false
	}
	h := sha256.New()
	writeInt := func(label string, value int) {
		_, _ = h.Write([]byte(label))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(strconv.Itoa(value)))
		_, _ = h.Write([]byte{'\n'})
	}
	writeBool := func(label string, value bool) { writeInt(label, boolToInt(value)) }
	writeStr := func(label, value string) {
		_, _ = h.Write([]byte(label))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{'\n'})
	}
	writeStrings := func(label string, values []string) {
		for i, value := range values {
			writeStr(label+":"+strconv.Itoa(i), value)
		}
	}
	writeMap := func(label string, values map[string]string) {
		for _, key := range sortedStringKeys(values) {
			writeStr(label+":"+key, values[key])
		}
	}
	writeBoolMap := func(label string, values map[string]bool) {
		for _, key := range sortedBoolKeys(values) {
			writeBool(label+":"+key, values[key])
		}
	}

	writeInt("absPaths", int(opts.AbsPaths))
	writeInt("sourcemap", int(opts.Sourcemap))
	writeStr("sourceRoot", opts.SourceRoot)
	writeInt("sourcesContent", int(opts.SourcesContent))
	writeInt("target", int(opts.Target))
	for i, engine := range opts.Engines {
		writeInt("engine:"+strconv.Itoa(i), int(engine.Name))
		writeStr("engineVersion:"+strconv.Itoa(i), engine.Version)
	}
	writeBoolMap("supported", opts.Supported)
	writeStr("mangleProps", opts.MangleProps)
	writeStr("reserveProps", opts.ReserveProps)
	writeInt("mangleQuoted", int(opts.MangleQuoted))
	writeInt("drop", int(opts.Drop))
	writeStrings("dropLabels", opts.DropLabels)
	writeBool("minifyWhitespace", opts.MinifyWhitespace)
	writeBool("minifyIdentifiers", opts.MinifyIdentifiers)
	writeBool("minifySyntax", opts.MinifySyntax)
	writeInt("lineLimit", opts.LineLimit)
	writeInt("charset", int(opts.Charset))
	writeInt("treeShaking", int(opts.TreeShaking))
	writeBool("ignoreAnnotations", opts.IgnoreAnnotations)
	writeInt("legalComments", int(opts.LegalComments))
	writeInt("jsx", int(opts.JSX))
	writeStr("jsxFactory", opts.JSXFactory)
	writeStr("jsxFragment", opts.JSXFragment)
	writeStr("jsxImportSource", opts.JSXImportSource)
	writeBool("jsxDev", opts.JSXDev)
	writeBool("jsxSideEffects", opts.JSXSideEffects)
	writeMap("define", opts.Define)
	writeStrings("pure", opts.Pure)
	writeBool("keepNames", opts.KeepNames)
	writeStr("globalName", opts.GlobalName)
	writeBool("bundle", opts.Bundle)
	writeBool("preserveSymlinks", opts.PreserveSymlinks)
	writeBool("splitting", opts.Splitting)
	writeStr("outfile", opts.Outfile)
	writeStr("outdir", opts.Outdir)
	writeStr("outbase", opts.Outbase)
	writeStr("workingDir", opts.AbsWorkingDir)
	writeInt("platform", int(opts.Platform))
	writeInt("format", int(opts.Format))
	writeStrings("external", opts.External)
	writeInt("packages", int(opts.Packages))
	writeMap("alias", opts.Alias)
	writeStrings("mainFields", opts.MainFields)
	writeStrings("conditions", opts.Conditions)
	loaders := make(map[string]string, len(opts.Loader))
	for ext, loader := range opts.Loader {
		loaders[ext] = strconv.Itoa(int(loader))
	}
	writeMap("loader", loaders)
	writeStrings("resolveExtensions", opts.ResolveExtensions)
	writeStr("tsconfig", opts.Tsconfig)
	writeStr("tsconfigRaw", opts.TsconfigRaw)
	writeMap("outExtension", opts.OutExtension)
	writeStr("publicPath", opts.PublicPath)
	writeStrings("inject", opts.Inject)
	writeMap("banner", opts.Banner)
	writeMap("footer", opts.Footer)
	writeStrings("nodePaths", opts.NodePaths)
	writeStr("entryNames", opts.EntryNames)
	writeStr("chunkNames", opts.ChunkNames)
	writeStr("assetNames", opts.AssetNames)
	writeStrings("entryPoints", opts.EntryPoints)
	for i, entry := range opts.EntryPointsAdvanced {
		writeStr("entryPointInput:"+strconv.Itoa(i), entry.InputPath)
		writeStr("entryPointOutput:"+strconv.Itoa(i), entry.OutputPath)
	}
	writeBool("write", opts.Write)
	writeBool("allowOverwrite", opts.AllowOverwrite)
	if opts.Stdin != nil {
		writeStr("stdinContents", opts.Stdin.Contents)
		writeStr("stdinResolveDir", opts.Stdin.ResolveDir)
		writeStr("stdinSourcefile", opts.Stdin.Sourcefile)
		writeInt("stdinLoader", int(opts.Stdin.Loader))
	}
	for i, plugin := range opts.Plugins {
		if plugin.Name == "" {
			return nil, false
		}
		writeStr("plugin:"+strconv.Itoa(i), plugin.Name)
	}
	return h.Sum(nil), true
}

func captureBundleConfigInputs(opts esbuild.BuildOptions, inputPaths []string, baseRoot string) ([]bundleInput, error) {
	dirs := map[string]struct{}{baseRoot: {}}
	for _, inputPath := range inputPaths {
		inputDir := filepath.Dir(filepath.Join(baseRoot, inputPath))
		for {
			dirs[inputDir] = struct{}{}
			if inputDir == baseRoot || filepath.Dir(inputDir) == inputDir {
				break
			}
			inputDir = filepath.Dir(inputDir)
		}
	}
	if opts.Tsconfig != "" {
		configPath := opts.Tsconfig
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseRoot, configPath)
		}
		dirs[filepath.Dir(configPath)] = struct{}{}
	}

	dirPaths := make([]string, 0, len(dirs))
	for dir := range dirs {
		dirPaths = append(dirPaths, dir)
	}
	slices.Sort(dirPaths)
	seen := make(map[string]struct{})
	var configInputs []bundleInput
	for _, dir := range dirPaths {
		for _, name := range []string{"package.json", "tsconfig.json", "jsconfig.json"} {
			configPath := filepath.Join(dir, name)
			rel, err := filepath.Rel(baseRoot, configPath)
			if err != nil {
				return nil, errors.Wrapf(err, "relativize bundle config %q", configPath)
			}
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			identity, err := bldr_manifest_builder.CaptureFileIdentity(configPath)
			if err != nil {
				if os.IsNotExist(err) {
					configInputs = append(configInputs, bundleInput{path: rel})
					continue
				}
				return nil, errors.Wrapf(err, "capture bundle config %q", configPath)
			}
			configInputs = append(configInputs, bundleInput{path: rel, identity: identity})
		}
	}
	if opts.Tsconfig != "" {
		configPath := opts.Tsconfig
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(baseRoot, configPath)
		}
		rel, err := filepath.Rel(baseRoot, configPath)
		if err != nil {
			return nil, errors.Wrapf(err, "relativize explicit bundle config %q", configPath)
		}
		if _, ok := seen[rel]; !ok {
			identity, err := bldr_manifest_builder.CaptureFileIdentity(configPath)
			if err != nil && !os.IsNotExist(err) {
				return nil, errors.Wrapf(err, "capture explicit bundle config %q", configPath)
			}
			seen[rel] = struct{}{}
			configInputs = append(configInputs, bundleInput{path: rel, identity: identity})
		}
	}
	return configInputs, nil
}

func writeBundleRecordAtomic(recordPath string, data []byte) error {
	tempFile, err := os.CreateTemp(filepath.Dir(recordPath), ".bundle-record-*")
	if err != nil {
		return errors.Wrap(err, "create bundle provenance temp file")
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return errors.Wrap(err, "write bundle provenance temp file")
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return errors.Wrap(err, "sync bundle provenance temp file")
	}
	if err := tempFile.Close(); err != nil {
		return errors.Wrap(err, "close bundle provenance temp file")
	}
	if err := os.Rename(tempPath, recordPath); err != nil {
		return errors.Wrap(err, "rename bundle provenance record")
	}
	removeTemp = false
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
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
		verify, err := resultOutputPaths(result, opts.Outdir)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{
			inputs: inputs,
			values: map[string]string{"filename": filename},
			verify: verify,
		}, nil
	})
	if err != nil {
		return "", err
	}
	return out.value("filename"), nil
}

// buildRendererCached builds the web renderer bundle and its index through the
// cache. The rendered index bytes and every generated output are verified.
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
	out, err := cache.build("renderer", opts, indexDigest[:], func() (*bundleBuildOutput, error) {
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), indexHTML, 0o644); err != nil {
			return nil, err
		}
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		cssPaths := collectRendererCSSPaths(result, buildDir)
		verify, err := resultOutputPaths(result, buildDir)
		if err != nil {
			return nil, err
		}
		verify = append([]string{"index.html"}, verify...)
		return &bundleBuildOutput{
			inputs: inputs,
			list:   cssPaths,
			verify: uniqueStrings(verify),
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out.list, nil
}

func resultOutputPaths(result esbuild.BuildResult, buildDir string) ([]string, error) {
	paths := make([]string, 0, len(result.OutputFiles))
	for _, output := range result.OutputFiles {
		rel, err := filepath.Rel(buildDir, output.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "relativize bundle output %q", output.Path)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.Errorf("bundle output escapes build directory: %s", output.Path)
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
