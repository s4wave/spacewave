package unixfs_world

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
)

// FolderLinkBatchFiles is the number of files one World transaction of a
// FolderLink sync adds. Each costs the batch a pending entry and its Blob
// block. The batch bounds how long the commit holds the World's writer: 512
// small files commit in about 80 ms, where 4096 take about 230 ms.
const FolderLinkBatchFiles = 512

// FolderLink keeps a Drive equal to a folder on this Device: each regular
// file's blocks stay in the file and are read from it, and only the file's
// Blob and the directory tree go to the World. Symlinks and special files are
// skipped.
type FolderLink struct {
	// eng is the World engine holding the Drive.
	eng world.Engine
	// idx is the SegmentIndex the files are linked in.
	idx *block_store_filesegment.Index
	// objKey is the Drive's object key.
	objKey string
	// folder is the absolute folder linked.
	folder string
	// dest is the path in the Drive the folder links to.
	dest []string
}

// NewFolderLink constructs the link of the absolute folder into the Drive at
// objKey, at the path dest within it, with its files' ranges kept in idx.
func NewFolderLink(eng world.Engine, idx *block_store_filesegment.Index, objKey, folder string, dest []string) (*FolderLink, error) {
	if !filepath.IsAbs(folder) {
		return nil, errors.Errorf("folder must be an absolute path: %q", folder)
	}
	if objKey == "" {
		return nil, world.ErrEmptyObjectKey
	}
	return &FolderLink{eng: eng, idx: idx, objKey: objKey, folder: folder, dest: dest}, nil
}

// Sync links each directory and regular file of the folder that the Drive
// lacks, and each file whose size, modification time or permissions differ
// from the Drive's entry. It reads only the files it links: an unchanged
// folder costs one lstat and one Drive node read per entry, and commits
// nothing. It commits every FolderLinkBatchFiles files and calls progress, if
// set, after each commit with the files linked so far and their bytes.
//
// The Drive is compared as of one read snapshot taken at the start, which the
// sync's own commits only extend. A file records the stat the walk took
// before LinkFile read it, so a file changed after that stat differs from its
// entry at the next sync and is linked again.
func (l *FolderLink) Sync(ctx context.Context, progress func(files, bytes uint64) error) error {
	// Read the World as of one snapshot.
	rtx, err := l.eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer rtx.Discard()

	// Find the Drive in the snapshot.
	fsType, _, err := LookupFsType(ctx, rtx, l.objKey)
	if err != nil {
		return err
	}
	obj, found, err := rtx.GetObject(ctx, l.objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Walk the folder against the Drive, then commit the last batch.
	s := &folderSync{FolderLink: l, ctx: ctx, ws: world.NewEngineWorldState(l.eng, true), fsType: fsType, progress: progress}
	defer s.release()
	_, _, err = AccessUnixfsObject(ctx, obj, false, fsType, func(root *unixfs_block.FSTree) error {
		dir, err := s.declareDest(root)
		if err != nil {
			return err
		}
		return s.dir(l.folder, l.dest, dir)
	}, nil)
	if err != nil {
		return err
	}
	return s.commit()
}

// folderSync is one FolderLink sync.
type folderSync struct {
	*FolderLink
	// ctx is the sync's context.
	ctx context.Context
	// ws writes the batches.
	ws world.WorldState
	// fsType is the Drive's filesystem type.
	fsType FSType
	// progress receives the totals after each commit, if set.
	progress func(files, bytes uint64) error

	// batch is the open batch, or nil.
	batch *BatchFSWriter
	// pending counts the files in batch.
	pending int
	// batchBytes counts the bytes of the files in batch.
	batchBytes uint64
	// files counts the files committed.
	files uint64
	// bytes counts the bytes of the files committed.
	bytes uint64
}

// writer returns the open batch, opening one when none is.
func (s *folderSync) writer() *BatchFSWriter {
	if s.batch == nil {
		s.batch = NewBatchFSWriter(s.ws, s.objKey, s.fsType, "")
	}
	return s.batch
}

// declareDest declares each directory of the destination path the Drive
// lacks, and returns the Drive's directory at dest, or nil when it was
// declared.
func (s *folderSync) declareDest(root *unixfs_block.FSTree) (*unixfs_block.FSTree, error) {
	dir := root
	for i, name := range s.dest {
		if dir != nil {
			child, dirent, err := dir.LookupFollowDirent(name)
			if err != nil {
				return nil, err
			}
			if dirent.GetIsDirectory() {
				dir = child
				continue
			}
			dir = nil
		}
		if err := s.writer().AddDir(s.ctx, s.dest[:i], name, 0o755, time.Now()); err != nil {
			return nil, err
		}
	}
	return dir, nil
}

// dir links the entries of the folder directory at path, which the Drive
// holds at parent as dir, or lacks when dir is nil.
func (s *folderSync) dir(path string, parent []string, dir *unixfs_block.FSTree) error {
	// List the directory in name order.
	ents, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, d := range ents {
		// Stat the entry; one removed since the listing is skipped.
		name := d.Name()
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}

		// Find the Drive's entry of the same name.
		var child *unixfs_block.FSTree
		var dirent *unixfs_block.Dirent
		if dir != nil {
			child, dirent, err = dir.LookupFollowDirent(name)
			if err != nil {
				return err
			}
		}

		// Declare a directory the Drive lacks, then walk it.
		childPath := filepath.Join(path, name)
		switch {
		case d.IsDir():
			if !dirent.GetIsDirectory() {
				child = nil
				if err := s.writer().AddDir(s.ctx, parent, name, info.Mode().Perm(), info.ModTime()); err != nil {
					return err
				}
			}
			if err := s.dir(childPath, append(parent[:len(parent):len(parent)], name), child); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if dirent.GetIsFile() && fileMatches(child.GetFSNode(), info) {
				continue
			}
			if err := s.link(childPath, parent, name, info); err != nil {
				return err
			}
		}
	}
	return nil
}

