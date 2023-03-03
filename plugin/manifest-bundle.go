package bldr_plugin

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

// NewPluginManifestBundle constructs a new PluginManifestBundle.
func NewPluginManifestBundle(refs []*PluginManifestRef, ts *timestamp.Timestamp) *PluginManifestBundle {
	return &PluginManifestBundle{
		PluginManifestRefs: refs,
		Timestamp:          ts,
	}
}

// NewPluginManifestBundleBlock constructs a new PluginManifestBundle block.
func NewPluginManifestBundleBlock() block.Block {
	return &PluginManifestBundle{}
}

// UnmarshalPluginManifestBundle unmarshals a PluginManifestBundle block from the cursor.
func UnmarshalPluginManifestBundle(bcs *block.Cursor) (*PluginManifestBundle, error) {
	return block.UnmarshalBlock[*PluginManifestBundle](bcs, NewPluginManifestBundleBlock)
}

// NewPluginManifestBundleEntryKey builds a ObjectKey for an entry in a bundle.
func NewPluginManifestBundleEntryKey(bundleObjKey string, meta *PluginManifestMeta) (string, error) {
	b := meta.MarshalB58()
	return bundleObjKey + "/" + b, nil
}

// Validate validates the PluginManifestBundle.
func (m *PluginManifestBundle) Validate() error {
	for i, manifestRef := range m.GetPluginManifestRefs() {
		if err := manifestRef.Validate(); err != nil {
			return errors.Wrapf(err, "plugin_manifest_refs[%d]", i)
		}
	}
	if err := m.GetTimestamp().Validate(true); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (m *PluginManifestBundle) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *PluginManifestBundle) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// FollowPluginManifestRefs follows the PluginManifestRefs sub-block.
func (m *PluginManifestBundle) FollowPluginManifestRefs(bcs *block.Cursor) (*sbset.SubBlockSet, *block.Cursor) {
	sbcs := bcs.FollowSubBlock(1)
	return NewPluginManifestRefSet(&m.PluginManifestRefs, sbcs), sbcs
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *PluginManifestBundle) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *PluginManifestBundle) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = NewPluginManifestRefSet(&m.PluginManifestRefs, nil)
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *PluginManifestBundle) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return NewPluginManifestSetSubBlockCtor(&m.PluginManifestRefs)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*PluginManifestBundle)(nil))
	_ block.BlockWithSubBlocks = ((*PluginManifestBundle)(nil))
)
