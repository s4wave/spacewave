//go:build !js

package entrypoint_browser_bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

// bundleCacheFormatVersion is bumped when the cache schema or compiler-owned
// output policy changes. Version 3 invalidates every esbuild-era record.
const bundleCacheFormatVersion = 3

const bundleCacheDirName = ".bundle-cache"

// bundleCache reuses browser bundle outputs only when their complete
// deterministic provenance and output content are still valid.
type bundleCache struct {
	le       *logrus.Entry
	dir      string
	baseRoot string
	buildDir string
	builds   atomic.Int64
	reuses   atomic.Int64
}

type bundleCacheSpec struct {
	compilerID  string
	request     []byte
	configFiles []string
}

// newBundleCache constructs a bundle cache writing provenance beside buildDir.
func newBundleCache(le *logrus.Entry, buildDir, baseRoot string) *bundleCache {
	return &bundleCache{
		le:       le,
		dir:      filepath.Join(buildDir, bundleCacheDirName),
		baseRoot: baseRoot,
		buildDir: buildDir,
	}
}

// Builds returns the number of bundles compiled through this cache.
func (bc *bundleCache) Builds() int { return int(bc.builds.Load()) }

// Reuses returns the number of bundles served from provenance by this cache.
func (bc *bundleCache) Reuses() int { return int(bc.reuses.Load()) }

// bundleBuildOutput is the result of building or reusing one bundle.
type bundleBuildOutput struct {
	inputs []string
	values map[string]string
	list   []string
	verify []string
}

func (o *bundleBuildOutput) value(key string) string {
	if o == nil {
		return ""
	}
	return o.values[key]
}

func (bc *bundleCache) build(
	name string,
	spec bundleCacheSpec,
	extraDigest []byte,
	doBuild func() (*bundleBuildOutput, error),
) (*bundleBuildOutput, error) {
	configDigest, cacheable := bc.configDigest(spec, extraDigest)
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

	if cached := bc.load(name, spec.compilerID, configDigest); cached != nil {
		bc.reuses.Add(1)
		bc.le.WithField("bundle", name).Debug("reusing cached browser bundle")
		return cached, nil
	}
	bc.removeRecordedOutputs(name)

	out, err := doBuild()
	if err != nil {
		return nil, err
	}
	bc.builds.Add(1)
	if len(out.inputs) == 0 || len(out.verify) == 0 {
		bc.le.WithField("bundle", name).Debug("not caching browser bundle: incomplete provenance")
		return out, nil
	}
	if err := bc.store(name, spec, configDigest, out); err != nil {
		bc.le.WithField("bundle", name).WithError(err).Warn("failed to persist browser bundle provenance")
	}
	return out, nil
}

func (bc *bundleCache) removeRecordedOutputs(name string) {
	data, err := os.ReadFile(bc.recordPath(name))
	if err != nil {
		return
	}
	record, err := parseBundleRecord(data)
	if err != nil {
		return
	}
	for _, output := range record.outputs {
		relativePath := filepath.Clean(output.path)
		if relativePath == "." ||
			relativePath == ".." ||
			filepath.IsAbs(relativePath) ||
			strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		_ = os.Remove(filepath.Join(bc.buildDir, relativePath))
	}
}

