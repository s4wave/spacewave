// Package forge_task defines the Task, the state machine that runs one Target
// against its current inputs.
//
// A Task links to its Target with PredTaskToTarget and to each Pass it creates
// with PredTaskToPass. PredTaskToSubtask and PredTaskToCached link child Tasks
// and a previous Task whose result it inherits. Its states:
//
//   - PENDING: waiting for the Target and inputs to resolve. Once they do, the
//     Task creates a Pass, keyed by its pass_nonce, and moves to RUNNING.
//   - RUNNING: a Pass is executing. A Target or input change cancels the Pass
//     and the Task returns to PENDING.
//   - CHECKING: the Pass completed and the Task checks its result.
//   - COMPLETE: done for the current Target and inputs, in success or failure.
//     A Target or watched input change returns the Task to PENDING. An
//     explicitly canceled Task stays stopped until retried.
//   - RETRY: waits for an input change before trying a new Pass.
//
// RetireTask ends worker demand for a one-shot Task and its attempts while
// keeping their bodies, results and history, marking each with
// PredObjectRetired so the intent survives a restart. The controller
// subpackage runs the state machine. tx holds its World operations.
package forge_task

import (
	"context"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/labels"
)

const (
	// PredTaskToTarget is the predicate linking Task to a Target.
	PredTaskToTarget = quad.IRI("forge/task-target")
	// PredTaskToPass is the predicate linking Task to a Pass.
	PredTaskToPass = quad.IRI("forge/task-pass")
	// PredTaskToSubtask is a graph predicate linking a parent Task to child Tasks.
	PredTaskToSubtask = quad.IRI("forge/task-subtask")
	// PredTaskToCached is a graph predicate linking a Task to a previous Task
	// whose result is inherited/cached.
	PredTaskToCached = quad.IRI("forge/task-cached")
)

// NewTaskBlock constructs a new Task block.
func NewTaskBlock() block.Block {
	return &Task{}
}

// UnmarshalTask unmarshals a task block from the cursor.
func UnmarshalTask(ctx context.Context, bcs *block.Cursor) (*Task, error) {
	return block.UnmarshalBlock[Task](ctx, bcs)
}

// newTaskToTargetQuad creates a quad linking a Task to a Target.
func newTaskToTargetQuad(taskObjKey, targetObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		taskObjKey,
		PredTaskToTarget.String(),
		targetObjKey,
		"",
	)
}

// NewTaskToPassQuad creates a quad linking a Task to a Pass.
func NewTaskToPassQuad(taskObjKey, passObjKey string, passNonce uint64) world.GraphQuad {
	var nonceVal string
	if passNonce != 0 {
		nonceVal = quad.IRI(strconv.FormatUint(passNonce, 10)).String()
	}
	return world.NewGraphQuadWithKeys(
		taskObjKey,
		PredTaskToPass.String(),
		passObjKey,
		nonceVal,
	)
}

// newTaskToCachedQuad creates a quad linking a Task to a previous Task whose
// result is inherited/cached.
func newTaskToCachedQuad(taskKey, cachedTaskKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		taskKey,
		PredTaskToCached.String(),
		cachedTaskKey,
		"",
	)
}

// NewTargetKey builds a object key for a task target.
func NewTargetKey(taskObjKey string) string {
	return strings.Join([]string{taskObjKey, "target"}, "/")
}

// NewPassKey builds a object key for a task pass.
func NewPassKey(taskObjKey string, passNonce uint64) string {
	return strings.Join([]string{
		taskObjKey,
		"pass",
		strconv.FormatUint(passNonce, 10),
	}, "/")
}

