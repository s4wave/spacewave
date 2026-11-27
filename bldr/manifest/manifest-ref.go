package bldr_manifest

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// NewManifestRef constructs a new ManifestRef.
func NewManifestRef(meta *ManifestMeta, ref *bucket.ObjectRef) *ManifestRef {
	return &ManifestRef{
		Meta:        meta,
		ManifestRef: ref,
	}
}

// UnmarshalManifestRef unmarshals a ManifestRef block from the cursor.
func UnmarshalManifestRef(ctx context.Context, bcs *block.Cursor) (*ManifestRef, error) {
	return block.UnmarshalBlock[ManifestRef](ctx, bcs)
}

// IsNil checks if the object is nil.
func (m *ManifestRef) IsNil() bool {
	return m == nil
}

// GetEmpty returns if the manifest and/or ref is empty.
func (m *ManifestRef) GetEmpty() bool {
	return m.GetMeta().GetManifestId() == "" || m.GetManifestRef().GetEmpty()
}

// Validate validates the ManifestRef.
func (m *ManifestRef) Validate() error {
	if err := m.GetMeta().Validate(false); err != nil {
		return errors.Wrap(err, "meta")
	}
	if err := m.GetManifestRef().Validate(); err != nil {
		return errors.Wrap(err, "manifest_ref")
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *ManifestRef) ApplySubBlock(id uint32, next block.SubBlock) error {
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
func (m *ManifestRef) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[2] = m.GetManifestRef()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *ManifestRef) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return bucket.NewObjectRefSubBlockCtor(&m.ManifestRef)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*ManifestRef)(nil)
	_ block.BlockWithSubBlocks = (*ManifestRef)(nil)
)
