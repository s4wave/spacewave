package forge_pass

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
)

const (
	// PassTypeID is the type identifier for a Pass.
	PassTypeID = "forge/pass"

	// PredPassToExecution is the predicate linking Pass to a Execution.
	PredPassToExecution = quad.IRI("forge/pass-execution")
)

// NewPassBlock constructs a new Pass block.
func NewPassBlock() block.Block {
	return &Pass{}
}

// NewPassToExecutionQuad creates a quad linking a Pass to a Execution.
func NewPassToExecutionQuad(passObjKey, executionObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		passObjKey,
		PredPassToExecution.String(),
		executionObjKey,
		"",
	)
}

// CreatePassWithTarget creates a pending Pass object in the world.
//
// Writes the Target to a block linked to by the Pass.
func CreatePassWithTarget(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	valueSet *forge_target.ValueSet,
	tgt *forge_target.Target,
	nonce uint64,
	replicas uint32,
	passPeerID string,
	ts *timestamp.Timestamp,
) (world.ObjectState, *bucket.ObjectRef, error) {
	ps := &Pass{
		PassState: State_PassState_PENDING,
		PeerId:    passPeerID,
		ValueSet:  valueSet,
		PassNonce: nonce,
		Replicas:  replicas,
		Timestamp: ts,
	}
	if err := ps.Validate(true); err != nil {
		return nil, nil, err
	}
	peerID, err := ps.ParsePeerID()
	if err != nil {
		return nil, nil, err
	}
	objState, rootRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(ps, true)
		tgtBcs := bcs.FollowRef(3, nil)
		tgtBcs.SetBlock(tgt, true)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// create the <type> ref
	typesState := world_types.NewTypesState(ctx, ws)
	err = typesState.SetObjectType(objKey, PassTypeID)
	if err != nil {
		return nil, nil, err
	}

	// create the keypair and link to it if necessary
	if len(peerID) != 0 {
		_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, objKey, peerID, "", nil)
		if err != nil {
			return nil, nil, err
		}
	}

	return objState, rootRef, nil
}

// UnmarshalPass unmarshals a pass block from the cursor.
func UnmarshalPass(bcs *block.Cursor) (*Pass, error) {
	return block.UnmarshalBlock[*Pass](bcs, NewPassBlock)
}

// Validate performs cursory checks of the Pass object.
func (e *Pass) Validate(allowEmptyRefs bool) error {
	if err := e.GetPassState().Validate(false); err != nil {
		return err
	}
	if _, err := e.ParsePeerID(); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := e.GetValueSet().Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}
	if e.GetTargetRef().GetEmpty() {
		if !allowEmptyRefs {
			return errors.New("target_ref: cannot be empty")
		}
	} else {
		if err := e.GetTargetRef().Validate(); err != nil {
			return errors.Wrap(err, "target_ref")
		}
	}
	if e.GetPassNonce() == 0 {
		return errors.New("pass_nonce cannot be zero")
	}
	if e.GetReplicas() == 0 {
		return errors.New("replicas cannot be zero")
	}
	if e.GetPassState() == State_PassState_COMPLETE {
		if err := e.GetResult().Validate(); err != nil {
			return errors.Wrap(err, "result")
		}
		if e.GetResult().GetSuccess() {
			replicas := int(e.GetReplicas())
			nexecStates := len(e.GetExecStates())
			if nexecStates != replicas {
				return errors.Errorf(
					"replicas(%d) must match len(exec_states) (%d)",
					replicas, nexecStates,
				)
			}
		} else if e.GetResult().IsEmpty() {
			return errors.New("result: cannot be empty when pass is complete")
		}
	} else {
		if !e.GetResult().IsEmpty() {
			return errors.New("result: cannot be set when pass is not complete")
		}
		if e.GetPassState() == State_PassState_PENDING {
			if len(e.GetExecStates()) != 0 {
				return errors.New("exec_states must be empty when pending")
			}
		}
	}

	if e.GetPassState() == State_PassState_CHECKING {
		execStates := e.GetExecStates()
		if len(execStates) != int(e.GetReplicas()) {
			return errors.New("exec_states len must match replicas in checking state")
		}
		for i, execState := range execStates {
			if !execState.GetResult().IsSuccessful() {
				return errors.Errorf("exec_states[%d]: must be successful in checking state", i)
			}
		}
	}

	return nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Pass) IsComplete() bool {
	return e.GetPassState() == State_PassState_COMPLETE
}

