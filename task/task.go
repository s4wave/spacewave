package forge_task

import (
	"context"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/aperturerobotics/cayley/quad"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

const (
	// TaskTypeID is the type identifier for a Task.
	TaskTypeID = "forge/task"

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
	return block.UnmarshalBlock[*Task](ctx, bcs, NewTaskBlock)
}

// NewTaskToTargetQuad creates a quad linking a Task to a Target.
func NewTaskToTargetQuad(taskObjKey, targetObjKey string) world.GraphQuad {
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

// NewTaskToSubtaskQuad creates a quad linking a parent Task to a child Task.
func NewTaskToSubtaskQuad(parentTaskKey, childTaskKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		parentTaskKey,
		PredTaskToSubtask.String(),
		childTaskKey,
		"",
	)
}

// NewTaskToCachedQuad creates a quad linking a Task to a previous Task whose
// result is inherited/cached.
func NewTaskToCachedQuad(taskKey, cachedTaskKey string) world.GraphQuad {
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
	ts *timestamp.Timestamp,
) (world.ObjectState, *bucket.ObjectRef, error) {
	if err := tgt.Validate(); err != nil {
		return nil, nil, err
	}

	ntask := &Task{
		TaskState: State_TaskState_PENDING,
		Name:      name,
		Replicas:  replicas,
		PeerId:    peerID.String(),
		Timestamp: ts,
	}
	if err := ntask.Validate(); err != nil {
		return nil, nil, err
	}

	objState, rootRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(ntask, true)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, ws, objKey, TaskTypeID)
	if err != nil {
		return nil, nil, err
	}

	// create the target
	tgtObjKey := NewTargetKey(objKey)
	_, _, err = forge_target.CreateTarget(ctx, ws, tgtObjKey, tgt)
	if err != nil {
		return nil, nil, err
	}

	// link target -> parent -> task
	err = world_parent.SetObjectParent(ctx, ws, tgtObjKey, objKey, false)
	if err != nil {
		return nil, nil, err
	}

	// link to the target
	err = ws.SetGraphQuad(ctx, NewTaskToTargetQuad(objKey, tgtObjKey))
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

// Validate performs cursory checks of the Task object.
func (e *Task) Validate() error {
	if err := e.GetTaskState().Validate(false); err != nil {
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
// bcs should point to the task
func (e *Task) SetTarget(bcs *block.Cursor, tgt *forge_target.Target) {
	tgtCs := bcs.FollowRef(7, nil)
	tgtCs.ClearAllRefs()
	tgtCs.SetBlock(tgt, true)
	e.TargetRef = nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Task) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Task) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
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
// Note: this does not include pending references (in a cursor)
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
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *Task) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 8:
		return forge_target.NewValueSetSubBlockCtor(&e.ValueSet)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Task)(nil))
	_ block.BlockWithRefs      = ((*Task)(nil))
	_ block.BlockWithSubBlocks = ((*Task)(nil))
)
