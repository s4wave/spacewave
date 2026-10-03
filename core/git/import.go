package s4wave_git

import (
	"context"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// CloneGitRepoToRef clones a remote Git repository through storage and
// returns its completed repo ref. Pass a World stage held until a transaction
// adopts the ref.
func CloneGitRepoToRef(
	ctx context.Context,
	ws world.WorldStorage,
	cloneOpts *git_block.CloneOpts,
	authMethod client.SSHAuth,
	progress sideband.Progress,
) (*bucket.ObjectRef, error) {
	// Configure the remote clone to populate repository storage without a checkout.
	cloneArgs := cloneOpts.BuildCloneOpts()
	cloneArgs.NoCheckout = true
	if authMethod != nil {
		cloneArgs.ClientOptions = append(cloneArgs.ClientOptions, client.WithSSHAuth(authMethod))
	}
	cloneArgs.Progress = progress

	return world.AccessObject(
		ctx,
		ws.AccessWorldState,
		nil,
		func(bcs *block.Cursor) error {
			// Open repository storage inside the World object transaction.
			root := git_block.NewRepo()
			bcs.SetBlock(root, true)
			store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
			if err != nil {
				return err
			}
			defer store.Close()

			// Clone the remote repository and commit its stored graph.
			_, err = git.CloneContext(ctx, store, memfs.New(), cloneArgs)
			if err != nil {
				return errors.Wrap(err, "clone")
			}
			return store.Commit()
		},
	)
}
