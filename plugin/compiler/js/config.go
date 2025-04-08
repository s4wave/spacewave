package bldr_plugin_compiler_js

import (
	"path"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/plugin/compiler/js"

// NewConfig constructs a new config.
func NewConfig() *Config {
	return &Config{}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// UpdateRelativeGoPackagePaths applies the root module path to the js_packages list.
// Returns the updated packages list and the mappings from jsPkg to jsPkgName.
func UpdateRelativeGoPackagePaths(jsPkgsList []string, rootModule string) ([]string, map[string]string) {
	mappings := make(map[string]string, len(jsPkgsList))
	pkgs := make([]string, len(jsPkgsList))
	for i, jsPkgName := range jsPkgsList {
		if strings.HasPrefix(jsPkgName, "./") {
			jsPkgName = strings.Join([]string{rootModule, jsPkgName[2:]}, "/")
		}
		pkgs[i] = jsPkgName
		mappings[jsPkgsList[i]] = jsPkgName
	}
	return pkgs, mappings
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	for i, moduleConf := range c.GetModules() {
		if err := moduleConf.Validate(); err != nil {
			return errors.Wrapf(err, "modules[%d]", i)
		}
	}
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
	if _, err := c.ParseEsbuildFlags(); err != nil {
		return errors.Wrap(err, "esbuild_flags")
	}
	for i, beConf := range c.GetBackendEntrypoints() {
		if err := beConf.Validate(); err != nil {
			return errors.Wrapf(err, "backend_entrypoints[%d]", i)
		}
	}
	for i, feConf := range c.GetFrontendEntrypoints() {
		if err := feConf.Validate(); err != nil {
			return errors.Wrapf(err, "frontend_entrypoints[%d]", i)
		}
	}
	for i, bundleConf := range c.GetEsbuildBundles() {
		if err := bundleConf.Validate(); err != nil {
			return errors.Wrapf(err, "esbuild_bundles[%d]", i)
		}
	}
	for i, bundleConf := range c.GetViteBundles() {
		if err := bundleConf.Validate(); err != nil {
			return errors.Wrapf(err, "vite_bundles[%d]", i)
		}
	}
	for buildTypeStr, buildTypeConf := range c.GetBuildTypes() {
		if err := bldr_manifest.BuildType(buildTypeStr).Validate(false); err != nil {
			return err
		}
		if err := buildTypeConf.Validate(); err != nil {
			return errors.Wrapf(err, "build_types[%s]", buildTypeStr)
		}
	}

	return nil
}

// ParseEsbuildFlags parsed the esbuild flags field, if set.
// Returns nil if no flags were set.
func (c *Config) ParseEsbuildFlags() (*esbuild_api.BuildOptions, error) {
	return bldr_esbuild_build.ParseEsbuildFlags(c.GetEsbuildFlags())
}

// Alloc allocates any nil maps.
func (c *Config) Alloc() {
	if c == nil {
		return
	}
	if c.HostConfigSet == nil {
		c.HostConfigSet = make(map[string]*configset_proto.ControllerConfig)
	}
}

// Merge merges the given build config into c.
func (c *Config) Merge(o *Config) {
	if o == nil {
		return
	}

	// allocate any maps
	c.Alloc()

	// merge config sets
	configset_proto.MergeConfigSetMaps(c.HostConfigSet, o.GetHostConfigSet())

	// append and sort web packages list
	for _, webPkgConfig := range o.GetWebPkgs() {
		c.WebPkgs, _ = bldr_web_bundler.WebPkgRefConfigSlice(c.WebPkgs).AppendWebPkgRefConfig(webPkgConfig)
	}
	bldr_web_bundler.SortWebPkgRefConfigs(c.WebPkgs)

	// override web plugin id
	if webPluginID := o.GetWebPluginId(); webPluginID != "" {
		c.WebPluginId = webPluginID
	}

	if o.GetDisableRpcFetch() {
		c.DisableRpcFetch = true
	}

	if o.GetDisableFetchAssets() {
		c.DisableFetchAssets = true
	}

	if esbuildFlags := o.GetEsbuildFlags(); len(esbuildFlags) != 0 {
		c.EsbuildFlags = append(c.EsbuildFlags, esbuildFlags...)
	}

	// Merge EsbuildBundles
	if esbuildBundles := o.GetEsbuildBundles(); len(esbuildBundles) != 0 {
		c.EsbuildBundles = append(c.EsbuildBundles, esbuildBundles...)
	}

	// Merge ViteBundles
	if viteBundles := o.GetViteBundles(); len(viteBundles) != 0 {
		c.ViteBundles = append(c.ViteBundles, viteBundles...)
	}

	// Merge ViteConfigPaths
	if viteConfigPaths := o.GetViteConfigPaths(); len(viteConfigPaths) != 0 {
		c.ViteConfigPaths = append(c.ViteConfigPaths, viteConfigPaths...)
	}

	// Override ViteDisableProjectConfig if true
	if o.GetViteDisableProjectConfig() {
		c.ViteDisableProjectConfig = true
	}
}

// FlattenBuildTypes flattens the build_type tree given the current build type.
//
// Clears the BuildTypes field and applies all relevant BuildType overrides to c.
func (c *Config) FlattenBuildTypes(filterBuildType bldr_manifest.BuildType) {
	mergeConfigs := []*Config{c}
	for len(mergeConfigs) != 0 {
		conf := mergeConfigs[len(mergeConfigs)-1]

		// Find the config for this specific build type (e.g., "dev" or "release")
		buildTypeConfig, ok := conf.GetBuildTypes()[filterBuildType.String()]
		conf.BuildTypes = nil // Clear the build types to avoid recursion
		if ok && !slices.Contains(mergeConfigs, buildTypeConfig) {
			// Add the build-type specific config to be processed
			mergeConfigs = append(mergeConfigs, buildTypeConfig)
			continue
		}

		// dequeue the current config from the stack
		mergeConfigs[len(mergeConfigs)-1] = nil
		mergeConfigs = mergeConfigs[:len(mergeConfigs)-1]

		// merge into base config (but skip if it's the original config to avoid self-merge)
		if conf != c {
			c.Merge(conf)
		}
	}
}

// Validate validates the JsModule configuration.
func (m *JsModule) Validate() error {
	if m.GetKind() == JsModuleKind_JS_MODULE_KIND_INVALID {
		return errors.New("js module kind cannot be invalid")
	}
	if m.GetPath() == "" {
		return errors.New("js module path cannot be empty")
	}
	// Note: vite_config_paths and disable_project_config are validated within the Vite compiler.
	return nil
}

// Validate validates the BackendEntrypoint configuration.
func (m *BackendEntrypoint) Validate() error {
	importPath := m.GetImportPath()
	if importPath == "" {
		return errors.New("backend entrypoint import path cannot be empty")
	}
	// Clean the path and check for path traversal attempts.
	// Note: path.Clean uses forward slashes regardless of OS.
	cleanedPath := path.Clean(importPath)
	if strings.HasPrefix(cleanedPath, "../") {
		return errors.Errorf("backend entrypoint import path cannot start with '..': %s", importPath)
	}
	// ImportName defaults to "default" if empty, so no validation needed.
	return nil
}

// Validate validates the FrontendEntrypoint configuration.
func (m *FrontendEntrypoint) Validate() error {
	importPath := m.GetImportPath()
	if importPath == "" {
		return errors.New("frontend entrypoint import path cannot be empty")
	}
	// Clean the path and check for path traversal attempts.
	// Note: path.Clean uses forward slashes regardless of OS.
	cleanedPath := path.Clean(importPath)
	if strings.HasPrefix(cleanedPath, "../") {
		return errors.Errorf("frontend entrypoint import path cannot start with '..': %s", importPath)
	}
	return nil
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
