package unixfs_world

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
// block. The merge builds off the World writer and holds it under a
// millisecond to publish; the batch bounds the build a commit repeats, under
// the writer, after it lost the race to other writers too often.
const FolderLinkBatchFiles = 512

// FolderLink keeps a Drive equal to a folder on this Device: each regular
// file's blocks stay in the file and are read from it, and only the file's
// Blob and the directory tree go to the World. Symlinks and special files are
// left out, and a Drive entry the folder lacks is removed.
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
	return &FolderLink{eng: eng, idx: idx, objKey: objKey, folder: filepath.Clean(folder), dest: dest}, nil
}

// Sync makes the Drive equal to the whole folder. It links each directory and
// regular file the Drive lacks and each file whose size, modification time or
// permissions differ from the Drive's entry, and removes each Drive entry the
// folder lacks. It reads only the files it links: an unchanged folder costs
// one lstat and one Drive node read per entry, and commits nothing. It
// commits every FolderLinkBatchFiles files and calls progress, if set, after
// each commit with the files linked so far and their bytes.
//
// Each directory is compared against a read snapshot of the Drive opened for
// that comparison alone, so no snapshot outlives a directory listing and the
// World keeps collecting while a long sync runs. A file records the stat the
// walk took before LinkFile read it, so a file changed after that stat
// differs from its entry at the next sync and is linked again.
func (l *FolderLink) Sync(ctx context.Context, progress func(files, bytes uint64) error) error {
	// Declare the destination, then walk the whole folder.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.declareDest(); err != nil {
		return err
	}
	if err := s.dir(l.folder, l.dest, true); err != nil {
		return err
	}

	// Commit the last batch.
	return s.commit()
}

// SyncDirs makes the Drive equal to the folder in the directories at dirs,
// paths relative to the folder, as Sync does, without walking their existing
// subdirectories: a watcher names each directory whose entries changed. A
// subdirectory the Drive lacks is walked whole. A directory the Drive lacks
// is synced from its nearest ancestor the Drive holds, and one the folder
// lacks is left to its parent's sync.
func (l *FolderLink) SyncDirs(ctx context.Context, dirs []string, progress func(files, bytes uint64) error) error {
	// Start the sync and declare the destination.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.declareDest(); err != nil {
		return err
	}

	// Sync parents before their children, so a walk of a new directory
	// covers its descendants.
	rels := make([][]string, 0, len(dirs))
	for _, d := range dirs {
		rels = append(rels, splitRel(d))
	}
	slices.SortFunc(rels, func(a, b []string) int { return slices.Compare(a, b) })
	rels = slices.CompactFunc(rels, slices.Equal)
	for _, rel := range rels {
		if err := s.syncDir(rel); err != nil {
			return err
		}
	}

	// Commit the last batch.
	return s.commit()
}

// splitRel splits a slash or separator path relative to the folder into its
// components, dropping empty and "." components.
func splitRel(rel string) []string {
	var out []string
	for part := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if part != "" && part != "." {
			out = append(out, part)
		}
	}
	return out
}

