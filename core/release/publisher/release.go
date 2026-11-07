package publisher

import (
	"cmp"
	"context"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
)

// ReleaseOptions configures one release of a Deployment.
type ReleaseOptions struct {
	// Deployment names the Space and the identifiers launchers mount.
	Deployment Deployment
	// Metadata is the release's channel metadata; its channel defaults to the
	// Deployment's.
	Metadata *release.ReleaseMetadata
	// ReleaseKey signs each selected manifest root.
	ReleaseKey crypto.PrivKey
	// DistKey signs the DistConfig; launchers pin its peer.
	DistKey crypto.PrivKey
	// Rev is the DistConfig control revision, above every earlier selector's.
	Rev uint64
	// Retain is the revisions of each manifest platform kept in the release
	// World; zero keeps DefaultRetain.
	Retain int
	// LauncherConfigs are applied to the launcher bus with the selector.
	LauncherConfigs map[string]*configset_proto.ControllerConfig
	// Publish writes the Deployment's Space. Its Space and CDN URL default to
	// the Deployment's.
	Publish cdn_publish.Options
}

// Released is the result of a release.
type Released struct {
	// Metadata is the staged channel metadata.
	Metadata *release.ReleaseMetadata
	// Checkpoint is the checkpoint the Space posted.
	Checkpoint *sobject.SOCheckpointInner
	// Head is the release World root the checkpoint names.
	Head *bucket.ObjectRef
	// DistConfig selects the channel for launchers.
	DistConfig *spacewave_launcher.DistConfig
	// Signed is DistConfig as the signed packed message launchers fetch.
	Signed string
}

// Release stages the channel in eng, publishes the release World to the
// Deployment's Space, and only then signs the DistConfig selecting the
// channel. Launchers mount the head of the Space's root pointer, so the root
// is posted before any selector naming the release exists.
//
// A launcher adopts a selector and fetches its root at once, so no selector
// is signed before its root is durable: a manifest whose content the World
// lacks stops the release before any upload, and a failed upload or
// checkpoint returns before signing. The caller must exclusively own eng and
// the Space's publisher for the release.
//
// Before staging, each manifest platform's revisions past the newest Retain
// are deleted from the release World, and the Space reclaims their content
// once the release is posted.
func Release(ctx context.Context, eng world.Engine, opts ReleaseOptions) (*Released, error) {
	return orderedRelease(ctx, eng, opts, nil)
}

// Reissue rolls back by releasing a copy of the retained manifest meta names
// at a revision above the newest of its platform. Launchers adopt the highest
// revision, so a rollback is a release. The copy is staged in the same
// transaction as the channel.
func Reissue(ctx context.Context, eng world.Engine, opts ReleaseOptions, meta *bldr_manifest.ManifestMeta) (*Released, error) {
	if meta == nil {
		return nil, errors.New("manifest meta to reissue is required")
	}
	manifestKey := opts.Deployment.withDefaults().ManifestKey
	return orderedRelease(ctx, eng, opts, func(ctx context.Context, ws world.WorldState) error {
		_, err := ReissueManifest(ctx, ws, manifestKey, meta)
		return err
	})
}

// orderedRelease runs the release, calling prepare on the staging
// transaction before retention and the channel are applied.
func orderedRelease(ctx context.Context, eng world.Engine, opts ReleaseOptions, prepare func(context.Context, world.WorldState) error) (*Released, error) {
	// Require the metadata and the selector's signer.
	d := opts.Deployment.withDefaults()
	if opts.Metadata == nil || opts.DistKey == nil {
		return nil, errors.New("release metadata and DistConfig key are required")
	}

	// Bind the metadata to the Deployment's project and channel.
	metadata := opts.Metadata.CloneVT()
	if metadata.GetChannelKey() == "" {
		metadata.ChannelKey = d.ChannelKey
	}
	if metadata.GetChannelKey() != d.ChannelKey {
		return nil, errors.Errorf("release channel %q is not the deployment's %q", metadata.GetChannelKey(), d.ChannelKey)
	}
	if metadata.GetProjectId() == "" {
		metadata.ProjectId = d.ProjectID
	}

	// Publish to the Deployment's Space.
	pub := opts.Publish
	if pub.DstSpaceID == "" {
		pub.DstSpaceID = d.SpaceID
	}
	if pub.CdnBaseURL == "" {
		pub.CdnBaseURL = d.CdnBaseURL
	}
	if pub.DstSpaceID != d.SpaceID {
		return nil, errors.Errorf("publication Space %q is not the deployment's %q", pub.DstSpaceID, d.SpaceID)
	}

	// Build the selector before any write, so a bad Deployment fails first.
	conf, err := d.DistConfig(opts.Rev, opts.LauncherConfigs)
	if err != nil {
		return nil, err
	}

	// Stage the channel over the retained manifests and commit it locally.
	retain := cmp.Or(opts.Retain, DefaultRetain)
	staged, err := stage(ctx, eng, d.ManifestKey, retain, metadata, opts.ReleaseKey, prepare)
	if err != nil {
		return nil, errors.Wrap(err, "stage release")
	}

	// Publish verifies every selected manifest's closure before uploading and
	// returns once the checkpoint naming the root is posted.
	checkpoint, head, err := publish(ctx, eng, staged, pub)
	if err != nil {
		return nil, errors.Wrap(err, "publish release")
	}

	// Sign the selector once the root is posted.
	signed, err := SignDistConfig(opts.DistKey, conf)
	if err != nil {
		return nil, err
	}
	return &Released{Metadata: staged, Checkpoint: checkpoint, Head: head, DistConfig: conf, Signed: signed}, nil
}

// stage applies prepare and retention, then records metadata's channel, in
// one committed write transaction.
func stage(
	ctx context.Context,
	eng world.Engine,
	manifestKey string,
	retain int,
	metadata *release.ReleaseMetadata,
	releaseKey crypto.PrivKey,
	prepare func(context.Context, world.WorldState) error,
) (*release.ReleaseMetadata, error) {
	// Stage in one write transaction, discarded unless it commits.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Apply the caller's change, then keep the newest retain revisions.
	if prepare != nil {
		if err := prepare(ctx, tx); err != nil {
			return nil, err
		}
	}
	if _, err := PruneManifests(ctx, tx, manifestKey, retain); err != nil {
		return nil, errors.Wrap(err, "retain manifests")
	}

	// Stage the channel and commit it.
	staged, err := StageChannel(ctx, tx, manifestKey, metadata, releaseKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return staged, nil
}
