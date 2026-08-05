package git_world

import (
	"context"
	"os"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const (
	// GitRepoTypeID is the type identifier for a git repo.
	GitRepoTypeID = "git/repo"
	// GitWorktreeTypeID is the type identifier for a git worktree.
	GitWorktreeTypeID = "git/worktree"
	// GitRepoWorktreePred is the predicate pointing to the worktree.
	GitRepoWorktreePred = "git/worktree"
	// GitWorktreeWorkdirPred is the predicate pointing to the workdir.
	GitWorktreeWorkdirPred = "git/workdir"
	// GitRepoPred is the predicate pointing to a repo.
	GitRepoPred = "git/repo"
)

// LookupGitOp performs the lookup operation for the git op types.
func LookupGitOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case GitInitOpId:
		return &GitInitOp{}, nil
	case GitCloneOpId:
		return &GitCloneOp{}, nil
	case GitCreateWorktreeOpId:
		return &GitCreateWorktreeOp{}, nil
	case GitFetchOpId:
		return &GitFetchOp{}, nil
	case GitWorktreeCheckoutOpId:
		return &GitWorktreeCheckoutOp{}, nil
	case GitStageOpId:
		return &GitStageOp{}, nil
	case GitUnstageOpId:
		return &GitUnstageOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupGitOp

// AccessRepo is a utility for AccessWorldState to access a repository.
// Ref can be nil to indicate creating a new repo.
// The block transaction is written upon completion and updated ObjectRef returned.
//
// configStore, indexStore, and worktree can be empty to use a default in-mem store.
// Returns the updated object ref and any error.
func AccessRepo(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	indexStore storer.IndexStorer,
	workdir billy.Filesystem,
	refStore git_block.ReferenceStore,
	cb func(repo *git.Repository) error,
) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, access, ref, func(bcs *block.Cursor) error {
		return AccessRepoWithCursor(ctx, bcs, indexStore, workdir, refStore, cb)
	})
}

// AccessRepoWithCursor accesses a repo reading / writing to a block cursor.
//
// setHeadCb is an optional callback to override updating HEAD.
func AccessRepoWithCursor(
	ctx context.Context,
	bcs *block.Cursor,
	indexStore storer.IndexStorer,
	workdir billy.Filesystem,
	refStore git_block.ReferenceStore,
	cb func(repo *git.Repository) error,
) error {
	// Resolve default stores and validate the persisted repository block.
	if indexStore == nil {
		indexStore = &memory.IndexStorage{}
	}
	if workdir == nil {
		workdir = memfs.New()
	}
	repob, err := git_block.UnmarshalRepo(ctx, bcs)
	if err != nil {
		return err
	}
	if err := repob.Validate(); err != nil {
		return err
	}

	// Open the block-backed git store and release it after the callback.
	store, err := git_block.NewStore(ctx, nil, bcs, indexStore, refStore)
	if err != nil {
		return err
	}
	defer store.Close()

	// Open or initialize the git repository in the requested worktree.
	repo, err := git.Open(store, workdir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repo, err = git.Init(store, git.WithWorkTree(workdir))
	}
	if err != nil {
		return err
	}

	// Run the repository callback before committing its block changes.
	if cb != nil {
		if err := cb(repo); err != nil {
			return err
		}
	}

	// Commit the repository mutation.
	return store.Commit()
}

// ValidateOrCreateRepo creates or checks a reference to a Repo.
// repoRef can be nil to create a new repo.
func ValidateOrCreateRepo(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	repoRef *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	// Create a repository block for an empty reference or validate the existing repository.
	var err error
	if repoRef.GetEmpty() {
		repoRef, err = world.AccessObject(ctx, accessState, nil, func(bcs *block.Cursor) error {
			bcs.SetBlock(git_block.NewRepo(), true)
			return nil
		})
	} else {
		// Validate the supplied reference before opening its repository block.
		if err := repoRef.Validate(); err != nil {
			return nil, err
		}
		_, err = world.AccessObject(ctx, accessState, repoRef, func(bcs *block.Cursor) error {
			// Confirm valid repo object.
			repo, err := git_block.UnmarshalRepo(ctx, bcs)
			if err == nil {
				err = repo.Validate()
			}
			return err
		})
	}

	// Return any repository creation or validation failure.
	if err != nil {
		return nil, err
	}
	return repoRef, nil
}

// AccessWorldObjectRepo attempts to look up a Repo in the world state.
// If the object did not exist, bcs will be empty, the object will be created.
// If updateWorld=true, and the result is different, will SetRootRef with change.
// Note: if updateWorld=true but ws is read-only, sets updateWorld=false.
// Returns the modified object ref, if it was stored, and any error.
// refStore can be nil
func AccessWorldObjectRepo(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	updateWorld bool,
	indexStore storer.IndexStorer,
	workdir billy.Filesystem,
	refStore git_block.ReferenceStore,
	cb func(repo *git.Repository) error,
) (*bucket.ObjectRef, bool, error) {
	return world.AccessWorldObject(ctx, ws, objKey, updateWorld, func(bcs *block.Cursor) error {
		return AccessRepoWithCursor(ctx, bcs, indexStore, workdir, refStore, cb)
	})
}

