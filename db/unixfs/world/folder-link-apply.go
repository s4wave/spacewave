package unixfs_world

import (
	"bytes"
	"context"
	"crypto/rand"
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
// stat and the SHA-256 digest of the file on disk holding it. A Drive file
// whose node differs from its base is a remote change; a folder file whose
// stat differs from its base is a local change.
//
// The stat picks each branch, but it does not prove a file's bytes: an edit
// in place that keeps the size, saved within the file system's timestamp
// granularity or with its time restored, keeps the stat. So the apply removes
// a file only once its digest equals the version it stands for, and keeps it
// otherwise. A base indexed without a digest proves nothing, and its file is
// kept until the folder publishes it again.
//
// A remote write fetches the member's bytes to a fetch file beside the path,
// renames the complete fetch to its staged name, then checks the disk. A path holding the base is atomically exchanged with
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
// Fetch and staged files are named for their path, and the one-way walk never
// publishes one. A fetch file a crash left holds part of a version the Drive
// holds, and goes. A staged file a crash left is the spec's held file: the
// member's version fetched before the exchange, or the file the exchange
// displaced. The node's first sync settles each before its remote pass changes
// a base: it removes one whose digest is the path's base's, or equal by
// content to the Drive's file at the path now, a version the Drive holds and
// the remote pass fetches again, and gives any other file its conflict name.
//
// The invariants, as the spec names them:
//
//   - NoLostVersion: every file version that reached the disk stays on the disk
//     or in the trash; an apply never unlinks a file, it only renames, and it
//     removes only a file whose stat and digest prove it the base or the
//     member's fetched version.
//   - MemberWriteKept: a member's version, once fetched, ends at the path or
//     under its conflict name, so a member's write is never dropped.
//   - RemoteReplacesOnlyBase: a member's write or delete replaces only a file
//     whose stat is its base; the check after the exchange or the move restores
//     any other.
//   - TrashOnlyDeleted: the trash holds only base versions a member deleted,
//     proven by stat and digest.
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
// scan, plus a fetch, a stat and three renames per changed file, and one read
// of the file a write displaces or a delete trashes.

// fetchPrefix prefixes the file a remote write fetches into, beside its
// path. The prefix is followed by a random tag of stageTagLen hex digits, a
// dash and the path's name, cut to fit a file name.
const fetchPrefix = ".spacewave-fetch-"

// stagePrefix prefixes the staged file a complete fetch is renamed to, with
// the fetch file's tag and name. It is as long as fetchPrefix.
const stagePrefix = ".spacewave-stage-"

// stageTagLen is the number of hex digits of a staged file's random tag.
const stageTagLen = 8

// conflictTagLen is the number of hex digits of a conflict name's digest.
const conflictTagLen = 16

// maxNameLen is the longest file name, in bytes, the platforms allow.
const maxNameLen = 255

// errRemoteMoved marks a remote file that changed after the pass listed it;
// the next pass applies its new version.
var errRemoteMoved = errors.New("remote file changed during the apply")

// ApplyStep names a step of applying a member's change to a file, after the
// phases of the LinkedFolderApply spec.
type ApplyStep int

const (
	// ApplyStaged follows the fetch of the member's version, before the
	// check of the disk.
	ApplyStaged ApplyStep = iota
	// ApplyChecked follows a check that found the base at the path, before
	// the exchange or the move to the trash.
	ApplyChecked
	// ApplyRestoring follows a check that found the path empty, before the
	// rename that never replaces.
	ApplyRestoring
	// ApplySwapped follows the exchange, before the check of the displaced
	// file.
	ApplySwapped
	// ApplyTrashed follows the move to the trash, before the check of the
	// trashed file.
	ApplyTrashed
)

// SetApplyHook sets fn to run at each step of applying a member's change, with
// the step and the file's path. It runs on the sync's goroutine, so the step
// waits for it; tests use it to act between the steps. Set it before a sync.
func (l *FolderLink) SetApplyHook(fn func(step ApplyStep, path string)) {
	l.applyHook = fn
}

// step runs the apply hook, if set, at step for the file at path.
func (s *folderSync) step(step ApplyStep, path string) {
	if s.applyHook != nil {
		s.applyHook(step, path)
	}
}

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
	// A one-way link takes nothing from the Drive.
	if !s.twoWay {
		return nil
	}

	// Settle the staged files a crash left before any base changes.
	if !s.recovered {
		if err := s.recoverStages(); err != nil {
			return err
		}
		s.recovered = true
	}

	// Walk the Drive's tree from the root, counting what could not apply.
	s.unapplied, s.applyErr = 0, nil
	_, err := s.applyDir(nil)
	return err
}

