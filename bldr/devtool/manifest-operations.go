//go:build !js

package devtool

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_pack "github.com/s4wave/spacewave/bldr/manifest/pack"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
)

// AssertManifestFetch requests a manifest on the devtool bus.
// The caller releases the reference when it no longer needs the fetch.
func (d *DevtoolBus) AssertManifestFetch(ctx context.Context, fetch bldr_manifest.FetchManifest, handler directive.ReferenceHandler) (directive.Instance, directive.Reference, error) {
	return d.b.AddDirective(ctx, fetch, handler)
}

// CollectManifests collects revisions of a manifest linked from the devtool host.
func (d *DevtoolBus) CollectManifests(ctx context.Context, manifestID string, platformIDs []string) ([]*bldr_manifest_world.CollectedManifest, []error, error) {
	return bldr_manifest_world.CollectManifestsForManifestID(ctx, d.worldState, manifestID, platformIDs, d.pluginHostObjectKey)
}

// ProduceManifestPack writes the requested artifact using the devtool bus, World and peer.
func (d *DevtoolBus) ProduceManifestPack(ctx context.Context, conf bldr_manifest_pack.ProducerConfig) (*bldr_manifest_pack.ManifestPackMetadata, error) {
	conf.Bus = d.b
	conf.WorldState = d.worldState
	conf.Sender = d.peerID
	return bldr_manifest_pack.ProduceManifestPack(ctx, &conf)
}

// ImportManifestPack imports a pack into the devtool World and verifies its manifests.
func (d *DevtoolBus) ImportManifestPack(ctx context.Context, meta *bldr_manifest_pack.ManifestPackMetadata, packBytes []byte) error {
	// Adopt the pack under the devtool peer's identity.
	if err := bldr_manifest_pack.ImportManifestPack(ctx, d.worldState, d.peerID, meta, packBytes); err != nil {
		return errors.Wrap(err, "import manifest pack")
	}

	// Check that the imported manifest tuples can be collected from the World.
	return errors.Wrap(bldr_manifest_pack.VerifyImportedManifests(ctx, d.worldState, meta), "verify imported manifest pack")
}
