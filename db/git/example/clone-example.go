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
		URL:      url,
		Progress: os.Stdout,
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