// recoverStages settles each fetch and staged file a crash left in the
// folder, before the node's first remote pass changes a base: it removes a
// fetch file, and settles a staged file as recoverStage describes. It lists
// each directory of the folder once, and reads the bases only of a directory
// holding a staged file.
func (s *folderSync) recoverStages() error {
	return filepath.WalkDir(s.folder, func(path string, d fs.DirEntry, err error) error {
		// Skip a directory removed during the walk.
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path != s.folder {
				return nil
			}
			return err
		}
		if d.IsDir() || !isApplyFile(d.Name()) {
			return nil
		}

		// Remove a fetch file: part of a version the Drive holds.
		if strings.HasPrefix(d.Name(), fetchPrefix) {
			if d.Type().IsRegular() {
				return os.Remove(path) //nolint:gosec // Remove never follows the last link, and the owner controls the folder's directories.
			}
			return nil
		}

		// Settle the staged file against its directory's bases.
		dirPath := filepath.Dir(path)
		relDir, err := filepath.Rel(s.folder, dirPath)
		if err != nil {
			return err
		}
		rel := splitRel(relDir)
		known, _, err := s.idx.FolderVersions(s.ctx, s.folder, rel)
		if err != nil {
			return err
		}
		return s.recoverStage(dirPath, append(slices.Clone(s.dest), rel...), d.Name(), known)
	})
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
	// Adopt a file the disk already holds, as one the one-way sync published,
	// once its bytes prove equal to the Drive's.
	path := filepath.Join(append([]string{s.folder}, rel...)...)
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil && info.Mode().IsRegular() && fileMatches(f.node, info) {
		digest, err := s.driveDigest(f.ref, path, info)
		if err != nil {
			return nil, err
		}
		if digest != nil {
			v := statVersion(f.ref, info)
			v.Digest = digest
			return v, nil
		}
	}

	// Fetch the member's bytes to a staged file beside the path.
	stage, v, err := s.stage(rel, f)
	if err != nil {
		return nil, err
	}

	// Until the exchange displaces the owner's file into it, the staged path
	// holds only the member's version, which the Drive holds, so a failed
	// apply removes it: a failed rename, conflict or check leaves it there.
	defer func() {
		if stage != "" {
			_ = os.Remove(stage)
		}
	}()
	s.step(ApplyStaged, path)

	// Check the disk, then place the member's version.
	info, err = os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// An empty path takes it by a rename that never replaces.
		s.step(ApplyRestoring, path)
		err = renameNoReplace(stage, path)
		if errors.Is(err, fs.ErrExist) {
			err = toConflict(stage, path, refTag(f.ref))
		}
	case err != nil:
	case versionMatches(v, info):
		// The disk may hold it already; a file whose bytes differ keeps the
		// path, and the member's version takes the conflict name.
		var digest []byte
		if digest, err = fileDigest(path, info); err == nil {
			if bytes.Equal(digest, v.GetDigest()) {
				err = os.Remove(stage)
			} else {
				err = toConflict(stage, path, refTag(f.ref))
			}
		}
	case base != nil && versionMatches(base, info):
		// The base is exchanged for it, and the displaced file checked. Once
		// exchanged, the staged path holds the owner's file: a failed check
		// leaves it for the next pass's recovery to settle by its digest.
		var swapped bool
		swapped, err = s.exchangeBase(stage, path, base, f)
		if swapped && err != nil {
			stage = ""
			s.recovered = false
		}
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
// which held base's stat at the check, then checks the displaced file, now at
// the staged path: it removes the base, proven by its stat and digest, and
// keeps anything else under its conflict name. A file system without the
// exchange keeps the path's file and gives the member's version the conflict
// name. swapped reports that the exchange happened, after which the staged
// path holds the displaced file.
func (s *folderSync) exchangeBase(stage, path string, base *block_store_filesegment.FolderVersion, f remoteFile) (swapped bool, err error) {
	// Exchange the files.
	s.step(ApplyChecked, path)
	err = exchange(stage, path)
	if errors.Is(err, errExchangeUnsupported) {
		return false, toConflict(stage, path, refTag(f.ref))
	}
	if err != nil {
		return false, err
	}

	// Check the displaced file.
	s.step(ApplySwapped, path)
	held, err := os.Lstat(stage)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	digest, err := fileDigest(stage, held)
	if err != nil {
		return true, err
	}
	if versionMatches(base, held) && bytes.Equal(digest, base.GetDigest()) {
		return true, os.Remove(stage)
	}
	return true, toConflict(stage, path, digestTag(digest))
}

