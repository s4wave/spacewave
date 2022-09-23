package plugin

import (
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/pkg/errors"
)

// NewPluginManifest constructs a new PluginManifest.
func NewPluginManifest(pluginID, entrypoint string) *PluginManifest {
	return &PluginManifest{PluginId: pluginID, Entrypoint: entrypoint}
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

// Validate validates the PluginManifest.
func (m *PluginManifest) Validate() error {
	if err := ValidatePluginID(m.GetPluginId()); err != nil {
		return ErrEmptyPluginID
	}
	if err := m.GetFsRef().Validate(); err != nil {
		return errors.Wrap(err, "fs_ref")
	}
	if m.GetEntrypoint() == "" {
		return ErrEmptyEntrypoint
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
		m.FsRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (m *PluginManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	n := make(map[uint32]*block.BlockRef)
	n[2] = m.GetFsRef()
	return n, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (m *PluginManifest) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return unixfs_block.NewFSNodeBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*PluginManifest)(nil))
	_ block.BlockWithRefs = ((*PluginManifest)(nil))
)
