package unixfs_sync_git

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/gitcmd"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
)

// SyncFromGitWorkdir calls "git ls-files" using os/exec to determine the list of
// files in a Git workdir. Includes untracked files and ignores gitignored files.
// It synchronizes the files from the list to the destination UnixFS.
// Files outside Git's selection remain outside the sync.
func SyncFromGitWorkdir(
	ctx context.Context,
	destHandle *unixfs.FSHandle,
	srcDir string,
	deleteMode unixfs_sync.DeleteMode,
	filterCb unixfs_sync.FilterCb,
) error {
	// Read the tracked and untracked paths Git selects for this worktree.
	files, err := gitcmd.ListGitFiles(srcDir)
	if err != nil {
		return err
	}

	// Git emits separate groups; binary-search membership needs one sorted list.
	slices.Sort(files)

	// Copy only selected files through the caller's additional filter.
	return unixfs_sync.SyncFromDisk(
		ctx,
		destHandle,
		srcDir,
		deleteMode,
		unixfs_sync.CombineFilterCbs(
			unixfs_sync.NewFilterFileList(files, true),
			filterCb,
		),
	)
}
