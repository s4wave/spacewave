package unixfs_world

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// pendingEntry is one accumulated dirent waiting to be merged at Commit.
type pendingEntry struct {
	// name is the final path component (leaf name).
	name string
	// nodeType is the entry type (file / dir / symlink).
	nodeType unixfs_block.NodeType
	// permissions is the entry mode bits (perm only).
	permissions fs.FileMode
	// ts is the entry mtime.
	ts *timestamppb.Timestamp
	// blobRef is the file content blob (FILE entries only).
	blobRef *block.BlockRef
	// symlink is the symlink payload (SYMLINK entries only).
	symlink *unixfs_block.FSSymlink
}

// pendingDir holds accumulated entries keyed by their parent path.
type pendingDir struct {
	// parentPath is the path from the fs root to this directory.
	parentPath []string
	// entries is the list of children added to this directory,
	// including an explicit AddDir of the directory itself recorded as a
	// child entry under its parent.
	entries []pendingEntry
}

// BatchFSWriter accumulates file, directory, and symlink entries keyed by
// parent path and commits them under a single world transaction.
//
// Local-only: Commit bypasses ApplyWorldOp and mutates the world object via
// AccessObjectState directly. Remote peers learn of the result only via
// packfile sync of the new root ref.
//
// Not safe for concurrent use by multiple goroutines. Intended for
// single-caller bulk imports (e.g. rootfs tar extraction).
type BatchFSWriter struct {
	// ws is the world state.
	ws world.WorldState
	// objKey is the fs object key.
	objKey string
	// fsType is the filesystem object type.
	fsType FSType
	// sender is the op sender peer id.
	sender peer.ID

	// committed is set after Commit has been called.
	committed bool
	// released is set after Release has been called.
	released bool

	// pending maps a joined parent-path key to accumulated entries under
	// that parent. The key encoding is produced by joinPathKey.
	pending map[string]*pendingDir

	metricsRecorder BatchFSWriterMetricsRecorder
}

// NewBatchFSWriter constructs a new BatchFSWriter bound to the given world
// object. The caller is responsible for invoking either Commit or Release.
func NewBatchFSWriter(
	ws world.WorldState,
	objKey string,
	fsType FSType,
	sender peer.ID,
) *BatchFSWriter {
	return &BatchFSWriter{
		ws:      ws,
		objKey:  objKey,
		fsType:  fsType,
		sender:  sender,
		pending: make(map[string]*pendingDir),
	}
}

// AddFile records a regular file entry to be created under parentPath at
// Commit time. The file blob is built eagerly here via the same per-op blob
// path as FsMknodWithContent (one btx.Write per blob); only the blob ref and
// entry metadata are accumulated. No parent directory mutation happens
// until Commit.
func (b *BatchFSWriter) AddFile(
	ctx context.Context,
	parentPath []string,
	name string,
	nodeType unixfs.FSCursorNodeType,
	dataLen int64,
	rdr io.Reader,
	permissions fs.FileMode,
	ts time.Time,
) error {
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}
	if dataLen < 0 {
		return errors.New("negative data length")
	}
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage: "ingest-file-start",
		Bytes: dataLen,
	})

	// Build the blob in an isolated object. Mirrors FsMknodWithContent
	// phase 1: exactly one btx.Write writes the blob blocks + computes the
	// root BlockRef. Parent dir is NOT touched.
	var blobRef *block.BlockRef
	if dataLen > 0 {
		objRef, err := world.AccessObject(
			ctx,
			b.ws.AccessWorldState,
			nil,
			func(bcs *block.Cursor) error {
				bcs.SetRefAtCursor(nil, true)
				_, berr := blob.BuildBlob(ctx, dataLen, rdr, bcs, nil)
				return berr
			},
		)
		if err != nil {
			return err
		}
		blobRef = objRef.GetRootRef()
	}

	pd := b.pendingDirFor(parentPath)
	pd.entries = append(pd.entries, pendingEntry{
		name:        name,
		nodeType:    unixfs_block.FSCursorNodeTypeToNodeType(nodeType),
		permissions: permissions.Perm(),
		ts:          unixfs_block.ToTimestamp(ts, true),
		blobRef:     blobRef,
	})
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage:          "ingest-file-complete",
		Bytes:          dataLen,
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
	})
	return nil
}

