package forge_lib_git_clone

import (
	"context"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/sideband"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CloneOrFetch clones the configured repository into ws, or fetches its cloned
// branch again when the repository object already exists. The fetch reuses the
// clone's remote, depth and tag mode, so a depth-one clone later receives only
// the branch's newest commit. A fetch moves the remote-tracking refs and leaves
// an existing worktree on its checked-out commit; a configured worktree that
// does not exist yet is created. Returns the repository's root reference.
// authMethod and progress can be nil.
func (c *Config) CloneOrFetch(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
	ts *timestamppb.Timestamp,
	authMethod client.SSHAuth,
	progress sideband.Progress,
) (*bucket.ObjectRef, error) {
	// Bind the worktree options to the repository object and timestamp.
	repoObjKey := c.GetObjectKey()
	cloneOpts := c.GetCloneOpts()
	worktreeOpts := c.GetWorktreeOpts().CloneVT()
	if worktreeOpts != nil {
		worktreeOpts.RepoObjectKey = repoObjKey
		worktreeOpts.Timestamp = ts
	}

	// Clone the repository when the World does not hold it yet.
	repoObj, exists, err := ws.GetObject(ctx, repoObjKey)
	world.ReleaseObjectState(repoObj)
	if err != nil {
		return nil, err
	}
	if !exists {
		le.Debugf("git: clone %q to object %q worktree %q", cloneOpts.GetUrl(), repoObjKey, worktreeOpts.GetObjectKey())
		return git_world.GitClone(ctx, ws, repoObjKey, sender, cloneOpts, authMethod, progress, worktreeOpts, ts)
	}

	// Fetch the cloned branch into the existing repository.
	le.Debugf("git: fetch %q into existing object %q", cloneOpts.GetUrl(), repoObjKey)
	repoRef, err := git_world.GitFetch(ctx, ws, repoObjKey, cloneOpts.BuildFetchOpts(), authMethod, progress)
	if err != nil {
		return nil, err
	}

	// Create the configured worktree when the repository has none yet.
	worktreeKey := worktreeOpts.GetObjectKey()
	if worktreeKey == "" || cloneOpts.GetDisableCheckout() {
		return repoRef, nil
	}
	worktreeObj, worktreeExists, err := ws.GetObject(ctx, worktreeKey)
	world.ReleaseObjectState(worktreeObj)
	if err != nil || worktreeExists {
		return repoRef, err
	}
	if _, err := worktreeOpts.ApplyWorldOp(ctx, le, ws, sender); err != nil {
		return nil, err
	}
	return repoRef, nil
}
