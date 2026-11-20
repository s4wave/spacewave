package unixfs_world

import (
	"context"
	"crypto/sha256"
	gohash "hash"
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

	// twoWay also applies the changes others make to the Drive to the
	// folder, as folder-link-apply.go describes.
	twoWay bool
	// seenDirs holds the node reference of each Drive directory, by path
	// relative to the folder joined by slashes, as of the last remote pass
	// that applied every change in it, so a pass skips each directory
	// unchanged since.
	seenDirs map[string]*block.BlockRef
	// unapplied counts the remote changes the last pass left for a later
	// one, as when it could not fetch a member's bytes.
	unapplied int
	// applyErr is the first error that left a remote change unapplied.
	applyErr error
	// recovered is set once a sync settled the staged files a crash left.
	recovered bool
	// applyHook runs at each step of applying a member's change, if set.
	applyHook func(step ApplyStep, path string)
}

// NewFolderLink constructs the link of the absolute folder into the Drive at
// objKey, at the path dest within it, with its files' ranges kept in idx. A
// two-way link also applies the changes others make to the Drive to the
// folder; a one-way link replaces them with the folder's files.
func NewFolderLink(eng world.Engine, idx *block_store_filesegment.Index, objKey, folder string, dest []string, twoWay bool) (*FolderLink, error) {
	if !filepath.IsAbs(folder) {
		return nil, errors.Errorf("folder must be an absolute path: %q", folder)
	}
	if objKey == "" {
		return nil, world.ErrEmptyObjectKey
	}
	if twoWay && !twoWaySupported {
		return nil, errors.New("two-way linked folders are not supported on this platform")
	}
	return &FolderLink{
		eng:      eng,
		idx:      idx,
		objKey:   objKey,
		folder:   filepath.Clean(folder),
		dest:     dest,
		changing: make(map[string]changingFile),
		twoWay:   twoWay,
		seenDirs: make(map[string]*block.BlockRef),
	}, nil
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
//
// A two-way sync first applies the changes others made to the Drive, and
// leaves each entry that holds a change it could not apply yet. A batch whose
// entries another writer changed after the comparison returns a
// *FileConflictError with nothing of it committed; syncing again applies that
// change first.
func (l *FolderLink) Sync(ctx context.Context, progress func(files, bytes uint64) error) error {
	// Apply the Drive's changes, declare the destination, then walk the whole
	// folder.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.applyRemote(); err != nil {
		return err
	}
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
	// Start the sync, apply the Drive's changes and declare the destination.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.applyRemote(); err != nil {
		return err
	}
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
	// Start the sync, apply the Drive's changes and declare the destination.
	s, err := l.begin(ctx, progress)
	if err != nil {
		return err
	}
	defer s.release()
	if err := s.applyRemote(); err != nil {
		return err
	}
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

	// wants holds, in a two-way sync, the version of each file the open batch
	// links, without its node reference, which the commit gives it.
	wants []wantedVersion
	// dropped holds, in a two-way sync, the folder-relative paths whose
	// versions the open batch removes from the Drive.
	dropped [][]string
}

// wantedVersion is the version a two-way batch gives one file.
type wantedVersion struct {
	// rel is the file's path relative to the folder.
	rel []string
	// version is its stat, without the node reference.
	version *block_store_filesegment.FolderVersion
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
	// checked makes a two-way link require the Drive to still hold prev at
	// the path, the file it compared, or nothing when prev is nil.
	checked bool
	// prev is the Drive's file a checked link replaces.
	prev *block.BlockRef
	// ownedDir makes a two-way sync replace the Drive's directory at the
	// path only when every file under it is a version the folder published.
	ownedDir bool
}