// AddDir records an explicit directory entry under parentPath. The sync
// driver (or equivalent caller) is responsible for providing an AddDir for
// every parent referenced by AddFile / AddSymlink that does not already
// exist in the target FSTree.
//
// Recording AddDir also creates a pendingDir slot for the child path so
// later AddFile / AddSymlink calls under that path attach to the correct
// parent even if they arrive before the dir's children.
func (b *BatchFSWriter) AddDir(
	ctx context.Context,
	parentPath []string,
	name string,
	permissions fs.FileMode,
	ts time.Time,
) error {
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}

	// Record the dir as a child entry under its parent so the parent merge
	// sees it at Commit time.
	parentPd := b.pendingDirFor(parentPath)
	parentPd.entries = append(parentPd.entries, pendingEntry{
		name:        name,
		nodeType:    unixfs_block.NodeType_NodeType_DIRECTORY,
		permissions: permissions.Perm(),
		ts:          unixfs_block.ToTimestamp(ts, true),
	})

	// Eagerly create the pendingDir slot keyed by the child path so any
	// subsequent child entry lands under a dir we know was explicitly
	// declared, guarding against intermediate parents that never had an
	// explicit AddDir.
	childPath := make([]string, 0, len(parentPath)+1)
	childPath = append(childPath, parentPath...)
	childPath = append(childPath, name)
	b.pendingDirFor(childPath)
	return nil
}

// AddSymlink records a symlink entry pointing at target under parentPath.
func (b *BatchFSWriter) AddSymlink(
	ctx context.Context,
	parentPath []string,
	name string,
	target []string,
	targetIsAbsolute bool,
	ts time.Time,
) error {
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" || len(target) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	sym := unixfs_block.NewFSSymlink(unixfs_block.NewFSPath(target, targetIsAbsolute))
	pd := b.pendingDirFor(parentPath)
	pd.entries = append(pd.entries, pendingEntry{
		name:     name,
		nodeType: unixfs_block.NodeType_NodeType_SYMLINK,
		// Symlink perms are not carried in the op today (see FsSymlinkOp).
		permissions: 0,
		ts:          unixfs_block.ToTimestamp(ts, true),
		symlink:     sym,
	})
	return nil
}

// Commit flushes every accumulated entry to the world object under a single
// AccessObjectState transaction. Touched directories are merged in
// depth-ascending order so a parent directory exists in the FSTree before
// any of its children are written. The trailing btx.Write inside
// AccessObjectState walks the dirty tree bottom-up to produce a single
// root-ref update.
func (b *BatchFSWriter) Commit(ctx context.Context) error {
	if b.released {
		return errors.New("batch writer released")
	}
	if b.committed {
		return errors.New("batch writer already committed")
	}
	b.committed = true

	if len(b.pending) == 0 {
		b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
			Stage:     "commit-empty",
			Committed: true,
		})
		return nil
	}
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage:          "commit-start",
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
	})

	obj, exists, err := b.ws.GetObject(ctx, b.objKey)
	if err != nil {
		return err
	}
	if !exists {
		return unixfs_errors.ErrNotExist
	}

	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
		if err != nil {
			return err
		}
		resolvedDirs := map[string]*unixfs_block.FSTree{
			joinPathKey(nil): root,
		}
		ordered := b.sortedPendingDirs()
		for _, pd := range ordered {
			if err := b.mergePendingDir(ctx, root, resolvedDirs, pd); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
			Stage:          "commit-complete",
			PendingDirs:    pendingDirs,
			PendingEntries: pendingEntries,
			Committed:      true,
		})
	}
	return err
}

// sortedPendingDirs returns pendingDirs ordered by ascending parent-path
// depth and lexicographic path. Parents are always merged before their
// children, so AddDir-created subdirs exist before child entries land.
func (b *BatchFSWriter) sortedPendingDirs() []*pendingDir {
	dirs := make([]*pendingDir, 0, len(b.pending))
	for _, pd := range b.pending {
		dirs = append(dirs, pd)
	}
	slices.SortStableFunc(dirs, func(a, b *pendingDir) int {
		if len(a.parentPath) != len(b.parentPath) {
			return len(a.parentPath) - len(b.parentPath)
		}
		for k := range a.parentPath {
			if a.parentPath[k] < b.parentPath[k] {
				return -1
			}
			if a.parentPath[k] > b.parentPath[k] {
				return 1
			}
		}
		return 0
	})
	return dirs
}

