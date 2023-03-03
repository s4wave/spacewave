package bldr_plugin_builder

import (
	"context"
	"io/fs"
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Validate validates the configuration.
func (c *PluginBuilderConfig) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if err := c.GetPluginManifestMeta().Validate(false); err != nil {
		return err
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
	return nil
}

// SetPluginManifestMeta sets the plugin manifest metadata.
func (c *PluginBuilderConfig) SetPluginManifestMeta(meta *plugin.PluginManifestMeta) {
	c.PluginManifestMeta = meta
}

// SetEngineId configures the world engine ID to attach to.
func (c *PluginBuilderConfig) SetEngineId(worldEngineID string) {
	c.EngineId = worldEngineID
}

// SetObjectKey configures the target plugin object key.
func (c *PluginBuilderConfig) SetObjectKey(pluginHostObjKey string) {
	c.ObjectKey = pluginHostObjKey
}

// SetLinkObjectKeys configures the list of object keys to link to.
func (c *PluginBuilderConfig) SetLinkObjectKeys(keys []string) {
	c.LinkObjectKeys = keys
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

// CommitPluginManifest is a shortcut for plugin_host.CommitPluginManifest.
func (c *PluginBuilderConfig) CommitPluginManifest(
	ctx context.Context,
	le *logrus.Entry,
	engine world.Engine,
	meta *plugin.PluginManifestMeta,
	entrypointFilename string,
	distFs,
	assetsFs fs.FS,
) (*plugin.PluginManifest, *bucket.ObjectRef, error) {
	pid, err := c.ParsePeerID()
	if err != nil {
		return nil, nil, err
	}
	ts := timestamp.Now()
	return plugin_host.CommitPluginManifest(
		ctx,
		le,
		engine,
		meta,
		entrypointFilename,
		distFs,
		assetsFs,
		c.GetObjectKey(),
		c.GetLinkObjectKeys(),
		pid,
		&ts,
	)
}