// goneEntry is a Drive entry the folder lacks.
type goneEntry struct {
	// name is the entry's name.
	name string
	// file is, in a two-way sync, the version of a file the removal
	// requires the Drive to still hold.
	file *block.BlockRef
	// dir marks a directory, which a two-way sync removes only when every
	// file under it is a version the folder published.
	dir bool
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
		// Read the file's base in a two-way link.
		parent := append(slices.Clone(s.dest), dirRel...)
		var known map[string]*block_store_filesegment.FolderVersion
		if s.twoWay {
			known, _, err = s.idx.FolderVersions(s.ctx, s.folder, dirRel)
			if err != nil {
				return err
			}
		}

		// Link it when the Drive holds its directory, and no remote change
		// waits at its path.
		a := entryAction{info: info, link: true}
		var held, ok bool
		err := s.view(func(root *unixfs_block.FSTree) error {
			// Find the directory, then the entry the link would replace.
			dir, err := lookupDir(root, parent)
			if err != nil || dir == nil {
				return err
			}
			held, ok = true, true
			if known != nil {
				dirent, err := dir.Lookup(name)
				if err != nil {
					return err
				}
				ok = checkChange(&a, dirent, known[name]) && !a.ownedDir
			}
			return nil
		})
		if err != nil {
			return err
		}
		if held {
			if !ok {
				return nil
			}
			return s.link(path, parent, name, a)
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
		// Never publish a fetched or staged file; the first remote pass
		// settles one a crash left.
		if isApplyFile(d.Name()) {
			continue
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}

	// Read the bases of a two-way link, then compare the entries with the
	// Drive's as of a snapshot.
	var known map[string]*block_store_filesegment.FolderVersion
	if s.twoWay {
		known, _, err = s.idx.FolderVersions(s.ctx, s.folder, parent[len(s.dest):])
		if err != nil {
			return err
		}
	}
	var acts []entryAction
	var gone []goneEntry
	err = s.view(func(root *unixfs_block.FSTree) error {
		dir, err := lookupDir(root, parent)
		if err != nil {
			return err
		}
		acts, gone, err = planDir(dir, infos, known)
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
	for _, g := range gone {
		if err := s.remove(parent, g); err != nil {
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
		if a.ownedDir {
			owned, err := s.ownTree(parent, name)
			if err != nil {
				return err
			}
			if !owned {
				continue
			}
		}
		switch {
		case a.link:
			if err := s.link(childPath, parent, name, a); err != nil {
				return err
			}
		case a.declare:
			if err := s.expect(parent, name, a); err != nil {
				return err
			}
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
//
// known holds the bases of the directory's files in a two-way link, and is nil
// in a one-way link. A two-way link leaves alone each entry whose Drive file
// differs from its base, a remote change the remote pass has not applied yet,
// and makes each of its changes require the Drive to still hold what it
// compared, so a remote change committed since is never replaced unseen.
func planDir(dir *unixfs_block.FSTree, infos []fs.FileInfo, known map[string]*block_store_filesegment.FolderVersion) ([]entryAction, []goneEntry, error) {
	// Walk both name-ordered listings together.
	var ents []*unixfs_block.Dirent
	if dir != nil {
		ents = dir.GetFSNode().GetDirectoryEntry()
	}
	var acts []entryAction
	var gone []goneEntry
	addGone := func(d *unixfs_block.Dirent) {
		if g, ok := planGone(d, known); ok {
			gone = append(gone, g)
		}
	}
	next := 0
	for _, info := range infos {
		// Drop the Drive entries before this one, then find its own.
		name := info.Name()
		for next < len(ents) && ents[next].GetName() < name {
			addGone(ents[next])
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
			a := entryAction{info: info, declare: true, replace: dirent != nil}
			if known != nil && !checkChange(&a, dirent, known[name]) {
				continue
			}
			acts = append(acts, a)
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
			a := entryAction{info: info, link: true}
			if known != nil && !checkChange(&a, dirent, known[name]) {
				continue
			}
			acts = append(acts, a)
		case dirent != nil:
			addGone(dirent)
		}
	}

	// Drop the Drive entries after the last one.
	for _, d := range ents[next:] {
		addGone(d)
	}
	return acts, gone, nil
}

// checkChange sets the checks a two-way change of the Drive entry dirent,
// nil when the Drive lacks one, needs on a, given the base of its file. It
// reports false when the Drive holds a remote change not applied yet, which
// the change would replace: a file other than its base, or no file where the
// base names one.
func checkChange(a *entryAction, dirent *unixfs_block.Dirent, base *block_store_filesegment.FolderVersion) bool {
	switch {
	case dirent == nil:
		a.checked = true
		return base == nil
	case dirent.GetIsFile():
		a.checked, a.prev = true, dirent.GetNodeRef()
		return base != nil && a.prev.EqualsRef(base.GetNodeRef())
	case dirent.GetIsDirectory():
		a.ownedDir = true
	}
	return true
}

// planGone returns the removal of the Drive entry d the folder lacks, and
// false when a two-way link, with the bases known, must keep it: a file other
// than its base, a remote change not applied yet.
func planGone(d *unixfs_block.Dirent, known map[string]*block_store_filesegment.FolderVersion) (goneEntry, bool) {
	g := goneEntry{name: d.GetName()}
	switch {
	case known == nil:
	case d.GetIsDirectory():
		g.dir = true
	case d.GetIsFile():
		base := known[g.name]
		if base == nil || !d.GetNodeRef().EqualsRef(base.GetNodeRef()) {
			return g, false
		}
		g.file = d.GetNodeRef()
	}
	return g, true
}

// remove removes the Drive entry g under parent. A two-way sync removes a
// file only while the Drive holds its base, and a directory only when every
// file under it is a base, dropping their bases with the commit.
func (s *folderSync) remove(parent []string, g goneEntry) error {
	switch {
	case !s.twoWay:
	case g.dir:
		owned, err := s.ownTree(parent, g.name)
		if err != nil || !owned {
			return err
		}
	case g.file != nil:
		if err := s.writer().ExpectFile(parent, g.name, g.file); err != nil {
			return err
		}
		s.dropped = append(s.dropped, append(slices.Clone(parent[len(s.dest):]), g.name))
	}
	return s.writer().Remove(parent, g.name)
}

// ownTree reports whether every file under the Drive directory parent/name is
// the base of its path, so a two-way sync may replace the directory, and then
// queues the drop of those bases with the open batch. A symlink or other entry
// the folder never publishes keeps the directory.
func (s *folderSync) ownTree(parent []string, name string) (bool, error) {
	// Read the bases under the directory.
	rel := append(slices.Clone(parent[len(s.dest):]), name)
	bases := make(map[string]*block.BlockRef)
	err := s.idx.FolderVersionsUnder(s.ctx, s.folder, rel, func(fileRel []string, v *block_store_filesegment.FolderVersion) error {
		bases[strings.Join(fileRel, "/")] = v.GetNodeRef()
		return nil
	})
	if err != nil {
		return false, err
	}

	// Compare each Drive file under it with its base.
	var files [][]string
	owned := true
	err = s.view(func(root *unixfs_block.FSTree) error {
		// Find the directory, then walk it depth first.
		dir, err := lookupDir(root, append(slices.Clone(parent), name))
		if err != nil || dir == nil {
			return err
		}
		var walk func(dir *unixfs_block.FSTree, rel []string) error
		walk = func(dir *unixfs_block.FSTree, rel []string) error {
			for i, d := range dir.GetFSNode().GetDirectoryEntry() {
				childRel := append(slices.Clone(rel), d.GetName())
				switch {
				case d.GetIsDirectory():
					child, _, err := dir.FollowDirent(i)
					if err != nil {
						return err
					}
					if err := walk(child, childRel); err != nil {
						return err
					}
				case d.GetIsFile() && d.GetNodeRef().EqualsRef(bases[strings.Join(childRel, "/")]):
					files = append(files, childRel)
				default:
					owned = false
				}
				if !owned {
					return nil
				}
			}
			return nil
		}
		return walk(dir, rel)
	})
	if err != nil || !owned {
		return false, err
	}
	s.dropped = append(s.dropped, files...)
	return true, nil
}

// expect records the check a two-way change of parent/name needs in the open
// batch.
func (s *folderSync) expect(parent []string, name string, a entryAction) error {
	switch {
	case !a.checked:
		return nil
	case a.prev != nil:
		s.dropped = append(s.dropped, append(slices.Clone(parent[len(s.dest):]), name))
		return s.writer().ExpectFile(parent, name, a.prev)
	default:
		return s.writer().ExpectMissing(parent, name)
	}
}

// fileMatches reports whether the Drive's file node has the size,
// modification time and permissions of info.
func fileMatches(node *unixfs_block.FSNode, info fs.FileInfo) bool {
	return node.GetFile().GetTotalSize() == uint64(info.Size()) && //nolint:gosec // a file size is not negative.
		node.GetModTime().AsTime().Equal(info.ModTime()) &&
		fs.FileMode(node.GetPermissions()) == info.Mode().Perm()
}

// link adds the file at path to the batch as name under parent, with the
// checks a, from the walk's stat, asks for, committing the batch once it holds
// FolderLinkBatchFiles files. A two-way sync records the file's version as
// its base once the batch commits.
func (s *folderSync) link(path string, parent []string, name string, a entryAction) error {
	// Skip a file whose backoff has not passed.
	info := a.info
	backoff, backingOff := s.changing[path]
	if backingOff && time.Now().Before(backoff.due) {
		return nil
	}

	// Link the file, hashing its bytes in a two-way sync; its index entries
	// commit before the batch does.
	var sum gohash.Hash
	if s.twoWay {
		sum = sha256.New()
	}
	err := s.writer().AddFileBlob(s.ctx, parent, name, info.Mode().Perm(), info.ModTime(), func(xfrm block.Transformer, hashType hash.HashType) (*blob.Blob, error) {
		return block_store_filesegment.LinkFile(s.ctx, s.idx, path, nil, xfrm, hashType, sum)
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

	// Check what the link replaces, and ask for its version.
	if s.twoWay {
		if a.prev != nil {
			// The new version replaces the base; its own lands at commit.
			if err := s.writer().ExpectFile(parent, name, a.prev); err != nil {
				return err
			}
		} else if a.checked {
			if err := s.writer().ExpectMissing(parent, name); err != nil {
				return err
			}
		}
		if err := s.writer().WantVersion(parent, name); err != nil {
			return err
		}
		rel := append(slices.Clone(parent[len(s.dest):]), name)
		version := statVersion(nil, info)
		version.Digest = sum.Sum(nil)
		s.wants = append(s.wants, wantedVersion{rel: rel, version: version})
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
	if err == nil {
		err = s.recordVersions()
	}
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

// recordVersions records, in a two-way sync, the versions the committed
// batch gave its linked files as their bases, and drops the bases of the
// files it removed, in one index write.
func (s *folderSync) recordVersions() error {
	// Read the versions the commit gave the linked files.
	if len(s.wants) == 0 && len(s.dropped) == 0 {
		return nil
	}
	vs, err := s.batch.Versions(s.ctx)
	if err != nil {
		return err
	}

	// Record them, and drop the removed files' bases.
	set := make(map[string]*block_store_filesegment.FolderVersion, len(vs))
	for i, w := range s.wants {
		w.version.NodeRef = vs[i].Version
		set[strings.Join(w.rel, "/")] = w.version
	}
	return s.idx.SetFolderVersions(s.ctx, s.folder, set, s.dropped)
}

// release drops the open batch and the versions it would have recorded.
func (s *folderSync) release() {
	if s.batch != nil {
		s.batch.Release()
		s.batch = nil
	}
	s.wants, s.dropped = nil, nil
}
