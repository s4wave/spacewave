package folder

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
)

// ErrFolderRootChanged is returned by a sync while the folder's root is
// missing or on another file system than the one it was linked on, as when
// the folder's disk is unmounted and its empty mount point remains: a sync
// would then remove the whole Drive. The sync removes nothing and resumes
// once the root is back on its file system, or after ResetFolderRoot records
// the file system it is on now. The error a sync returns is a
// *FolderRootChangedError.
var ErrFolderRootChanged = errors.New("folder root changed")

// FolderRootChangedError is the ErrFolderRootChanged of one folder, with the
// file-system identities that differ.
type FolderRootChangedError struct {
	// Folder is the linked folder.
	Folder string
	// Recorded is the identity of the file system the folder was linked on.
	Recorded uint64
	// Current is the identity of the file system the root is on now, or 0
	// when Missing.
	Current uint64
	// Missing is set when the root does not exist.
	Missing bool
}

// Error describes the folder and both identities.
func (e *FolderRootChangedError) Error() string {
	if e.Missing {
		return fmt.Sprintf("%s: %s is missing; it was linked on file system %d", ErrFolderRootChanged, e.Folder, e.Recorded)
	}
	return fmt.Sprintf("%s: %s is on file system %d, linked on %d", ErrFolderRootChanged, e.Folder, e.Current, e.Recorded)
}

// Is matches ErrFolderRootChanged.
func (e *FolderRootChangedError) Is(target error) bool {
	return target == ErrFolderRootChanged
}

// ResetFolderRoot records the file system the folder's root is on now as the
// one it is linked on, as after its disk returns with another identity, and
// returns the identity recorded before, zero when none was, and the new one.
// A link waiting for its root then resumes with a full sync, which removes
// every Drive entry the folder lacks: the caller vouches that the folder on
// this file system is the one linked.
func ResetFolderRoot(ctx context.Context, idx *block_store_filesegment.Index, folder string) (previous, current uint64, err error) {
	// Read the root's identity and the recorded one.
	if !filepath.IsAbs(folder) {
		return 0, 0, errors.Errorf("folder must be an absolute path: %q", folder)
	}
	folder = filepath.Clean(folder)
	current, err = folderRootID(folder)
	if err != nil {
		return 0, 0, err
	}
	previous, _, err = idx.FolderRoot(ctx, folder)
	if err != nil {
		return 0, 0, err
	}

	// Record the new one.
	return previous, current, idx.SetFolderRoot(ctx, folder, current)
}
