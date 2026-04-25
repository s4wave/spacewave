package resource_git

import (
	"slices"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/pkg/errors"
)

// SnapshotRepo reads the initial state of the repo into a RepoSnapshot.
func SnapshotRepo(repo *git.Repository, snap *RepoSnapshot) error {
	headRef, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			snap.IsEmpty = true
			return nil
		}
		return errors.Wrap(err, "head")
	}

	snap.HeadRef = headRef.Name().Short()
	snap.HeadCommitHash = headRef.Hash().String()

	commit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return errors.Wrap(err, "head commit")
	}

	snap.LastCommit = &CommitSnapshot{
		Hash:            commit.Hash.String(),
		Message:         commit.Message,
		AuthorName:      commit.Author.Name,
		AuthorEmail:     commit.Author.Email,
		AuthorTimestamp: commit.Author.When.Unix(),
	}

	tree, err := commit.Tree()
	if err != nil {
		return errors.Wrap(err, "head tree")
	}
	snap.ReadmePath = findReadme(tree)

	refs, err := repo.References()
	if err != nil {
		return errors.Wrap(err, "references")
	}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		if name == plumbing.HEAD {
			return nil
		}
		hash := ref.Hash()
		info := RefSnapshot{
			Name:       name.Short(),
			CommitHash: hash.String(),
			IsHead:     hash == headRef.Hash() && name == headRef.Name(),
		}
		if name.IsBranch() {
			snap.Branches = append(snap.Branches, info)
		} else if name.IsTag() {
			snap.Tags = append(snap.Tags, info)
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "iterate references")
	}

	return nil
}

// readmeNames is the list of README filenames to search for, in priority order.
var readmeNames = []string{
	"readme.md",
	"readme.rst",
	"readme.txt",
	"readme",
}

// findReadme searches the root tree for a README file.
// Follows GitHub's heuristic: case-insensitive matching with .md, .rst, .txt, or no extension.
func findReadme(tree *object.Tree) string {
	for _, entry := range tree.Entries {
		lower := strings.ToLower(entry.Name)
		if slices.Contains(readmeNames, lower) {
			return entry.Name
		}
	}
	return ""
}
