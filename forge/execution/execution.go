package forge_execution

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

const (
	// ExecutionTypeID is the type identifier for an Execution.
	ExecutionTypeID = "forge/execution"
	// PredExecutionToWorker links an Execution to its placement's Worker.
	PredExecutionToWorker = quad.IRI("forge/execution-worker")
)

// NewExecutionToWorkerQuad creates a quad linking an Execution to its Worker.
func NewExecutionToWorkerQuad(executionObjKey, workerObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(executionObjKey, PredExecutionToWorker.String(), workerObjKey, "")
}

// NewExecutionBlock constructs a new Execution block.
func NewExecutionBlock() block.Block {
	return &Execution{}
}

// CreateExecutionWithTarget creates a pending Execution object in the world.
//
// Writes the Target to a block linked to by the Execution.
// execPeerID is the peer ID to assign to the Execution.
// Replacing the Execution replaces its placement and sole Worker edge; a nil
// placement removes the edge. The caller owns the World transaction.
func CreateExecutionWithTarget(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	execPeerID peer.ID,
	valueSet *forge_target.ValueSet,
	tgt *forge_target.Target,
	placement *forge_worker.Placement,
	ts *timestamp.Timestamp,
) (*bucket.ObjectRef, error) {
	// Require the Execution peer to belong to the selected Worker.
	if placement != nil {
		if err := placement.ValidateLinked(ctx, ws); err != nil {
			return nil, errors.Wrap(err, "placement")
		}
		if execPeerID.String() != placement.GetPeerId() {
			return nil, errors.Errorf("execution peer %s does not match placement peer %s", execPeerID, placement.GetPeerId())
		}
	}

	// Replace the Execution body and its Target block.
	rootRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		// Store the pending Execution and its Target without retaining previous blocks.
		bcs.ClearAllRefs()
		bcs.SetBlock(&Execution{
			ExecutionState: State_ExecutionState_PENDING,
			PeerId:         execPeerID.String(),
			Placement:      placement.CloneVT(),
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

	// Register the Execution's object type.
	err = world_types.SetObjectType(ctx, ws, objKey, ExecutionTypeID)
	if err != nil {
		return nil, err
	}

	// Link the Execution to its authenticated peer keypair.
	_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, objKey, execPeerID, "", nil)
	if err != nil {
		return nil, err
	}

	// Replace the Worker relationship with the current placement in this transaction.
	quads, err := ws.LookupGraphQuads(ctx, NewExecutionToWorkerQuad(objKey, ""), 0)
	if err != nil {
		return nil, err
	}
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return nil, err
		}
	}
	if placement != nil {
		if err := ws.SetGraphQuad(ctx, NewExecutionToWorkerQuad(objKey, placement.GetWorkerObjectKey())); err != nil {
			return nil, err
		}
	}

	return rootRef, nil
}

// BindExecutionWorker records the Worker taking custody of an unplaced Execution.
// It returns false when another Worker has custody or the Execution is complete.
// An empty placement peer selects the Execution peer for a Worker tracking all peers.
// The caller must hold a write transaction; the body and graph commit together.
func BindExecutionWorker(ctx context.Context, ws world.WorldState, objKey string, placement *forge_worker.Placement) (bool, error) {
	// Read current custody from the caller's transaction, not a watched snapshot.
	execution, err := world.LookupObjectBody[*Execution](ctx, ws, objKey, NewExecutionBlock)
	if err != nil {
		return false, err
	}

	// Resolve the Execution peer when the Worker tracks all its linked peers.
	placement = placement.CloneVT()
	if placement.GetPeerId() == "" {
		placement.PeerId = execution.GetPeerId()
	}
	if current := execution.GetPlacement(); current != nil {
		return current.EqualVT(placement), nil
	}
	if execution.IsComplete() {
		return false, nil
	}

	// Require the selected Worker to carry the Execution's assigned peer.
	if err := placement.ValidateLinked(ctx, ws); err != nil {
		return false, err
	}
	if execution.GetPeerId() != placement.GetPeerId() {
		return false, errors.New("execution peer_id does not match placement")
	}

	// Bind the Execution body and its sole Worker relationship atomically.
	_, _, err = world.AccessWorldObject(ctx, ws, objKey, true, func(cursor *block.Cursor) error {
		execution.Placement = placement.CloneVT()
		cursor.SetBlock(execution, true)
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := ws.SetGraphQuad(ctx, NewExecutionToWorkerQuad(objKey, placement.GetWorkerObjectKey())); err != nil {
		return false, err
	}
	return true, nil
}

// UnmarshalExecution unmarshals an execution block from the cursor.
func UnmarshalExecution(ctx context.Context, bcs *block.Cursor) (*Execution, error) {
	return block.UnmarshalBlock[*Execution](ctx, bcs, NewExecutionBlock)
}

// Validate performs cursory checks of the execution object.
func (e *Execution) Validate() error {
	// Require the Execution peer to match its placement.
	if p := e.GetPlacement(); p != nil {
		if err := p.Validate(); err != nil {
			return errors.Wrap(err, "placement")
		}
		if e.GetPeerId() != p.GetPeerId() {
			return errors.New("execution peer_id does not match placement")
		}
	}

	// Validate the Execution state, peer, timestamp, and claim.
	if err := e.GetExecutionState().Validate(false); err != nil {
		return err
	}
	if _, err := e.ParsePeerID(); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if claim := e.GetClaim(); claim != nil {
		if err := claim.Validate(); err != nil {
			return errors.Wrap(err, "claim")
		}
	}

	// Require a Target reference and a valid value set.
	if err := e.GetTargetRef().Validate(false); err != nil {
		return errors.Wrap(err, "target_ref")
	}
	if err := e.GetValueSet().Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}

	// Require a result exactly when the Execution is complete.
	if e.GetExecutionState() == State_ExecutionState_COMPLETE {
		if err := e.GetResult().Validate(); err != nil {
			return errors.Wrap(err, "result")
		}
		if e.GetResult().IsEmpty() {
			return errors.New("result: cannot be empty when execution is complete")
		}
		return nil
	}
	if !e.GetResult().IsEmpty() {
		return errors.New("result: cannot be set when execution is not complete")
	}
	return nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Execution) IsComplete() bool {
	return e.GetExecutionState() == State_ExecutionState_COMPLETE
}

// CheckPeerID checks if the peer ID matches the Execution.
func (e *Execution) CheckPeerID(id peer.ID) error {
	// Accept any peer when the Execution is unassigned.
	if len(e.GetPeerId()) == 0 {
		return nil
	}

	// Decode the Execution's assigned peer.
	currPeerID, err := e.ParsePeerID()
	if err != nil {
		return err
	}

	// Reject a peer that differs from the Execution's assignment.
	currPeerIDStr := currPeerID.String()
	idStr := id.String()
	if currPeerIDStr != idStr {
		return errors.Wrapf(
			forge_value.ErrUnexpectedPeerID,
			"expected %s got %s", currPeerIDStr, idStr,
		)
	}

	return nil
}

// ParsePeerID parses the peer ID field.
// Returns an empty peer ID if not set.
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
// Values may be nil.
func (e *Execution) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{3: e.GetValueSet(), 5: e.GetResult()}
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
// Values may be nil. Pending cursor references are excluded.
func (e *Execution) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{4: e.GetTargetRef()}, nil
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
	_ block.Block              = (*Execution)(nil)
	_ block.BlockWithSubBlocks = (*Execution)(nil)
	_ block.BlockWithRefs      = (*Execution)(nil)
)
