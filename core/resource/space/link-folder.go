package resource_space

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// linkBatchFiles is the number of files one World transaction of LinkFolder
// adds. Each costs the batch a pending entry and its Blob block.
const linkBatchFiles = 4096

// LinkFolder links a folder on this Device into a Drive: each file's blocks
// stay in the file and are read from it, and only the file's Blob and the
// directory tree go to the Space. It streams the files and bytes linked after
// each committed batch, then a final message with done set. Symlinks and
// special files are skipped.
func (r *SpaceResource) LinkFolder(
	req *s4wave_space.LinkFolderRequest,
	strm s4wave_space.SRPCSpaceResourceService_LinkFolderStream,
) error {
	// Require an absolute folder and a store that can serve linked files.
	ctx := strm.Context()
	folder := req.GetFolder()
	if !filepath.IsAbs(folder) {
		return errors.Errorf("folder must be an absolute path: %q", folder)
	}
	if req.GetObjectKey() == "" {
		return errors.New("object key is required")
	}
	files := r.space.GetSharedObject().GetBlockStore().GetFileSegments()
	if files == nil {
		return errors.New("this Space's block store cannot link files")
	}

	// Resolve the Drive and the destination path in it.
	ws := world.NewEngineWorldState(r.space.GetWorldEngine(), true)
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, req.GetObjectKey())
	if err != nil {
		return err
	}
	var dest []string
	for part := range strings.SplitSeq(req.GetPath(), "/") {
		if part != "" {
			dest = append(dest, part)
		}
	}

	// Walk the folder, committing a batch every linkBatchFiles files.
	l := &folderLink{ctx: ctx, ws: ws, objKey: req.GetObjectKey(), fsType: fsType, idx: files.GetIndex(), folder: folder, dest: dest, strm: strm}
	defer l.release()
	if err := l.declareDest(); err != nil {
		return err
	}
	if err := filepath.WalkDir(folder, l.visit); err != nil {
		return err
	}
	if err := l.commit(); err != nil {
		return err
	}
	return strm.Send(&s4wave_space.LinkFolderProgress{Files: l.files, Bytes: l.bytes, Done: true})
}

// folderLink is one LinkFolder walk.
type folderLink struct {
	// ctx is the request context.
	ctx context.Context
	// ws is the Space World.
	ws world.WorldState
	// objKey is the Drive's object key.
	objKey string
	// fsType is the Drive's filesystem type.
	fsType unixfs_world.FSType
	// idx is the SegmentIndex the files are linked in.
	idx *block_store_filesegment.Index
	// folder is the absolute folder walked.
	folder string
	// dest is the path in the Drive the folder links to.
	dest []string
	// strm receives the progress.
	strm s4wave_space.SRPCSpaceResourceService_LinkFolderStream

	// batch is the open batch, or nil.
	batch *unixfs_world.BatchFSWriter
	// pending counts the files in batch.
	pending int
	// files counts the files committed.
	files uint64
	// bytes counts the bytes of the files committed.
	bytes uint64
	// batchBytes counts the bytes of the files in batch.
	batchBytes uint64
}

// writer returns the open batch, opening one when none is.
func (l *folderLink) writer() *unixfs_world.BatchFSWriter {
	if l.batch == nil {
		l.batch = unixfs_world.NewBatchFSWriter(l.ws, l.objKey, l.fsType, "")
	}
	return l.batch
}

// declareDest declares each directory of the destination path; Mkdir keeps
// one that exists.
func (l *folderLink) declareDest() error {
	for i, name := range l.dest {
		if err := l.writer().AddDir(l.ctx, l.dest[:i], name, 0o755, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

// visit adds one walked entry to the batch.
func (l *folderLink) visit(path string, d fs.DirEntry, err error) error {
	// Locate the entry in the Drive.
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(l.folder, path)
	if err != nil || rel == "." {
		return err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	parent := append(append([]string(nil), l.dest...), parts[:len(parts)-1]...)
	name := parts[len(parts)-1]

	// Declare a directory, and skip anything but a regular file.
	info, err := d.Info()
	if err != nil {
		return err
	}
	if d.IsDir() {
		return l.writer().AddDir(l.ctx, parent, name, info.Mode().Perm(), info.ModTime())
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	// Link the file; its index entries commit before the batch does.
	err = l.writer().AddFileBlob(l.ctx, parent, name, info.Mode().Perm(), info.ModTime(), func(xfrm block.Transformer, hashType hash.HashType) (*blob.Blob, error) {
		return block_store_filesegment.LinkFile(l.ctx, l.idx, path, nil, xfrm, hashType)
	})
	if err != nil {
		return errors.Wrapf(err, "link %s", path)
	}
	l.pending++
	l.batchBytes += uint64(info.Size()) //nolint:gosec // a file size is not negative.
	if l.pending < linkBatchFiles {
		return nil
	}
	return l.commit()
}

// commit commits the open batch and reports the progress.
func (l *folderLink) commit() error {
	// Commit the batch's entries in one World transaction.
	if l.batch == nil {
		return nil
	}
	err := l.batch.Commit(l.ctx)
	l.release()
	if err != nil {
		return err
	}

	// Report the files linked so far.
	l.files += uint64(l.pending) //nolint:gosec // a count is not negative.
	l.bytes += l.batchBytes
	l.pending, l.batchBytes = 0, 0
	return l.strm.Send(&s4wave_space.LinkFolderProgress{Files: l.files, Bytes: l.bytes})
}

// release drops the open batch.
func (l *folderLink) release() {
	if l.batch != nil {
		l.batch.Release()
		l.batch = nil
	}
}
