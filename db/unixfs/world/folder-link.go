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

// ChangingBackoff is how long syncs skip a file that changed during each of
// its reads before one reads it again. Each further such read doubles the
// wait, up to ChangingBackoffMax, and a read that links the file ends it. A
// file written without pause, such as a log, a database or a disk image, is
// then read a few times a minute at first and once per ChangingBackoffMax
// after that, instead of LinkAttempts times per sync.
const ChangingBackoff = 5 * time.Second

// ChangingBackoffMax bounds the wait of a changing file.
const ChangingBackoffMax = 5 * time.Minute

// changingFile is the backoff of a file that changed during each read.
type changingFile struct {
	// wait is the current wait.
	wait time.Duration
	// due is when a sync may read the file again.
	due time.Time
}

// FolderLink keeps a Drive equal to a folder on this Device: each regular
// file's blocks stay in the file and are read from it, and only the file's
// Blob and the directory tree go to the World. Symlinks and special files are
// left out, and a Drive entry the folder lacks is removed. The first sync
// records the file-system identity of the folder's root in the index, and
// every later sync refuses to run while the root's identity differs.
//
// A FolderLink runs one sync at a time.
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
	// changing holds the backoff of each file, by absolute path, that changed
	// during each read of its last attempt.
	changing map[string]changingFile
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
	return &FolderLink{eng: eng, idx: idx, objKey: objKey, folder: filepath.Clean(folder), dest: dest, changing: make(map[string]changingFile)}, nil
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

// Reindex links the files at the paths files, relative to the folder, again
// whatever their stat, as after the store found their bytes changed under an
// unchanged stat. A path that is no longer a regular file, or whose directory
// the Drive lacks, is synced through its directory instead.
func (l *FolderLink) Reindex(ctx context.Context, files []string, progress func(files, bytes uint64) error) error {
	// Start the sync and declare the destination.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.declareDest(); err != nil {
		return err
	}

	// Link each file, or sync its directory.
	for _, file := range files {
		if err := s.reindex(splitRel(file)); err != nil {
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

// Changing returns the number of files the syncs skip because they changed
// during each read, and the earliest time one is due to be read again: zero
// when none is.
func (l *FolderLink) Changing() (int, time.Time) {
	var due time.Time
	for _, c := range l.changing {
		if due.IsZero() || c.due.Before(due) {
			due = c.due
		}
	}
	return len(l.changing), due
}

// DueChanging returns the paths, relative to the folder, of the changing
// files due to be read again by now. Reindex reads them.
func (l *FolderLink) DueChanging(now time.Time) []string {
	var out []string
	for path, c := range l.changing {
		if !now.Before(c.due) {
			if rel, err := filepath.Rel(l.folder, path); err == nil {
				out = append(out, rel)
			}
		}
	}
	return out
}

// CheckRoot returns ErrFolderRootChanged while the folder's root is missing
// or on another file system than the one recorded when it was linked.
func (l *FolderLink) CheckRoot(ctx context.Context) error {
	_, err := l.checkRoot(ctx)
	return err
}

// checkRoot returns the file-system identity of the folder's root, recording
// it when the index holds none, or ErrFolderRootChanged when it differs from
// the recorded one.
func (l *FolderLink) checkRoot(ctx context.Context) (uint64, error) {
	// Read the root's identity and the recorded one.
	id, statErr := folderRootID(l.folder)
	rec, found, err := l.idx.FolderRoot(ctx, l.folder)
	if err != nil {
		return 0, err
	}

	// Refuse a root that left its file system; record the first one.
	switch {
	case found && errors.Is(statErr, fs.ErrNotExist):
		return 0, &FolderRootChangedError{Folder: l.folder, Recorded: rec, Missing: true}
	case statErr != nil:
		return 0, statErr
	case !found:
		return id, l.idx.SetFolderRoot(ctx, l.folder, id)
	case rec != id:
		return 0, &FolderRootChangedError{Folder: l.folder, Recorded: rec, Current: id}
	}
	return id, nil
}

// begin checks the folder's root, looks up the Drive's filesystem type and
// starts a sync.
func (l *FolderLink) begin(ctx context.Context, progress func(files, bytes uint64) error) (*folderSync, error) {
	// Require the root's file system.
	rootID, err := l.checkRoot(ctx)
	if err != nil {
		return nil, err
	}

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
	return &folderSync{FolderLink: l, ctx: ctx, ws: world.NewEngineWorldState(l.eng, true), fsType: fsType, rootID: rootID, progress: progress}, nil
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
	// rootID is the file-system identity of the folder's root.
	rootID uint64
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

// reindex links the file at the relative path rel whatever its stat, or
// syncs its directory when it is no longer a regular file or the Drive lacks
// its directory.
func (s *folderSync) reindex(rel []string) error {
	// Stat the file.
	if len(rel) == 0 {
		return nil
	}
	dirRel, name := rel[:len(rel)-1], rel[len(rel)-1]
	path := filepath.Join(append([]string{s.folder}, rel...)...)
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Link a regular file into the directory the Drive holds.
	if err == nil && info.Mode().IsRegular() {
		parent := append(slices.Clone(s.dest), dirRel...)
		var held bool
		err := s.view(func(root *unixfs_block.FSTree) error {
			dir, err := lookupDir(root, parent)
			held = dir != nil
			return err
		})
		if err != nil {
			return err
		}
		if held {
			return s.link(path, parent, name, info)
		}
	}

	// Sync the directory otherwise, ending the backoff of a file that is
	// gone.
	delete(s.changing, path)
	return s.syncDir(dirRel)
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

	// Remove the Drive entries the folder lacks, once the root is shown to
	// be on its file system still: an unmount during the sync empties it.
	if len(gone) != 0 {
		if id, err := folderRootID(s.folder); err != nil || id != s.rootID {
			return &FolderRootChangedError{Folder: s.folder, Recorded: s.rootID, Current: id, Missing: err != nil}
		}
	}
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
	// Skip a file whose backoff has not passed.
	backoff, backingOff := s.changing[path]
	if backingOff && time.Now().Before(backoff.due) {
		return nil
	}

	// Link the file; its index entries commit before the batch does.
	err := s.writer().AddFileBlob(s.ctx, parent, name, info.Mode().Perm(), info.ModTime(), func(xfrm block.Transformer, hashType hash.HashType) (*blob.Blob, error) {
		return block_store_filesegment.LinkFile(s.ctx, s.idx, path, nil, xfrm, hashType)
	})
	switch {
	case errors.Is(err, block_store_filesegment.ErrFileChanging):
		// A file still being written keeps its Drive entry and waits out a
		// doubled backoff.
		backoff.wait = min(max(2*backoff.wait, ChangingBackoff), ChangingBackoffMax)
		backoff.due = time.Now().Add(backoff.wait)
		s.changing[path] = backoff
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// A file removed since the listing keeps its Drive entry until the
		// change the watcher reports.
		delete(s.changing, path)
		return nil
	case err != nil:
		return errors.Wrapf(err, "link %s", path)
	}
	delete(s.changing, path)

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
