package bldr_web_bundler_vite_compiler

import (
	"github.com/aperturerobotics/bldr/manifest"
	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/web/bundler/vite/compiler"

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

// Validate validates the configuration.
func (c *Config) Validate() error {
	for buildTypeStr, buildTypeConf := range c.GetBuildTypes() {
		if err := bldr_manifest.BuildType(buildTypeStr).Validate(false); err != nil {
			return err
		}
		if err := buildTypeConf.Validate(); err != nil {
			return errors.Wrapf(err, "build_types[%s]", buildTypeStr)
		}
	}
	for _, bundle := range c.GetBundles() {
		if err := bundle.Validate(); err != nil {
			return errors.Wrap(err, "bundle")
		}
	}

	return nil
}

// Merge merges the given build config into c.
func (c *Config) Merge(o *Config) {
	if o == nil {
		return
	}

	// append and sort web packages list
	for _, webPkgConfig := range o.GetWebPkgs() {
		c.WebPkgs, _ = bldr_web_bundler.WebPkgRefConfigSlice(c.WebPkgs).AppendWebPkgRefConfig(webPkgConfig)
	}
	bldr_web_bundler.SortWebPkgRefConfigs(c.WebPkgs)

	// merge bundles
	if bundles := o.GetBundles(); len(bundles) != 0 {
		c.Bundles = append(c.Bundles, bundles...)
	}

	// merge build types
	if buildTypes := o.GetBuildTypes(); len(buildTypes) != 0 {
		if c.BuildTypes == nil {
			c.BuildTypes = make(map[string]*Config)
		}
		for buildType, buildConfig := range buildTypes {
			if existing, ok := c.BuildTypes[buildType]; ok {
				existing.Merge(buildConfig)
			} else {
				c.BuildTypes[buildType] = buildConfig
			}
		}
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

// Validate validates the ViteBundleMeta configuration.
func (e *ViteBundleMeta) Validate() error {
	if e.GetId() == "" {
		return errors.New("bundle id is required")
	}
	for _, entrypoint := range e.GetEntrypoints() {
		if err := entrypoint.Validate(); err != nil {
			return errors.Wrap(err, "entrypoint")
		}
	}
	return nil
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