// stage fetches the remote file f at rel into a new fetch file beside its
// path, with f's permissions and modification time, synced to disk, then
// renames it to its staged name. It returns the staged file's path and f's
// version, with the staged file's stat and digest. A Drive file no longer
// holding f returns errRemoteMoved.
func (s *folderSync) stage(rel []string, f remoteFile) (string, *block_store_filesegment.FolderVersion, error) {
	// Create the fetch file in the path's directory.
	dir := filepath.Join(append([]string{s.folder}, rel[:len(rel)-1]...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	tmp, fetch, err := createFetch(dir, f.name)
	if err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, *block_store_filesegment.FolderVersion, error) {
		_ = tmp.Close()
		_ = os.Remove(fetch)
		return "", nil, err
	}

	// Copy the bytes from the Drive file, if it still holds f.
	sum := sha256.New()
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

		// Copy its bytes, hashing them.
		h, err := child.BuildFileHandle(s.ctx)
		if err != nil {
			return err
		}
		defer h.Close()
		_, err = io.Copy(io.MultiWriter(tmp, sum), h)
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
		_ = os.Remove(fetch)
		return "", nil, err
	}

	// Give it the node's modification time, and take its stat.
	mtime := f.node.GetModTime().AsTime()
	if err := os.Chtimes(fetch, mtime, mtime); err != nil {
		_ = os.Remove(fetch)
		return "", nil, err
	}
	info, err := os.Lstat(fetch)
	if err != nil {
		_ = os.Remove(fetch)
		return "", nil, err
	}

	// Rename the complete fetch to its staged name.
	stage := filepath.Join(dir, stagePrefix+strings.TrimPrefix(filepath.Base(fetch), fetchPrefix))
	if err := renameNoReplace(fetch, stage); err != nil {
		_ = os.Remove(fetch)
		return "", nil, err
	}
	v := statVersion(f.ref, info)
	v.Digest = sum.Sum(nil)
	return stage, v, nil
}

