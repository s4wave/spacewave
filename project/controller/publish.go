//go:build !js

package bldr_project_controller

import (
	"context"
	"slices"
	"sort"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// PublishTargets publishes to the given publish target(s)
// Filters manifests to the given build type.
// If the given build type is empty, skips filtering.
func (c *Controller) PublishTargets(ctx context.Context, remote string, targets []string, buildType bldr_manifest.BuildType) error {
	if len(remote) == 0 {
		return bldr_project.ErrEmptyRemoteID
	}
	if len(targets) == 0 {
		return errors.New("publish called with no targets")
	}

	conf := c.GetConfig()
	projConfig := conf.GetProjectConfig()
	publishTargets := projConfig.GetPublish()

	// add a reference to the source remote
	remoteWorld, remoteRef, err := c.WaitRemote(ctx, remote)
	if err != nil {
		return err
	}
	defer remoteRef.Release()

	remoteObjKey := remoteRef.GetRemoteConfig().GetObjectKey()
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		le := c.le.WithFields(logrus.Fields{
			"target":     target,
			"src-remote": remote,
		})
		publishTarget := publishTargets[target]

		// cleanup list of remotes
		destRemoteIDs := slices.Clone(publishTarget.GetRemotes())
		sort.Strings(destRemoteIDs)
		destRemoteIDs = slices.Compact(destRemoteIDs)
		if len(destRemoteIDs) != 0 && destRemoteIDs[0] == "" {
			destRemoteIDs = destRemoteIDs[1:]
		}
		if len(destRemoteIDs) == 0 {
			le.Warn("skipping target with no remotes")
			continue
		}

		// cleanup list of manifest ids
		manifestIDs := publishTarget.DedupeManifests()
		if len(manifestIDs) == 0 {
			le.Warn("skipping target with no manifest ids")
			continue
		}

		// cleanup list of platform ids
		platformIDs := publishTarget.DedupePlatformIDs()

		// cleanup list of source object keys
		srcObjectKeys := publishTarget.DedupeSrcObjectKeys()
		if len(srcObjectKeys) == 0 {
			// default to remoteObjKey
			srcObjectKeys = []string{remoteObjKey}
		}

		// cleanup/check list of storage overrides for manifests
		manifestStorage := make(map[string]*bldr_project.PublishStorageConfig, len(manifestIDs))
		for _, manifestID := range manifestIDs {
			baseConfig := publishTarget.GetStorage().CloneVT()
			if baseConfig == nil {
				baseConfig = &bldr_project.PublishStorageConfig{}
			}
			baseConfig.Merge(publishTarget.GetManifestStorage()[manifestID])
			manifestStorage[manifestID] = baseConfig
		}

		// search for all manifests for manifestIDs
		var cmanifests map[string][]*bldr_manifest_world.CollectedManifest
		var cmanifestErrs []error
		if err := func() error {
			wtx, err := remoteWorld.NewTransaction(ctx, false)
			if err != nil {
				return err
			}
			defer wtx.Discard()

			cmanifests, cmanifestErrs, err = bldr_manifest_world.CollectManifests(ctx, wtx, platformIDs, srcObjectKeys...)
			return err
		}(); err != nil {
			return err
		}
		for _, manifestErr := range cmanifestErrs {
			le.WithError(manifestErr).Warn("skipping invalid manifest")
		}

		// filter by build type
		if buildType != "" {
			for manifestID, collectedManifests := range cmanifests {
				cmanifests[manifestID] = bldr_manifest_world.FilterCollectedManifestsByBuildType(collectedManifests, buildType)
			}
		}

		// filter by platform ids if length > 1, if length == 1 we already filtered above
		if len(platformIDs) > 1 {
			bldr_manifest_world.FilterCollectedManifestsMapByPlatformID(cmanifests, platformIDs)
		}

		// filter to just the latest manifest (first in list) for each platform id
		if !publishTarget.GetAllManifestRevs() {
			for manifestID, collectedManifests := range cmanifests {
				cmanifests[manifestID] = bldr_manifest_world.FilterCollectedManifestsByFirst(collectedManifests)
			}
		}

		// warn for no manifests found
		var anyManifests bool
		for _, manifestID := range manifestIDs {
			manifests := cmanifests[manifestID]
			if len(manifests) == 0 {
				c.le.WithField("manifest-id", manifestID).Warn("no manifests found")
				delete(cmanifests, manifestID)
			} else {
				anyManifests = true
			}
		}
		// check if there is nothing to do
		if !anyManifests {
			le.Warn("no manifests matched: nothing to do")
			continue
		}

		// copy each manifest to each target remote
		for _, destRemoteID := range destRemoteIDs {
			le := le.WithField("dest-remote", destRemoteID)

			destRemoteEng, destRemoteRef, err := c.WaitRemote(ctx, destRemoteID)
			if err != nil {
				return errors.Wrap(err, "remote "+destRemoteID)
			}

			destRemoteConf := destRemoteRef.GetRemoteConfig()
			destRemotePeerID, err := destRemoteConf.ParsePeerID()
			if err != nil {
				destRemoteRef.Release()
				return errors.Wrap(err, "remote "+destRemoteID+" peer id")
			}

			// Get the destination base object key to create the store & link objects to.
			destStoreObjKey, destLinkObjKeys := destRemoteConf.CleanupLinkObjectKeys()
			if tgtObjKey := publishTarget.GetDestObjectKey(); tgtObjKey != "" {
				destStoreObjKey = tgtObjKey
			}
			if len(destStoreObjKey) == 0 {
				le.Warn("no destination object key specified")
				destRemoteRef.Release()
				return errors.Wrap(world.ErrEmptyObjectKey, "remote "+destRemoteID)
			}

			// Ensure the destination world store object exists.
			if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, destRemoteEng, destStoreObjKey); err != nil {
				return err
			}

			// Copy the manifests to the destination world.
			pErr := func() error {
				for _, manifestID := range manifestIDs {
					for _, manifest := range cmanifests[manifestID] {
						le := manifest.Manifest.Meta.Logger(le)
						destRemoteTx, err := destRemoteEng.NewTransaction(ctx, true)
						if err != nil {
							return err
						}
						defer destRemoteTx.Discard()

						destManifestObjKey := bldr_manifest.NewManifestKey(destStoreObjKey, manifest.Manifest.GetMeta())
						le.
							WithField("copy-manifest-dest-key", destManifestObjKey).
							Debug("copying manifest to destination remote")

						// set the transform config using the bucket cursor
						storageConf := manifestStorage[manifestID]
						accessDestManifest := func(
							ctx context.Context,
							baseRef *bucket.ObjectRef,
							cb func(*bucket_lookup.Cursor) error,
						) error {
							return destRemoteTx.AccessWorldState(
								ctx,
								baseRef,
								func(bls *bucket_lookup.Cursor) error {
									nextRef := bls.GetRef().Clone()
									nextRef.BucketId = bls.GetOpArgs().GetBucketId()
									nextRef.RootRef = nil

									// Adjust the world state cursor to use custom transform config.
									xfrmConf := storageConf.GetTransformConfFromRef().GetTransformConf()
									if xfrmOverride := storageConf.GetTransformConf(); !xfrmOverride.GetEmpty() {
										xfrmConf = xfrmOverride
									}
									if !xfrmConf.GetEmpty() {
										nextRef.TransformConf = xfrmConf.Clone()
										nextRef.TransformConfRef = nil
									}

									nextCs, err := bls.FollowRef(ctx, nextRef)
									if err != nil {
										return err
									}
									defer nextCs.Release()

									return cb(nextCs)
								},
							)
						}

						manifestTs := storageConf.GetTimestamp()
						if manifestTs.GetEmpty() {
							manifestTs = timestamp.Now()
						}

						_, destManifestObjRef, err := bldr_manifest_world.DeepCopyManifest(
							ctx,
							le,
							remoteWorld.AccessWorldState,
							manifest.ManifestRef,
							destRemoteTx,
							accessDestManifest,
							destManifestObjKey,
							destLinkObjKeys,
							destRemotePeerID,
							manifestTs.CloneVT(),
						)
						if err == nil {
							err = destRemoteTx.Commit(ctx)
						}
						if err != nil {
							return err
						}

						_ = destManifestObjRef
						le.Info("wrote manifest to destination")
					}
				}
				return nil
			}()
			destRemoteRef.Release()
			if pErr != nil {
				le.WithError(pErr).Warn("publish to remote failed")
				return errors.Wrap(pErr, "remote "+destRemoteID)
			}
		}
	}

	return nil
}
