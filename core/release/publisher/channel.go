package publisher

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// ChannelDirectoryKey is the release channel directory consumed by launchers.
const ChannelDirectoryKey = "release/metadata"

// StageChannel records a release over the current Bldr publication manifests.
// Other channels remain intact. The caller owns the transaction and commits
// only after all release checks succeed.
func StageChannel(ctx context.Context, ws world.WorldState, manifestKey string, metadata *release.ReleaseMetadata) (*release.ReleaseMetadata, error) {
	// Select one current release manifest for each executable or plugin platform.
	if metadata == nil || manifestKey == "" {
		return nil, errors.New("release metadata and manifest key are required")
	}
	if key := metadata.GetChannelKey(); key == "." || key == ".." || strings.ContainsAny(key, "/\\") {
		return nil, errors.New("release channel must be one object-key segment")
	}
	groups, manifestErrors, err := manifest_world.CollectManifests(ctx, ws, nil, manifestKey)
	if err != nil {
		return nil, err
	}
	if len(manifestErrors) != 0 {
		return nil, manifestErrors[0]
	}
	var selected []*manifest_world.CollectedManifest
	for _, manifests := range groups {
		selected = append(selected, manifests...)
	}
	selected = manifest_world.FilterCollectedManifestsByBuildType(selected, bldr_manifest.BuildType_RELEASE)
	selected = manifest_world.FilterCollectedManifestsByLatestRev(selected)
	metadata = metadata.CloneVT()
	metadata.ManifestRefs = nil
	for _, manifest := range selected {
		metadata.ManifestRefs = append(metadata.ManifestRefs, &bldr_manifest.ManifestRef{
			Meta: manifest.Manifest.GetMeta().CloneVT(), ManifestRef: manifest.ManifestRef.CloneVT(),
		})
	}
	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	// Load the existing directory so publishing one channel preserves the rest.
	directory := &release.ChannelDirectory{}
	object, exists, err := ws.GetObject(ctx, ChannelDirectoryKey)
	world.ReleaseObjectState(object)
	if err != nil {
		return nil, err
	}
	if exists {
		_, _, err := world.AccessWorldObject(ctx, ws, ChannelDirectoryKey, false, func(cursor *block.Cursor) error {
			value, err := block.UnmarshalBlock[*release.ChannelDirectory](ctx, cursor, func() block.Block { return &release.ChannelDirectory{} })
			directory = value
			return err
		})
		if err != nil {
			return nil, err
		}
		if err := directory.Validate(); err != nil {
			return nil, err
		}
	}

	// Store metadata before the directory begins referring to it.
	ref, _, err := world.AccessWorldObject(ctx, ws, path.Join(ChannelDirectoryKey, metadata.GetChannelKey()), true, func(cursor *block.Cursor) error {
		cursor.ClearAllRefs()
		cursor.SetBlock(metadata, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	directory.Channels = slices.DeleteFunc(directory.Channels, func(entry *release.ChannelEntry) bool {
		return entry.GetChannelKey() == metadata.GetChannelKey()
	})
	directory.Channels = append(directory.Channels, &release.ChannelEntry{
		ChannelKey: metadata.GetChannelKey(), ReleaseMetadataRef: ref.GetRootRef(),
	})
	_, _, err = world.AccessWorldObject(ctx, ws, ChannelDirectoryKey, true, func(cursor *block.Cursor) error {
		cursor.ClearAllRefs()
		cursor.SetBlock(directory, true)
		return nil
	})
	return metadata, err
}
