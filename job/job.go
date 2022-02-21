package forge_job

import (
	"context"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
	"github.com/cayleygraph/quad"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	// JobTypeID is the type identifier for a Job.
	JobTypeID = "forge/job"

	// PredJobToTask is the predicate linking Job to a Task.
	PredJobToTask = quad.IRI("forge/job-task")
)

// NewJobBlock constructs a new Job block.
func NewJobBlock() block.Block {
	return &Job{}
}

// NewJobToTaskQuad creates a quad linking a Job to a Task.
func NewJobToTaskQuad(jobObjKey, taskObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		jobObjKey,
		PredJobToTask.String(),
		taskObjKey,
		"",
	)
}

// NewTaskKey creates a key for a task on a job with a name.
func NewTaskKey(jobKey, taskName string) string {
	return strings.Join([]string{
		jobKey,
		"task",
		taskName,
	}, "/")
}

// CreateJobWithTasks creates a pending Job object in the world.
//
// TasksPeer sets the peer ID to set on the tasks. Can be empty.
func CreateJobWithTasks(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	tasks map[string]*forge_target.Target,
	tasksPeer peer.ID,
	ts *timestamp.Timestamp,
) (world.ObjectState, *bucket.ObjectRef, error) {
	njob := &Job{
		JobState:  State_JobState_PENDING,
		Timestamp: ts,
	}
	if err := njob.Validate(); err != nil {
		return nil, nil, err
	}
	objState, rootRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(njob, true)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// create the <type> ref
	typesState := world_types.NewTypesState(ctx, ws)
	err = typesState.SetObjectType(objKey, JobTypeID)
	if err != nil {
		return objState, rootRef, err
	}

	// create the tasks & targets & links
	parentState := world_parent.NewParentState(ws)
	for taskName, taskTgt := range tasks {
		if err := forge_task.ValidateName(taskName); err != nil {
			return nil, nil, errors.Wrapf(err, "tasks[%s]", taskName)
		}
		taskKey := NewTaskKey(objKey, taskName)
		replicas := uint32(1)
		_, _, err = forge_task.CreateTaskWithTarget(ctx, ws, sender, taskKey, taskName, taskTgt, tasksPeer, replicas, ts)
		if err != nil {
			return objState, rootRef, errors.Wrapf(err, "tasks[%s]", taskName)
		}

		// create parent link
		err = parentState.SetObjectParent(ctx, taskKey, objKey, false)
		if err != nil {
			return objState, rootRef, err
		}

		// create job -> task link
		err = ws.SetGraphQuad(NewJobToTaskQuad(objKey, taskKey))
		if err != nil {
			return objState, rootRef, err
		}
	}

	return objState, rootRef, nil
}

// UnmarshalJob unmarshals a pass block from the cursor.
func UnmarshalJob(bcs *block.Cursor) (*Job, error) {
	vi, err := bcs.Unmarshal(NewJobBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*Job)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Job) IsComplete() bool {
	return e.GetJobState() == State_JobState_COMPLETE
}

// Validate performs cursory checks of the Job object.
func (e *Job) Validate() error {
	if err := e.GetJobState().Validate(false); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if e.GetJobState() == State_JobState_COMPLETE {
		if err := e.GetResult().Validate(); err != nil {
			return errors.Wrap(err, "result")
		}
		if e.GetResult().IsEmpty() {
			return errors.New("result: cannot be empty when pass is complete")
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Job) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Job) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *Job) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*forge_value.Result)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.Result = v
	case 6:
		// no-op
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *Job) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[2] = e.GetResult()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *Job) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return forge_value.NewResultSubBlockCtor(&e.Result)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Job)(nil))
	_ block.BlockWithSubBlocks = ((*Job)(nil))
)