// fileMatches reports whether the Drive's file node has the size,
// modification time and permissions of info.
func fileMatches(node *unixfs_block.FSNode, info fs.FileInfo) bool {
	return node.GetFile().GetTotalSize() == uint64(info.Size()) && //nolint:gosec // a file size is not negative.
		node.GetModTime().AsTime().Equal(info.ModTime()) &&
		fs.FileMode(node.GetPermissions()) == info.Mode().Perm()
}

// link adds the file at path to the batch as name under parent, committing
// the batch once it holds FolderLinkBatchFiles files.
func (s *folderSync) link(path string, parent []string, name string, info fs.FileInfo) error {
	// Link the file; its index entries commit before the batch does.
	err := s.writer().AddFileBlob(s.ctx, parent, name, info.Mode().Perm(), info.ModTime(), func(xfrm block.Transformer, hashType hash.HashType) (*blob.Blob, error) {
		return block_store_filesegment.LinkFile(s.ctx, s.idx, path, nil, xfrm, hashType)
	})
	if err != nil {
		return errors.Wrapf(err, "link %s", path)
	}

	// Commit a full batch.
	s.pending++
	s.batchBytes += uint64(info.Size()) //nolint:gosec // a file size is not negative.
	if s.pending < FolderLinkBatchFiles {
		return nil
	}
	return s.commit()
}

// commit commits the open batch in one World transaction and reports the
// totals.
func (s *folderSync) commit() error {
	// Commit the batch, if any.
	if s.batch == nil {
		return nil
	}
	err := s.batch.Commit(s.ctx)
	s.release()
	if err != nil {
		return err
	}

	// Report the files linked so far.
	s.files += uint64(s.pending) //nolint:gosec // a count is not negative.
	s.bytes += s.batchBytes
	s.pending, s.batchBytes = 0, 0
	if s.progress == nil {
		return nil
	}
	return s.progress(s.files, s.bytes)
}

// release drops the open batch.
func (s *folderSync) release() {
	if s.batch != nil {
		s.batch.Release()
		s.batch = nil
	}
}
