//go:build js

// Package metashard implements a metadata store backed by a B+tree page file
// in OPFS with dual superblocks and transactional commit.
package metashard

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
	"github.com/sirupsen/logrus"
)

// MetaShard is a metadata store backed by a single OPFS page file
// with dual superblocks and B+tree page store.
type MetaShard struct {
	dir        js.Value
	lockPrefix string
	pageSize   int
	pager      *OpfsPager
	le         *logrus.Entry

	mu         sync.RWMutex
	rootPage   pagestore.PageID
	generation uint64
	// stateLoaded reports whether rootPage and generation describe a superblock
	// this process has read and validated. It distinguishes an empty shard from
	// one that has never been loaded, which both carry generation zero.
	stateLoaded bool
	// loadedSupers holds the super-a and super-b bytes this state was loaded
	// from, and is what the reload shortcut compares against. Both slots are
	// kept rather than the chosen one, because validation may reject the newer
	// slot and fall back to the older: a shortcut that named only the chosen
	// state would then miss on every read while the rejected slot sits on disk.
	// The bytes identify the state rather than merely dating it, which matters
	// because corruption recovery deletes the page file and creates a new
	// database in its place: comparing the root page, page count, and freelist
	// alongside the generation is what keeps a replacement from passing for the
	// state it replaced.
	loadedSupers [2][pagestore.SuperblockSize]byte
	// revalidations counts full read-validate-rebuild passes, so a test can
	// assert that a run of reads over an unchanged shard performs one.
	revalidations uint64
	testHook      func(string) error
	// resetGenerationFloor is the greatest generation this process has seen
	// in validated state or decoded from on-disk superblocks before reset.
	resetGenerationFloor uint64
}

// NewMetaShard opens or creates a meta shard in the given OPFS directory.
func NewMetaShard(dir js.Value, lockPrefix string, pageSize int, le *logrus.Entry) (*MetaShard, error) {
	ctx, task := trace.NewTask(context.Background(), "hydra/metashard/open")
	defer task.End()
	if pageSize == 0 {
		pageSize = pagestore.DefaultPageSize
	}

	ms := &MetaShard{
		dir:        dir,
		lockPrefix: lockPrefix,
		pageSize:   pageSize,
		pager:      NewOpfsPager(dir, "pages.dat", pageSize),
		le:         le,
		rootPage:   pagestore.InvalidPage,
	}
	_, lockTask := trace.NewTask(ctx, "hydra/metashard/open/acquire-state-lock")
	release, err := ms.acquireStateLock(context.Background(), false)
	if err != nil {
		lockTask.End()
		return nil, errors.Wrap(err, "acquire meta read lock")
	}
	lockTask.End()
	err = ms.reloadCommittedState()
	release()
	if err != nil {
		if !IsCorruptError(err) {
			return nil, err
		}
		if err := ms.recoverCorruptState(context.Background()); err != nil {
			return nil, err
		}
	}
	return ms, nil
}

// Get looks up a key. Returns value, found, error.
func (ms *MetaShard) Get(key []byte) ([]byte, bool, error) {
	val, found, _, err := ms.getAt(context.Background(), key)
	return val, found, err
}

// getAt looks up a key and reports the commit generation that served it, so a
// caller spanning several reads can tell whether they came from one generation.
func (ms *MetaShard) getAt(ctx context.Context, key []byte) ([]byte, bool, uint64, error) {
	tree, generation, release, err := ms.openCommittedTreeForRead(ctx)
	if err != nil {
		return nil, false, 0, err
	}
	val, found, err := tree.Get(key)
	release()
	if err == nil || !IsCorruptError(err) {
		return val, found, generation, err
	}
	if err := ms.recoverCorruptState(ctx); err != nil {
		return nil, false, 0, errors.Wrap(err, "recover corrupt meta shard")
	}
	tree, generation, release, err = ms.openCommittedTreeForRead(ctx)
	if err != nil {
		return nil, false, 0, err
	}
	defer release()
	val, found, err = tree.Get(key)
	return val, found, generation, err
}

