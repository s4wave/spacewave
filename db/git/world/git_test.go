//go:build test_git_clone_world

package git_world

import (
	"os"
	"testing"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	git_block "github.com/s4wave/spacewave/db/git/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
)

// TestGitClone tests cloning to a world.
func TestGitClone(t *testing.T) {
	// Start a World with the git op handlers.
	ctx, le, ws, cleanup, sender := newGitWorldState(t)
	defer cleanup()

	// Name the repository, its worktree and workdir, and the operation time.
	objKey := "test-git-repo"
	worktreeKey := objKey + "/worktree"
	workdirKey := "test-git-workdir"
	opTs := unixfs_block.FillPlaceholderTimestamp(nil)
	ts := opTs.AsTime()

	// Clone this repository with a worktree.
	outRef, err := GitClone(
		ctx,
		ws,
		objKey,
		sender,
		&git_block.CloneOpts{
			Url: "../../",
		},
		nil,
		os.Stderr,
		&GitCreateWorktreeOp{
			ObjectKey:     worktreeKey,
			CreateWorkdir: true,
			WorkdirRef: &unixfs_world.UnixfsRef{
				ObjectKey: workdirKey,
				FsType:    unixfs_world.FSType_FSType_FS_NODE,
			},
			CheckoutOpts: &git_block.CheckoutOpts{
				Force: true,
			},
			Timestamp: opTs,
		},
		opTs,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("cloned to reference: %s", outRef.MarshalString())

	// Create a second worktree at HEAD.
	altWorktreeKey := "other-worktree"
	altWorkdirKey := "other-workdir"
	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: altWorkdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	le.Info("checking out second worktree")
	err = GitCreateWorktree(
		ctx,
		ws,
		sender,
		altWorktreeKey,
		objKey,
		workdirRef,
		true,
		&git_block.CheckoutOpts{Force: true},
		false,
		ts,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Log the second worktree's files and status.
	err = AccessWorldObjectRepoWithWorktree(
		ctx,
		le,
		ws,
		objKey, altWorktreeKey,
		ts, false, sender,
		func(repo *git.Repository, workDir billy.Filesystem) error {
			// List the workdir contents.
			le.Info("showing workdir contents")
			files, err := workDir.ReadDir("")
			if err != nil {
				return err
			}
			le.Debugf("workdir contains %d files", len(files))
			for _, f := range files {
				fi, fiErr := f.Info()
				if fiErr != nil {
					le.Debugf("? %s (info err: %v)", f.Name(), fiErr)
					continue
				}
				le.Debugf(
					"%v %s",
					fi.Mode().String(),
					f.Name(),
				)
			}

			// Read the worktree status.
			le.Info("showing git status")
			wt, err := repo.Worktree()
			if err != nil {
				return err
			}
			status, err := wt.Status()
			if err != nil {
				return err
			}

			// Log and check for symlink-related modifications.
			var modifiedCount int
			for path, fs := range status {
				if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
					le.Debugf("status: %c%c %s", fs.Staging, fs.Worktree, path)
					modifiedCount++
				}
			}
			if modifiedCount == 0 {
				le.Debug("status: clean")
			} else {
				le.Debugf("status: %d files modified", modifiedCount)
			}

			// Check symlinks via Lstat.
			for path, fs := range status {
				if fs.Worktree == git.Modified {
					lfi, lErr := workDir.Lstat(path)
					if lErr != nil {
						le.Debugf("lstat %s: %v", path, lErr)
						continue
					}
					le.Debugf("lstat %s: mode=%v symlink=%v",
						path, lfi.Mode(), lfi.Mode()&os.ModeSymlink != 0)
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}
