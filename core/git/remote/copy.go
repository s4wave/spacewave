package git_remote

import (
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/revlist"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
)

// CopyObjects writes the objects reachable from each commit in hashes in repo
// into the local repository at gitDir, as one packfile holding only what that
// repository lacks: the walk stops at the commits its refs and HEAD reach, and
// at repo's shallow boundary. Each copied commit that is shallow in repo
// becomes shallow in the local repository, so Git does not look for its
// parents.
func CopyObjects(repo *git.Repository, gitDir string, hashes []string) error {
	// Collect the local repository's tips, where the walk stops.
	local, err := openLocal(gitDir)
	if err != nil {
		return err
	}
	haves, err := localTips(local)
	if err != nil {
		return err
	}

	// List the objects the local repository lacks.
	wants := make([]plumbing.Hash, len(hashes))
	for i, hash := range hashes {
		wants[i] = plumbing.NewHash(hash)
	}
	objs, err := revlist.Objects(repo.Storer, wants, haves)
	if err != nil {
		return errors.Wrap(err, "list objects to copy")
	}
	if len(objs) == 0 {
		return nil
	}

	// Write them to the local repository as one packfile.
	pw, ok := local.Storer.(storer.PackfileWriter)
	if !ok {
		return errors.New("local repository cannot store a packfile")
	}
	w, err := pw.PackfileWriter()
	if err != nil {
		return err
	}
	if _, err := packfile.NewEncoder(w, repo.Storer, false).Encode(objs, 10); err != nil {
		_ = w.Close()
		return errors.Wrap(err, "copy objects to the local repository")
	}
	if err := w.Close(); err != nil {
		return errors.Wrap(err, "copy objects to the local repository")
	}

	// Carry over the shallow boundary among the copied commits.
	return copyShallow(repo, local, objs)
}

// localTips returns the hashes the local repository's refs and HEAD point to.
func localTips(local *git.Repository) ([]plumbing.Hash, error) {
	// Start from a detached HEAD, which no ref points to, then add each
	// ref.
	var tips []plumbing.Hash
	if head, err := local.Storer.Reference(plumbing.HEAD); err == nil && head.Type() == plumbing.HashReference {
		tips = append(tips, head.Hash())
	}
	refs, err := local.Storer.IterReferences()
	if err != nil {
		return nil, err
	}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference {
			tips = append(tips, ref.Hash())
		}
		return nil
	})
	return tips, err
}

// copyShallow marks each of objs that is shallow in repo as shallow in local.
func copyShallow(repo, local *git.Repository, objs []plumbing.Hash) error {
	// Find the copied shallow commits.
	shallow, err := repo.Storer.Shallow()
	if err != nil || len(shallow) == 0 {
		return err
	}
	copied := make(map[plumbing.Hash]struct{}, len(objs))
	for _, h := range objs {
		copied[h] = struct{}{}
	}
	var add []plumbing.Hash
	for _, h := range shallow {
		if _, ok := copied[h]; ok {
			add = append(add, h)
		}
	}
	if len(add) == 0 {
		return nil
	}

	// Add them to the local repository's shallow list.
	have, err := local.Storer.Shallow()
	if err != nil {
		return err
	}
	return local.Storer.SetShallow(append(have, add...))
}
