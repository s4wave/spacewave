package bldr_plugin_compiler_go

import (
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/bldr/util/merge"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"golang.org/x/mod/module"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/plugin/compiler/go"

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
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// UpdateRelativeGoPackagePaths applies the root module path to the go_packages list.
// Returns the updated packages list and the mappings from goPkg to goPkgName.
func UpdateRelativeGoPackagePaths(goPkgsList []string, rootModule string) ([]string, map[string]string) {
	mappings := make(map[string]string, len(goPkgsList))
	pkgs := make([]string, len(goPkgsList))
	for i, goPkgName := range goPkgsList {
		if strings.HasPrefix(goPkgName, "./") {
			goPkgName = strings.Join([]string{rootModule, goPkgName[2:]}, "/")
		}
		pkgs[i] = goPkgName
		mappings[goPkgsList[i]] = goPkgName
	}
	return pkgs, mappings
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if projID := c.GetProjectId(); projID != "" {
		if err := bldr_project.ValidateProjectID(projID); err != nil {
			return errors.Wrap(err, "project_id")
		}
	}
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
	for i, impPath := range c.GetGoPkgs() {
		// relative paths will be resolved later
		impPath = strings.TrimPrefix(impPath, "./")
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "go_packages[%d]: invalid import path", i)
		}
	}
	if dlvAddr := c.GetDelveAddr(); dlvAddr != "" {
		if err := ValidateDelveAddr(dlvAddr); err != nil {
			return errors.Wrap(err, "delve_addr")
		}
	}
	if _, err := c.ParseEsbuildFlags(); err != nil {
		return errors.Wrap(err, "esbuild_flags")
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
	if c.ConfigSet == nil {
		c.ConfigSet = make(map[string]*configset_proto.ControllerConfig)
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
	configset_proto.MergeConfigSetMaps(c.ConfigSet, o.GetConfigSet())
	configset_proto.MergeConfigSetMaps(c.HostConfigSet, o.GetHostConfigSet())

	// append and sort go packages list
	merge.MergeAndSortSlices(&c.GoPkgs, o.GetGoPkgs())

	// append and sort web packages list
	for _, webPkgConfig := range o.GetWebPkgs() {
		c.WebPkgs, _ = bldr_web_bundler.WebPkgRefConfigSlice(c.WebPkgs).AppendWebPkgRefConfig(webPkgConfig)
	}
	bldr_web_bundler.SortWebPkgRefConfigs(c.WebPkgs)

	// override project id
	if cproj := o.GetProjectId(); cproj != "" {
		c.ProjectId = cproj
	}

	// override web plugin id
	if webPluginID := o.GetWebPluginId(); webPluginID != "" {
		c.WebPluginId = webPluginID
	}

	if o.GetDisableRpcFetch() {
		c.DisableRpcFetch = true
	}

	if daddr := o.GetDelveAddr(); daddr != "" {
		c.DelveAddr = daddr
	}

	c.EnableCgo = c.EnableCgo.Merge(o.GetEnableCgo())
	c.EnableTinygo = c.EnableCgo.Merge(o.GetEnableTinygo())
	c.EnableCompression = c.EnableCompression.Merge(o.GetEnableCgo())

	if esbuildFlags := o.GetEsbuildFlags(); len(esbuildFlags) != 0 {
		c.EsbuildFlags = append(c.EsbuildFlags, esbuildFlags...)
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

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
