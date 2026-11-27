package unixfs_world

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// maxHeldFileBytes bounds the small file contents a batch holds as bytes
// until Commit, the same as the blob buffer's default pending bytes. Past it a
// small file is built as a blob, which the file node embeds and the stage
// holds until Commit returns.
const maxHeldFileBytes = 64 << 20

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
	// blobRef is the file content blob (FILE entries only), unset when data
	// holds the contents.
	blobRef *block.BlockRef
	// data holds the contents of a FILE entry the file node embeds, so no
	// blob block is built for it.
	data []byte
	// symlink is the symlink payload (SYMLINK entries only).
	symlink *unixfs_block.FSSymlink
	// replace replaces an existing file's node instead of overwriting its
	// changed ranges, which reads the old contents (FILE entries only).
	replace bool
	// remove removes the entry, and with a directory its subtree, instead of
	// adding one. The other fields are unset.
	remove bool
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
// File blobs are buffered in memory and written into the writer's stage once
// at Commit, before the tree merge. The buffer drains into the stage early
// only when it reaches its capacity bounds. A file the file node embeds, at
// most file.MaxInlineRawBlobSize bytes, is held as bytes instead, up to
// maxHeldFileBytes per batch, and written into its node by the merge.
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
	// expects are the files Commit checks before it merges anything.
	expects []fileExpect
	// wants are the full paths of the files Versions reports, in WantVersion
	// order.
	wants [][]string
	// root is the object root Commit published, set when it succeeds.
	root *bucket.ObjectRef

	// stage owns the blob blocks the buffer drains before Commit publishes
	// them, opened with storage and released by Commit or Release.
	stage world.WorldStage
	// storage is the staged world storage cursor blobs are built against,
	// opened by the first AddFile with data and released by Commit or Release.
	storage *bucket_lookup.Cursor
	// blobs buffers blob blocks until Commit writes them into the stage.
	blobs *block.BufferedStore
	// heldBytes counts the small file contents pending entries hold as data,
	// at most maxHeldFileBytes.
	heldBytes int

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
// Commit time. The file blob is built eagerly into the writer's blob buffer.
// Only the blob ref and entry metadata are accumulated. Contents the file
// node embeds are held as bytes instead, as the BatchFSWriter doc describes.
// No parent directory changes until Commit.
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
	// Validate the file entry before accepting it into BatchFSWriter.
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}
	if dataLen < 0 {
		return errors.New("negative data length")
	}

	// Record the start of the file ingest before building its blob.
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage: "ingest-file-start",
		Bytes: dataLen,
	})

	// Keep contents the file node will embed as bytes, while the batch holds
	// less than maxHeldFileBytes of them: a blob block built for them would
	// never be referenced, and the stage would hold the block until Commit
	// returns.
	// Build other contents as a blob in an isolated transaction whose writes
	// stay in the blob buffer. The parent dir is NOT touched.
	var blobRef *block.BlockRef
	var data []byte
	switch {
	case dataLen > 0 && dataLen <= file.MaxInlineRawBlobSize && b.heldBytes+int(dataLen) <= maxHeldFileBytes:
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(rdr, data); err != nil {
			return err
		}
		b.heldBytes += len(data)
	case dataLen > 0:
		var err error
		blobRef, err = b.buildBlob(ctx, dataLen, rdr)
		if err != nil {
			return err
		}
	}

	// Queue the file metadata.
	b.queueFile(ctx, parentPath, name, unixfs_block.FSCursorNodeTypeToNodeType(nodeType), permissions, ts, blobRef, data, dataLen, false)
	return nil
}

