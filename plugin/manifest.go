package plugin

import (
	"context"
	"io/fs"

	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

// NewPluginManifest constructs a new PluginManifest.
func NewPluginManifest(pluginID, entrypoint string, buildType BuildType) *PluginManifest {
	return &PluginManifest{
		PluginId:   pluginID,
		Entrypoint: entrypoint,
		BuildType:  string(buildType),
	}
}

// NewPluginManifestBlock constructs a new PluginManifest block.
func NewPluginManifestBlock() block.Block {
	return &PluginManifest{}
}

// UnmarshalPluginManifest unmarshals a PluginManifest block from the cursor.
func UnmarshalPluginManifest(bcs *block.Cursor) (*PluginManifest, error) {
	vi, err := bcs.Unmarshal(NewPluginManifestBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*PluginManifest)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// CreatePluginManifest creates the plugin manifest at the block cursor.
func CreatePluginManifest(
	ctx context.Context,
	bcs *block.Cursor,
	pluginID, entrypoint string,
	distFs, assetsFs fs.FS,
	buildType BuildType,
	ts *timestamp.Timestamp,
) (*PluginManifest, error) {
	pluginManifest := NewPluginManifest(pluginID, entrypoint, buildType)
	bcs.SetBlock(pluginManifest, true)

	// setup the distribution filesystem.
	if err := unixfs_block.CreateFromFS(ctx, bcs.FollowRef(2, nil), distFs, ts); err != nil {
		return nil, err
	}
	// setup the assets filesystem.
	if err := unixfs_block.CreateFromFS(ctx, bcs.FollowRef(4, nil), assetsFs, ts); err != nil {
		return nil, err
	}

	// done
	return pluginManifest, nil
}

// Validate validates the PluginManifest.
func (m *PluginManifest) Validate() error {
	if err := ValidatePluginID(m.GetPluginId()); err != nil {
		return ErrEmptyPluginID
	}
	if err := m.GetDistFsRef().Validate(); err != nil {
		return errors.Wrap(err, "dist_fs_ref")
	}
	if err := m.GetAssetsFsRef().Validate(); err != nil {
		return errors.Wrap(err, "assets_fs_ref")
	}
	if m.GetEntrypoint() == "" {
		return ErrEmptyEntrypoint
	}
	if err := ToBuildType(m.GetBuildType()).Validate(false); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (m *PluginManifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *PluginManifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (m *PluginManifest) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		m.DistFsRef = ptr
	case 4:
		m.AssetsFsRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (m *PluginManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	n := make(map[uint32]*block.BlockRef)
	n[2] = m.GetDistFsRef()
	n[4] = m.GetAssetsFsRef()
	return n, nil
}

// FollowDistFs follows the DistFsRef.
func (m *PluginManifest) FollowDistFs(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(2, m.GetDistFsRef())
}

// FollowAssetsFs follows the AssetsFsRef.
func (m *PluginManifest) FollowAssetsFsRef(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(4, m.GetAssetsFsRef())
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (m *PluginManifest) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return unixfs_block.NewFSNodeBlock
	case 4:
		return unixfs_block.NewFSNodeBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*PluginManifest)(nil))
	_ block.BlockWithRefs = ((*PluginManifest)(nil))
)
