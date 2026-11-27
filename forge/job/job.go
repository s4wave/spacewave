package forge_job

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

const (
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

// UnmarshalJob unmarshals a Job block from the cursor.
func UnmarshalJob(ctx context.Context, bcs *block.Cursor) (*Job, error) {
	return block.UnmarshalBlock[Job](ctx, bcs)
}

// IsComplete checks if the job is in the COMPLETE state.
func (e *Job) IsComplete() bool {
	return e.GetJobState() == State_JobState_COMPLETE
}

// Validate performs cursory checks of the Job object.
func (e *Job) Validate() error {
	// Validate the Job placement when one is present.
	if placement := e.GetPlacement(); placement != nil {
		if err := placement.Validate(); err != nil {
			return errors.Wrap(err, "placement")
		}
	}

	// Validate the Job state and timestamp.
	if err := e.GetJobState().validate(false); err != nil {
		return err
	}
	if err := e.GetTimestamp().Validate(false); err != nil {
		return err
	}

	// Require a valid nonempty result for a completed Job.
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

// CompletionResult aggregates completed tasks in their supplied order. It returns
// nil while any task is incomplete. taskKeys must correspond to tasks, as returned
// by CollectJobTasks. The first failed task determines the failure summary.
func CompletionResult(tasks []*forge_task.Task, taskKeys []string) *forge_value.Result {
	result := forge_value.NewResultWithSuccess()
	for i, task := range tasks {
		if !task.IsComplete() {
			return nil
		}
		if result.GetSuccess() && !task.GetResult().GetSuccess() {
			result = forge_value.NewResultWithError(errors.Errorf("task %s failed: %s", taskKeys[i], task.GetResult().GetFailError()))
		}
	}
	return result
}

// _ is a type assertion
var (
	_ block.Block              = (*Job)(nil)
	_ block.BlockWithSubBlocks = (*Job)(nil)
)