func (bc *bundleCache) configDigest(spec bundleCacheSpec, extraDigest []byte) ([]byte, bool) {
	if spec.compilerID == "" || len(spec.request) == 0 {
		return nil, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("bldr browser bundle cache v" + strconv.Itoa(bundleCacheFormatVersion)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(spec.compilerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(spec.request)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(extraDigest)
	return h.Sum(nil), true
}

// load returns the reusable output for name, or nil if provenance is missing,
// stale, incomplete, or its recorded outputs are gone.
func (bc *bundleCache) load(name, compilerID string, configDigest []byte) *bundleBuildOutput {
	data, err := os.ReadFile(bc.recordPath(name))
	if err != nil {
		return nil
	}
	record, err := parseBundleRecord(data)
	if err != nil {
		bc.le.WithField("bundle", name).WithError(err).Debug("ignoring unreadable bundle provenance")
		return nil
	}
	if record.formatVersion != bundleCacheFormatVersion || record.compilerID != compilerID {
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

// store captures source, configuration, and output identities atomically.
func (bc *bundleCache) store(name string, spec bundleCacheSpec, configDigest []byte, out *bundleBuildOutput) error {
	if err := os.MkdirAll(bc.dir, 0o755); err != nil {
		return err
	}
	record := &bundleRecord{
		formatVersion: bundleCacheFormatVersion,
		compilerID:    spec.compilerID,
		configDigest:  hex.EncodeToString(configDigest),
		values:        out.values,
		list:          out.list,
	}
	for _, inputPath := range out.inputs {
		relativePath, err := cacheRelativePath(bc.baseRoot, inputPath)
		if err != nil {
			return err
		}
		identity, err := bldr_manifest_builder.CaptureFileIdentity(filepath.Join(bc.baseRoot, relativePath))
		if err != nil {
			return errors.Wrapf(err, "capture identity for bundle input %q", inputPath)
		}
		record.inputs = append(record.inputs, bundleInput{path: relativePath, identity: identity})
	}
	configInputs, err := captureBundleConfigInputs(spec.configFiles, bc.baseRoot)
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

func cacheRelativePath(baseRoot, path string) (string, error) {
	absolutePath := path
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(baseRoot, absolutePath)
	}
	relativePath, err := filepath.Rel(baseRoot, absolutePath)
	if err != nil {
		return "", errors.Wrapf(err, "relativize bundle path %q", path)
	}
	return filepath.Clean(relativePath), nil
}

func captureBundleConfigInputs(configFiles []string, baseRoot string) ([]bundleInput, error) {
	paths := make([]string, 0, len(configFiles))
	seen := make(map[string]struct{}, len(configFiles))
	for _, configFile := range configFiles {
		relativePath, err := cacheRelativePath(baseRoot, configFile)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[relativePath]; ok {
			continue
		}
		seen[relativePath] = struct{}{}
		paths = append(paths, relativePath)
	}
	slices.Sort(paths)
	inputs := make([]bundleInput, 0, len(paths))
	for _, relativePath := range paths {
		identity, err := bldr_manifest_builder.CaptureFileIdentity(filepath.Join(baseRoot, relativePath))
		if err != nil {
			if os.IsNotExist(err) {
				inputs = append(inputs, bundleInput{path: relativePath})
				continue
			}
			return nil, errors.Wrapf(err, "capture bundle config %q", relativePath)
		}
		inputs = append(inputs, bundleInput{path: relativePath, identity: identity})
	}
	return inputs, nil
}

func browserBundleConfigFiles(bldrDistRoot, compilerID string) []string {
	toolRoot := bldrDistRoot
	if _, err := os.Stat(filepath.Join(toolRoot, "web", "bundler")); err != nil {
		toolRoot = filepath.Join(bldrDistRoot, "bldr")
	}
	files := []string{
		filepath.Join(bldrDistRoot, "go.mod"),
		filepath.Join(filepath.Dir(bldrDistRoot), "go.mod"),
		filepath.Join(bldrDistRoot, "package.json"),
		filepath.Join(bldrDistRoot, "tsconfig.json"),
		filepath.Join(bldrDistRoot, "global.d.ts"),
		filepath.Join(toolRoot, "dist", "deps", "package.json"),
		filepath.Join(toolRoot, "dist", "deps", "bun.lock"),
	}
	switch compilerID {
	case rolldownBrowserCompilerID:
		files = append(files,
			filepath.Join(toolRoot, "web", "bundler", "rolldown", "run-build.mjs"),
		)
	case viteBrowserCompilerID:
		files = append(files, viteBrowserCompilerConfigFiles(toolRoot)...)
	case rendererBrowserCompilerID:
		files = append(files,
			filepath.Join(toolRoot, "web", "bundler", "rolldown", "run-build.mjs"),
		)
		files = append(files, viteBrowserCompilerConfigFiles(toolRoot)...)
	}
	return files
}

func viteBrowserCompilerConfigFiles(toolRoot string) []string {
	bundlerRoot := filepath.Join(toolRoot, "web", "bundler")
	viteRoot := filepath.Join(bundlerRoot, "vite")
	return []string{
		filepath.Join(bundlerRoot, "bundler.pb.ts"),
		filepath.Join(viteRoot, "build.ts"),
		filepath.Join(viteRoot, "go-ts-resolver.ts"),
		filepath.Join(viteRoot, "module-preload.ts"),
		filepath.Join(viteRoot, "output-naming.ts"),
		filepath.Join(viteRoot, "plugin.ts"),
		filepath.Join(viteRoot, "vite.pb.ts"),
		filepath.Join(viteRoot, "vite_srpc.pb.ts"),
		filepath.Join(viteRoot, "vite.ts"),
		filepath.Join(viteRoot, "web-pkg-naming.ts"),
	}
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

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func buildServiceWorkerCached(
	ctx context.Context,
	stateDir string,
	cache *bundleCache,
	bldrDistRoot,
	buildDir,
	_ string,
	minify,
	sourcemaps,
	devMode bool,
) (string, error) {
	return buildSingleFileWorkerCached(
		ctx, stateDir, cache, "service-worker", bldrDistRoot, buildDir,
		serviceWorkerSpec(minify, sourcemaps, devMode),
	)
}

func buildSharedWorkerCached(
	ctx context.Context,
	stateDir string,
	cache *bundleCache,
	bldrDistRoot,
	buildDir,
	_ string,
	minify,
	sourcemaps,
	devMode bool,
) (string, error) {
	return buildSingleFileWorkerCached(
		ctx, stateDir, cache, "shared-worker", bldrDistRoot, buildDir,
		sharedWorkerSpec(minify, sourcemaps, devMode),
	)
}

func buildOpfsWorkerCached(
	ctx context.Context,
	stateDir string,
	cache *bundleCache,
	bldrDistRoot,
	buildDir,
	_ string,
	minify,
	sourcemaps,
	devMode bool,
) (string, error) {
	return buildSingleFileWorkerCached(
		ctx, stateDir, cache, "opfs-worker", bldrDistRoot, buildDir,
		opfsWorkerSpec(minify, sourcemaps, devMode),
	)
}

func buildSingleFileWorkerCached(
	ctx context.Context,
	stateDir string,
	cache *bundleCache,
	name,
	bldrDistRoot,
	buildDir string,
	scriptSpec browserScriptSpec,
) (string, error) {
	request := browserScriptRequest(bldrDistRoot, buildDir, scriptSpec)
	requestJSON, err := request.MarshalJSON()
	if err != nil {
		return "", errors.Wrap(err, "marshal browser worker cache request")
	}
	out, err := cache.build(
		name,
		bundleCacheSpec{
			compilerID:  rolldownBrowserCompilerID,
			request:     requestJSON,
			configFiles: browserBundleConfigFiles(bldrDistRoot, rolldownBrowserCompilerID),
		},
		nil,
		func() (*bundleBuildOutput, error) {
			filename, result, err := buildWorkerBundle(
				ctx, cache.le, stateDir, bldrDistRoot, buildDir, scriptSpec,
			)
			if err != nil {
				return nil, err
			}
			return &bundleBuildOutput{
				inputs: result.GetInputs(),
				values: map[string]string{"filename": filename},
				verify: resultOutputPaths(result),
			}, nil
		},
	)
	if err != nil {
		return "", err
	}
	return out.value("filename"), nil
}

func buildRendererCached(
	ctx context.Context,
	stateDir string,
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
	rendererOpts, err := browserRendererSpec(
		sourcesRoot, bldrDistRoot, buildDir,
		runtimeJsPath, runtimeSwPath, runtimeShwPath, runtimeOpfsWorkerPath,
		webStartupSrcPath, entrypointHash,
		minify, sourcemaps, forceDedicatedWorkers, forceMessagePortWorkerComms, devMode,
	)
	if err != nil {
		return nil, err
	}
	directJSON, err := directRendererRequest(bldrDistRoot, buildDir, rendererOpts).MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "marshal direct renderer cache request")
	}
	viteJSON, err := configFreeRendererRequest(
		bldrDistRoot,
		filepath.Join(stateDir, "vite-renderer"),
		rendererOpts,
	).MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "marshal Vite renderer cache request")
	}
	requestJSON := append(append(directJSON, 0), viteJSON...)
	indexDigest := sha256.Sum256(indexHTML)
	out, err := cache.build(
		"renderer",
		bundleCacheSpec{
			compilerID:  rendererBrowserCompilerID,
			request:     requestJSON,
			configFiles: browserBundleConfigFiles(bldrDistRoot, rendererBrowserCompilerID),
		},
		indexDigest[:],
		func() (*bundleBuildOutput, error) {
			if err := os.WriteFile(filepath.Join(buildDir, "index.html"), indexHTML, 0o644); err != nil {
				return nil, err
			}
			result, err := BuildRenderer(
				ctx, cache.le, stateDir, bldrDistRoot, buildDir, rendererOpts,
			)
			if err != nil {
				return nil, err
			}
			return &bundleBuildOutput{
				inputs: result.InputFiles,
				list:   result.CSSPaths,
				verify: uniqueStrings(append([]string{"index.html"}, result.OutputFiles...)),
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return out.list, nil
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
