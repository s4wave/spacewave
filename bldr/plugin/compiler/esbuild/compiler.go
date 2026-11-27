// Package esbuild builds JavaScript plugins from a source filesystem and
// integrity-pinned npm tarballs, without a package manager or disk checkout.
package esbuild

import (
	"context"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	platform "github.com/s4wave/spacewave/bldr/platform"
	js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	"github.com/s4wave/spacewave/bldr/project/bunlock"
	bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bundle "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	view "github.com/s4wave/spacewave/bldr/web/view"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// Compiler is a manifest build target backed by source and dependency blocks.
// It borrows immutable source, config and verified dependency inputs from its
// caller, which retains them until every BuildManifest call returns. Each call
// owns and disposes its esbuild contexts and commits output through Bldr's World.
type Compiler struct {
	// le receives compilation and manifest commit diagnostics.
	le *logrus.Entry
	// bus resolves the output World selected by BuilderConfig.
	bus bus.Bus
	// source is the caller's retained, immutable UnixFS snapshot.
	source *unixfs.FSHandle
	// dependencies is the caller's verified tarball tree.
	dependencies kvtx.TxOps
	// config selects the plugin entrypoint, assets and expansion allowance.
	config *Config
	// repository configures Bldr's shared plugin entrypoint when building modules.
	repository *js.Config
}

// NewCompiler constructs a target without loading or starting a build.
// source, dependencies and config must remain immutable during builds. The
// caller fetches missing pins through bunlock.Fetcher and commits that tree.
func NewCompiler(le *logrus.Entry, b bus.Bus, source *unixfs.FSHandle, dependencies kvtx.TxOps, config *Config) (*Compiler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Compiler{le: le, bus: b, source: source, dependencies: dependencies, config: config}, nil
}

// NewRepositoryCompiler is the deferred compiler boundary for validated
// projects. Each module becomes an asset. The host's Bldr entrypoint starts
// backend modules and configures frontend views with the existing lifecycle.
func NewRepositoryCompiler(ctx context.Context, le *logrus.Entry, b bus.Bus, source *unixfs.FSHandle, dependencies kvtx.TxOps, plugin *js.Config, maxExpandedSize int64) (builder.Controller, error) {
	// Preserve the module order so entrypoint configuration and output paths agree.
	conf := &Config{
		Entrypoint: "plugin.mjs", WebPkgs: plugin.GetWebPkgs(), MaxExpandedSize: maxExpandedSize,
		Assets: make([]*bundle.EsbuildBundleEntrypoint, 0, len(plugin.GetModules())),
	}
	for i, mod := range plugin.GetModules() {
		switch mod.GetKind() {
		case js.JsModuleKind_JS_MODULE_KIND_BACKEND, js.JsModuleKind_JS_MODULE_KIND_FRONTEND:
		default:
			return nil, errors.Errorf("unknown js module kind: %s", mod.GetKind())
		}
		conf.Assets = append(conf.Assets, &bundle.EsbuildBundleEntrypoint{
			InputPath: path.Clean(mod.GetPath()), OutputPath: "modules/" + strconv.Itoa(i),
		})
	}

	// Retain only the configuration needed by this build, without starting esbuild.
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return &Compiler{le: le, bus: b, source: source, dependencies: dependencies, config: conf, repository: plugin.CloneVT()}, nil
}

// BuildManifest compiles once and returns the committed manifest and source
// provenance through Bldr's existing build result interface. Cancellation stops
// compiler work before manifest publication. It never commits the dependency tree.
func (c *Compiler) BuildManifest(ctx context.Context, args *builder.BuildManifestArgs, host builder.BuildManifestHost) (*builder.BuilderResult, error) {
	// Resolve the requested metadata and restrict this target to JavaScript.
	conf := args.GetBuilderConfig()
	meta, target, err := conf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}
	if target.GetBasePlatformID() != platform.PlatformID_JS {
		c.le.Warnf("skipping build for non-js platform: %v", target.GetInputPlatformID())
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Read lock pins from the same source snapshot that supplies plugin code.
	source := unixfs_iofs.NewFS(ctx, c.source)
	lock, err := fs.ReadFile(source, "bun.lock")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var packages []bunlock.Package
	if err == nil {
		packages, err = bunlock.Parse(lock)
		if err != nil {
			return nil, err
		}
	}

	// Share the host's Bldr packages and any explicitly excluded frontend packages.
	shared := slices.Clone(external.BldrExternal)
	shared = append(shared, slices.Sorted(maps.Keys(bundler.ExcludedWebPkgIDs(c.config.WebPkgs)))...)
	modules := NewModules(ctx, source, c.dependencies, packages, c.config.MaxExpandedSize, shared)
	if len(lock) != 0 {
		if _, _, err := modules.read("project/bun.lock"); err != nil {
			return nil, err
		}
	}

	// Compile the host entrypoint and optional separately served frontend assets.
	dist, assets := memfs.New(), memfs.New()
	buildType := manifest.ToBuildType(meta.GetBuildType())
	minify := conf.GetBuildPolicy().ResolveJsMinification(buildType)
	sourcemaps := conf.GetBuildPolicy().ResolveJsSourcemaps(buildType)
	if c.repository == nil {
		if err := c.compile(ctx, modules, []api.EntryPoint{{InputPath: c.config.Entrypoint, OutputPath: "plugin"}}, nil, dist, minify, sourcemaps); err != nil {
			return nil, err
		}
	}
	if len(c.config.Assets) != 0 {
		entries := make([]api.EntryPoint, len(c.config.Assets))
		for i, entry := range c.config.Assets {
			entries[i] = api.EntryPoint{InputPath: entry.GetInputPath(), OutputPath: entry.GetOutputPath()}
		}
		if err := c.compile(ctx, modules, entries, nil, assets, minify, sourcemaps); err != nil {
			return nil, err
		}
	}

	// Use the host's shared entrypoint lifecycle for repository modules and views.
	if c.repository != nil {
		entrypoint, err := c.repositoryEntrypoint(assets)
		if err != nil {
			return nil, err
		}
		if err := c.compile(ctx, modules, nil, &api.StdinOptions{Contents: entrypoint, Loader: api.LoaderJS}, dist, minify, sourcemaps); err != nil {
			return nil, err
		}
		for _, name := range []string{"bldr.star", "package.json"} {
			if _, _, err := modules.read("project/" + name); err != nil {
				return nil, err
			}
		}
	}

	// Commit the output blocks through the same manifest API as other build targets.
	engine := world.NewBusEngine(ctx, c.bus, conf.GetEngineId())
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	built, ref, err := conf.CommitManifest(ctx, c.le, tx, meta, "plugin.mjs", dist, assets)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return builder.NewBuilderResult(built, ref, modules.InputManifest()), nil
}

// compile writes a single esbuild result into an ephemeral output filesystem.
func (c *Compiler) compile(ctx context.Context, modules *Modules, entries []api.EntryPoint, stdin *api.StdinOptions, output billy.Filesystem, minify, sourcemaps bool) error {
	// Keep all source resolution in the block plugin and all output in memory.
	outdir := filepath.Join(string(filepath.Separator), "output")
	opts := api.BuildOptions{
		AbsWorkingDir:       string(filepath.Separator),
		Outdir:              outdir,
		EntryPointsAdvanced: entries,
		Stdin:               stdin,
		Plugins:             []api.Plugin{modules.Plugin()},

		Bundle:    true,
		Splitting: true,
		Format:    api.FormatESModule,
		Platform:  api.PlatformBrowser,
		Target:    api.ES2022,
		JSX:       api.JSXAutomatic,
		LogLevel:  api.LogLevelSilent,

		OutExtension: map[string]string{".js": ".mjs"},
		EntryNames:   "[dir]/[name]",
		ChunkNames:   "chunks/[name]-[hash]",
		AssetNames:   "assets/[name]-[hash]",
		Loader: map[string]api.Loader{
			".module.css": api.LoaderLocalCSS,
			".gif":        api.LoaderFile,
			".jpeg":       api.LoaderFile,
			".jpg":        api.LoaderFile,
			".png":        api.LoaderFile,
			".svg":        api.LoaderFile,
			".woff":       api.LoaderFile,
			".woff2":      api.LoaderFile,
		},

		MinifySyntax:      minify,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
	}
	if stdin != nil {
		opts.Outfile = filepath.Join(outdir, "plugin.mjs")
		opts.Outdir = ""
		opts.Splitting = false
	}
	if sourcemaps {
		opts.Sourcemap = api.SourceMapLinked
	}

	// Dispose the compiler after a single rebuild and forward caller cancellation.
	buildContext, contextErr := api.Context(opts)
	if contextErr != nil {
		return contextErr
	}
	defer buildContext.Dispose()
	stopCancel := context.AfterFunc(ctx, buildContext.Cancel)
	defer stopCancel()
	result := buildContext.Rebuild()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := build.BuildResultToErr(result); err != nil {
		return err
	}

	// Copy every emitted chunk, stylesheet, source map and asset into the manifest tree.
	for _, file := range result.OutputFiles {
		rel, err := filepath.Rel(outdir, file.Path)
		if err != nil {
			return err
		}
		if err := util.WriteFile(output, filepath.ToSlash(rel), file.Contents, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// GetControllerInfo describes this JavaScript manifest build target.
func (c *Compiler) GetControllerInfo() *controller.Info {
	return controller.NewInfo("bldr/plugin/compiler/esbuild", controller.MustParseVersion("0.0.1"), "block-backed JavaScript plugin compiler")
}

// Execute has no background work. BuildManifest controls compilation lifetime.
func (c *Compiler) Execute(ctx context.Context) error { return nil }

// Close has no retained compiler resources to release.
func (c *Compiler) Close() error { return nil }

// repositoryEntrypoint maps emitted assets to Bldr's existing runtime records.
func (c *Compiler) repositoryEntrypoint(assets billy.Filesystem) (string, error) {
	// Clone explicit entrypoints so generated entries never mutate caller state.
	conf := c.repository.CloneVT()
	conf.Modules = nil
	for i, mod := range c.repository.GetModules() {
		// Modules without an entrypoint remain available as bundled assets.
		if !mod.GetEntrypoint() {
			continue
		}
		name := "modules/" + strconv.Itoa(i)
		if mod.GetKind() == js.JsModuleKind_JS_MODULE_KIND_BACKEND {
			conf.BackendEntrypoints = append(conf.BackendEntrypoints, &js.BackendEntrypoint{ImportPath: "/assets/" + name + ".mjs"})
			continue
		}

		// Bind each view to the immutable emitted script and its shared packages.
		packages := make(map[string]string, len(external.BldrExternal)+len(conf.GetWebPkgs()))
		for _, id := range external.BldrExternal {
			packages[id] = ""
		}
		for _, pkg := range conf.GetWebPkgs() {
			if pkg.GetExclude() {
				packages[pkg.GetId()] = ""
			}
		}
		entry := &js.FrontendEntrypoint{
			WebViewId: mod.GetWebViewId().CloneVT(), WebViewParentId: mod.GetWebViewParentId().CloneVT(),
			SetRenderMode: &view.SetRenderModeRequest{
				RenderMode: view.RenderMode_RenderMode_REACT_COMPONENT,
				ScriptPath: name + ".mjs", WebPkgPaths: packages,
			},
		}

		// Attach the stylesheet emitted for this frontend module, when present.
		css := name + ".css"
		_, cssErr := assets.Stat(css)
		if cssErr != nil && !errors.Is(cssErr, fs.ErrNotExist) {
			return "", cssErr
		}
		if cssErr == nil {
			entry.SetHtmlLinks = &view.SetHtmlLinksRequest{Clear: true, SetLinks: map[string]*view.HtmlLink{
				"css-" + strconv.Itoa(i): {Rel: "stylesheet", Href: css},
			}}
		}
		conf.FrontendEntrypoints = append(conf.FrontendEntrypoints, entry)
	}

	// Parse configuration with Bldr's schema codec inside the shared runtime.
	data, err := conf.MarshalJSON()
	if err != nil {
		return "", err
	}
	return `import {createPluginEntrypoint} from "@aptre/bldr/plugin-entrypoint"; export default createPluginEntrypoint(` + string(data) + ");", nil
}

// HandleDirective resolves no directives. The build system invokes
// BuildManifest.
func (c *Compiler) HandleDirective(ctx context.Context, instance directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// SupportsStartupManifestCache requires a fresh build for caller-held snapshots.
func (c *Compiler) SupportsStartupManifestCache() bool { return false }

// GetSupportedPlatforms selects the JavaScript plugin runtime.
func (c *Compiler) GetSupportedPlatforms() []string { return []string{platform.PlatformID_JS} }

// _ is a type assertion.
var _ builder.Controller = (*Compiler)(nil)
