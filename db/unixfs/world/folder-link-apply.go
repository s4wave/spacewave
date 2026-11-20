package unixfs_world

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
)

// A two-way FolderLink applies the changes others make to the Drive to the
// folder, as the LinkedFolderApply spec beside this file states. The folder's
// node keeps one FolderVersion per file in the SegmentIndex: the base, the
// Drive version it last published from the folder or applied to it, with the
// stat of the file on disk holding it. A Drive file whose node differs from
// its base is a remote change; a folder file whose stat differs from its base
// is a local change.
//
// A remote write fetches the member's bytes to a staged file beside the path,
// then checks the disk. A path holding the base is atomically exchanged with
// the staged file, and the displaced file is checked again: the base is
// removed, anything else, an edit landing between the check and the exchange,
// keeps a conflict name. An empty path takes the staged file by a rename that
// never replaces, or the conflict name when a file appeared. A path holding
// another version keeps it, and the member's version takes the conflict name.
// A remote delete of a file still holding its base moves it to the OS trash and
// checks it there: an edit landing between the check and the move goes back
// to the path by a rename that never replaces, or to the conflict name. A file
// edited before the check is kept, and the next sync publishes it again.
//
// The invariants, as the spec names them:
//
//   - NoLostVersion: every file version that reached the disk stays on the disk
//     or in the trash; an apply never unlinks a file, it only renames, and it
//     removes only a displaced file whose stat is the base's.
//   - MemberWriteKept: a member's version, once fetched, ends at the path or
//     under its conflict name, so a member's write is never dropped.
//   - RemoteReplacesOnlyBase: a member's write or delete replaces only a file
//     whose stat is its base; the check after the exchange or the move restores
//     any other.
//   - TrashOnlyDeleted: the trash holds only base versions a member deleted.
//
// The producers of a remote change, each a commit to the Drive this node did
// not make: a member's mounted Drive (UploadTree), the CLI's fs write, rm, mv
// and mkdir through the unixfs resource, fs sync and restore-s3, another
// linked folder or Device bound to the Drive, and account replication bringing
// any of those from another Device. The node's own commits record their
// versions as the base, so it never applies its own change. The one-shot fs
// link writes from this Device's folder, so its files match the disk and are
// adopted as the base without a write.
//
// A remote pass walks only the Drive directories whose node changed since the
// last pass that applied every change in them: one Drive read when nothing
// changed. A changed directory costs one node read per entry and one index
// scan, plus a fetch, a stat and two renames per changed file.

// stagePrefix prefixes the staged file a remote write fetches into, beside
// its path.
const stagePrefix = ".spacewave-stage-"

// conflictTagLen is the number of hex digits of a conflict name's digest.
const conflictTagLen = 16

// maxNameLen is the longest file name, in bytes, the platforms allow.
const maxNameLen = 255

// errRemoteMoved marks a remote file that changed after the pass listed it;
// the next pass applies its new version.
var errRemoteMoved = errors.New("remote file changed during the apply")

// remoteFile is one file of a Drive directory.
type remoteFile struct {
	// name is the file's name.
	name string
	// ref is the reference of the file's node.
	ref *block.BlockRef
	// node is the file's node.
	node *unixfs_block.FSNode
}

// Unapplied returns the number of remote changes the last pass left for a
// later one, and the first error that left one. A later sync tries them again.
func (l *FolderLink) Unapplied() (int, error) {
	return l.unapplied, l.applyErr
}

// ApplyRemote applies the changes others made to the Drive to the folder, in
// a two-way link. It does nothing in a one-way link.
func (l *FolderLink) ApplyRemote(ctx context.Context) error {
	s, err := l.begin(ctx, nil)
	if err != nil {
		return err
	}
	defer s.release()
	return s.applyRemote()
}

// applyRemote applies the Drive's changes to the folder in a two-way sync.
func (s *folderSync) applyRemote() error {
	if !s.twoWay {
		return nil
	}
	s.unapplied, s.applyErr = 0, nil
	_, err := s.applyDir(nil)
	return err
}

// leave records a remote change the pass leaves for a later one.
func (s *folderSync) leave(err error) {
	s.unapplied++
	if s.applyErr == nil {
		s.applyErr = err
	}
}