// WriteTx executes a write transaction. The function fn receives the tree
// and may call Put/Delete. After fn returns, the transaction is committed
// by writing dirty pages and flipping the superblock.
func (ms *MetaShard) WriteTx(fn func(tree *pagestore.Tree) error) error {
	ctx, task := trace.NewTask(context.Background(), "hydra/metashard/write-tx")
	defer task.End()
	// Acquire write lock.
	release, err := ms.acquireStateLock(context.Background(), true)
	if err != nil {
		return errors.Wrap(err, "acquire meta write lock")
	}
	defer release()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if err := ms.reloadCommittedStateLocked(true); err != nil {
		if !IsCorruptError(err) {
			return errors.Wrap(err, "reload committed state")
		}
		if err := ms.resetCorruptStateLocked(err); err != nil {
			return errors.Wrap(err, "reset corrupt meta shard")
		}
	}
	if ms.generation == math.MaxUint64 {
		return &generationFloorError{generation: ms.generation}
	}
	tree, gen := ms.openCommittedTreeLocked()

	// Execute mutations.
	if err := fn(tree); err != nil {
		if closeErr := ms.pager.Close(); closeErr != nil {
			return errors.Wrapf(closeErr, "close page file after failed write tx (%v)", err)
		}
		return err
	}
	if err := ms.callTestHook("after-mutate"); err != nil {
		if closeErr := ms.pager.Close(); closeErr != nil {
			return errors.Wrapf(closeErr, "close page file after test hook (%v)", err)
		}
		return err
	}

	// Commit ordering:
	// 1. All mutated pages are written into pages.dat through the pager.
	// 2. Flush and close pages.dat so the new root never points at not-yet-
	//    durable page bytes.
	// 3. Flip the alternate superblock.
	freelistPage, err := ms.pager.PersistFreelist()
	if err != nil {
		return errors.Wrap(err, "persist freelist")
	}
	if err := ms.callTestHook("after-freelist"); err != nil {
		if closeErr := ms.pager.Close(); closeErr != nil {
			return errors.Wrapf(closeErr, "close page file after test hook (%v)", err)
		}
		return err
	}
	{
		_, subtask := trace.NewTask(ctx, "hydra/metashard/write-tx/flush-pages")
		ms.pager.Flush()
		subtask.End()
	}
	if err := ms.pager.Close(); err != nil {
		return errors.Wrap(err, "close page file before superblock flip")
	}
	if err := ms.callTestHook("after-page-close"); err != nil {
		return err
	}

	gen++
	if gen == 1 {
		// Nothing was committed before this, so this commit creates the
		// database. Start its generations at a fresh epoch instead of at one,
		// because corruption recovery deletes the page file and creates a new
		// database in its place. Counting from one again would let the
		// replacement publish generations the database it replaced had already
		// published, and another instance holding cached state decides whether
		// that state is current by comparing what it loaded against what is on
		// disk.
		gen, err = newGenerationEpoch()
		if err != nil {
			return err
		}
		// A random epoch lands below the replaced database about half the time,
		// and a replacement that publishes lower generations is the same stale
		// cache problem the epoch exists to prevent. Continue from just above the
		// floor when the draw does not clear it. That stays inside the replaced
		// database's epoch, which is harmless: every generation it produces is
		// above every generation that database published.
		if gen <= ms.resetGenerationFloor {
			if ms.resetGenerationFloor == math.MaxUint64 {
				return &generationFloorError{generation: ms.resetGenerationFloor}
			}
			gen = ms.resetGenerationFloor + 1
		}
	}
	sb := pagestore.Superblock{
		Magic:        pagestore.SuperblockMagic,
		Version:      1,
		Generation:   gen,
		RootPage:     tree.RootID(),
		FreelistPage: freelistPage,
		PageCount:    ms.pager.PageCount(),
	}

	slot, slotIndex := "super-a", 0
	if gen%2 == 0 {
		slot, slotIndex = "super-b", 1
	}
	var sbBuf [pagestore.SuperblockSize]byte
	pagestore.EncodeSuperblock(sbBuf[:], &sb)

	if err := writeSuper(ms.dir, slot, sbBuf[:]); err != nil {
		return errors.Wrap(err, "write superblock")
	}
	if err := ms.callTestHook("after-superblock-write"); err != nil {
		return err
	}

	ms.rootPage = tree.RootID()
	ms.generation = gen
	// The other slot still holds what the reload before this write read, so
	// updating the written one keeps the pair equal to what is on disk.
	ms.loadedSupers[slotIndex] = sbBuf

	return nil
}

