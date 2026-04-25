package s4wave_git_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	var snap resource_git.WorktreeSnapshot

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

	_, _, err = git_world.AccessWorldObjectWorktree(
		ctx, ws, objectKey, false, nil,
		func(bcs *block.Cursor, wt *git_world.Worktree) error {
			hrs, err := wt.FollowHeadRefStore(bcs)
			if err != nil {
				return err
			}
			headRef, err := hrs.GetReference(plumbing.HEAD)
			if err != nil {
				return err
			}
			if headRef != nil {
				snap.CheckedOutRef = headRef.Name().Short()
				snap.HeadCommitHash = headRef.Hash().String()
			}
			return nil
		},
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "access worktree")
	}

	wdRef, err := git_world.WorktreeLookupWorkdirRef(ctx, ws, objectKey)
	if err == nil && wdRef != nil {
		snap.HasWorkdir = true
		snap.WorkdirObjectKey = wdRef.GetObjectKey()
		snap.WorkdirRef = wdRef
	}

	resource := resource_git.NewGitWorktreeResource(ws, engine, objectKey, &snap)
	return resource.GetMux(), func() {}, nil
}