// applyDir applies the Drive's changes in the directory at rel, relative to
// the folder, and its subdirectories. It reports whether it applied every one.
// A directory the Drive lacks is left to the sync, which declares the
// destination again.
func (s *folderSync) applyDir(rel []string) (bool, error) {
	// List the Drive directory, unless it is unchanged since the last pass.
	key := strings.Join(rel, "/")
	var dirRef *block.BlockRef
	var files []remoteFile
	var subdirs []string
	var found, unchanged bool
	err := s.view(func(root *unixfs_block.FSTree) error {
		// Find the directory and compare its node with the last pass's.
		dir, err := lookupDir(root, append(slices.Clone(s.dest), rel...))
		if err != nil || dir == nil {
			return err
		}
		found = true
		dirRef = dir.GetCursorRef().Clone()
		seen, ok := s.seenDirs[key]
		if unchanged = ok && !dirRef.GetEmpty() && dirRef.EqualsRef(seen); unchanged {
			return nil
		}

		// Take its files' nodes and its subdirectories' names.
		for i, d := range dir.GetFSNode().GetDirectoryEntry() {
			switch {
			case d.GetIsDirectory():
				subdirs = append(subdirs, d.GetName())
			case d.GetIsFile():
				child, _, err := dir.FollowDirent(i)
				if err != nil {
					return err
				}
				files = append(files, remoteFile{name: d.GetName(), ref: d.GetNodeRef().Clone(), node: child.GetFSNode().CloneVT()})
			}
		}
		return nil
	})
	if err != nil || !found || unchanged {
		return found, err
	}

	// Read the bases of the directory's files.
	known, knownDirs, err := s.idx.FolderVersions(s.ctx, s.folder, rel)
	if err != nil {
		return false, err
	}

	// Apply the deletes first, so a name that changed type is free for its
	// new entry: each file and directory with a base the Drive lacks.
	isDir := make(map[string]bool, len(files)+len(subdirs))
	for _, f := range files {
		isDir[f.name] = false
	}
	for _, name := range subdirs {
		isDir[name] = true
	}
	done := true
	var drop [][]string
	for name, base := range known {
		if dir, ok := isDir[name]; ok && !dir {
			continue
		}
		fileRel := append(slices.Clone(rel), name)
		if err := s.applyDelete(fileRel, base); err != nil {
			if cerr := s.ctx.Err(); cerr != nil {
				return false, cerr
			}
			s.leave(err)
			done = false
			continue
		}
		drop = append(drop, fileRel)
	}
	for _, name := range knownDirs {
		if isDir[name] {
			continue
		}
		ok, err := s.deleteTree(append(slices.Clone(rel), name))
		if err != nil {
			return false, err
		}
		done = done && ok
	}

	// Apply each file whose node differs from its base, adopting one the disk
	// already holds.
	set := make(map[string]*block_store_filesegment.FolderVersion)
	for _, f := range files {
		base := known[f.name]
		if base != nil && f.ref.EqualsRef(base.GetNodeRef()) {
			continue
		}
		fileRel := append(slices.Clone(rel), f.name)
		v, err := s.applyWrite(fileRel, f, base)
		if err != nil {
			if cerr := s.ctx.Err(); cerr != nil {
				return false, cerr
			}
			s.leave(err)
			done = false
			continue
		}
		set[strings.Join(fileRel, "/")] = v
	}
	if err := s.idx.SetFolderVersions(s.ctx, s.folder, set, drop); err != nil {
		return false, err
	}

	// Apply the subdirectories, then skip this directory in later passes once
	// all of it applied.
	for _, name := range subdirs {
		ok, err := s.applyDir(append(slices.Clone(rel), name))
		if err != nil {
			return false, err
		}
		done = done && ok
	}
	if done {
		s.seenDirs[key] = dirRef
	}
	return done, nil
}