// AddFileBlob records a regular file entry whose contents are the blob build
// returns, as AddFile records one built from a reader. build addresses the
// contents with the World's transform xfrm and hash type and returns their
// Blob. The writer stores only the Blob block, so the blocks the Blob
// references must already be readable, as the blocks of a linked file are.
//
// The entry replaces an existing file's node rather than overwriting the
// ranges that differ, since that comparison reads the old contents, which a
// relinked file no longer holds, and costs a read of both versions under the
// World's writer.
func (b *BatchFSWriter) AddFileBlob(
	ctx context.Context,
	parentPath []string,
	name string,
	permissions fs.FileMode,
	ts time.Time,
	build func(xfrm block.Transformer, hashType hash.HashType) (*blob.Blob, error),
) error {
	// Validate the file entry before accepting it into BatchFSWriter.
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}

	// Build the Blob with the World's transform and hash type.
	if err := b.openStorage(ctx); err != nil {
		return err
	}
	bl, err := build(b.storage.GetTransformer(), b.storage.GetBlockStore().GetHashType())
	if err != nil {
		return err
	}
	size := int64(bl.GetTotalSize()) //nolint:gosec // a file's size is an int64.

	// Write the Blob block into the blob buffer. An empty file has no blob.
	var blobRef *block.BlockRef
	if size != 0 {
		btx, bcs := b.storage.BuildTransactionWithStore(nil, b.blobs)
		btx.SetWriteBuffer(b.blobs)
		bcs.SetBlock(bl, true)
		blobRef, _, err = btx.Write(ctx, true)
		if err != nil {
			return err
		}
	}

	// Queue the file metadata.
	b.queueFile(ctx, parentPath, name, unixfs_block.NodeType_NodeType_FILE, permissions, ts, blobRef, nil, size, true)
	return nil
}

// queueFile queues a FILE entry under parentPath and records the completed
// ingest of its dataLen bytes. replace marks an entry that replaces an
// existing file's node.
func (b *BatchFSWriter) queueFile(
	ctx context.Context,
	parentPath []string,
	name string,
	nodeType unixfs_block.NodeType,
	permissions fs.FileMode,
	ts time.Time,
	blobRef *block.BlockRef,
	data []byte,
	dataLen int64,
	replace bool,
) {
	// Queue the file metadata under its parent.
	pd := b.pendingDirFor(parentPath)
	pd.entries = append(pd.entries, pendingEntry{
		name:        name,
		nodeType:    nodeType,
		permissions: permissions.Perm(),
		ts:          unixfs_block.ToTimestamp(ts, true),
		blobRef:     blobRef,
		data:        data,
		replace:     replace,
	})

	// Record the pending directory and entry counts after accepting the file.
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage:          "ingest-file-complete",
		Bytes:          dataLen,
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
	})
}

// openStorage opens the staged World storage cursor and its shared blob
// buffer on first use.
func (b *BatchFSWriter) openStorage(ctx context.Context) error {
	// Reuse the open storage.
	if b.storage != nil {
		return nil
	}

	// Stage the World and build its storage cursor.
	stage, err := b.ws.StageWorldState(ctx)
	if err != nil {
		return err
	}
	storage, err := stage.BuildStorageCursor(ctx)
	if err != nil {
		stage.Release()
		return err
	}
	b.stage, b.storage = stage, storage
	b.blobs = block.NewBufferedStore(ctx, storage.GetBlockStore())
	return nil
}

// buildBlob writes a blob of dataLen bytes from rdr into the blob buffer and
// returns its root reference.
func (b *BatchFSWriter) buildBlob(ctx context.Context, dataLen int64, rdr io.Reader) (*block.BlockRef, error) {
	// Open the staged World storage cursor on first use.
	if err := b.openStorage(ctx); err != nil {
		return nil, err
	}

	// The borrowed write buffer keeps the blob's root write from draining.
	btx, bcs := b.storage.BuildTransactionWithStore(nil, b.blobs)
	btx.SetWriteBuffer(b.blobs)
	bcs.SetRefAtCursor(nil, true)
	if _, err := blob.BuildBlob(ctx, dataLen, rdr, bcs, nil); err != nil {
		return nil, err
	}
	ref, _, err := btx.Write(ctx, true)
	return ref, err
}

// ExpectFile makes Commit refuse the whole batch with a *FileConflictError
// unless the file at parentPath/name has the node reference version. Commit
// checks every expectation in its transaction against the root it publishes
// onto, before merging any entry, so a write committed first through any
// writer is never replaced unseen.
func (b *BatchFSWriter) ExpectFile(parentPath []string, name string, version *block.BlockRef) error {
	if version.GetEmpty() {
		return errors.New("expected file version is empty")
	}
	return b.expect(parentPath, name, version)
}