// mergePendingDir merges accumulated entries into the FSTree node at
// pd.parentPath. Does not recurse; each pending child whose own accumulated
// entries live in another pendingDir is handled in a separate mergePendingDir
// invocation.
func (b *BatchFSWriter) mergePendingDir(
	ctx context.Context,
	root *unixfs_block.FSTree,
	resolvedDirs map[string]*unixfs_block.FSTree,
	pd *pendingDir,
) error {
	dir := root
	if len(pd.parentPath) != 0 {
		key := joinPathKey(pd.parentPath)
		dir = resolvedDirs[key]
		if dir == nil {
			var err error
			dir, _, err = unixfs_block.LookupFSTreePath(root, pd.parentPath)
			if err != nil {
				return errors.Wrapf(
					err,
					"batch writer parent path %q not present in fs and not declared via AddDir",
					strings.Join(pd.parentPath, "/"),
				)
			}
			resolvedDirs[key] = dir
		}
	}

	for i := range pd.entries {
		e := &pd.entries[i]
		switch e.nodeType {
		case unixfs_block.NodeType_NodeType_DIRECTORY:
			// Mkdir is idempotent: an existing dir with the same name
			// is reused rather than duplicated.
			children, err := dir.Mkdir(e.permissions, e.ts, e.name)
			if err != nil {
				return err
			}
			if child := children[e.name]; child != nil {
				childPath := make([]string, 0, len(pd.parentPath)+1)
				childPath = append(childPath, pd.parentPath...)
				childPath = append(childPath, e.name)
				resolvedDirs[joinPathKey(childPath)] = child
			}
		case unixfs_block.NodeType_NodeType_FILE:
			existing, err := dir.Lookup(e.name)
			if err != nil {
				return err
			}
			childPath := make([]string, 0, len(pd.parentPath)+1)
			childPath = append(childPath, pd.parentPath...)
			childPath = append(childPath, e.name)
			if existing != nil && existing.GetNodeType() == unixfs_block.NodeType_NodeType_FILE {
				// syncExistingFile rewrites content while preserving the
				// inode. A successful overwrite must not end the merge:
				// entries recorded after it in the same batch still apply.
				if err := b.syncExistingFile(ctx, root, childPath, e.blobRef, e.ts); err != nil {
					return err
				}
				continue
			}
			if existing != nil {
				// Pass nil ts so Remove does not bump the parent
				// directory's ModTime. The per-op path leaves the parent
				// untouched on a file-content overwrite (billy OpenFile +
				// Write path), and the batch commit must produce a root ref
				// byte-identical to that per-op path.
				deleteResolvedDirSubtree(resolvedDirs, childPath)
				if _, err := dir.Remove([]string{e.name}, nil); err != nil {
					return err
				}
			}
			if _, err := dir.Mknod(e.name, e.nodeType, nil, e.permissions, e.ts); err != nil {
				return err
			}
			if !e.blobRef.GetEmpty() {
				if err := unixfs_block.WriteBlob(ctx, root, childPath, 0, e.blobRef, false, true, e.ts); err != nil {
					return err
				}
			}
		case unixfs_block.NodeType_NodeType_SYMLINK:
			// Symlink(checkExist=false) already replaces an existing
			// dirent in-place, preserving sort order.
			childPath := make([]string, 0, len(pd.parentPath)+1)
			childPath = append(childPath, pd.parentPath...)
			childPath = append(childPath, e.name)
			deleteResolvedDirSubtree(resolvedDirs, childPath)
			if _, err := dir.Symlink(false, e.name, e.symlink, e.ts); err != nil {
				return err
			}
		default:
			return errors.Errorf("unsupported batch entry node type: %s", e.nodeType.String())
		}
	}
	return nil
}

func deleteResolvedDirSubtree(resolvedDirs map[string]*unixfs_block.FSTree, path []string) {
	key := joinPathKey(path)
	for resolvedPath := range resolvedDirs {
		if resolvedPath == key || strings.HasPrefix(resolvedPath, key+"\x00") {
			delete(resolvedDirs, resolvedPath)
		}
	}
}

