//go:build !js

package bldr_project_validate

import (
	"context"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
)

// projectFile is the Starlark project file a plugin repository declares.
const projectFile = "bldr.star"

// evaluateTimeout bounds the Starlark evaluation of an untrusted project.
const evaluateTimeout = 10 * time.Second

// buildConfigPrefixes name root files that Vite or PostCSS load and run as
// code during the build.
var buildConfigPrefixes = []string{"vite.config.", "postcss.config.", ".postcssrc"}

// Validate checks the untrusted Bldr project at the root of fsys before any of
// its code runs. It evaluates bldr.star as Starlark that reads only fsys and
// reads package.json and bun.lock. Each rule the project breaks is a refusal;
// an error means the check itself could not complete.
func Validate(ctx context.Context, fsys fs.FS) (*Validation, error) {
	// Check the root files, the project config and the npm package.
	v := &Validation{}
	if err := v.checkRootFiles(fsys); err != nil {
		return nil, err
	}
	if err := v.checkProject(ctx, fsys); err != nil {
		return nil, err
	}
	if err := v.checkPackage(fsys); err != nil {
		return nil, err
	}

	// Order the plugins and dependencies for display.
	slices.SortFunc(v.Plugins, func(a, b *Plugin) int {
		return strings.Compare(a.GetManifestId(), b.GetManifestId())
	})
	slices.SortFunc(v.Dependencies, func(a, b *Dependency) int {
		return strings.Compare(a.GetName()+"@"+a.GetVersion(), b.GetName()+"@"+b.GetVersion())
	})
	return v, nil
}

// refuse records a refusal, once per distinct reason.
func (v *Validation) refuse(kind RefusalKind, reason string) {
	for _, r := range v.Refusals {
		if r.GetReason() == reason {
			return
		}
	}
	v.Refusals = append(v.Refusals, &Refusal{Kind: kind, Reason: reason})
}

// checkRootFiles refuses root files the build would read beyond bldr.star.
func (v *Validation) checkRootFiles(fsys fs.FS) error {
	// List the project root.
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return errors.Wrap(err, "read project root")
	}

	// Refuse a YAML project config and build tool configs that run code.
	for _, entry := range entries {
		name := entry.Name()
		if name == "bldr.yaml" {
			v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "The repository has a bldr.yaml, which the build would read in addition to bldr.star.")
		}
		for _, prefix := range buildConfigPrefixes {
			if strings.HasPrefix(name, prefix) {
				v.refuse(RefusalKind_REFUSAL_KIND_VITE_CONFIG, "The repository has "+name+", which would run as code during the build.")
			}
		}
	}
	return nil
}

// checkProject evaluates bldr.star and checks each manifest it declares.
func (v *Validation) checkProject(ctx context.Context, fsys fs.FS) error {
	// Require the project file before evaluating it.
	if _, err := fs.Stat(fsys, projectFile); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "The repository has no bldr.star.")
			return nil
		}
		return errors.Wrap(err, "stat "+projectFile)
	}

	// Evaluate the project within fsys under a deadline.
	evalCtx, cancel := context.WithTimeout(ctx, evaluateTimeout)
	defer cancel()
	result, err := bldr_project_starlark.EvaluateFS(evalCtx, fsys, projectFile)
	if err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "bldr.star does not evaluate: "+err.Error())
		return nil
	}

	// Refuse project sections a plugin repository does not need.
	conf := result.Config
	if len(conf.GetExtends()) != 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "bldr.star extends another project.")
	}
	if len(conf.GetBuild())+len(conf.GetRemotes())+len(conf.GetPublish()) != 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "bldr.star declares build, remote or publish entries; a plugin repository declares only manifests.")
	}
	if len(conf.GetManifests()) == 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "bldr.star declares no plugins.")
	}

	// Check each manifest in id order so refusals read the same each time.
	ids := make([]string, 0, len(conf.GetManifests()))
	for id := range conf.GetManifests() {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		v.checkManifest(id, conf.GetManifests()[id])
	}
	return nil
}

// checkManifest admits a JavaScript plugin manifest and refuses any other.
func (v *Validation) checkManifest(id string, mc *bldr_project.ManifestConfig) {
	// Refuse every builder but the JavaScript plugin compiler.
	name := strconv.Quote(id)
	switch builder := mc.GetBuilder().GetId(); builder {
	case bldr_plugin_compiler_js.ConfigID:
	case bldr_plugin_compiler_go.ConfigID:
		v.refuse(RefusalKind_REFUSAL_KIND_GO_PLUGIN, "Plugin "+name+" is a Go plugin; only JavaScript plugins may build from a repository.")
		return
	default:
		v.refuse(RefusalKind_REFUSAL_KIND_GO_PLUGIN, "Plugin "+name+" uses the "+strconv.Quote(builder)+" builder; only JavaScript plugins may build from a repository.")
		return
	}

	// Decode the JavaScript compiler config.
	conf := &bldr_plugin_compiler_js.Config{}
	if data := mc.GetBuilder().GetConfig(); len(data) != 0 {
		if err := conf.UnmarshalJSON(data); err != nil {
			v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "Plugin "+name+" has a config that does not parse: "+err.Error())
			return
		}
	}
	v.checkJsConfig(name, conf)

	// Record the plugin and its modules.
	plugin := &Plugin{ManifestId: id, Description: mc.GetDescription()}
	for _, mod := range conf.GetModules() {
		plugin.Modules = append(plugin.Modules, &Module{Kind: mod.GetKind(), Path: mod.GetPath()})
	}
	v.Plugins = append(v.Plugins, plugin)
}

// checkJsConfig refuses JavaScript compiler options that run or read code the
// validation cannot show, including in build and platform overrides.
func (v *Validation) checkJsConfig(name string, conf *bldr_plugin_compiler_js.Config) {
	// Refuse Vite configuration and bundles, which load project code.
	if len(conf.GetViteBundles()) != 0 || len(conf.GetViteConfigPaths()) != 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_VITE_CONFIG, "Plugin "+name+" sets a Vite config or bundle, which would run as code during the build.")
	}

	// Refuse module paths outside the project and per-module Vite configs.
	for _, mod := range conf.GetModules() {
		if len(mod.GetViteConfigPaths()) != 0 {
			v.refuse(RefusalKind_REFUSAL_KIND_VITE_CONFIG, "Plugin "+name+" sets a Vite config for module "+strconv.Quote(mod.GetPath())+", which would run as code during the build.")
		}
		if clean := path.Clean(mod.GetPath()); !fs.ValidPath(clean) || clean == "." {
			v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "Plugin "+name+" names module "+strconv.Quote(mod.GetPath())+" outside the repository.")
		}
	}

	// Refuse raw bundler options and host controllers.
	if len(conf.GetEsbuildBundles()) != 0 || len(conf.GetEsbuildFlags()) != 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "Plugin "+name+" sets esbuild bundles or flags; declare JavaScript modules instead.")
	}
	if len(conf.GetHostConfigSet()) != 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "Plugin "+name+" starts controllers on the plugin host.")
	}

	// Apply the same rules to every override.
	for _, overrides := range []map[string]*bldr_plugin_compiler_js.Config{conf.GetBuildTypes(), conf.GetPlatformTypes()} {
		for _, override := range overrides {
			v.checkJsConfig(name, override)
		}
	}
}
