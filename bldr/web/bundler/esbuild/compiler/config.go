package bldr_web_bundler_esbuild_compiler

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/web/bundler/esbuild/compiler"

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
	for platformTypeStr, platformTypeConf := range c.GetPlatformTypes() {
		if platformTypeStr == "" {
			return errors.New("platform_types key cannot be empty")
		}
		if err := platformTypeConf.Validate(); err != nil {
			return errors.Wrapf(err, "platform_types[%s]", platformTypeStr)
		}
	}
	for _, bundle := range c.GetBundles() {
		if err := bundle.Validate(); err != nil {
			return errors.Wrap(err, "bundle")
		}
	}

	return nil
}

// ParseEsbuildFlags parsed the esbuild flags field, if set.
// Returns nil if no flags were set.
func (c *Config) ParseEsbuildFlags() (*esbuild_api.BuildOptions, error) {
	return bldr_esbuild_build.ParseEsbuildFlags(c.GetEsbuildFlags())
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

	// merge esbuild flags
	if esbuildFlags := o.GetEsbuildFlags(); len(esbuildFlags) != 0 {
		c.EsbuildFlags = append(c.EsbuildFlags, esbuildFlags...)
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

	// merge platform types
	if platformTypes := o.GetPlatformTypes(); len(platformTypes) != 0 {
		if c.PlatformTypes == nil {
			c.PlatformTypes = make(map[string]*Config)
		}
		for platformType, platformConfig := range platformTypes {
			if existing, ok := c.PlatformTypes[platformType]; ok {
				existing.Merge(platformConfig)
			} else {
				c.PlatformTypes[platformType] = platformConfig
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

// FlattenPlatformTypes flattens the platform_types tree given the current build platform.
//
// Checks both the full platform ID (e.g., "desktop/darwin/arm64") and the base
// platform ID (e.g., "desktop"). Full ID match is applied first, then base ID.
// Clears the PlatformTypes field and applies all relevant overrides to c.
func (c *Config) FlattenPlatformTypes(buildPlatform bldr_platform.Platform) {
	platformTypes := c.GetPlatformTypes()
	c.PlatformTypes = nil
	if len(platformTypes) == 0 {
		return
	}

	fullID := buildPlatform.GetPlatformID()
	baseID := buildPlatform.GetBasePlatformID()

	// Apply full platform ID match first.
	if conf, ok := platformTypes[fullID]; ok {
		conf.PlatformTypes = nil
		c.Merge(conf)
	}
	// Apply base platform ID match second (if different from full).
	if baseID != fullID {
		if conf, ok := platformTypes[baseID]; ok {
			conf.PlatformTypes = nil
			c.Merge(conf)
		}
	}
}

// Validate validates the EsbuildBundleMeta configuration.
func (e *EsbuildBundleMeta) Validate() error {
	if e.GetId() == "" {
		return errors.New("bundle id is required")
	}
	if _, err := bldr_esbuild_build.ParseEsbuildFlags(e.GetEsbuildFlags()); err != nil {
		return errors.Wrap(err, "esbuild_flags")
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
