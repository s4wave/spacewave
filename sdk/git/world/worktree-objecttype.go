package s4wave_git_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// GitWorktreeTypeID is the object type ID for git/worktree objects.
const GitWorktreeTypeID = git_world.GitWorktreeTypeID

// GitWorktreeType is the ObjectType for git/worktree objects.
var GitWorktreeType = objecttype.NewObjectType(GitWorktreeTypeID, GitWorktreeFactory)

// GitWorktreeFactory creates a GitWorktreeResource from a world object.
func GitWorktreeFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Require the World state before reading the worktree.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Collect the repository and checkout state for the resource.
	var snap resource_git.WorktreeSnapshot

	// Resolve the repository linked from this worktree.
	gqs, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(objectKey, git_world.GitRepoPred, "", ""),
		1,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup repo predicate")
	}
	if len(gqs) == 0 {
		return nil, nil, errors.New("no linked repo found for worktree")
	}
	repoObjKey, err := world.GraphValueToKey(gqs[0].GetObj())
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse repo object key")
	}
	snap.RepoObjectKey = repoObjKey

	// Read the checked-out branch from the worktree's HEAD ref store.
	headRef, err := git_world.LookupWorktreeHead(ctx, ws, objectKey, repoObjKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "access worktree")
	}
	if headRef != nil {
		snap.CheckedOutRef = headRef.Name().Short()
		snap.HeadCommitHash = headRef.Hash().String()
	}

	// Record whether the worktree has a linked working directory.
	wdRef, err := git_world.WorktreeLookupWorkdirRef(ctx, ws, objectKey)
	if err == nil && wdRef != nil {
		snap.HasWorkdir = true
		snap.WorkdirObjectKey = wdRef.GetObjectKey()
		snap.WorkdirRef = wdRef
	}

	// Serve the captured checkout state through its resource.
	resource := resource_git.NewGitWorktreeResource(ws, engine, objectKey, &snap)
	return resource.GetMux(), func() {}, nil
}
