package forge_execution

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

const (
	// ExecutionTypeID is the type identifier for a Execution.
	ExecutionTypeID = "forge/execution"
)

// NewExecutionBlock constructs a new Execution block.
func NewExecutionBlock() block.Block {
	return &Execution{}
}

// CreateExecutionWithTarget creates a pending Execution object in the world.
//
// Writes the Target to a block linked to by the Execution.
// peerID is the peer id to assign to the execution.
func CreateExecutionWithTarget(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	execPeerID peer.ID,
	valueSet *forge_target.ValueSet,
	tgt *forge_target.Target,
	ts *timestamp.Timestamp,
) (*bucket.ObjectRef, error) {
	rootRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(&Execution{
			ExecutionState: State_ExecutionState_PENDING,
			PeerId:         execPeerID.String(),
			ValueSet:       valueSet,
			Timestamp:      ts,
		}, true)
		tgtBcs := bcs.FollowRef(4, nil)
		tgtBcs.SetBlock(tgt, true)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, ws, objKey, ExecutionTypeID)
	if err != nil {
		return nil, err
	}

	// create the keypair and link to it if necessary
	_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, objKey, execPeerID, "", nil)
	if err != nil {
		return nil, err
	}

	return rootRef, nil
}

// UnmarshalExecution unmarshals an execution block from the cursor.
func UnmarshalExecution(ctx context.Context, bcs *block.Cursor) (*Execution, error) {
	return block.UnmarshalBlock[*Execution](ctx, bcs, NewExecutionBlock)
}

// Validate performs cursory checks of the execution object.
func (e *Execution) Validate() error {
	if err := e.GetExecutionState().Validate(false); err != nil {
		return err
	}
	if _, err := e.ParsePeerID(); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}
	// disallow empty reference to target
	if err := e.GetTargetRef().Validate(false); err != nil {
		return errors.Wrap(err, "target_ref")
	}
	if err := e.GetValueSet().Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}

	if e.GetExecutionState() == State_ExecutionState_COMPLETE {
		if err := e.GetResult().Validate(); err != nil {
			return errors.Wrap(err, "result")
		}
		if e.GetResult().IsEmpty() {
			return errors.New("result: cannot be empty when execution is complete")
		}
	} else {
		if !e.GetResult().IsEmpty() {
			return errors.New("result: cannot be set when execution is not complete")
		}
	}
	return nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Execution) IsComplete() bool {
	return e.GetExecutionState() == State_ExecutionState_COMPLETE
}

// CheckPeerID checks if the peer ID matches the Execution.
func (e *Execution) CheckPeerID(id peer.ID) error {
	// accept any peer id if field is unset
	if len(e.GetPeerId()) == 0 {
		return nil
	}

	currPeerID, err := e.ParsePeerID()
	if err != nil {
		return err
	}

	// basic string comparison
	currPeerIDStr := currPeerID.String()
	idStr := id.String()
	if currPeerIDStr != idStr {
		return errors.Wrapf(
			forge_value.ErrUnexpectedPeerID,
			"expected %s got %s", currPeerIDStr, idStr,
		)
	}

	// match
	return nil
}

// ParsePeerID parses the peer ID field.
// Returns empty if not set.
func (e *Execution) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(e.GetPeerId())
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Execution) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Execution) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *Execution) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	switch id {
	case 3:
		v, ok := next.(*forge_target.ValueSet)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.ValueSet = v
	case 5:
		v, ok := next.(*forge_value.Result)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.Result = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *Execution) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[3] = e.GetValueSet()
	m[5] = e.GetResult()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *Execution) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 3:
		return forge_target.NewValueSetSubBlockCtor(&e.ValueSet)
	case 5:
		return forge_value.NewResultSubBlockCtor(&e.Result)
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (e *Execution) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 4:
		e.TargetRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (e *Execution) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[4] = e.GetTargetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (e *Execution) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 4:
		return forge_target.NewTargetBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Execution)(nil))
	_ block.BlockWithSubBlocks = ((*Execution)(nil))
	_ block.BlockWithRefs      = ((*Execution)(nil))
)