// ScanPrefix iterates over entries matching the prefix.
func (ms *MetaShard) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) error {
	entries, err := ms.collectPrefix(context.Background(), prefix)
	if err != nil {
		return err
	}
	for i := range entries {
		if !fn(entries[i].key, entries[i].value) {
			return nil
		}
	}
	return nil
}

// collectPrefix materializes every committed entry under prefix. The walk runs
// entirely inside one shared-lock hold, so the returned entries are a
// consistent view of one generation.
func (ms *MetaShard) collectPrefix(ctx context.Context, prefix []byte) ([]metaEntry, error) {
	entries, _, err := ms.collectPrefixAt(ctx, prefix)
	return entries, err
}

// collectPrefixAt materializes every committed entry under prefix and reports
// the commit generation that served them.
func (ms *MetaShard) collectPrefixAt(ctx context.Context, prefix []byte) ([]metaEntry, uint64, error) {
	tree, generation, release, err := ms.openCommittedTreeForRead(ctx)
	if err != nil {
		return nil, 0, err
	}
	entries, err := scanPrefixEntries(tree, prefix)
	release()
	if err == nil || !IsCorruptError(err) {
		return entries, generation, err
	}
	if err := ms.recoverCorruptState(ctx); err != nil {
		return nil, 0, errors.Wrap(err, "recover corrupt meta shard")
	}
	tree, generation, release, err = ms.openCommittedTreeForRead(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	entries, err = scanPrefixEntries(tree, prefix)
	return entries, generation, err
}

// Generation returns the current commit generation.
func (ms *MetaShard) Generation() uint64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.generation
}

// Revalidations reports how many times this shard has walked the committed
// tree to validate it. The walk is O(tree) and holds the shared metadata lock,
// so a run of reads over a shard nothing has committed to costs one.
func (ms *MetaShard) Revalidations() uint64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.revalidations
}

// RefreshGeneration reloads the committed superblock and returns its generation.
func (ms *MetaShard) RefreshGeneration() (uint64, error) {
	return ms.RefreshGenerationContext(context.Background())
}

// RefreshGenerationContext reloads the committed superblock and returns its generation.
func (ms *MetaShard) RefreshGenerationContext(ctx context.Context) (uint64, error) {
	release, err := ms.acquireStateLock(ctx, false)
	if err != nil {
		return 0, errors.Wrap(err, "acquire meta read lock")
	}

	if err := ms.reloadCommittedState(); err != nil {
		release()
		if !IsCorruptError(err) {
			return 0, errors.Wrap(err, "reload committed state")
		}
		if err := ms.recoverCorruptState(ctx); err != nil {
			return 0, errors.Wrap(err, "recover corrupt meta shard")
		}
		return ms.Generation(), nil
	}
	release()
	return ms.Generation(), nil
}

// Close releases open metadata page-file handles.
func (ms *MetaShard) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.pager.Close()
}

// OpenCommittedTree opens a tree at the currently committed root.
func (ms *MetaShard) OpenCommittedTree() (*pagestore.Tree, uint64) {
	ms.mu.RLock()
	rootPage := ms.rootPage
	generation := ms.generation
	ms.mu.RUnlock()
	return pagestore.OpenTree(ms.pager, rootPage), generation
}

