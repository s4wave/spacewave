package forge_lib_skiffos_audit

import (
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/pkg/errors"
)

// buildrootPath is the path of the Buildroot submodule in a SkiffOS tree.
const buildrootPath = "buildroot"

// releaseTagPrefix selects SkiffOS's release tags: all of its tags.
const releaseTagPrefix = "refs/tags/"

// skiffTagPrefix selects the Buildroot fork's release tags.
const skiffTagPrefix = "refs/tags/skiff-"

// readRevision returns commit with the Buildroot commit its tree pins, both
// undescribed.
func readRevision(repo *git.Repository, commit plumbing.Hash) (*Revision, error) {
	// Read the commit's tree.
	c, err := repo.CommitObject(commit)
	if err != nil {
		return nil, errors.Wrapf(err, "commit %s", commit)
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, errors.Wrapf(err, "commit %s", commit)
	}

	// The Buildroot gitlink records the pinned commit.
	entry, err := tree.FindEntry(buildrootPath)
	if err != nil {
		return nil, errors.Wrapf(err, "commit %s: %s", commit, buildrootPath)
	}
	if entry.Mode != filemode.Submodule {
		return nil, errors.Errorf("commit %s: %s is not a gitlink", commit, buildrootPath)
	}
	return &Revision{
		Skiffos:   &Described{Commit: commit.String()},
		Buildroot: &Described{Commit: entry.Hash.String()},
	}, nil
}

// readFreshness relates the build's commit to upstream's head in the upstream
// Repo. It counts the commits behind exactly, as git rev-list --count
// build..head does, by walking both histories.
func readFreshness(repo *git.Repository, build, head plumbing.Hash) (isReport_Freshness, error) {
	// A build at the head is current.
	if build == head {
		return &Report_Current{Current: &Current{}}, nil
	}

	// A build the head does not reach holds a commit an update would drop.
	ahead, err := ancestors(repo, head)
	if err != nil {
		return nil, err
	}
	if _, ok := ahead[build]; !ok {
		return &Report_Diverged{Diverged: &Diverged{}}, nil
	}

	// The build's history lies inside the head's, so the difference in size
	// counts the commits the build lacks.
	behind, err := ancestors(repo, build)
	if err != nil {
		return nil, err
	}
	return &Report_Behind{Behind: &Behind{Commits: uint32(len(ahead) - len(behind))}}, nil // #nosec G115 -- positive, as above, and far below 1<<32
}

// describeAll describes each commit by its nearest tag among the Repo's tags
// whose ref name starts with prefix, and returns how many such tags it holds.
func describeAll(repo *git.Repository, prefix string, ds ...*Described) (int, error) {
	// Map each tagged commit to its tag, peeling annotated tags. Of several
	// tags on one commit the greatest name wins, so the choice is stable.
	tags, err := repo.Tags()
	if err != nil {
		return 0, err
	}
	tagged := make(map[plumbing.Hash]string)
	var count int
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		// Count and map only the tags under prefix.
		name := ref.Name()
		if !strings.HasPrefix(name.String(), prefix) {
			return nil
		}
		count++
		target, err := peel(repo, ref.Hash())
		if err != nil {
			return errors.Wrap(err, name.String())
		}
		if short := name.Short(); short > tagged[target] {
			tagged[target] = short
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Describe each commit against the map.
	for _, d := range ds {
		if err := describe(repo, d, tagged); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// peel returns the commit a tag ref selects, through any annotated tags.
func peel(repo *git.Repository, h plumbing.Hash) (plumbing.Hash, error) {
	for {
		tag, err := repo.TagObject(h)
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return h, nil
		}
		if err != nil {
			return plumbing.ZeroHash, err
		}
		h = tag.Target
	}
}

// describe sets d's tag to the tagged commit fewest parent steps from d's
// commit, searching breadth first, and its distance to that count. A commit
// the Repo lacks, such as a shallow boundary's parent, ends its path.
func describe(repo *git.Repository, d *Described, tagged map[plumbing.Hash]string) error {
	// Search from d's commit, at distance zero.
	type step struct {
		hash plumbing.Hash
		dist uint32
	}
	start := plumbing.NewHash(d.GetCommit())
	queue := []step{{hash: start}}
	seen := map[plumbing.Hash]struct{}{start: {}}
	for len(queue) != 0 {
		// Stop at the first tagged commit.
		s := queue[0]
		queue = queue[1:]
		if tag, ok := tagged[s.hash]; ok {
			d.Tag, d.Distance = tag, s.dist
			return nil
		}

		// Queue the commit's unseen parents one step further.
		c, err := repo.CommitObject(s.hash)
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		for _, p := range c.ParentHashes {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				queue = append(queue, step{hash: p, dist: s.dist + 1})
			}
		}
	}
	return nil
}

// ancestors returns head and every commit it descends from that the Repo
// holds. A commit the Repo lacks, such as a shallow boundary's parent, ends
// its path.
func ancestors(repo *git.Repository, head plumbing.Hash) (map[plumbing.Hash]struct{}, error) {
	// Walk depth first, keeping only the commits the Repo holds.
	seen := map[plumbing.Hash]struct{}{head: {}}
	stack := []plumbing.Hash{head}
	for len(stack) != 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c, err := repo.CommitObject(h)
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			delete(seen, h)
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, p := range c.ParentHashes {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				stack = append(stack, p)
			}
		}
	}
	return seen, nil
}
