package forge_job

import (
	"context"
	"strings"

	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
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

// NewJobTaskKey creates a key for a task on a job with a name.
func NewJobTaskKey(jobKey, taskName string) string {
	return strings.Join([]string{
		jobKey,
		"task",
		taskName,
	}, "/")
}

// UnmarshalJob unmarshals a pass block from the cursor.
func UnmarshalJob(ctx context.Context, bcs *block.Cursor) (*Job, error) {
	return block.UnmarshalBlock[*Job](ctx, bcs, NewJobBlock)
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
			return errors.New("result: cannot be empty when job is complete")
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (e *Job) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (e *Job) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
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