// openCommittedTreeForRead opens the committed snapshot under the shared
// metadata lock. The returned release closes the snapshot and drops the lock,
// and the caller must hold it for the whole tree walk: a commit recycles freed
// pages immediately, so a walk that overlaps one reads pages that now belong to
// another subtree and silently reports missing keys.
func (ms *MetaShard) openCommittedTreeForRead(ctx context.Context) (*pagestore.Tree, uint64, func(), error) {
	releaseLock, err := ms.acquireStateLock(ctx, false)
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "acquire meta read lock")
	}
	if err := ms.reloadCommittedState(); err != nil {
		releaseLock()
		if !IsCorruptError(err) {
			return nil, 0, nil, errors.Wrap(err, "reload committed state")
		}
		if err := ms.recoverCorruptState(ctx); err != nil {
			return nil, 0, nil, errors.Wrap(err, "recover corrupt meta shard")
		}
		releaseLock, err = ms.acquireStateLock(ctx, false)
		if err != nil {
			return nil, 0, nil, errors.Wrap(err, "reacquire meta read lock")
		}
		if err := ms.reloadCommittedState(); err != nil {
			releaseLock()
			return nil, 0, nil, errors.Wrap(err, "reload recovered state")
		}
	}
	tree, generation, closeSnapshot := ms.openCommittedSnapshotTree()
	return tree, generation, func() {
		closeSnapshot()
		releaseLock()
	}, nil
}

func (ms *MetaShard) reloadCommittedState() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	return ms.reloadCommittedStateLocked(false)
}

// reloadCommittedStateLocked brings rootPage, generation, and the pager up to
// the newest valid superblock on disk.
//
// revalidate forces the full read-validate-rebuild path even when the newest
// superblock is the generation already loaded. Readers pass false: they only
// need to know whether another agent has committed since. Writers and
// corruption recovery pass true, because they go on to allocate pages from the
// freelist and must rebuild it from the committed chain rather than trust the
// in-memory copy left behind by an earlier commit.
func (ms *MetaShard) reloadCommittedStateLocked(revalidate bool) error {
	var aBuf [pagestore.SuperblockSize]byte
	var bBuf [pagestore.SuperblockSize]byte
	if err := readSuper(ms.dir, "super-a", aBuf[:]); err != nil {
		return err
	}
	if err := readSuper(ms.dir, "super-b", bBuf[:]); err != nil {
		return err
	}

	// A decodable generation is evidence about the database on disk even when
	// validation later rejects the page tree behind it.
	for _, buf := range [][pagestore.SuperblockSize]byte{aBuf, bBuf} {
		if sb, err := pagestore.DecodeSuperblock(buf[:]); err == nil &&
			sb.Generation > ms.resetGenerationFloor {
			ms.resetGenerationFloor = sb.Generation
		}
	}

	// Every read operation reloads, because another agent may have committed
	// since the last one. Reloading in full costs a whole-tree validation walk
	// and a pager rebuild that drops the page cache, so a point read would cost
	// O(tree) and a run of M reads O(M*tree). The superblocks themselves say
	// whether any of that is necessary: when both are byte for byte the ones
	// this state was loaded from, nothing has been committed since, the state in
	// hand is that state, and it was validated when it was loaded.

	if !revalidate && ms.stateLoaded &&
		aBuf == ms.loadedSupers[0] && bBuf == ms.loadedSupers[1] {
		return nil
	}
	ms.revalidations++

	validatePager := NewOpfsPager(ms.dir, "pages.dat", ms.pageSize)
	sb, err := pickValidSuperblock(validatePager, aBuf[:], bBuf[:])
	if closeErr := validatePager.Close(); closeErr != nil && err == nil {
		err = errors.Wrap(closeErr, "close validation pager")
	}
	if err != nil {
		return err
	}

	if err := ms.pager.Close(); err != nil {
		return errors.Wrap(err, "close committed pager")
	}
	ms.pager = NewOpfsPager(ms.dir, "pages.dat", ms.pageSize)

	rootPage := pagestore.InvalidPage
	var gen uint64
	if sb != nil {
		ms.pager.SetPageCount(sb.PageCount)
		if err := ms.pager.LoadFreelist(sb.FreelistPage); err != nil {
			return errors.Wrap(err, "load freelist")
		}
		rootPage = sb.RootPage
		gen = sb.Generation
	}
	if sb == nil {
		ms.pager.SetPageCount(0)
		if err := ms.pager.LoadFreelist(pagestore.InvalidPage); err != nil {
			return err
		}
	}

	ms.rootPage = rootPage
	ms.generation = gen
	ms.loadedSupers = [2][pagestore.SuperblockSize]byte{aBuf, bBuf}
	ms.stateLoaded = true
	return nil
}