// AccessWorktreeWithCursor accesses a Worktree reading / writing to a block cursor.
func AccessWorktreeWithCursor(
	ctx context.Context,
	bcs *block.Cursor,
	cb func(bcs *block.Cursor, wt *Worktree) error,
) error {
	wt, err := UnmarshalWorktree(ctx, bcs)
	if err != nil {
		return err
	}
	if err := wt.Validate(); err != nil {
		return err
	}
	if cb != nil {
		if err := cb(bcs, wt); err != nil {
			return err
		}
	}
	return nil
}

// AccessWorktree is a utility for AccessWorldState to access a worktree.
// Ref can be nil to indicate creating a new worktree.
// The block transaction is written upon completion and updated ObjectRef returned.
func AccessWorktree(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	cb func(bcs *block.Cursor, wt *Worktree) error,
) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, access, ref, func(bcs *block.Cursor) error {
		return AccessWorktreeWithCursor(ctx, bcs, cb)
	})
}

// AccessWorldObjectWorktree attempts to look up a Worktree in the world state.
// If the object did not exist, bcs will be empty, the object will be created.
// If updateWorld=true, and the result is different, will SetRootRef with change.
// Note: if updateWorld=true but ws is read-only, sets updateWorld=false.
// Returns the modified object ref, if it was stored, and any error.
func AccessWorldObjectWorktree(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	updateWorld bool,
	workdir billy.Filesystem,
	cb func(bcs *block.Cursor, wt *Worktree) error,
) (*bucket.ObjectRef, bool, error) {
	return world.AccessWorldObject(ctx, ws, objKey, updateWorld, func(bcs *block.Cursor) error {
		return AccessWorktreeWithCursor(ctx, bcs, cb)
	})
}

// AccessWorldObjectRepoWithWorktree accesses a repository with a worktree.
func AccessWorldObjectRepoWithWorktree(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	repoObjKey, worktreeObjKey string,
	ts time.Time,
	updateWorld bool,
	sender peer.ID,
	cb func(repo *git.Repository, workDir billy.Filesystem) error,
) error {
	workdirRef, err := WorktreeLookupWorkdirRef(ctx, ws, worktreeObjKey)
	if err != nil {
		return err
	}

	// Open the referenced UnixFS workdir with write access when requested.
	writeWorkdir := updateWorld && sender != ""
	wdFsHandle, err := unixfs_world.BuildFSFromUnixfsRef(ctx, le, ws, sender, workdirRef, true, writeWorkdir, ts)
	if err != nil {
		return err
	}
	defer wdFsHandle.Release()

	// Wrap the UnixFS handle as the billy worktree filesystem.
	wdBfs := unixfs_billy.NewBillyFilesystem(ctx, wdFsHandle, "", ts)

	// Access the worktree object and persist its updated block when writable.
	_, _, err = AccessWorldObjectWorktree(ctx, ws, worktreeObjKey, updateWorld, wdBfs, func(bcs *block.Cursor, wt *Worktree) error {
		if updateWorld {
			bcs.SetBlock(wt, true)
		}

		// Resolve the repository's HEAD reference store before invoking the callback.
		hrs, err := wt.FollowHeadRefStore(bcs)
		if err != nil {
			return err
		}

		// Access the repository object and run the caller callback against both handles.
		_, _, err = AccessWorldObjectRepo(ctx, ws, repoObjKey, updateWorld, wt, wdBfs, hrs, func(repo *git.Repository) error {
			if cb != nil {
				return cb(repo, wdBfs)
			}
			return nil
		})
		if updateWorld && err == nil {
			bcs.SetBlock(wt, true)
		}
		return err
	})
	return err
}

