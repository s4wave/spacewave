// Package forge_lib_git_allocation_mount resolves an allocation's Workdir
// through its Git Worktree. The allocation records themselves carry no Git
// code, so Forge's Job lifecycle settles them without linking Git.
package forge_lib_git_allocation_mount

import (
	"context"

	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
	"github.com/s4wave/spacewave/net/peer"
)

// Resolve returns the allocation's authorized identity and live Workdir.
// Job allocations survive individual Execution completion until
// forge_allocation.ReleaseJobAllocations.
func Resolve(ctx context.Context, ws world.WorldState, key string) (peer.ID, *unixfs_world.UnixfsRef, error) {
	// Require an active allocation before resolving any mutable filesystem.
	alloc, err := forge_allocation.Lookup(ctx, ws, key)
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