// syncExistingFile replays the existing-file overwrite semantics used by the
// per-op path: truncate the file to the incoming size, then compare and write
// only mismatching chunks. This preserves the inode and its history-sensitive
// file layout instead of recreating a fresh one-shot file node.
func (b *BatchFSWriter) syncExistingFile(
	ctx context.Context,
	root *unixfs_block.FSTree,
	path []string,
	blobRef *block.BlockRef,
	ts *timestamppb.Timestamp,
) error {
	readExisting := func(offset int64, p []byte) (int, error) {
		node, _, err := unixfs_block.LookupFSTreePath(root, path)
		if err != nil {
			return 0, err
		}
		fh, err := node.BuildFileHandle(ctx)
		if err != nil {
			return 0, err
		}
		defer fh.Close()
		if _, err := fh.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		}
		n, err := io.ReadAtLeast(fh, p, len(p))
		if err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return n, err
	}

	node, _, err := unixfs_block.LookupFSTreePath(root, path)
	if err != nil {
		return err
	}

	var srcSize int64
	var srcRdr io.ReadCloser
	if !blobRef.GetEmpty() {
		blobCs := node.GetCursor().DetachTransaction()
		blobCs.SetRefAtCursor(blobRef, true)
		srcBlob, err := blob.UnmarshalBlob(ctx, blobCs)
		if err != nil {
			return err
		}
		if srcBlob.GetTotalSize() > math.MaxInt64 {
			return errors.New("source blob size exceeds int64")
		}
		srcSize = int64(srcBlob.GetTotalSize()) //nolint:gosec
		srcRdr, err = blob.NewReader(ctx, blobCs)
		if err != nil {
			return err
		}
		defer srcRdr.Close()
	}

	if err := unixfs_block.TruncateFile(ctx, root, path, srcSize, ts); err != nil {
		return err
	}
	if srcSize == 0 {
		return nil
	}

	readBuffer := make([]byte, 32*1024)
	compareBuffer := make([]byte, len(readBuffer))
	var offset int64
	for {
		nreadSrc, err := srcRdr.Read(readBuffer)
		if err != nil && err != io.EOF {
			return err
		}
		if nreadSrc == 0 {
			if offset != srcSize {
				return errors.Errorf("wrote %d but expected %d", offset, srcSize)
			}
			break
		}

		readChunk := readBuffer[:nreadSrc]
		compareChunk := compareBuffer[:nreadSrc]
		nreadDst, readErr := readExisting(offset, compareChunk)
		if readErr != nil {
			return readErr
		}
		if nreadDst == 0 {
			return errors.Errorf("read 0 bytes but expected %d", nreadSrc)
		}
		compareChunk = compareChunk[:nreadDst]
		if !bytes.Equal(readChunk[:len(compareChunk)], compareChunk) {
			// TruncateFile already matched the source size above, so writing the
			// incoming blob as an overlay preserves the per-op range history
			// without leaving stale tail ranges behind it.
			if err := unixfs_block.WriteBlob(ctx, root, path, 0, blobRef, false, false, ts); err != nil {
				return err
			}
			return nil
		}
		offset += int64(len(readChunk))
		if err == io.EOF {
			break
		}
	}
	return nil
}

// Release discards any accumulated state without committing. Safe to call
// after Commit; after Release the writer rejects all further calls.
//
// Released blobs already live in the world's block store (AddFile writes
// them via AccessObject at record time, not at Commit time). Release does
// not attempt to reclaim those blobs; they will be garbage-collected by
// the block-store GC as part of normal world upkeep.
func (b *BatchFSWriter) Release() {
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.released = true
	b.pending = nil
	b.recordBatchFSWriterMetric(context.Background(), BatchFSWriterMetric{
		Stage:          "release",
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
		Released:       true,
	})
}

// checkOpen returns an error if the writer is no longer accepting entries.
func (b *BatchFSWriter) checkOpen() error {
	if b.released {
		return errors.New("batch writer released")
	}
	if b.committed {
		return errors.New("batch writer already committed")
	}
	return nil
}

// pendingDirFor returns the pendingDir for parentPath, creating it on first
// access. parentPath is stored by value (copied) so callers may mutate the
// argument slice after return.
func (b *BatchFSWriter) pendingDirFor(parentPath []string) *pendingDir {
	key := joinPathKey(parentPath)
	pd, ok := b.pending[key]
	if ok {
		return pd
	}
	pd = &pendingDir{parentPath: slices.Clone(parentPath)}
	b.pending[key] = pd
	return pd
}

// joinPathKey produces a stable map key from a path slice. The 0x00 byte is
// forbidden in FS path components, so it is safe as a separator.
func joinPathKey(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, "\x00")
}