// FollowTargetRef follows the reference to the pass target.
// bcs should point to the pass.
func (e *Pass) FollowTargetRef(bcs *block.Cursor) (*forge_target.Target, *block.Cursor, error) {
	tgtCs := bcs.FollowRef(3, e.GetTargetRef())
	tgt, err := forge_target.UnmarshalTarget(tgtCs)
	if err != nil {
		return nil, nil, err
	}
	return tgt, tgtCs, nil
}

// ParsePeerID parses the peer ID field.
// Returns empty if not set.
func (e *Pass) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(e.GetPeerId())
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Pass) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Pass) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *Pass) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 4:
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
	case 8:
		// no-op
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *Pass) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = e.GetValueSet()
	m[5] = e.GetResult()
	m[8] = NewExecStateSubBlockSet(&e.ExecStates, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *Pass) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return forge_target.NewValueSetSubBlockCtor(&e.ValueSet)
	case 5:
		return forge_value.NewResultSubBlockCtor(&e.Result)
	case 8:
		return NewExecStateSubBlockSetCtor(&e.ExecStates)
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (e *Pass) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 3:
		e.TargetRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (e *Pass) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[3] = e.GetTargetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (e *Pass) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 3:
		return forge_target.NewTargetBlock
	}
	return nil
}

// ComputeOutputsWithStates computes the pass outputs with exec states.
func ComputeOutputsWithStates(outputs []*forge_target.Output, execStates []*ExecState, replicas int) (forge_value.ValueSlice, error) {
	// promote the first successful exec state value set to the pass
	if len(execStates) == 0 {
		return nil, errors.New("exec_states cannot be empty")
	}
	if len(execStates) < replicas && replicas != 0 {
		return nil, errors.Errorf("expected %d replicas but got %d", replicas, len(execStates))
	}

	execOutputValues := make([]forge_value.ValueSlice, len(execStates))
	for i, execState := range execStates {
		if err := execState.GetExecutionState().EnsureMatches(forge_execution.State_ExecutionState_COMPLETE); err != nil {
			return nil, errors.Wrapf(err, "exec_states[%d]", i)
		}
		execOutputValues[i] = execState.GetValueSet().GetOutputs()
	}

	// Compute the execution outputs.
	return forge_execution.ComputeExecutionOutputs(outputs, execOutputValues, false)
}

// ApplyExecStates updates the exec states field with the list of Executions.
// bcs can be nil
func (e *Pass) ApplyExecStates(
	bcs *block.Cursor,
	execObjKeys []string,
	execObjs []*forge_execution.Execution,
) error {
	if len(execObjKeys) != len(execObjs) {
		return errors.New("apply exec states: exec objects slice len must match keys slice")
	}

	states := make([]*ExecState, len(execObjs))
	for i, obj := range execObjs {
		objKey := execObjKeys[i]
		states[i] = NewExecState(objKey, obj)
		if err := states[i].Validate(); err != nil {
			return errors.Wrapf(err, "executions[%d]", i)
		}
	}

	if bcs != nil {
		sbcs := bcs.FollowSubBlock(8)
		sbcs.ClearAllRefs()
	}

	e.ExecStates = states
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Pass)(nil))
	_ block.BlockWithSubBlocks = ((*Pass)(nil))
	_ block.BlockWithRefs      = ((*Pass)(nil))
)
