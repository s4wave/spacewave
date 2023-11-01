package bldr_plugin_compiler

import (
	"sort"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/module"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

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

// GetConfigID returns the unique string for this configuration type.

// UpdateRelativeGoPackagePaths applies the root module path to the go_packages list.
func UpdateRelativeGoPackagePaths(goPkgsList []string, rootModule string) []string {
	pkgs := make([]string, len(goPkgsList))
	for i, goPkgName := range goPkgsList {
		if strings.HasPrefix(goPkgName, "./") {
			goPkgName = strings.Join([]string{rootModule, goPkgName[2:]}, "/")
		}
		pkgs[i] = goPkgName
	}
	return pkgs
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
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
	return bldr_esbuild.ParseEsbuildFlags(c.GetEsbuildFlags())
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

	mergeList := func(mergeTo *[]string, mergeFrom []string) {
		var dirty bool
		dest := *mergeTo
		for _, value := range mergeFrom {
			if value != "" && !slices.Contains(dest, value) {
				dirty = true
				dest = append(dest, value)
			}
		}
		if dirty {
			sort.Strings(dest)
			*mergeTo = dest
		}
	}

	// append and sort go packages list
	mergeList(&c.GoPkgs, o.GetGoPkgs())

	// append and sort web packages list
	mergeList(&c.WebPkgs, o.GetWebPkgs())

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

	if o.GetDisableFetchAssets() {
		c.DisableFetchAssets = true
	}

	if daddr := o.GetDelveAddr(); daddr != "" {
		c.DelveAddr = daddr
	}

	if o.GetEnableCgo() {
		c.EnableCgo = true
	}

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

		buildTypeConfig, ok := conf.GetBuildTypes()[filterBuildType.String()]
		if ok && !slices.Contains(mergeConfigs, buildTypeConfig) {
			mergeConfigs = append(mergeConfigs, buildTypeConfig)
			continue
		}

		// clear BuildTypes and dequeue
		conf.BuildTypes = nil
		mergeConfigs[len(mergeConfigs)-1] = nil
		mergeConfigs = mergeConfigs[:len(mergeConfigs)-1]

		// merge into base config
		if conf != c {
			c.Merge(conf)
		}
	}
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
