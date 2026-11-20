package resource_space

import (
	"strings"

	"github.com/pkg/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// LinkFolder links a folder on this Device into a Drive: each file's blocks
// stay in the file and are read from it, and only the file's Blob and the
// directory tree go to the Space. A folder linked before is synced: only
// entries that changed since are linked again. It streams the files and bytes
// linked after each committed batch, then a final message with done set.
// Symlinks and special files are skipped.
func (r *SpaceResource) LinkFolder(
	req *s4wave_space.LinkFolderRequest,
	strm s4wave_space.SRPCSpaceResourceService_LinkFolderStream,
) error {
	// Require a store that can serve linked files.
	ctx := strm.Context()
	files := r.space.GetSharedObject().GetBlockStore().GetFileSegments()
	if files == nil {
		return errors.New("this Space's block store cannot link files")
	}

	// Sync the folder into the destination path of the Drive.
	var dest []string
	for part := range strings.SplitSeq(req.GetPath(), "/") {
		if part != "" {
			dest = append(dest, part)
		}
	}
	link, err := unixfs_world.NewFolderLink(r.space.GetWorldEngine(), files.GetIndex(), req.GetObjectKey(), req.GetFolder(), dest)
	if err != nil {
		return err
	}
	var linked, linkedBytes uint64
	err = link.Sync(ctx, func(files, bytes uint64) error {
		linked, linkedBytes = files, bytes
		return strm.Send(&s4wave_space.LinkFolderProgress{Files: files, Bytes: bytes})
	})
	if err != nil {
		return err
	}
	return strm.Send(&s4wave_space.LinkFolderProgress{Files: linked, Bytes: linkedBytes, Done: true})
}
