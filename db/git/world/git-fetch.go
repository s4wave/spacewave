package git_world

import (
	"context"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// GitFetchOpId is the git fetch operation id.
var GitFetchOpId = "hydra/git/fetch"

// GitFetch performs a git fetch operation against an existing repo in the world.
// Fetches new objects from the remote into the existing world object repo.
// authMethod and progress can be empty.
func GitFetch(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	fetchOpts *git_block.FetchOpts,
	authMethod client.SSHAuth,
	progress sideband.Progress,
) error {
	fetchArgs := fetchOpts.BuildFetchOpts()
	if authMethod != nil {
		fetchArgs.ClientOptions = append(fetchArgs.ClientOptions, client.WithSSHAuth(authMethod))
	}
	fetchArgs.Progress = progress

	_, _, err := AccessWorldObjectRepo(
		ctx,
		ws,
		objKey,
		true,
		&memory.IndexStorage{},
		nil,
		nil,
		func(repo *git.Repository) error {
			err := repo.FetchContext(ctx, fetchArgs)
			if errors.Is(err, git.NoErrAlreadyUpToDate) {
				return nil
			}
			return err
		},
	)
	return err
}

// NewGitFetchOp constructs a new GitFetchOp.
func NewGitFetchOp(objKey string, fetchOpts *git_block.FetchOpts) *GitFetchOp {
	return &GitFetchOp{
		ObjectKey: objKey,
		FetchOpts: fetchOpts,
	}
}

// GetOperationTypeId returns the operation type identifier.
func (o *GitFetchOp) GetOperationTypeId() string {
	return GitFetchOpId
}

// Validate checks the fetch operation.
func (o *GitFetchOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetFetchOpts().Validate(); err != nil {
		return errors.Wrap(err, "fetch_opts")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitFetchOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()

	// check that the object exists
	_, exists, err := worldHandle.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, errors.Wrap(world.ErrObjectNotFound, objKey)
	}

	// perform the fetch
	err = GitFetch(ctx, worldHandle, objKey, o.GetFetchOpts(), nil, nil)
	return false, err
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitFetchOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *GitFetchOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *GitFetchOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitFetchOp)(nil))
