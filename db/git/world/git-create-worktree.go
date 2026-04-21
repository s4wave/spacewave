package git_world

import (
	"context"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	git_block "github.com/s4wave/spacewave/db/git/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// GitCreateWorktreeOpId is the git init operation id.
var GitCreateWorktreeOpId = "hydra/git/create-worktree"

// GitCreateWorktree creates a new git worktree attached to a repo and workdir.
func GitCreateWorktree(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey, repoObjKey string,
	workdirRef *unixfs_world.UnixfsRef,
	createWorkdir bool,
	checkoutOpts *git_block.CheckoutOpts,
	disableCheckout bool,
	ts time.Time,
) error {
	createOp := NewGitCreateWorktreeOp(objKey, repoObjKey, workdirRef, createWorkdir, checkoutOpts, disableCheckout, ts)
	_, _, err := ws.ApplyWorldOp(ctx, createOp, sender)
	return err
}

// NewGitCreateWorktreeOp constructs a new GitCreateWorktreeOp block.
// workdirObjKey, workdirPath, and ref can be empty.
func NewGitCreateWorktreeOp(
	objKey string,
	repoObjKey string,
	workdirRef *unixfs_world.UnixfsRef,
	createWorkdir bool,
	checkoutOpts *git_block.CheckoutOpts,
	disableCheckout bool,
	ts time.Time,
) *GitCreateWorktreeOp {
	return &GitCreateWorktreeOp{
		ObjectKey:       objKey,
		RepoObjectKey:   repoObjKey,
		WorkdirRef:      workdirRef,
		CreateWorkdir:   createWorkdir,
		CheckoutOpts:    checkoutOpts,
		DisableCheckout: disableCheckout,
		Timestamp:       unixfs_block.ToTimestamp(ts, false),
	}
}

// NewGitCreateWorktreeOpBlock constructs a new GitCreateWorktreeOp block.
func NewGitCreateWorktreeOpBlock() block.Block {
	return &GitCreateWorktreeOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *GitCreateWorktreeOp) GetOperationTypeId() string {
	return GitCreateWorktreeOpId
}

// Validate checks the create worktree operation.
func (o *GitCreateWorktreeOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetWorkdirRef().Validate(); err != nil {
		return errors.Wrap(err, "workdir_ref")
	}
	if err := o.GetCheckoutOpts().Validate(); err != nil {
		return errors.Wrap(err, "checkout_opts")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitCreateWorktreeOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	repoObjKey := o.GetRepoObjectKey()
	if objKey == "" || repoObjKey == "" {
		return false, world.ErrEmptyObjectKey
	}

	ts := o.GetTimestamp().AsTime()
	workdirRef := o.GetWorkdirRef()
	createWorkdir := o.GetCreateWorkdir()
	disableCheckout := o.GetDisableCheckout()

	var checkoutOpts *git.CheckoutOptions
	if !disableCheckout {
		checkoutOpts, err = o.GetCheckoutOpts().BuildCheckoutOpts()
		if err != nil {
			return false, err
		}
	}

	// create worktree and checkout
	err = CreateWorldObjectWorktree(
		ctx,
		le,
		worldHandle,
		objKey,
		repoObjKey,
		workdirRef,
		createWorkdir,
		checkoutOpts,
		sender,
		ts,
	)
	if err != nil {
		return false, err
	}

	// success
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitCreateWorktreeOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *GitCreateWorktreeOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *GitCreateWorktreeOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitCreateWorktreeOp)(nil))
