package forge_target

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// NewTargetBlock constructs a new block with type Target.
func NewTargetBlock() block.Block {
	return &Target{}
}

// UnmarshalTarget unmarshals an target block from the cursor.
func UnmarshalTarget(ctx context.Context, bcs *block.Cursor) (*Target, error) {
	return block.UnmarshalBlock[Target](ctx, bcs)
}

// CreateTarget writes the Target to a typed world object.
func CreateTarget(
	ctx context.Context,
	ws world.WorldState,
	objKey string, t *Target,
) (world.ObjectState, *bucket.ObjectRef, error) {
	state, ref, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(t, true)
		return nil
	})
	if err != nil {
		return state, ref, err
	}
	if err := world_types.SetObjectType(ctx, ws, objKey, TargetTypeID); err != nil {
		world.ReleaseObjectState(state)
		return nil, nil, err
	}
	return state, ref, nil
}

// Validate performs cursory validation of the target.
func (t *Target) Validate() error {
	// Require a Target before validating its fields.
	if t == nil {
		return errors.New("target cannot be empty")
	}

	// Require unique names for the Target inputs.
	inputSet := newSubBlockSetContainer(&t.Inputs, nil)
	if err := inputSet.ValidateUnique(true); err != nil {
		return errors.Wrap(err, "inputs")
	}

	// Require unique names for the Target outputs.
	outputSet := newSubBlockSetContainer(&t.Outputs, nil)
	if err := outputSet.ValidateUnique(true); err != nil {
		return errors.Wrap(err, "outputs")
	}

	// Validate the Target execution definition.
	if err := t.GetExec().Validate(); err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (t *Target) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (t *Target) GetSubBlocks() map[uint32]block.SubBlock {
	// Expose the Target input, output, and execution sub-blocks by field ID.
	m := make(map[uint32]block.SubBlock)
	m[1] = t.GetSubBlockCtor(1)(false)
	m[2] = t.GetSubBlockCtor(2)(false)
	m[3] = t.GetSubBlockCtor(3)(false)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *Target) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(bool) block.SubBlock {
			return newSubBlockSetContainer(&t.Inputs, nil)
		}
	case 2:
		return func(bool) block.SubBlock {
			return newSubBlockSetContainer(&t.Outputs, nil)
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := t.Exec
			if v == nil && create {
				v = &Exec{}
				t.Exec = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*Target)(nil)
	_ block.BlockWithSubBlocks = (*Target)(nil)
)
