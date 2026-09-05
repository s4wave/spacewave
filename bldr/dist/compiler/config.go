package bldr_dist_compiler

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/util/merge"
	"golang.org/x/mod/module"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/dist/compiler"

// NewConfig constructs a new config.
func NewConfig() *Config {
	return &Config{}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate host configuration and the project boundary before source inputs.
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
	if projectID := c.GetProjectId(); projectID != "" {
		if err := bldr_project.ValidateProjectID(projectID); err != nil {
			return err
		}
	}

	// Require a supported startup source and an exact platform for every embed.
	if _, err := c.ParseWebStartupPath(); err != nil {
		return err
	}
	for i, em := range c.GetEmbedManifests() {
		if em == nil {
			return errors.Errorf("embed_manifests[%d]: nil entry", i)
		}
		if em.GetManifestId() == "" {
			return errors.Errorf("embed_manifests[%d]: manifest_id is required", i)
		}
		if em.GetPlatformId() == "" {
			return errors.Errorf("embed_manifests[%d]: platform_id is required (fully qualified, e.g. desktop/darwin/arm64, js)", i)
		}
		if _, err := bldr_platform.ParsePlatform(em.GetPlatformId()); err != nil {
			return errors.Wrapf(err, "embed_manifests[%d]: platform_id", i)
		}
	}

	// Native CLI packages must resolve as Go import paths.
	for i, impPath := range c.GetCliPkgs() {
		impPath = strings.TrimPrefix(impPath, "./")
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "cli_pkgs[%d]: invalid import path", i)
		}
	}
	return nil
}

// ParseWebStartupPath validates and cleans the web startup path.
// If unset, returns "", nil.
func (c *Config) ParseWebStartupPath() (string, error) {
	// An omitted startup source leaves browser startup to the distribution.
	startupPath := c.GetLoadWebStartup()
	if len(startupPath) == 0 {
		return "", nil
	}

	// Normalize the project-relative path before validating its file type.
	startupPath = path.Clean(startupPath)
	if startupPath[0] == '/' {
		return "", errors.New("load_web_startup: must be a relative path")
	}
	startupPathExt := path.Ext(startupPath)
	if startupPathExt != ".js" && startupPathExt != ".tsx" && startupPathExt != ".ts" {
		return "", errors.New("load_web_startup: must be a .js, .tsx, or .ts file")
	}
	if strings.HasPrefix(startupPath, "../") {
		return "", errors.New("load_web_startup: must be relative to ./")
	}
	return startupPath, nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
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

	// Allocate host configuration before applying layered overrides.
	c.Alloc()

	// Merge embedded manifests by their exact manifest/platform identity.
	for _, em := range o.GetEmbedManifests() {
		if em == nil {
			continue
		}
		if embedManifestIndex(c.EmbedManifests, em) >= 0 {
			continue
		}
		c.EmbedManifests = append(c.EmbedManifests, em.CloneVT())
	}
	slices.SortFunc(c.EmbedManifests, compareEmbedManifest)

	// Merge plugin and CLI imports without duplicate startup work.
	merge.MergeAndSortSlices(&c.LoadPlugins, o.GetLoadPlugins())
	merge.MergeAndSortSlices(&c.CliPkgs, o.GetCliPkgs())

	// Merge controller configuration through its existing override rules.
	configset_proto.MergeConfigSetMaps(c.HostConfigSet, o.GetHostConfigSet())

	// Override project identity only when explicitly configured.
	if cproj := o.GetProjectId(); cproj != "" {
		c.ProjectId = cproj
	}

	// Preserve unspecified compiler options and apply explicit selections.
	c.EnableCgo = c.EnableCgo.Merge(o.GetEnableCgo())
	if goCompiler := o.GetGoCompiler(); goCompiler != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_DEFAULT {
		c.GoCompiler = goCompiler
	}
	c.EnableCompression = c.EnableCompression.Merge(o.GetEnableCompression())
	c.EmbedNativeVolume = c.EmbedNativeVolume.Merge(o.GetEmbedNativeVolume())
}

// Normalize sorts and deduplicates the fields.
func (c *Config) Normalize() {
	if c == nil {
		return
	}

	// Canonicalize manifest tuples before comparing or caching configuration.
	slices.SortFunc(c.EmbedManifests, compareEmbedManifest)
	c.EmbedManifests = slices.CompactFunc(c.EmbedManifests, equalEmbedManifest)

	// Canonicalize plugin and command imports independently of input order.
	slices.Sort(c.LoadPlugins)
	c.LoadPlugins = slices.Compact(c.LoadPlugins)
	slices.Sort(c.CliPkgs)
	c.CliPkgs = slices.Compact(c.CliPkgs)
}

// compareEmbedManifest orders EmbedManifest entries by (manifest_id, platform_id).
func compareEmbedManifest(a, b *EmbedManifest) int {
	if c := cmp.Compare(a.GetManifestId(), b.GetManifestId()); c != 0 {
		return c
	}
	return cmp.Compare(a.GetPlatformId(), b.GetPlatformId())
}

// equalEmbedManifest reports whether two EmbedManifest entries refer to the
// same (manifest_id, platform_id) tuple.
func equalEmbedManifest(a, b *EmbedManifest) bool {
	return a.GetManifestId() == b.GetManifestId() &&
		a.GetPlatformId() == b.GetPlatformId()
}

// embedManifestIndex returns the index of em in s by (manifest_id, platform_id),
// or -1 if not present.
func embedManifestIndex(s []*EmbedManifest, em *EmbedManifest) int {
	for i, existing := range s {
		if equalEmbedManifest(existing, em) {
			return i
		}
	}
	return -1
}

// _ is a type assertion
var _ builder.ControllerConfig = (*Config)(nil)