// generationEpochShift splits a generation into a per-database epoch above it
// and a commit counter below it. The counter keeps 32 bits, far more commits
// than a metadata shard makes in the lifetime of a browser profile, and the
// epoch takes the other 32 at random, so two databases reaching the same commit
// count still carry different generations.
const generationEpochShift = 32

// newGenerationEpoch returns the first generation of a newly created database.
func newGenerationEpoch() (uint64, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, errors.Wrap(err, "read generation epoch")
	}
	return uint64(binary.BigEndian.Uint32(buf[:]))<<generationEpochShift | 1, nil
}

func (ms *MetaShard) recoverCorruptState(ctx context.Context) error {
	release, err := ms.acquireStateLock(ctx, true)
	if err != nil {
		return errors.Wrap(err, "acquire meta write lock")
	}
	defer release()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Recovery runs because a read of the loaded generation failed, so the
	// generation match that lets an ordinary reload skip validation is exactly
	// the condition under test here.
	if err := ms.reloadCommittedStateLocked(true); err == nil {
		return nil
	} else if !IsCorruptError(err) {
		return err
	} else {
		return ms.resetCorruptStateLocked(err)
	}
}

func (ms *MetaShard) openCommittedTreeLocked() (*pagestore.Tree, uint64) {
	return pagestore.OpenTree(ms.pager, ms.rootPage), ms.generation
}

func (ms *MetaShard) openCommittedSnapshotTree() (*pagestore.Tree, uint64, func()) {
	ms.mu.RLock()
	rootPage := ms.rootPage
	generation := ms.generation
	pageCount := ms.pager.PageCount()
	ms.mu.RUnlock()

	pager := NewOpfsPager(ms.dir, "pages.dat", ms.pageSize)
	pager.SetPageCount(pageCount)
	return pagestore.OpenTree(pager, rootPage), generation, func() {
		_ = pager.Close()
	}
}

