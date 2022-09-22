package plugin

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

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

// Validate validates the PluginManifest.
func (m *PluginManifest) Validate() error {
	if err := ValidatePluginID(m.GetPluginId()); err != nil {
		return ErrEmptyPluginID
	}
	if err := m.GetFsRef().Validate(); err != nil {
		return errors.Wrap(err, "fs_ref")
	}
	if m.GetFsRef().GetEmpty() {
		return errors.New("fs_ref: plugin filesystem cannot be empty")
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

// ApplySubBlock applies a sub-block change with a field id.
func (m *PluginManifest) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *PluginManifest) GetSubBlocks() map[uint32]block.SubBlock {
	n := make(map[uint32]block.SubBlock)
	n[2] = m.GetFsRef
	return n
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *PluginManifest) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return bucket.NewObjectRefSubBlockCtor(&m.FsRef)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*PluginManifest)(nil))
	_ block.BlockWithSubBlocks = ((*PluginManifest)(nil))
)