// deleteTree applies a member's delete of the directory at rel: each file
// under it holding its base goes to the trash, then each emptied directory is
// removed. A directory holding anything else stays. It reports whether it
// applied every delete.
func (s *folderSync) deleteTree(rel []string) (bool, error) {
	// Delete each file with a base under the directory.
	done := true
	var drop [][]string
	err := s.idx.FolderVersionsUnder(s.ctx, s.folder, rel, func(fileRel []string, base *block_store_filesegment.FolderVersion) error {
		if err := s.applyDelete(fileRel, base); err != nil {
			if cerr := s.ctx.Err(); cerr != nil {
				return cerr
			}
			s.leave(err)
			done = false
			return nil
		}
		drop = append(drop, slices.Clone(fileRel))
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := s.idx.SetFolderVersions(s.ctx, s.folder, nil, drop); err != nil {
		return false, err
	}

	// Remove the emptied directories, deepest first; os.Remove refuses one
	// that holds anything.
	root := filepath.Join(append([]string{s.folder}, rel...)...)
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for _, dir := range slices.Backward(dirs) {
		_ = os.Remove(dir)
	}
	return done, nil
}

// applyWrite applies a member's write of the remote file f at rel, whose base
// is base, nil when the folder holds no version of it. It returns the new
// base: f's version, with the stat of the file holding it.
func (s *folderSync) applyWrite(rel []string, f remoteFile, base *block_store_filesegment.FolderVersion) (*block_store_filesegment.FolderVersion, error) {
	// Adopt a file the disk already holds, as one the one-way sync published.
	path := filepath.Join(append([]string{s.folder}, rel...)...)
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil && info.Mode().IsRegular() && fileMatches(f.node, info) {
		return statVersion(f.ref, info), nil
	}

	// Fetch the member's bytes to a staged file beside the path.
	stage, staged, err := s.stage(rel, f)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stage != "" {
			_ = os.Remove(stage)
		}
	}()
	v := statVersion(f.ref, staged)

	// Check the disk, then place the member's version.
	info, err = os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// An empty path takes it by a rename that never replaces.
		err = renameNoReplace(stage, path)
		if errors.Is(err, fs.ErrExist) {
			err = toConflict(stage, path, refTag(f.ref))
		}
	case err != nil:
	case versionMatches(v, info):
		// The disk holds it already.
		err = os.Remove(stage)
	case base != nil && versionMatches(base, info):
		// The base is exchanged for it, and the displaced file checked.
		err = s.exchangeBase(stage, path, base, f)
	default:
		// Another version keeps the path; the member's takes the conflict
		// name.
		err = toConflict(stage, path, refTag(f.ref))
	}
	if err != nil {
		return nil, err
	}
	stage = ""
	return v, nil
}

// exchangeBase atomically exchanges the staged file and the file at path,
// which held base at the check, then checks the displaced file, now at the
// staged path: it removes the base and keeps anything else under its conflict
// name. A file system without the exchange keeps the path's file and gives the
// member's version the conflict name.
func (s *folderSync) exchangeBase(stage, path string, base *block_store_filesegment.FolderVersion, f remoteFile) error {
	// Exchange the files.
	err := exchange(stage, path)
	if errors.Is(err, errExchangeUnsupported) {
		return toConflict(stage, path, refTag(f.ref))
	}
	if err != nil {
		return err
	}

	// Check the displaced file.
	held, err := os.Lstat(stage)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case versionMatches(base, held):
		return os.Remove(stage)
	}
	tag, err := contentTag(stage, held)
	if err != nil {
		return err
	}
	return toConflict(stage, path, tag)
}