func (ms *MetaShard) acquireStateLock(ctx context.Context, exclusive bool) (func(), error) {
	release, err := filelock.AcquireWebLockContext(ctx, ms.lockPrefix+"/meta/write", exclusive)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func scanPrefixEntries(tree *pagestore.Tree, prefix []byte) ([]metaEntry, error) {
	var entries []metaEntry
	err := tree.ScanPrefix(prefix, func(key, value []byte) bool {
		entries = append(entries, metaEntry{
			key:   bytes.Clone(key),
			value: bytes.Clone(value),
		})
		return true
	})
	return entries, err
}

func (ms *MetaShard) resetCorruptStateLocked(cause error) error {
	ms.le.WithError(cause).
		WithField("lock-prefix", ms.lockPrefix).
		Warn("resetting corrupt OPFS metadata")
	return ms.resetCommittedStateLocked()
}

func (ms *MetaShard) resetCommittedStateLocked() error {
	floor := ms.resetGenerationFloor
	if ms.generation > floor {
		floor = ms.generation
	}
	if floor == math.MaxUint64 {
		return &generationFloorError{generation: floor}
	}
	ms.resetGenerationFloor = floor
	if err := ms.pager.Close(); err != nil {
		return errors.Wrap(err, "close page file")
	}
	for _, name := range []string{"pages.dat", "super-a", "super-b"} {
		if err := opfs.DeleteFile(ms.dir, name); err != nil && !opfs.IsNotFound(err) {
			return errors.Wrapf(err, "delete %s", name)
		}
	}

	ms.pager = NewOpfsPager(ms.dir, "pages.dat", ms.pageSize)
	ms.pager.SetPageCount(0)
	if err := ms.pager.LoadFreelist(pagestore.InvalidPage); err != nil {
		return err
	}

	ms.rootPage = pagestore.InvalidPage
	ms.generation = 0
	ms.loadedSupers = [2][pagestore.SuperblockSize]byte{}
	return nil
}

func (ms *MetaShard) callTestHook(stage string) error {
	if ms.testHook != nil {
		return ms.testHook(stage)
	}
	return nil
}

func pickValidSuperblock(pager *OpfsPager, a, b []byte) (*pagestore.Superblock, error) {
	sa, errA := pagestore.DecodeSuperblock(a)
	sb, errB := pagestore.DecodeSuperblock(b)
	if errA != nil && errB != nil {
		return nil, nil
	}
	if errA != nil {
		return validateOnlySuperblock(pager, "super-b", sb)
	}
	if errB != nil {
		return validateOnlySuperblock(pager, "super-a", sa)
	}
	if sb.Generation > sa.Generation {
		valid, err := validateSuperblock(pager, sb)
		if err == nil {
			return valid, nil
		}
		fallback, fallbackErr := validateSuperblock(pager, sa)
		if fallbackErr == nil {
			return fallback, nil
		}
		return nil, NewCorruptError(errors.Errorf(
			"newest super-b invalid: %v; fallback super-a invalid: %v",
			err,
			fallbackErr,
		))
	}
	valid, err := validateSuperblock(pager, sa)
	if err == nil {
		return valid, nil
	}
	fallback, fallbackErr := validateSuperblock(pager, sb)
	if fallbackErr == nil {
		return fallback, nil
	}
	return nil, NewCorruptError(errors.Errorf(
		"newest super-a invalid: %v; fallback super-b invalid: %v",
		err,
		fallbackErr,
	))
}

func validateOnlySuperblock(
	pager *OpfsPager,
	slot string,
	sb *pagestore.Superblock,
) (*pagestore.Superblock, error) {
	valid, err := validateSuperblock(pager, sb)
	if err != nil {
		return nil, NewCorruptError(errors.Wrapf(err, "%s invalid", slot))
	}
	return valid, nil
}

func validateSuperblock(pager *OpfsPager, sb *pagestore.Superblock) (*pagestore.Superblock, error) {
	pager.SetPageCount(sb.PageCount)
	if err := pager.LoadFreelist(sb.FreelistPage); err != nil {
		return nil, errors.Wrap(err, "load freelist")
	}
	if sb.RootPage == pagestore.InvalidPage {
		return sb, nil
	}
	if uint32(sb.RootPage) >= sb.PageCount {
		return nil, errors.Errorf("root page %d outside page count %d", sb.RootPage, sb.PageCount)
	}
	tree := pagestore.OpenTree(pager, sb.RootPage)
	if err := tree.ScanPrefix(nil, func(_, _ []byte) bool { return true }); err != nil {
		return nil, errors.Wrapf(
			err,
			"validate page tree generation %d root page %d freelist page %d page count %d",
			sb.Generation,
			sb.RootPage,
			sb.FreelistPage,
			sb.PageCount,
		)
	}
	return sb, nil
}

// readSuper reads a superblock file through an async handle so concurrent
// shared metadata readers do not contend on exclusive sync access handles.
// A shard that has never committed has no superblock file, so a missing file
// leaves buf zeroed and reports success. Every other failure is returned,
// because a zeroed buffer is indistinguishable from generation zero and would
// otherwise hide committed state.
func readSuper(dir js.Value, name string, buf []byte) error {
	data, err := opfs.ReadFile(dir, name)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "read superblock %s", name)
	}
	copy(buf, data)
	return nil
}

// writeSuper writes a superblock to OPFS.
func writeSuper(dir js.Value, name string, data []byte) error {
	if !opfs.PreferSyncAccessHandles() {
		return opfs.WriteFile(dir, name, data)
	}
	f, err := opfs.CreateSyncFile(dir, name)
	if err != nil {
		return err
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		f.Close()
		return err
	}
	f.Flush()
	return f.Close()
}
