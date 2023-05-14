package forge_value

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
)

// NewWorldObjectSnapshot converts a ObjectState into a WorldObjectSnapshot.
//
// ws: can be nil, if unset, type and parents will be empty.
func NewWorldObjectSnapshot(ctx context.Context, obj world.ObjectState, ws world.WorldState) (*WorldObjectSnapshot, error) {
	objRef, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}

	snap := &WorldObjectSnapshot{
		Key:     obj.GetKey(),
		RootRef: objRef,
		Rev:     rev,
	}

	if ws != nil {
		objType, err := world_types.GetObjectType(ctx, ws, snap.Key)
		if err != nil {
			return nil, err
		}
		snap.ObjectType = objType

		objParentState := world_parent.NewParentState(ws)
		objParent, err := objParentState.GetObjectParent(ctx, snap.Key)
		if err != nil {
			return nil, err
		}
		snap.ObjectParent = objParent
	}

	return snap, nil
}

// IsNil checks if the object is nil.
func (s *WorldObjectSnapshot) IsNil() bool {
	return s == nil
}

// GetEmpty checks if the WorldObjectSnapshot is empty.
func (s *WorldObjectSnapshot) GetEmpty() bool {
	return s.GetKey() == "" || s.GetRev() == 0
}

// ToBucketRef converts the snapshot into a ObjectRef.
// Returns nil if the block ref or bucket ref was empty.
func (s *WorldObjectSnapshot) ToBucketRef() (*bucket.ObjectRef, error) {
	return s.GetRootRef(), nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *WorldObjectSnapshot) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *WorldObjectSnapshot) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (s *WorldObjectSnapshot) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		if next == nil {
			s.RootRef = nil
			return nil
		}
		sb, ok := next.(*bucket.ObjectRef)
		if !ok {
			return block.ErrUnexpectedType
		}
		s.RootRef = sb
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (s *WorldObjectSnapshot) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[2] = s.GetRootRef()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (s *WorldObjectSnapshot) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			n := s.GetRootRef()
			if n == nil && create {
				n = &bucket.ObjectRef{}
				s.RootRef = n
			}
			return n
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*WorldObjectSnapshot)(nil))
	_ block.BlockWithSubBlocks = ((*WorldObjectSnapshot)(nil))
)
