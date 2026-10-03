package forge_lib_git_allocation

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	"github.com/s4wave/spacewave/net/peer"
)

// ListJobAllocations returns the checkout allocations retained by the Jobs.
func ListJobAllocations(ctx context.Context, ws world.WorldState, jobKeys ...string) ([]string, error) {
	return world.CollectGraphPathStepWithKeys(ctx, ws, jobKeys, world.GraphPathDirectionOut, PredJobToAllocation.String(), allocationGraphPathLimit)
}

// BindJobAllocations grants an Execution use of its Job's retained checkouts.
// The caller must establish the Execution's Job membership in the same transaction.
func BindJobAllocations(ctx context.Context, ws world.WorldState, jobKey, executionKey string) error {
	// Resolve only the Job's indexed allocations.
	keys, err := ListJobAllocations(ctx, ws, jobKey)
	if err != nil {
		return err
	}

	// Retain the Job owner while publishing each Execution's checkout capability.
	for _, key := range keys {
		alloc, err := Lookup(ctx, ws, key)
		if err != nil {
			return err
		}
		if alloc.GetCleanupState() != "active" {
			return errors.New("Job workspace is settled")
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(executionKey, PredExecutionToAllocation.String(), key, "")); err != nil {
			return err
		}
	}
	return nil
}

// ResolveMount returns the allocation's authorized identity and live Workdir.
// Job allocations survive individual Execution completion until ReleaseJobAllocations.
func ResolveMount(ctx context.Context, ws world.WorldState, key string) (peer.ID, *unixfs_world.UnixfsRef, error) {
	// Require an active allocation before resolving any mutable filesystem.
	alloc, err := Lookup(ctx, ws, key)
	if err != nil {
		return "", nil, err
	}
	if alloc.GetCleanupState() != "active" {
		return "", nil, errors.New("workspace allocation is settled")
	}

	// Resolve the grant from the Job allocation or the owning Execution.
	sender, err := peer.IDB58Decode(alloc.PeerID)
	if alloc.GetJobObjectKey() == "" {
		execution, state, lookupErr := forge_execution.LookupExecution(ctx, ws, alloc.GetExecutionObjectKey())
		world.ReleaseObjectState(state)
		if lookupErr != nil {
			return "", nil, lookupErr
		}
		sender, err = execution.ParsePeerID()
	}
	if err != nil {
		return "", nil, err
	}

	// Use the Worktree's live reference and require the allocated Workdir identity.
	ref, err := git_world.WorktreeLookupWorkdirRef(ctx, ws, alloc.GetWorktreeObjectKey())
	if err != nil {
		return "", nil, err
	}
	if ref.GetObjectKey() != alloc.GetWorkdirObjectKey() {
		return "", nil, errors.New("allocation Workdir differs from its Worktree")
	}
	return sender, ref, nil
}

// ReleaseJobAllocations removes a settled Job's private Worktrees and Workdirs.
// Shared repository objects and commits remain. The caller owns the transaction
// and must drain the Job's Executions before releasing their filesystems.
func ReleaseJobAllocations(ctx context.Context, ws world.WorldState, jobKey string) error {
	// Resolve the retained checkouts through the Job's allocation index.
	keys, err := ListJobAllocations(ctx, ws, jobKey)
	if err != nil {
		return err
	}

	// Remove each private checkout and retain an idempotent settlement record.
	for _, key := range keys {
		alloc, err := Lookup(ctx, ws, key)
		if err != nil {
			return err
		}
		if alloc.GetCleanupState() == "released" {
			continue
		}

		// Drain every bound Execution before deleting its mutable checkout.
		executions, err := world.CollectGraphPathStepWithKeys(ctx, ws, []string{key}, world.GraphPathDirectionIn, PredExecutionToAllocation.String(), allocationGraphPathLimit)
		if err != nil {
			return err
		}
		for _, executionKey := range executions {
			execution, state, err := forge_execution.LookupExecution(ctx, ws, executionKey)
			world.ReleaseObjectState(state)
			if err != nil {
				return err
			}
			if !execution.IsComplete() {
				return errors.New("cannot settle a Job with active workspace Executions")
			}
		}

		// Remove the private checkout while preserving the shared repository.
		for _, objectKey := range []string{alloc.GetWorktreeObjectKey(), alloc.GetWorkdirObjectKey()} {
			if _, err := ws.DeleteObject(ctx, objectKey); err != nil {
				return err
			}
		}
		if err := markJobAllocationReleased(ctx, ws, key); err != nil {
			return err
		}
	}
	return nil
}

// markJobAllocationReleased records release after both checkout objects are removed.
func markJobAllocationReleased(ctx context.Context, ws world.WorldState, key string) error {
	_, _, err := world.AccessWorldObject(ctx, ws, key, true, func(bcs *block.Cursor) error {
		// Record the final checkout release on the retained allocation.
		alloc, err := UnmarshalAllocation(ctx, bcs)
		if err != nil {
			return err
		}
		alloc.CleanupState = "released"
		bcs.SetBlock(alloc, true)
		return nil
	})
	return err
}