// stage fetches the remote file f at rel into a new staged file beside its
// path, with f's permissions and modification time, synced to disk. It returns
// the staged file's path and stat. A Drive file no longer holding f returns
// errRemoteMoved.
func (s *folderSync) stage(rel []string, f remoteFile) (string, fs.FileInfo, error) {
	// Create the staged file in the path's directory.
	dir := filepath.Join(append([]string{s.folder}, rel[:len(rel)-1]...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp(dir, stagePrefix+"*")
	if err != nil {
		return "", nil, err
	}
	stage := tmp.Name()
	fail := func(err error) (string, fs.FileInfo, error) {
		_ = tmp.Close()
		_ = os.Remove(stage)
		return "", nil, err
	}

	// Copy the bytes from the Drive file, if it still holds f.
	err = s.view(func(root *unixfs_block.FSTree) error {
		// Find the Drive file, and require it to hold f still.
		parent, err := lookupDir(root, append(slices.Clone(s.dest), rel[:len(rel)-1]...))
		if err != nil {
			return err
		}
		if parent == nil {
			return errRemoteMoved
		}
		child, dirent, err := parent.LookupFollowDirent(f.name)
		if err != nil {
			return err
		}
		if !dirent.GetIsFile() || !dirent.GetNodeRef().EqualsRef(f.ref) {
			return errRemoteMoved
		}

		// Copy its bytes.
		h, err := child.BuildFileHandle(s.ctx)
		if err != nil {
			return err
		}
		defer h.Close()
		_, err = io.Copy(tmp, h)
		return err
	})
	if err != nil {
		return fail(errors.Wrapf(err, "fetch %s", strings.Join(rel, "/")))
	}

	// Give it the node's permissions, durably.
	if err := tmp.Chmod(fs.FileMode(f.node.GetPermissions()).Perm()); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(stage)
		return "", nil, err
	}

	// Give it the node's modification time, and take its stat.
	mtime := f.node.GetModTime().AsTime()
	if err := os.Chtimes(stage, mtime, mtime); err != nil {
		_ = os.Remove(stage)
		return "", nil, err
	}
	info, err := os.Lstat(stage)
	if err != nil {
		_ = os.Remove(stage)
		return "", nil, err
	}
	return stage, info, nil
}

// applyDelete applies a member's delete of the file at rel, whose base is
// base. A file holding its base moves to the OS trash and is checked there:
// an edit that landed before the move goes back to the path by a rename that
// never replaces, or to its conflict name. A file holding anything else is
// kept, and the sync publishes it again.
func (s *folderSync) applyDelete(rel []string, base *block_store_filesegment.FolderVersion) error {
	// Check the disk.
	path := filepath.Join(append([]string{s.folder}, rel...)...)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !versionMatches(base, info) {
		return nil
	}

	// Move it to the trash, then check what moved.
	trashed, forget, err := moveToTrash(path)
	if err != nil {
		return errors.Wrapf(err, "trash %s", path)
	}
	held, err := os.Lstat(trashed)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case versionMatches(base, held):
		return nil
	}

	// Restore an edit, to the path when it is still empty.
	err = renameNoReplace(trashed, path)
	if errors.Is(err, fs.ErrExist) {
		var tag string
		if tag, err = contentTag(trashed, held); err == nil {
			err = toConflict(trashed, path, tag)
		}
	}
	if err != nil {
		return err
	}
	return forget()
}

// statVersion returns the version with node reference ref and the stat info.
func statVersion(ref *block.BlockRef, info fs.FileInfo) *block_store_filesegment.FolderVersion {
	return &block_store_filesegment.FolderVersion{
		NodeRef: ref.Clone(),
		Size:    info.Size(),
		ModTime: info.ModTime().UnixNano(),
		Mode:    uint32(info.Mode().Perm()),
	}
}

// versionMatches reports whether info is a regular file with v's stat.
func versionMatches(v *block_store_filesegment.FolderVersion, info fs.FileInfo) bool {
	return info.Mode().IsRegular() &&
		info.Size() == v.GetSize() &&
		info.ModTime().UnixNano() == v.GetModTime() &&
		uint32(info.Mode().Perm()) == v.GetMode()
}

// refTag returns the conflict tag of the Drive version ref: the first hex
// digits of the SHA-256 of its string form, the same on every Device.
func refTag(ref *block.BlockRef) string {
	sum := sha256.Sum256([]byte(ref.MarshalString()))
	return hex.EncodeToString(sum[:])[:conflictTagLen]
}

// contentTag returns the conflict tag of the file at path, a version no
// Drive holds: the first hex digits of the SHA-256 of its bytes.
func contentTag(path string, info fs.FileInfo) (string, error) {
	// Hash the bytes the stat counted.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyN(h, f, info.Size()); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:conflictTagLen], nil
}

// toConflict moves the file at src beside path under path's conflict name
// with tag, by a rename that never replaces: a taken name gains a counter.
func toConflict(src, path, tag string) error {
	dir, name := filepath.Split(path)
	for n := 1; ; n++ {
		suffix := ".conflict-" + tag
		if n > 1 {
			suffix += "-" + strconv.Itoa(n)
		}
		err := renameNoReplace(src, filepath.Join(dir, conflictName(name, suffix)))
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
}

// conflictName returns name with suffix, cutting name on a UTF-8 boundary so
// the result fits in maxNameLen bytes.
func conflictName(name, suffix string) string {
	keep := min(len(name), maxNameLen-len(suffix))
	for keep > 0 && keep < len(name) && !utf8.RuneStart(name[keep]) {
		keep--
	}
	return name[:keep] + suffix
}
