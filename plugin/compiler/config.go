package bldr_plugin_compiler

import (
	"sort"
	"strings"

	builder "github.com/aperturerobotics/bldr/manifest/builder"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	esbuild_cli "github.com/evanw/esbuild/pkg/cli"
	shellquote "github.com/kballard/go-shellquote"
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

	return nil
}

// ParseEsbuildFlags parsed the esbuild flags field, if set.
// Returns nil if no flags were set.
func (c *Config) ParseEsbuildFlags() (*esbuild_api.BuildOptions, error) {
	var args []string
	for _, flagStr := range c.GetEsbuildFlags() {
		flagArgs, err := shellquote.Split(flagStr)
		if err != nil {
			return nil, err
		}
		args = append(args, flagArgs...)
	}
	if len(args) == 0 {
		return nil, nil
	}

	opts, err := esbuild_cli.ParseBuildOptions(args)
	if err != nil {
		return nil, err
	}
	return &opts, nil
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

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
