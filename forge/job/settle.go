package forge_job

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// Settle ends a retained Job's lifetime and releases its private Git checkouts.
// COMPLETE between Tasks retains those checkouts. The caller must drain all
// Executions before settlement and owns the World transaction.
func Settle(ctx context.Context, ws world.WorldState, jobKey string) error {
	// Require every retained Task to be terminal before removing its workspace.
	tasks, _, err := CollectJobTasks(ctx, ws, jobKey)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if !task.IsComplete() {
			return errors.New("cannot settle a Job with active Tasks")
		}
	}

	// Remove private checkout objects while preserving shared commits and receipts.
	if err := forge_allocation.ReleaseJobAllocations(ctx, ws, jobKey); err != nil {
		return err
	}

	// Publish a canceled terminal result so the Cluster no longer schedules this Job.
	_, _, err = world.AccessWorldObject(ctx, ws, jobKey, true, func(bcs *block.Cursor) error {
		// Freeze the terminal Job result after releasing its checkout.
		job, err := UnmarshalJob(ctx, bcs)
		if err != nil {
			return err
		}
		job.JobState = State_JobState_COMPLETE
		job.Result = &forge_value.Result{Canceled: true, FailError: "Job settled"}
		if err := job.Validate(); err != nil {
			return err
		}
		bcs.SetBlock(job, true)
		return nil
	})
	return err
}
