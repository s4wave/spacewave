package unixfs_sync_git

import (
	"context"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/aperturerobotics/util/gitcmd"
)

// SyncFromGitWorkdir calls "git ls-files" using os/exec to determine the list of
// files in a Git workdir. Includes untracked and ignores gitignored files.
// It synchronizes the files from the list to the destination UnixFS.
func SyncFromGitWorkdir(
	ctx context.Context,
	destHandle *unixfs.FSHandle,
	srcDir string,
	deleteMode unixfs_sync.DeleteMode,
	filterCb unixfs_sync.FilterCb,
) error {
	files, err := gitcmd.ListGitFiles(srcDir)
	if err != nil {
		return err
	}

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