// ExpectMissing makes Commit refuse the whole batch with a *FileConflictError
// unless parentPath/name holds nothing: no file, directory or symlink. A batch
// that adds a directory under this expectation never merges into one another
// writer created first. Commit checks it as it checks ExpectFile.
func (b *BatchFSWriter) ExpectMissing(parentPath []string, name string) error {
	return b.expect(parentPath, name, nil)
}

// expect records that parentPath/name must hold the file with version, or
// nothing when version is nil, when Commit applies the batch.
func (b *BatchFSWriter) expect(parentPath []string, name string, version *block.BlockRef) error {
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}
	b.expects = append(b.expects, fileExpect{
		path:    append(slices.Clone(parentPath), name),
		version: version.Clone(),
	})
	return nil
}

// WantVersion makes Versions report the version the file at parentPath/name
// has in the root Commit publishes. The batch must write that file.
func (b *BatchFSWriter) WantVersion(parentPath []string, name string) error {
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}
	b.wants = append(b.wants, append(slices.Clone(parentPath), name))
	return nil
}

// Versions returns, after Commit succeeded, the version of each file
// WantVersion selected, in that order. It reads them from the root the commit
// published, which no later writer changes, so each is the version that
// commit gave its file whatever committed since.
func (b *BatchFSWriter) Versions(ctx context.Context) ([]FileVersion, error) {
	// Require a published root, and skip the read when nothing was asked.
	if len(b.wants) == 0 {
		return nil, nil
	}
	if b.root == nil {
		return nil, errors.New("batch writer has not committed")
	}

	// Read the versions from the published root.
	obj, exists, err := b.ws.GetObject(ctx, b.objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, unixfs_errors.ErrNotExist
	}
	var out []FileVersion
	err = obj.AccessWorldState(ctx, b.root, func(bls *bucket_lookup.Cursor) error {
		// Open the published filesystem tree read-only and look up each path.
		_, bcs := bls.BuildTransaction(nil)
		root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
		if err != nil {
			return err
		}
		out, err = readFileVersions(root, b.wants)
		return err
	})
	return out, err
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
	// Validate the directory entry before accepting it into BatchFSWriter.
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
	// subsequent child entry lands under an explicitly declared dir,
	// guarding against intermediate parents that never had an explicit
	// AddDir.
	childPath := make([]string, 0, len(parentPath)+1)
	childPath = append(childPath, parentPath...)
	childPath = append(childPath, name)
	b.pendingDirFor(childPath)
	return nil
}

// Remove records the removal of the entry name under parentPath, with its
// subtree when it is a directory. Removing an absent entry does nothing. A
// later entry of the same name in the batch adds it again, so Remove then
// AddDir replaces a file with a directory.
func (b *BatchFSWriter) Remove(parentPath []string, name string) error {
	// Validate the entry before accepting it into BatchFSWriter.
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" {
		return unixfs_errors.ErrEmptyPath
	}

	// Queue the removal under its parent directory.
	pd := b.pendingDirFor(parentPath)
	pd.entries = append(pd.entries, pendingEntry{name: name, remove: true})
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
	// Validate the symlink entry before accepting it into BatchFSWriter.
	if err := b.checkOpen(); err != nil {
		return err
	}
	if name == "" || len(target) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	// Queue the symlink target and metadata under its parent directory.
	sym := unixfs_block.NewFSSymlink(unixfs_block.NewFSPath(target, targetIsAbsolute))
	pd := b.pendingDirFor(parentPath)
	pd.entries = append(pd.entries, pendingEntry{
		name:     name,
		nodeType: unixfs_block.NodeType_NodeType_SYMLINK,
		// Symlink perms are not carried in the op (see FsSymlinkOp).
		permissions: 0,
		ts:          unixfs_block.ToTimestamp(ts, true),
		symlink:     sym,
	})
	return nil
}

