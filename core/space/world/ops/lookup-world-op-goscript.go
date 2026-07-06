//go:build goscript

package space_world_ops

import (
	"context"

	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
)

// LookupWorldOp looks up the GoScript-supported built-in space world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		lookupLocalGitOp,
		lookupCoreWorldOp,
	}).LookupOp(ctx, opTypeID)
}

func lookupLocalGitOp(_ context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case git_world.GitInitOpId:
		return &git_world.GitInitOp{}, nil
	case git_world.GitCreateWorktreeOpId:
		return &git_world.GitCreateWorktreeOp{}, nil
	}
	return nil, nil
}
