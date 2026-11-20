package forge_lib_skiffos

import (
	"bytes"
	"context"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/pkg/errors"
	git_remote "github.com/s4wave/spacewave/core/git/remote"
	git_world "github.com/s4wave/spacewave/db/git/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// InputSkiffOS names the input holding the SkiffOS source, a Repo snapshot
// whose HEAD commit the build checks out.
const InputSkiffOS = "skiffos"

// submodule is one submodule a commit records: its name in .gitmodules, its
// path in the tree and the commit its gitlink pins.
type submodule struct {
	name, path string
	commit     plumbing.Hash
}

// syncSource brings the checkout at dir to the HEAD commit of the Repo the
// skiffos input holds, each submodule at the commit that commit pins, and
// returns the commit. It copies only the objects the checkout lacks, and
// refuses local changes rather than discard them, since the reuse key covers
// only the Repo.
func (c *Controller) syncSource(ctx context.Context, dir string) (string, error) {
	// Resolve the input to the Repo's object ref.
	iv, ok := c.inputVals[InputSkiffOS]
	if !ok {
		return "", errors.Errorf("input %q not found in input values", InputSkiffOS)
	}
	val, err := forge_target.InputValueToValue(iv)
	if err != nil {
		return "", errors.Wrapf(err, "input %q", InputSkiffOS)
	}
	if val == nil || val.IsEmpty() {
		return "", errors.Errorf("input %q is empty", InputSkiffOS)
	}
	ref, err := val.ToBucketRef()
	if err != nil {
		return "", errors.Wrapf(err, "input %q", InputSkiffOS)
	}

	// Create the checkout on the first build.
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		if _, err := runGit(ctx, "", "init", "-q", dir); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	// Check out the Repo's HEAD commit while the Repo is open.
	var commit plumbing.Hash
	err = git_world.ReadRepo(ctx, c.handle.AccessStorage, ref, func(repo *git.Repository) error {
		head, err := repo.Head()
		if err != nil {
			return errors.Wrap(err, "read HEAD")
		}
		commit = head.Hash()
		return syncRepo(ctx, repo, dir, filepath.Join(dir, ".git"), commit)
	})
	if err != nil {
		return "", errors.Wrapf(err, "input %q", InputSkiffOS)
	}

	// Refuse local changes, including submodules and untracked files.
	status, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if len(status) != 0 {
		return "", errors.Errorf("SkiffOS checkout has local changes:\n%s", status)
	}
	return commit.String(), nil
}

// syncRepo checks out commit in the worktree at dir, whose Git directory is
// gitDir, copying it from repo unless gitDir holds it. A worktree whose HEAD
// already names commit is left as it is, on its branch if it has one. It then
// does the same for each submodule the commit pins, from repo's module of
// that name, with the submodule's Git directory under gitDir/modules as Git
// places it.
func syncRepo(ctx context.Context, repo *git.Repository, dir, gitDir string, commit plumbing.Hash) error {
	// Move only a worktree whose HEAD is elsewhere or unborn.
	head, err := runGit(ctx, dir, "rev-parse", "-q", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != commit.String() {
		if err := checkoutCommit(ctx, repo, dir, gitDir, commit, false); err != nil {
			return err
		}
	}

	// Bring each submodule to its pinned commit, linking its worktree to its
	// Git directory when the link is missing, so Git commands there never
	// reach the superproject. Git creates the directory or reuses it.
	mods, err := readSubmodules(repo, commit)
	if err != nil {
		return err
	}
	for _, m := range mods {
		subDir := filepath.Join(dir, m.path)
		subGitDir := filepath.Join(gitDir, "modules", m.name)
		_, err := os.Lstat(filepath.Join(subDir, ".git"))
		linked := errors.Is(err, fs.ErrNotExist)
		if linked {
			if err := os.MkdirAll(filepath.Dir(subGitDir), 0o755); err != nil {
				return err
			}
			if _, err := runGit(ctx, dir, "init", "-q", "--separate-git-dir", subGitDir, subDir); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		// Open the Repo's module, fill a newly linked worktree, which holds
		// no tracked files, then sync the submodule like the superproject.
		store, err := repo.Storer.Module(m.name)
		if err != nil {
			return errors.Wrapf(err, "submodule %s", m.name)
		}
		sub, err := git.Open(store, nil)
		if err != nil {
			return errors.Wrapf(err, "submodule %s", m.name)
		}
		if linked {
			if err := checkoutCommit(ctx, sub, subDir, subGitDir, m.commit, true); err != nil {
				return errors.Wrapf(err, "submodule %s", m.name)
			}
		}
		if err := syncRepo(ctx, sub, subDir, subGitDir, m.commit); err != nil {
			return errors.Wrapf(err, "submodule %s", m.name)
		}
	}

	// Record the submodules as initialized, so the build's own submodule
	// commands find them in place and fetch nothing.
	if len(mods) != 0 {
		if _, err := runGit(ctx, dir, "submodule", "init", "-q"); err != nil {
			return err
		}
	}
	return nil
}

// checkoutCommit copies commit from repo unless gitDir holds it, then checks
// it out detached in dir. Git refuses to overwrite local changes unless fill
// is set, which writes every tracked file into a worktree that holds none.
func checkoutCommit(ctx context.Context, repo *git.Repository, dir, gitDir string, commit plumbing.Hash, fill bool) error {
	// Copy the commit's objects unless the local repository holds the commit.
	if _, err := runGit(ctx, dir, "cat-file", "-e", commit.String()+"^{commit}"); err != nil {
		if err := git_remote.CopyObjects(repo, gitDir, []string{commit.String()}); err != nil {
			return err
		}
	}

	// Check out the commit.
	args := []string{"checkout", "-q", "--detach"}
	if fill {
		args = append(args, "--force")
	}
	_, err := runGit(ctx, dir, append(args, commit.String())...)
	return err
}

// readSubmodules returns the submodules commit pins, in name order, from its
// .gitmodules and gitlinks. A name or path that would leave the repository is
// refused, since each names a directory the checkout writes.
func readSubmodules(repo *git.Repository, commit plumbing.Hash) ([]submodule, error) {
	// Read the commit's tree.
	c, err := repo.CommitObject(commit)
	if err != nil {
		return nil, err
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	// Read .gitmodules from the tree; without it there are none.
	f, err := tree.File(".gitmodules")
	if errors.Is(err, object.ErrFileNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data, err := f.Contents()
	if err != nil {
		return nil, err
	}
	mods := config.NewModules()
	if err := mods.Unmarshal([]byte(data)); err != nil {
		return nil, errors.Wrap(err, "parse .gitmodules")
	}

	// Pair each submodule with the commit its gitlink pins.
	out := make([]submodule, 0, len(mods.Submodules))
	for _, name := range slices.Sorted(maps.Keys(mods.Submodules)) {
		path := mods.Submodules[name].Path
		if !filepath.IsLocal(name) || !filepath.IsLocal(path) {
			return nil, errors.Errorf("submodule %q at %q leaves the repository", name, path)
		}
		entry, err := tree.FindEntry(path)
		if err != nil {
			return nil, errors.Wrapf(err, "submodule %s", name)
		}
		if entry.Mode != filemode.Submodule {
			return nil, errors.Errorf("submodule %s: %s is not a gitlink", name, path)
		}
		out = append(out, submodule{name: name, path: path, commit: entry.Hash})
	}
	return out, nil
}

// runGit runs git with args in dir, or the current directory when dir is
// empty, and returns its standard output. An error carries Git's message.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	// Run git in dir, keeping its error output for the error.
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
