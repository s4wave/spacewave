package bldr_plugin

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// NewPluginManifestRef constructs a new PluginManifestRef.
func NewPluginManifestRef(meta *PluginManifestMeta, ref *bucket.ObjectRef) *PluginManifestRef {
	return &PluginManifestRef{
		Meta:        meta,
		ManifestRef: ref,
	}
}

// NewPluginManifestRefBlock constructs a new PluginManifestRef block.
func NewPluginManifestRefBlock() block.Block {
	return &PluginManifestRef{}
}

// UnmarshalPluginManifestRef unmarshals a PluginManifestRef block from the cursor.
func UnmarshalPluginManifestRef(bcs *block.Cursor) (*PluginManifestRef, error) {
	return block.UnmarshalBlock[*PluginManifestRef](bcs, NewPluginManifestRefBlock)
}

// CreatePluginManifestRef creates the plugin manifest ref at the block cursor.
func CreatePluginManifestRef(
	bcs *block.Cursor,
	meta *PluginManifestMeta,
	ref *bucket.ObjectRef,
) (*PluginManifestRef, error) {
	pluginManifestRef := NewPluginManifestRef(meta.CloneVT(), ref.Clone())
	bcs.SetBlock(pluginManifestRef, true)
	return pluginManifestRef, nil
}

// Validate validates the PluginManifestRef.
func (m *PluginManifestRef) Validate() error {
	if err := m.GetMeta().Validate(false); err != nil {
		return errors.Wrap(err, "meta")
	}
	if err := m.GetManifestRef().Validate(); err != nil {
		return errors.Wrap(err, "manifest_ref")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (m *PluginManifestRef) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *PluginManifestRef) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// FollowManifestRef follows the ManifestRef sub-block.
func (m *PluginManifestRef) FollowManifestRef(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowSubBlock(2)
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *PluginManifestRef) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*bucket.ObjectRef)
		if ok {
			m.ManifestRef = v
		} else {
			return block.ErrUnexpectedType
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *PluginManifestRef) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[2] = m.GetManifestRef()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *PluginManifestRef) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return bucket.NewObjectRefSubBlockCtor(&m.ManifestRef)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*PluginManifestRef)(nil))
	_ block.BlockWithSubBlocks = ((*PluginManifestRef)(nil))
)