// CreateTaskWithTarget creates a pending Task and Target object in the world.
func CreateTaskWithTarget(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	name string,
	tgt *forge_target.Target,
	peerID peer.ID,
	replicas uint32,
	placement *forge_worker.Placement,
	ts *timestamp.Timestamp,
) (world.ObjectState, *bucket.ObjectRef, error) {
	// Validate the linked placement and Target before creating the Task.
	if placement != nil {
		if err := placement.ValidateLinked(ctx, ws); err != nil {
			return nil, nil, errors.Wrap(err, "placement")
		}
	}
	if err := tgt.Validate(); err != nil {
		return nil, nil, err
	}

	// Construct and validate the pending Task record.
	ntask := &Task{
		TaskState: State_TaskState_PENDING,
		Name:      name,
		Replicas:  replicas,
		PeerId:    peerID.String(),
		Placement: placement.CloneVT(),
		Timestamp: ts,
	}
	if err := ntask.Validate(); err != nil {
		return nil, nil, err
	}

	// Create the Task object and initialize its block contents.
	objState, rootRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(ntask, true)
		return nil
	})
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, nil, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, ws, objKey, TaskTypeID)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, nil, err
	}

	// create the target
	tgtObjKey := NewTargetKey(objKey)
	var createdObject world.ObjectState
	createdObject, _, err = forge_target.CreateTarget(ctx, ws, tgtObjKey, tgt)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, nil, err
	}

	// link target -> parent -> task
	err = world_parent.SetObjectParent(ctx, ws, tgtObjKey, objKey, false)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, nil, err
	}

	// link to the target
	err = ws.SetGraphQuad(ctx, newTaskToTargetQuad(objKey, tgtObjKey))
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, nil, err
	}

	// create the keypair and link to it if necessary
	if len(peerID) != 0 {
		_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, objKey, peerID, "", nil)
		if err != nil {
			world.ReleaseObjectState(objState)
			return nil, nil, err
		}
	}

	return objState, rootRef, err
}

// ValidateName validates the name of a task.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	if err := labels.ValidateDNSLabel(name); err != nil {
		return errors.Wrap(err, "name")
	}
	return nil
}

// Validate checks the Task object cursorily.
func (e *Task) Validate() error {
	// Validate Task placement, state, timestamp, name, and replica requirements.
	if p := e.GetPlacement(); p != nil {
		if err := p.Validate(); err != nil {
			return errors.Wrap(err, "placement")
		}
	}
	if err := e.GetTaskState().validate(false); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := ValidateName(e.GetName()); err != nil {
		return err
	}
	if e.GetReplicas() == 0 {
		return errors.New("replicas cannot be zero")
	}
	if e.GetPlacement() != nil && e.GetReplicas() != 1 {
		return errors.New("placed task must have one replica")
	}

	// Validate the Task target reference and value set for its current state.
	if e.GetTargetRef().GetEmpty() {
		if ts := e.GetTaskState(); ts != State_TaskState_PENDING {
			return errors.Errorf("target_ref: cannot be empty in state: %s", ts.String())
		}
	} else {
		if err := e.GetTargetRef().Validate(false); err != nil {
			return errors.Wrap(err, "target_ref")
		}
	}
	if err := e.GetValueSet().Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}

	// Require a nonempty result exactly when the Task is complete.
	if e.GetTaskState() == State_TaskState_COMPLETE {
		if err := e.GetResult().Validate(); err != nil {
			return errors.Wrap(err, "result")
		}
		if e.GetResult().IsEmpty() {
			return errors.New("result: cannot be empty when task is complete")
		}
	} else {
		if !e.GetResult().IsEmpty() {
			return errors.New("result: cannot be set when task is not complete")
		}
	}

	return nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Task) IsComplete() bool {
	return e.GetTaskState() == State_TaskState_COMPLETE
}

// FollowTargetRef follows the reference to the Task target.
// bcs should point to the task.
func (e *Task) FollowTargetRef(ctx context.Context, bcs *block.Cursor) (*forge_target.Target, *block.Cursor, error) {
	tgtCs := bcs.FollowRef(7, e.GetTargetRef())
	tgt, err := forge_target.UnmarshalTarget(ctx, tgtCs)
	if err != nil {
		return nil, nil, err
	}
	return tgt, tgtCs, nil
}

// SetTarget updates the target with a new block.
// bcs should point to the task.
func (e *Task) SetTarget(bcs *block.Cursor, tgt *forge_target.Target) {
	tgtCs := bcs.FollowRef(7, nil)
	tgtCs.ClearAllRefs()
	tgtCs.SetBlock(tgt, true)
	e.TargetRef = nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (e *Task) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 7:
		e.TargetRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: the returned map does not include pending references (in a cursor).
func (e *Task) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[7] = e.GetTargetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (e *Task) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 7:
		return forge_target.NewTargetBlock
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *Task) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 8:
		v, ok := next.(*forge_target.ValueSet)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.ValueSet = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *Task) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[8] = e.GetValueSet()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at the reference id. Can return nil to indicate invalid reference
// id.
func (e *Task) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 8:
		return forge_target.NewValueSetSubBlockCtor(&e.ValueSet)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*Task)(nil)
	_ block.BlockWithRefs      = (*Task)(nil)
	_ block.BlockWithSubBlocks = (*Task)(nil)
)