// Commit flushes every accumulated entry to the world object under a single
// AccessObjectState call. Touched directories are merged in depth-ascending
// order so a parent directory exists in the FSTree before any of its children
// are written. The buffered blob blocks are written once into the writer's
// stage first, and each staged merge attempt builds through the same stage and
// references them.
//
// Invariants: the upload publishes all of its entries or none. An attempt
// that lost the race to another writer merges again onto the new root and
// checks every expectation there, so it never overwrites a write it did not
// merge onto. The stage holds the staged blobs, and the staged merges that
// reference them, from staging until Commit returns, on every exit, including
// an error and a cancelled context.
func (b *BatchFSWriter) Commit(ctx context.Context) error {
	// Claim the BatchFSWriter commit and release its storage when it finishes.
	if b.released {
		return errors.New("batch writer released")
	}
	if b.committed {
		return errors.New("batch writer already committed")
	}
	b.committed = true
	defer b.releaseStorage()

	// Record whether the commit has pending directory entries to publish.
	if len(b.pending) == 0 {
		b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
			Stage:     "commit-empty",
			Committed: true,
		})
		return nil
	}

	// Record the pending directory and entry counts before committing the tree.
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
		Stage:          "commit-start",
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
	})

	// Acquire the filesystem World object for the batch commit.
	obj, exists, err := b.ws.GetObject(ctx, b.objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !exists {
		return unixfs_errors.ErrNotExist
	}

	// Write the buffered blobs into the stage once, so no attempt writes them
	// again. The stage holds them until releaseStorage.
	if b.blobs != nil {
		if err := b.blobs.Flush(ctx); err != nil {
			return err
		}
	}

	// Merge onto the object root through the blob stage, so each put that
	// references a blob releases the stage's root on it inside the upload's
	// own commit, and the stage releases only the top of the tree.
	//
	// Invariants: every blob stays held from its put until the published root
	// references it or the stage is released, on every attempt and every
	// exit. A lost attempt's directories stay in the stage until Commit
	// returns and hold the blobs they referenced, so a retry never finds a
	// blob unheld. The attempt under the writer builds through its
	// transaction, and the stage keeps holding the blobs it has not released
	// until Commit returns.
	var build world.AccessWorldStateFunc
	if b.storage != nil {
		build = world.NewAccessWorldStateFunc(b.storage)
	}
	var attempt int
	published, _, err := world.AccessObjectStateThrough(ctx, obj, build, func(bcs *block.Cursor) error {
		// Count the attempt for the commit-merged metric.
		attempt++

		// Open the filesystem tree and merge parent directories before their children.
		root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
		if err != nil {
			return err
		}
		resolvedDirs := map[string]*unixfs_block.FSTree{
			joinPathKey(nil): root,
		}

		// Refuse the batch unless every expected file is as the writer read it.
		for _, e := range b.expects {
			if err := checkFileExpect(root, e); err != nil {
				return err
			}
		}

		// Merge the pending directories in parent-first order.
		ordered := b.sortedPendingDirs()
		for _, pd := range ordered {
			if err := b.mergePendingDir(ctx, root, resolvedDirs, pd); err != nil {
				return err
			}
		}
		b.recordBatchFSWriterMetric(ctx, BatchFSWriterMetric{
			Stage:          "commit-merged",
			PendingDirs:    pendingDirs,
			PendingEntries: pendingEntries,
			Attempt:        attempt,
		})
		return nil
	})

	// Record successful publication of the pending filesystem entries.
	if err == nil {
		b.root = published
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
// pd.parentPath. Does not recurse. Each pending child whose own accumulated
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
		if e.remove {
			// Remove the entry and forget the directories resolved under it.
			// Pass nil ts so the parent keeps its ModTime, as a replaced file
			// does.
			childPath := make([]string, 0, len(pd.parentPath)+1)
			childPath = append(childPath, pd.parentPath...)
			childPath = append(childPath, e.name)
			deleteResolvedDirSubtree(resolvedDirs, childPath)
			if _, err := dir.Remove([]string{e.name}, nil); err != nil {
				return err
			}
			continue
		}
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
			if existing != nil && existing.GetNodeType() == unixfs_block.NodeType_NodeType_FILE && !e.replace {
				// syncExistingFile rewrites content while preserving the
				// inode. A successful overwrite must not end the merge:
				// entries recorded after it in the same batch still apply.
				if err := b.syncExistingFile(ctx, root, childPath, e); err != nil {
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
			if err := writeEntryContents(ctx, root, childPath, e, true); err != nil {
				return err
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
// only mismatching chunks. The overwrite preserves the inode and its
// history-sensitive file layout instead of recreating a fresh one-shot file
// node.
func (b *BatchFSWriter) syncExistingFile(
	ctx context.Context,
	root *unixfs_block.FSTree,
	path []string,
	e *pendingEntry,
) error {
	// Prepare an existing-file reader for comparing the incoming blob.
	readExisting := func(offset int64, p []byte) (int, error) {
		// Resolve the existing file node at the requested path.
		node, _, err := unixfs_block.LookupFSTreePath(root, path)
		if err != nil {
			return 0, err
		}

		// Open an existing-file handle for this comparison read.
		fh, err := node.BuildFileHandle(ctx)
		if err != nil {
			return 0, err
		}
		defer fh.Close()

		// Read the existing file range and normalize a short read to EOF.
		if _, err := fh.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		}
		n, err := io.ReadAtLeast(fh, p, len(p))
		if err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return n, err
	}

	// Resolve the destination file before opening the incoming blob.
	node, _, err := unixfs_block.LookupFSTreePath(root, path)
	if err != nil {
		return err
	}

	// Open the incoming contents and determine their file size.
	var srcSize int64
	var srcRdr io.ReadCloser
	switch {
	case e.data != nil:
		srcSize = int64(len(e.data))
		srcRdr = io.NopCloser(bytes.NewReader(e.data))
	case !e.blobRef.GetEmpty():
		var err error
		srcSize, srcRdr, err = node.OpenBlob(ctx, e.blobRef)
		if err != nil {
			return err
		}
		defer srcRdr.Close()
	}

	// Match the existing file length to the incoming blob before comparing content.
	if err := unixfs_block.TruncateFile(ctx, root, path, srcSize, e.ts); err != nil {
		return err
	}
	if srcSize == 0 {
		return nil
	}

	// Compare incoming chunks with the existing file and replace differing content.
	readBuffer := make([]byte, 32*1024)
	compareBuffer := make([]byte, len(readBuffer))
	var offset int64
	for {
		// Read the next incoming chunk and require the complete source length.
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

		// Read the matching range from the existing file for comparison.
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

		// Overlay the incoming blob when existing file content differs.
		if !bytes.Equal(readChunk[:len(compareChunk)], compareChunk) {
			// TruncateFile already matched the source size above, so writing the
			// incoming blob as an overlay preserves the per-op range history
			// without leaving stale tail ranges behind it.
			return writeEntryContents(ctx, root, path, e, false)
		}

		// Advance the comparison until the incoming blob reaches EOF.
		offset += int64(len(readChunk))
		if err == io.EOF {
			break
		}
	}
	return nil
}

// writeEntryContents writes the contents of the FILE entry e at the start of
// the file at path: its held bytes, or its blob as a range of the file.
// forceUseBlob keeps the blob as its own range instead of merging it with the
// range before it.
func writeEntryContents(ctx context.Context, root *unixfs_block.FSTree, path []string, e *pendingEntry, forceUseBlob bool) error {
	switch {
	case e.data != nil:
		return unixfs_block.WriteAt(ctx, root, nil, path, 0, int64(len(e.data)), bytes.NewReader(e.data), e.ts)
	case !e.blobRef.GetEmpty():
		return unixfs_block.WriteBlob(ctx, root, path, 0, e.blobRef, false, forceUseBlob, e.ts)
	}
	return nil
}

// Release discards any accumulated state without committing. Safe to call
// after Commit. After Release the writer rejects all further calls.
//
// Buffered blobs are dropped. Blobs the buffer already drained for capacity
// are released with the writer's stage for block-store GC to reclaim.
func (b *BatchFSWriter) Release() {
	// Discard the pending entries and storage, then record the released batch.
	pendingDirs, pendingEntries := b.pendingMetricCounts()
	b.released = true
	b.pending = nil
	b.releaseStorage()
	b.recordBatchFSWriterMetric(context.Background(), BatchFSWriterMetric{
		Stage:          "release",
		PendingDirs:    pendingDirs,
		PendingEntries: pendingEntries,
		Released:       true,
	})
}

// releaseStorage drops the blob buffer and releases the storage cursor and
// its stage.
func (b *BatchFSWriter) releaseStorage() {
	if b.storage != nil {
		b.storage.Release()
		b.storage = nil
	}
	if b.stage != nil {
		b.stage.Release()
		b.stage = nil
	}
	b.blobs = nil
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
	// Reuse the pending directory record or retain a copy of its parent path.
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
