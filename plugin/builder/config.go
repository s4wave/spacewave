package plugin_builder

import (
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// Validate validates the configuration.
func (c *PluginBuilderConfig) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if len(c.GetPluginPlatformId()) == 0 {
		return plugin.ErrEmptyPlatformID
	}
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if c.GetSourcePath() == "" {
		return errors.Wrap(plugin.ErrEmptyPath, "source path")
	}
	if !path.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if c.GetWorkingPath() == "" {
		return errors.Wrap(plugin.ErrEmptyPath, "working path")
	}
	if !path.IsAbs(c.GetWorkingPath()) {
		return errors.New("working path must be absolute")
	}
	if err := plugin.ToBuildType(c.GetBuildType()).Validate(false); err != nil {
		return err
	}
	return nil
}

// SetPluginId configures the plugin ID to build.
func (c *PluginBuilderConfig) SetPluginId(pluginID string) {
	c.PluginId = pluginID
}

// SetEngineId configures the world engine ID to attach to.
func (c *PluginBuilderConfig) SetEngineId(worldEngineID string) {
	c.EngineId = worldEngineID
}

// SetPluginHostKey configures the plugin host object key.
func (c *PluginBuilderConfig) SetPluginHostKey(pluginHostObjKey string) {
	c.PluginHostKey = pluginHostObjKey
}

// SetPluginPlatformId configures the platform ID to compile for.
func (c *PluginBuilderConfig) SetPluginPlatformId(pluginPlatformID string) {
	c.PluginPlatformId = pluginPlatformID
}

// SetSourcePath configures the path to the source code root.
func (c *PluginBuilderConfig) SetSourcePath(sourcePath string) {
	c.SourcePath = sourcePath
}

// SetWorkingPath configures the path to the working root.
func (c *PluginBuilderConfig) SetWorkingPath(workingPath string) {
	c.WorkingPath = workingPath
}

// ParsePeerID parses the peer id field.
func (c *PluginBuilderConfig) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