// applyDelete applies a member's delete of the file at rel, whose base is
// base. A file holding its base's stat moves to the OS trash and is checked
// there: anything but the base, proven by its stat and digest, such as an edit
// that landed before the move, goes back to the path by a rename that never
// replaces, or to its conflict name. A file holding anything else is
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

	// Move it to the trash, then check what moved; the base, proven by its
	// stat and digest, stays there.
	s.step(ApplyChecked, path)
	trashed, forget, err := moveToTrash(path)
	if err != nil {
		return errors.Wrapf(err, "trash %s", path)
	}
	s.step(ApplyTrashed, path)
	held, err := os.Lstat(trashed)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if versionMatches(base, held) {
		digest, err := fileDigest(trashed, held)
		if err != nil {
			return err
		}
		if bytes.Equal(digest, base.GetDigest()) {
			return nil
		}
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

// contentTag returns the conflict tag of the file at path, with stat info, a
// version no Drive holds.
func contentTag(path string, info fs.FileInfo) (string, error) {
	digest, err := fileDigest(path, info)
	if err != nil {
		return "", err
	}
	return digestTag(digest), nil
}

// digestTag returns the conflict tag of a version no Drive holds, from its
// digest: its first hex digits.
func digestTag(digest []byte) string {
	return hex.EncodeToString(digest)[:conflictTagLen]
}

// fileDigest returns the SHA-256 of the bytes of the file at path that its
// stat info counted.
func fileDigest(path string, info fs.FileInfo) ([]byte, error) {
	// Open the file.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Hash the bytes the stat counted.
	h := sha256.New()
	if _, err := io.CopyN(h, f, info.Size()); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return h.Sum(nil), nil
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

// conflictName returns name with suffix, cutting name so the result fits in
// a file name.
func conflictName(name, suffix string) string {
	return cutName(name, maxNameLen-len(suffix)) + suffix
}

// cutName returns name cut on a UTF-8 boundary to at most room bytes.
func cutName(name string, room int) string {
	keep := max(min(len(name), room), 0)
	for keep > 0 && keep < len(name) && !utf8.RuneStart(name[keep]) {
		keep--
	}
	return name[:keep]
}

// createFetch creates a new fetch file in dir for the file named name, open
// for writing.
func createFetch(dir, name string) (*os.File, string, error) {
	for {
		var tag [stageTagLen / 2]byte
		if _, err := rand.Read(tag[:]); err != nil {
			return nil, "", err
		}
		head := fetchPrefix + hex.EncodeToString(tag[:]) + "-"
		path := filepath.Join(dir, head+cutName(name, maxNameLen-len(head)))
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, path, err
	}
}

// isApplyFile reports whether name is a fetch or staged file's.
func isApplyFile(name string) bool {
	return strings.HasPrefix(name, fetchPrefix) || strings.HasPrefix(name, stagePrefix)
}

// stagedName returns the name of the file the staged file named stageName
// was fetched for, cut as createFetch cut it.
func stagedName(stageName string) string {
	rest := strings.TrimPrefix(stageName, stagePrefix)
	if len(rest) > stageTagLen && rest[stageTagLen] == '-' {
		return rest[stageTagLen+1:]
	}
	return rest
}

// recoverStage settles the staged file stageName a crash left in the folder
// directory dirPath, linked to the Drive directory parent, as the spec's held
// file. known holds the bases of the directory's files.
// A file with its path's base's digest, or equal by content to the Drive's
// file at the path now, is removed: the Drive holds that version. Any other
// file takes its conflict name.
func (s *folderSync) recoverStage(dirPath string, parent []string, stageName string, known map[string]*block_store_filesegment.FolderVersion) error {
	// Leave alone anything but a regular file.
	stage := filepath.Join(dirPath, stageName)
	info, err := os.Lstat(stage)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return nil
	}
	if err != nil {
		return err
	}

	// Remove the base, or the version the Drive holds at the path now.
	name := stagedName(stageName)
	digest, err := fileDigest(stage, info)
	if err != nil {
		return err
	}
	if base := known[name]; base != nil && bytes.Equal(digest, base.GetDigest()) {
		return os.Remove(stage)
	}
	held, err := s.inDrive(stage, info, parent, name)
	if err != nil {
		return err
	}
	if held {
		return os.Remove(stage)
	}

	// Keep any other under its conflict name.
	return toConflict(stage, filepath.Join(dirPath, name), digestTag(digest))
}

// inDrive reports whether the file at stage, with stat info, equals by content
// the file the Drive directory parent holds at name now. A Drive file it
// cannot read counts as unequal, so the file is kept.
func (s *folderSync) inDrive(stage string, info fs.FileInfo, parent []string, name string) (bool, error) {
	var held bool
	err := s.view(func(root *unixfs_block.FSTree) error {
		// Find the Drive's file at name.
		dir, err := lookupDir(root, parent)
		if err != nil || dir == nil {
			return nil
		}
		child, dirent, err := dir.LookupFollowDirent(name)
		if err != nil || child == nil || !dirent.GetIsFile() {
			return nil
		}
		held, _ = sameContent(s.ctx, child, stage, info, nil)
		return nil
	})
	return held, err
}

// driveDigest returns the digest of the file at path, with stat info, if it
// equals by content the Drive's version ref, and nil otherwise. A Drive version
// it cannot read counts as unequal.
func (s *folderSync) driveDigest(ref *block.BlockRef, path string, info fs.FileInfo) ([]byte, error) {
	var digest []byte
	err := s.view(func(root *unixfs_block.FSTree) error {
		// Open the version ref, then compare and hash the disk's bytes.
		cs := root.GetCursor().DetachTransaction()
		cs.SetRefAtCursor(ref, true)
		tree, err := unixfs_block.NewFSTree(s.ctx, cs, unixfs_block.NodeType_NodeType_UNKNOWN)
		if err != nil {
			return nil
		}
		sum := sha256.New()
		if same, _ := sameContent(s.ctx, tree, path, info, sum); same {
			digest = sum.Sum(nil)
		}
		return nil
	})
	return digest, err
}

// sameContent reports whether the Drive file tree holds the bytes of the file
// at path, with stat info, feeding sum, if set, the file's bytes it read.
func sameContent(ctx context.Context, tree *unixfs_block.FSTree, path string, info fs.FileInfo, sum io.Writer) (bool, error) {
	// Compare the sizes, then open both files.
	if tree.GetFSNode().GetFile().GetTotalSize() != uint64(info.Size()) { //nolint:gosec // a file size is not negative.
		return false, nil
	}
	h, err := tree.BuildFileHandle(ctx)
	if err != nil {
		return false, err
	}
	defer h.Close()
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if sum == nil {
		sum = io.Discard
	}

	// Compare them a chunk at a time, up to the size the stat counted.
	a, b := make([]byte, 64<<10), make([]byte, 64<<10)
	for left := info.Size(); left > 0; {
		n := int(min(left, int64(len(a))))
		if _, err := io.ReadFull(h, a[:n]); err != nil {
			return false, err
		}
		if _, err := io.ReadFull(f, b[:n]); err != nil || !bytes.Equal(a[:n], b[:n]) {
			return false, nil
		}
		_, _ = sum.Write(b[:n])
		left -= int64(n)
	}
	return true, nil
}