// CreateWorldObjectWorktree creates a worktree attached to a workdir and repo.
//
// le can be nil
// if checkoutOpts is nil, skips checkout
func CreateWorldObjectWorktree(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	worktreeObjKey, repoObjKey string,
	workdirRef *unixfs_world.UnixfsRef,
	createWorkdir bool,
	checkoutOpts *git.CheckoutOptions,
	sender peer.ID,
	ts time.Time,
) error {
	// Ensure the referenced workdir object exists before creating the worktree.
	workdirObjKey := workdirRef.GetObjectKey()
	_, wdObjExists, err := ws.GetObject(ctx, workdirObjKey)
	if err != nil {
		return err
	}
	if !wdObjExists {
		if !createWorkdir {
			return errors.Wrap(world.ErrObjectNotFound, "workdir")
		}

		// Initialize the missing workdir when creation was requested.
		_, _, err = unixfs_world.FsInit(ctx, ws, sender, workdirObjKey, workdirRef.GetFsType(), nil, false, ts)
		if err != nil {
			return err
		}
	}

	// Allocate the worktree block that will hold checkout metadata.
	wtree := &Worktree{}

	// Configure checkout staging and cleanup for the optional checkout path.
	disableCheckout := checkoutOpts == nil
	var checkoutDir string
	defer func() {
		if checkoutDir != "" {
			_ = os.RemoveAll(checkoutDir)
		}
	}()

	// Create or update the worktree object and materialize checkout files when enabled.
	_, _, err = world.AccessWorldObject(ctx, ws, worktreeObjKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(wtree, true)

		if !disableCheckout {
			// Materialize the repository, perform checkout, and capture its index.
			hrs, err := wtree.FollowHeadRefStore(bcs)
			if err != nil {
				return err
			}
			checkoutDir, err = materializeRepoToTempWorkdir(
				ctx,
				ws,
				repoObjKey,
				true,
				wtree,
				hrs,
				nil,
				func(repo *git.Repository, _ billy.Filesystem) error {
					if le != nil {
						le.Infof("checkout: branch=%s hash=%s", checkoutOpts.Branch, checkoutOpts.Hash)
					}
					if err := checkoutRepoWorktree(repo, checkoutOpts); err != nil {
						return errors.Wrapf(err, "checkout branch %s", checkoutOpts.Branch)
					}
					idx, err := repo.Storer.Index()
					if err != nil {
						return err
					}
					if err := wtree.SetIndex(idx); err != nil {
						return err
					}
					return nil
				},
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if checkoutDir != "" {
		// Synchronize materialized checkout files into the UnixFS workdir.
		if err := syncFSToUnixfsRefBatch(
			ctx,
			ws,
			workdirRef,
			sender,
			ts,
			os.DirFS(checkoutDir),
		); err != nil {
			return err
		}
	}

	// Record the worktree type in world metadata.
	if err := world_types.SetObjectType(ctx, ws, worktreeObjKey, GitWorktreeTypeID); err != nil {
		return err
	}

	// Record the worktree's parent repository relationship.
	err = world_parent.SetObjectParent(ctx, ws, worktreeObjKey, repoObjKey, false)
	if err != nil {
		return err
	}

	// Record the worktree-to-repository graph edge.
	err = ws.SetGraphQuad(
		ctx,
		world.NewGraphQuadWithKeys(worktreeObjKey, GitRepoPred, repoObjKey, ""),
	)
	if err != nil {
		return err
	}

	// Encode the workdir reference and record its graph edge.
	refValue := &unixfs_world.RefValue{
		FsType: workdirRef.GetFsType(),
		Path:   workdirRef.GetPath().Clone(),
	}
	refValueKey, err := refValue.MarshalToKey()
	if err != nil {
		return err
	}
	refValueGv := world.GraphValueToString(world.KeyToGraphValue(refValueKey))
	err = ws.SetGraphQuad(
		ctx,
		world.NewGraphQuadWithKeys(worktreeObjKey, GitWorktreeWorkdirPred, workdirRef.GetObjectKey(), refValueGv),
	)
	if err != nil {
		return err
	}

	// Record the repository-to-worktree graph edge.
	err = ws.SetGraphQuad(
		ctx,
		world.NewGraphQuadWithKeys(repoObjKey, GitRepoWorktreePred, worktreeObjKey, ""),
	)
	if err != nil {
		return err
	}

	// success
	return nil
}

// WorktreeLookupWorkdirRef looks up the unixfs ref to the workdir from graph quads.
func WorktreeLookupWorkdirRef(
	ctx context.Context,
	worldHandle world.WorldState,
	objKey string,
) (*unixfs_world.UnixfsRef, error) {
	// Read the workdir graph edge and normalize a missing edge to ErrQuadNotFound.
	gqs, err := worldHandle.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(objKey, GitWorktreeWorkdirPred, "", ""), 1)
	if len(gqs) == 0 && err == nil {
		err = world.ErrQuadNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "workdir")
	}
	gq := gqs[0]

	workdirObjKey, err := world.GraphValueToKey(gq.GetObj())
	if err != nil {
		return nil, errors.Wrap(err, "workdir: graph quad object")
	}

	// Decode the optional ref-value label attached to the graph edge.
	var workdirRefValue *unixfs_world.RefValue
	if refValueKey := gq.GetLabel(); len(refValueKey) != 0 {
		refValueGv, err := world.GraphValueToKey(refValueKey)
		if err == nil {
			workdirRefValue, err = unixfs_world.UnmarshalRefValueFromKey(refValueGv)
		}
		if err != nil {
			return nil, errors.Wrap(err, "workdir: graph quad label")
		}
	}

	// Build and validate the UnixFS reference returned to the caller.
	ref := &unixfs_world.UnixfsRef{
		ObjectKey: workdirObjKey,
		FsType:    workdirRefValue.GetFsType(),
		Path:      workdirRefValue.GetPath(),
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return ref, nil
}
