package git_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
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
	case GitCreateWorktreeOpId:
		return &GitCreateWorktreeOp{}, nil
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
	if indexStore == nil {
		indexStore = &memory.IndexStorage{}
	}
	if workdir == nil {
		workdir = memfs.New()
	}
	repob, err := git_block.UnmarshalRepo(bcs)
	if err != nil {
		return err
	}
	if err := repob.Validate(); err != nil {
		return err
	}
	store, err := git_block.NewStore(ctx, nil, bcs, indexStore, refStore)
	if err != nil {
		return err
	}
	defer store.Close()
	repo, err := git.Open(store, workdir)
	if err != nil {
		return err
	}
	if cb != nil {
		if err := cb(repo); err != nil {
			return err
		}
	}
	return store.Commit()
}

// ValidateOrCreateRepo creates or checks a reference to a Repo.
// repoRef can be nil to create a new repo.
func ValidateOrCreateRepo(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	repoRef *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	var err error
	if repoRef.GetEmpty() {
		repoRef, err = world.AccessObject(ctx, accessState, nil, func(bcs *block.Cursor) error {
			bcs.SetBlock(git_block.NewRepo(), true)
			return nil
		})
	} else {
		if err := repoRef.Validate(); err != nil {
			return nil, err
		}
		_, err = world.AccessObject(ctx, accessState, repoRef, func(bcs *block.Cursor) error {
			// Confirm valid repo object.
			repo, err := git_block.UnmarshalRepo(bcs)
			if err == nil {
				err = repo.Validate()
			}
			return err
		})
	}
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
	wt, err := UnmarshalWorktree(bcs)
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
	workdir billy.Filesystem,
	cb func(bcs *block.Cursor, wt *Worktree) error,
) (*bucket.ObjectRef, error) {
	if workdir == nil {
		workdir = memfs.New()
	}
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

	// open the workdir fs
	workdirFs, err := unixfs_world.BuildFSFromUnixfsRef(ctx, le, ws, sender, workdirRef, true, false, ts)
	if err != nil {
		return err
	}
	defer workdirFs.Release()

	wdFsHandle, err := workdirFs.AddRootReference(ctx)
	if err != nil {
		return err
	}
	defer wdFsHandle.Release()

	// construct billy fs
	wdBfs := unixfs.NewBillyFilesystem(ctx, wdFsHandle, "", ts)

	// access worktree object
	_, _, err = AccessWorldObjectWorktree(ctx, ws, worktreeObjKey, updateWorld, wdBfs, func(bcs *block.Cursor, wt *Worktree) error {
		// access repo
		hrs, err := wt.FollowHeadRefStore(bcs)
		if err != nil {
			return err
		}
		_, _, err = AccessWorldObjectRepo(ctx, ws, repoObjKey, updateWorld, wt, wdBfs, hrs, func(repo *git.Repository) error {
			if cb != nil {
				return cb(repo, wdBfs)
			}
			return nil
		})
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
	// ensure workdir exists
	workdirObjKey := workdirRef.GetObjectKey()
	_, wdObjExists, err := ws.GetObject(workdirObjKey)
	if err != nil {
		return err
	}
	if !wdObjExists {
		if !createWorkdir {
			return errors.Wrap(world.ErrObjectNotFound, "workdir")
		}

		// init the workdir
		err = unixfs_world.FsInit(ctx, ws, sender, workdirObjKey, workdirRef.GetFsType(), nil, 0, false, ts)
		if err != nil {
			return err
		}
	}

	// open the workdir fs
	workdirFs, err := unixfs_world.BuildFSFromUnixfsRef(ctx, le, ws, sender, workdirRef, createWorkdir, false, ts)
	if err != nil {
		return err
	}
	defer workdirFs.Release()

	wdFsHandle, err := workdirFs.AddRootReference(ctx)
	if err != nil {
		return err
	}
	defer wdFsHandle.Release()

	// construct billy fs
	wdBfs := unixfs.NewBillyFilesystem(ctx, wdFsHandle, "", ts)

	// create worktree
	wtree := &Worktree{}

	// checkout
	disableCheckout := checkoutOpts == nil

	// create worktree object
	_, _, err = world.AccessWorldObject(ctx, ws, worktreeObjKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(wtree, true)

		if !disableCheckout {
			// call git to checkout to the worktree
			hrs, err := wtree.FollowHeadRefStore(bcs)
			if err != nil {
				return err
			}
			_, _, err = AccessWorldObjectRepo(
				ctx,
				ws,
				repoObjKey, false,
				wtree,
				wdBfs,
				hrs,
				func(repo *git.Repository) error {
					wt, err := repo.Worktree()
					if err != nil {
						return err
					}

					if checkoutOpts.Branch == "" && checkoutOpts.Hash.IsZero() {
						// checkout the HEAD
						href, err := repo.Head()
						if err != nil {
							return err
						}
						checkoutOpts.Branch = href.Name()
					}

					return wt.Checkout(checkoutOpts)
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

	// worktree type -> types/git/worktree
	typesState := world_types.NewTypesState(ctx, ws)
	if err := typesState.SetObjectType(worktreeObjKey, GitWorktreeTypeID); err != nil {
		return err
	}

	// worktree parent -> repo
	parentsState := world_parent.NewParentState(ws)
	err = parentsState.SetObjectParent(ctx, worktreeObjKey, repoObjKey, false)
	if err != nil {
		return err
	}

	// worktree git/repo -> repo
	err = ws.SetGraphQuad(
		world.NewGraphQuadWithKeys(worktreeObjKey, GitRepoPred, repoObjKey, ""),
	)
	if err != nil {
		return err
	}

	// worktree git/workdir -> workdir <ref-value>
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
		world.NewGraphQuadWithKeys(worktreeObjKey, GitWorktreeWorkdirPred, workdirRef.GetObjectKey(), refValueGv),
	)
	if err != nil {
		return err
	}

	// repo git/worktree -> worktree
	err = ws.SetGraphQuad(
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
	// access the workdir
	gqs, err := worldHandle.LookupGraphQuads(world.NewGraphQuadWithKeys(objKey, GitWorktreeWorkdirPred, "", ""), 1)
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

	// get the ref opts
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
