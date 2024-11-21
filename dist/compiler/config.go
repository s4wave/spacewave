package bldr_dist_compiler

import (
	"path"
	"slices"
	"strings"

	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/bldr/util/merge"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
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
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
	if projectID := c.GetProjectId(); projectID != "" {
		if err := bldr_project.ValidateProjectID(projectID); err != nil {
			return err
		}
	}
	if _, err := c.ParseWebStartupPath(); err != nil {
		return err
	}
	return nil
}

// ParseWebStartupPath validates and cleans the web startup path.
// If unset, returns "", nil
func (c *Config) ParseWebStartupPath() (string, error) {
	startupPath := c.GetLoadWebStartup()
	if len(startupPath) == 0 {
		return "", nil
	}
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

	// allocate any maps
	c.Alloc()

	// merge EmbedManifests
	merge.MergeAndSortSlices(&c.EmbedManifests, o.GetEmbedManifests())

	// merge LoadPlugins
	merge.MergeAndSortSlices(&c.LoadPlugins, o.GetLoadPlugins())

	// merge config sets
	configset_proto.MergeConfigSetMaps(c.HostConfigSet, o.GetHostConfigSet())

	// override project id
	if cproj := o.GetProjectId(); cproj != "" {
		c.ProjectId = cproj
	}

	c.EnableCgo = c.EnableCgo.Merge(o.GetEnableCgo())
	c.EnableTinygo = c.EnableCgo.Merge(o.GetEnableTinygo())
	c.EnableCompression = c.EnableCompression.Merge(o.GetEnableCgo())
}

// Normalize sorts and deduplicates the fields.
func (c *Config) Normalize() {
	if c == nil {
		return
	}

	slices.Sort(c.EmbedManifests)
	c.EmbedManifests = slices.Compact(c.EmbedManifests)

	slices.Sort(c.LoadPlugins)
	c.LoadPlugins = slices.Compact(c.LoadPlugins)
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