// begin looks up the Drive's filesystem type and starts a sync.
func (l *FolderLink) begin(ctx context.Context, progress func(files, bytes uint64) error) (*folderSync, error) {
	// Read the type from a snapshot held only for the lookup.
	rtx, err := l.eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer rtx.Discard()
	fsType, _, err := LookupFsType(ctx, rtx, l.objKey)
	if err != nil {
		return nil, err
	}
	return &folderSync{FolderLink: l, ctx: ctx, ws: world.NewEngineWorldState(l.eng, true), fsType: fsType, progress: progress}, nil
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
	// walked holds the folder-relative paths, joined by "/", of the
	// directories this sync walked whole.
	walked map[string]struct{}

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

// entryAction is what a sync does with one entry of a folder directory.
type entryAction struct {
	// info is the entry's lstat.
	info fs.FileInfo
	// link links the entry, a regular file the Drive lacks or holds changed.
	link bool
	// declare declares the entry, a directory the Drive lacks as one, and
	// walks it whole; replace removes the Drive's other entry first.
	declare, replace bool
	// walk walks the entry, a directory the Drive holds.
	walk bool
}

// writer returns the open batch, opening one when none is.
func (s *folderSync) writer() *BatchFSWriter {
	if s.batch == nil {
		s.batch = NewBatchFSWriter(s.ws, s.objKey, s.fsType, "")
	}
	return s.batch
}

// view calls cb with the Drive's root as of a read snapshot held only for cb.
func (s *folderSync) view(cb func(root *unixfs_block.FSTree) error) error {
	// Open the snapshot and find the Drive in it.
	rtx, err := s.eng.NewTransaction(s.ctx, false)
	if err != nil {
		return err
	}
	defer rtx.Discard()
	obj, found, err := rtx.GetObject(s.ctx, s.objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Read its root.
	_, _, err = AccessUnixfsObject(s.ctx, obj, false, s.fsType, cb, nil)
	return err
}

// lookupDir returns the Drive's directory at path under root, or nil when the
// Drive lacks a directory there.
func lookupDir(root *unixfs_block.FSTree, path []string) (*unixfs_block.FSTree, error) {
	dir := root
	for _, name := range path {
		child, dirent, err := dir.LookupFollowDirent(name)
		if err != nil || !dirent.GetIsDirectory() {
			return nil, err
		}
		dir = child
	}
	return dir, nil
}

// declareDest declares each directory of the destination path the Drive
// lacks, replacing an entry of another type.
func (s *folderSync) declareDest() error {
	// Find the first component the Drive lacks as a directory.
	missing, replace := len(s.dest), false
	err := s.view(func(root *unixfs_block.FSTree) error {
		dir := root
		for i, name := range s.dest {
			child, dirent, err := dir.LookupFollowDirent(name)
			if err != nil {
				return err
			}
			if !dirent.GetIsDirectory() {
				missing, replace = i, dirent != nil
				return nil
			}
			dir = child
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Declare it and the components below it.
	for i := missing; i < len(s.dest); i++ {
		if i == missing && replace {
			if err := s.writer().Remove(s.dest[:i], s.dest[i]); err != nil {
				return err
			}
		}
		if err := s.writer().AddDir(s.ctx, s.dest[:i], s.dest[i], 0o755, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

// syncDir syncs the folder directory at the relative path rel without
// walking its existing subdirectories, from the nearest ancestor the Drive
// holds. A directory this sync already walked whole, or one under it, is
// skipped.
func (s *folderSync) syncDir(rel []string) error {
	// Skip a directory a walk of an ancestor covered.
	for i := len(rel); i >= 0; i-- {
		if _, ok := s.walked[strings.Join(rel[:i], "/")]; ok {
			return nil
		}
	}

	// Find the nearest ancestor the Drive holds.
	err := s.view(func(root *unixfs_block.FSTree) error {
		for len(rel) != 0 {
			dir, err := lookupDir(root, append(slices.Clone(s.dest), rel...))
			if err != nil || dir != nil {
				return err
			}
			rel = rel[:len(rel)-1]
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Sync it.
	path := filepath.Join(append([]string{s.folder}, rel...)...)
	return s.dir(path, append(slices.Clone(s.dest), rel...), false)
}

// dir syncs the entries of the folder directory at path into the Drive's
// directory at parent, walking the subdirectories the Drive holds when all is
// set. A subdirectory the Drive lacks is always walked whole. A directory
// removed from the folder since its parent listed it is skipped.
func (s *folderSync) dir(path string, parent []string, all bool) error {
	// List the directory in name order and stat each entry; one removed
	// since the listing is left out.
	ents, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) && path != s.folder {
		return nil
	}
	if err != nil {
		return err
	}
	infos := make([]fs.FileInfo, 0, len(ents))
	for _, d := range ents {
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}

	// Compare the entries with the Drive's as of a snapshot.
	var acts []entryAction
	var gone []string
	err = s.view(func(root *unixfs_block.FSTree) error {
		dir, err := lookupDir(root, parent)
		if err != nil {
			return err
		}
		acts, gone, err = planDir(dir, infos)
		return err
	})
	if err != nil {
		return err
	}

	// Remove the Drive entries the folder lacks.
	for _, name := range gone {
		if err := s.writer().Remove(parent, name); err != nil {
			return err
		}
	}

	// Link the changed files and declare and walk the directories.
	if len(parent) >= len(s.dest) && (all || len(acts) != 0) {
		s.markWalked(parent, all)
	}
	for _, a := range acts {
		name := a.info.Name()
		childPath := filepath.Join(path, name)
		childParent := append(parent[:len(parent):len(parent)], name)
		switch {
		case a.link:
			if err := s.link(childPath, parent, name, a.info); err != nil {
				return err
			}
		case a.declare:
			if a.replace {
				if err := s.writer().Remove(parent, name); err != nil {
					return err
				}
			}
			if err := s.writer().AddDir(s.ctx, parent, name, a.info.Mode().Perm(), a.info.ModTime()); err != nil {
				return err
			}
			s.markWalked(childParent, true)
			if err := s.dir(childPath, childParent, true); err != nil {
				return err
			}
		case a.walk && all:
			if err := s.dir(childPath, childParent, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// markWalked records the Drive directory at parent as walked whole when all
// is set.
func (s *folderSync) markWalked(parent []string, all bool) {
	if !all {
		return
	}
	if s.walked == nil {
		s.walked = make(map[string]struct{})
	}
	s.walked[strings.Join(parent[len(s.dest):], "/")] = struct{}{}
}

// planDir compares the folder entries infos, sorted by name, with the Drive's
// directory dir, nil when the Drive lacks it. It returns the action for each
// directory and regular file, and the Drive entries the folder lacks as a
// directory or regular file.
func planDir(dir *unixfs_block.FSTree, infos []fs.FileInfo) ([]entryAction, []string, error) {
	// Walk both name-ordered listings together.
	var ents []*unixfs_block.Dirent
	if dir != nil {
		ents = dir.GetFSNode().GetDirectoryEntry()
	}
	var acts []entryAction
	var gone []string
	next := 0
	for _, info := range infos {
		// Drop the Drive entries before this one, then find its own.
		name := info.Name()
		for next < len(ents) && ents[next].GetName() < name {
			gone = append(gone, ents[next].GetName())
			next++
		}
		var dirent *unixfs_block.Dirent
		didx := next
		if next < len(ents) && ents[next].GetName() == name {
			dirent = ents[next]
			next++
		}

		// Decide what to do with the entry.
		switch {
		case info.IsDir():
			if dirent.GetIsDirectory() {
				acts = append(acts, entryAction{info: info, walk: true})
				continue
			}
			acts = append(acts, entryAction{info: info, declare: true, replace: dirent != nil})
		case info.Mode().IsRegular():
			if dirent.GetIsFile() {
				child, _, err := dir.FollowDirent(didx)
				if err != nil {
					return nil, nil, err
				}
				if fileMatches(child.GetFSNode(), info) {
					continue
				}
			}
			acts = append(acts, entryAction{info: info, link: true})
		case dirent != nil:
			gone = append(gone, name)
		}
	}

	// Drop the Drive entries after the last one.
	for _, d := range ents[next:] {
		gone = append(gone, d.GetName())
	}
	return acts, gone, nil
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
