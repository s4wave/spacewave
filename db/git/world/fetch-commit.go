package git_world

import (
	"context"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/pkg/errors"
)

// fetchSourcePath is the path FetchCommit's in-process transport serves the
// source repository at.
const fetchSourcePath = "/source"

// FetchCommit fetches commit and its tree at depth 1 from src into repo,
// points branch at it, and points HEAD at branch. Both repositories are open,
// so the transfer runs in process: repo receives only the commit's objects it
// lacks, and records the commit as shallow. A branch already at commit is
// left as it is.
func FetchCommit(ctx context.Context, repo, src *git.Repository, commit plumbing.Hash, branch plumbing.ReferenceName) error {
	// Fetch the commit into the branch through a transport serving src.
	remote := git.NewRemote(repo.Storer, &config.RemoteConfig{Name: "source", URLs: []string{fetchSourcePath}})
	err := remote.FetchContext(ctx, &git.FetchOptions{
		RemoteName:    "source",
		Depth:         1,
		Tags:          git.NoTags,
		RefSpecs:      []config.RefSpec{config.RefSpec("+" + commit.String() + ":" + branch.String())},
		ClientOptions: []client.Option{client.WithLoader(transport.MapLoader{fetchSourcePath: src.Storer})},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return errors.Wrapf(err, "fetch %s", commit)
	}

	// Point HEAD at the branch.
	return repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, branch))
}
