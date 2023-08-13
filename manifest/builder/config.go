package bldr_manifest_builder

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Validate validates the configuration.
func (c *BuilderConfig) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if err := c.GetManifestMeta().Validate(false); err != nil {
		return err
	}
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if c.GetSourcePath() == "" {
		return errors.Wrap(manifest.ErrEmptyPath, "source path")
	}
	if !filepath.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if c.GetWorkingPath() == "" {
		return errors.Wrap(manifest.ErrEmptyPath, "working path")
	}
	if !filepath.IsAbs(c.GetWorkingPath()) {
		return errors.New("working path must be absolute")
	}
	return nil
}

// ParsePeerID parses the peer id field.
func (c *BuilderConfig) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// CommitManifest is a shortcut for CommitManifest.
func (c *BuilderConfig) CommitManifest(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFs,
	assetsFs fs.FS,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	pid, err := c.ParsePeerID()
	if err != nil {
		return nil, nil, err
	}
	return manifest_world.CommitManifest(
		ctx,
		le,
		ws,
		ws.AccessWorldState,
		meta,
		entrypointFilename,
		distFs,
		assetsFs,
		c.GetObjectKey(),
		c.GetLinkObjectKeys(),
		pid,
		timestamp.Now(),
	)
}
