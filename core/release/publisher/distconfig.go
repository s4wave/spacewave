package publisher

import (
	"cmp"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/crypto"
)

// Deployment names where one app's releases are published and how its
// launchers mount them. The identifiers derive from ProjectID unless set, so a
// publisher and its launchers agree.
type Deployment struct {
	// ProjectID is the Bldr project ID the DistConfig is for.
	ProjectID string
	// ChannelKey selects the release channel.
	ChannelKey string
	// SpaceID is the CDN Space holding the release World.
	SpaceID string
	// CdnBaseURL serves the Space's packs.
	CdnBaseURL string
	// RootPointerBaseURL serves the Space's root pointer when it is not
	// CdnBaseURL.
	RootPointerBaseURL string
	// CacheBlockStoreID is the launcher block store caching the Space's
	// blocks, or empty for none.
	CacheBlockStoreID string
	// ReleaseAuthorityPluginID is the launcher plugin pinning release
	// signers, or empty for none.
	ReleaseAuthorityPluginID string

	// EngineID is the release World engine ID; default <project>-release-world.
	EngineID string
	// BucketID is the local release World's bucket; default <project>-release.
	BucketID string
	// ManifestKey is the object collecting the manifests; default
	// <project>/release/manifests.
	ManifestKey string
}

// withDefaults fills the identifiers derived from ProjectID.
func (d Deployment) withDefaults() Deployment {
	// Derive each unset identifier from the project.
	p := d.ProjectID
	d.EngineID = cmp.Or(d.EngineID, p+"-release-world")
	d.BucketID = cmp.Or(d.BucketID, p+"-release")
	d.ManifestKey = cmp.Or(d.ManifestKey, p+"/release/manifests")
	return d
}

// HostConfigSet returns the controllers a launcher's host config runs to mount
// the release World from the CDN Space. The World follows the head of the
// Space's root pointer, which Release posts before it signs a selector.
//
// Launchers drop release-world controllers from a signed DistConfig, so this
// set belongs in the launcher's host config, not in the selector.
func (d Deployment) HostConfigSet() (map[string]*configset_proto.ControllerConfig, error) {
	// Require the project and Space.
	if d.ProjectID == "" || d.SpaceID == "" || d.CdnBaseURL == "" {
		return nil, errors.New("project ID, Space ID and CDN base URL are required")
	}
	d = d.withDefaults()

	// Mount the World from the Space; its block store answers for the Space ID.
	confs := map[string]configset.ControllerConfig{
		"release-world": configset.NewControllerConfig(1, &cdn_world_controller.Config{
			EngineId:           d.EngineID,
			SpaceId:            d.SpaceID,
			CdnBaseUrl:         d.CdnBaseURL,
			RootPointerBaseUrl: d.RootPointerBaseURL,
			CacheBlockStoreId:  d.CacheBlockStoreID,
		}),
		// Manifest refs name the local World's bucket, and refs the Space
		// rewrites name the Space; both read the Space's store.
		"release-world-cdn-bucket": configset.NewControllerConfig(1, &block_store_bucket.Config{
			BlockStoreId:  d.SpaceID,
			BucketStoreId: d.SpaceID,
			BucketConfig:  &bucket.Config{Id: d.BucketID, Rev: 1},
		}),
		"release-world-space-bucket": configset.NewControllerConfig(1, &block_store_bucket.Config{
			BlockStoreId:  d.SpaceID,
			BucketStoreId: d.SpaceID,
			BucketConfig:  &bucket.Config{Id: d.SpaceID, Rev: 1},
		}),
		"release-world-fetch": configset.NewControllerConfig(1, &manifest_fetch_world.Config{
			EngineId:                 d.EngineID,
			ObjectKeys:               []string{d.ManifestKey},
			ReleaseAuthorityPluginId: d.ReleaseAuthorityPluginID,
		}),
	}

	// Encode each controller config.
	out := make(map[string]*configset_proto.ControllerConfig, len(confs))
	for key, conf := range confs {
		enc, err := configset_proto.NewControllerConfig(conf, false)
		if err != nil {
			return nil, errors.Wrap(err, key)
		}
		out[key] = enc
	}
	return out, nil
}

// DistConfig returns the selector of the Deployment's channel at control
// revision rev, applying launcherConfigs to the launcher bus. Launchers adopt
// the highest revision they verify.
func (d Deployment) DistConfig(rev uint64, launcherConfigs map[string]*configset_proto.ControllerConfig) (*spacewave_launcher.DistConfig, error) {
	// Build and validate the selector.
	conf := &spacewave_launcher.DistConfig{
		ProjectId:         d.ProjectID,
		Rev:               rev,
		ChannelKey:        d.ChannelKey,
		LauncherConfigSet: launcherConfigs,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

// SignDistConfig signs conf with key as the packed message launchers fetch
// and verify against their pinned signers.
func SignDistConfig(key crypto.PrivKey, conf *spacewave_launcher.DistConfig) (string, error) {
	data, err := spacewave_launcher.EncodeSignedDistConfig(key, conf)
	if err != nil {
		return "", errors.Wrap(err, "sign DistConfig")
	}
	return packedmsg.EncodePackedMessage(data), nil
}
