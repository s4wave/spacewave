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
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

type worldStorageAccessor interface {
	AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error
}

// CloneGitRepoToRef clones a remote Git repository and returns its completed repo ref.
func CloneGitRepoToRef(
	ctx context.Context,
	ws worldStorageAccessor,
	cloneOpts *git_block.CloneOpts,
	authMethod client.SSHAuth,
	progress sideband.Progress,
) (*bucket.ObjectRef, error) {
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
			root := git_block.NewRepo()
			bcs.SetBlock(root, true)
			store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
			if err != nil {
				return err
			}
			defer store.Close()

			_, err = git.CloneContext(ctx, store, memfs.New(), cloneArgs)
			if err != nil {
				return errors.Wrap(err, "clone")
			}
			return store.Commit()
		},
	)
}
