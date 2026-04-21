package bldr_manifest

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

// NewManifestBundle constructs a new ManifestBundle.
func NewManifestBundle(refs []*ManifestRef, ts *timestamp.Timestamp) *ManifestBundle {
	return &ManifestBundle{
		ManifestRefs: refs,
		Timestamp:    ts,
	}
}

// NewManifestBundleBlock constructs a new ManifestBundle block.
func NewManifestBundleBlock() block.Block {
	return &ManifestBundle{}
}

// UnmarshalManifestBundle unmarshals a ManifestBundle block from the cursor.
func UnmarshalManifestBundle(ctx context.Context, bcs *block.Cursor) (*ManifestBundle, error) {
	return block.UnmarshalBlock[*ManifestBundle](ctx, bcs, NewManifestBundleBlock)
}

// NewManifestBundleEntryKey builds a ObjectKey for an entry in a bundle.
func NewManifestBundleEntryKey(bundleObjKey string, meta *ManifestMeta) (string, error) {
	b := meta.MarshalB58()
	return bundleObjKey + "/" + b, nil
}

// NewManifestBundleSubBlockCtor returns the sub-block constructor.
func NewManifestBundleSubBlockCtor(r **ManifestBundle) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if v != nil || !create {
			return v
		}
		v = &ManifestBundle{}
		*r = v
		return v
	}
}

// IsNil checks if the object is nil.
func (m *ManifestBundle) IsNil() bool {
	return m == nil
}

// Validate validates the ManifestBundle.
func (m *ManifestBundle) Validate() error {
	for i, manifestRef := range m.GetManifestRefs() {
		if err := manifestRef.Validate(); err != nil {
			return errors.Wrapf(err, "manifest_refs[%d]", i)
		}
	}
	if err := m.GetTimestamp().Validate(true); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (m *ManifestBundle) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *ManifestBundle) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// FollowManifestRefs follows the ManifestRefs sub-block.
func (m *ManifestBundle) FollowManifestRefs(bcs *block.Cursor) (*sbset.SubBlockSet, *block.Cursor) {
	sbcs := bcs.FollowSubBlock(1)
	return NewManifestRefSet(&m.ManifestRefs, sbcs), sbcs
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *ManifestBundle) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *ManifestBundle) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = NewManifestRefSet(&m.ManifestRefs, nil)
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *ManifestBundle) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return NewManifestRefSetSubBlockCtor(&m.ManifestRefs)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ManifestBundle)(nil))
	_ block.BlockWithSubBlocks = ((*ManifestBundle)(nil))
)
