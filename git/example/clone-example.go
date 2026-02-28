package git_examples

import (
	"context"
	"os"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage"
	"github.com/sirupsen/logrus"
)

// RunCloneExample attempts to perform a clone demo into the given interfaces.
// This is used for several of the code toys.
func RunCloneExample(
	ctx context.Context,
	le *logrus.Entry,
	url string,
	storage storage.Storer,
	worktree billy.Filesystem,
) error {
	cloneOpts := &git.CloneOptions{
		// The (possibly remote) repository URL to clone from.
		URL: url,
		// Auth credentials, if required, to use with the remote repository.
		// Auth transport.AuthMethod
		// Name of the remote to be added, by default `origin`.
		// RemoteName string
		// Remote branch to clone.
		// ReferenceName plumbing.ReferenceName
		// Fetch only ReferenceName if true.
		// SingleBranch bool
		// No checkout of HEAD after clone if true.
		// NoCheckout bool
		// Limit fetching to the specified number of commits.
		// Depth int
		// RecurseSubmodules after the clone is created, initialize all submodules
		// within, using their default settings. This option is ignored if the
		// cloned repository does not have a worktree.
		// RecurseSubmodules SubmoduleRescursivity
		// Progress is where the human readable information sent by the server is
		// stored, if nil nothing is stored and the capability (if supported)
		// no-progress, is sent to the server to avoid send this information.
		Progress: os.Stdout,
		// Tags describe how the tags will be fetched from the remote repository,
		// by default is AllTags.
		// Tags TagMode
	}
	repo, err := git.CloneContext(
		ctx,
		storage,
		worktree,
		cloneOpts,
	)
	if err != nil {
		return err
	}
	le.Info("cloned")
	_ = repo

	files, err := worktree.ReadDir("")
	if err != nil {
		return err
	}
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		le.Debugf(
			"%v %s",
			info.Mode().String(),
			f.Name(),
		)
	}
	return nil
}
