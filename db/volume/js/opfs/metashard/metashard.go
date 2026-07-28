//go:build js

// Package metashard implements a metadata store backed by a B+tree page file
// in OPFS with dual superblocks and transactional commit.
package metashard

import (
	"bytes"
	"context"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
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
	testHook   func(string) error
}

// NewMetaShard opens or creates a meta shard in the given OPFS directory.
func NewMetaShard(dir js.Value, lockPrefix string, pageSize int, le *logrus.Entry) (*MetaShard, error) {
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
	release, err := ms.acquireStateLock(false)
	if err != nil {
		return nil, errors.Wrap(err, "acquire meta read lock")
	}
	err = ms.reloadCommittedState()
	release()
	if err != nil {
		if !IsCorruptError(err) {
			return nil, err
		}
		if err := ms.recoverCorruptState(); err != nil {
			return nil, err
		}
	}
	return ms, nil
}

// Get looks up a key. Returns value, found, error.
func (ms *MetaShard) Get(key []byte) ([]byte, bool, error) {
	tree, _, closeSnapshot, releaseLock, err := ms.openCommittedTreeForRead()
	if err != nil {
		return nil, false, err
	}
	val, found, err := tree.Get(key)
	closeSnapshot()
	releaseLock()
	if err == nil || !IsCorruptError(err) {
		return val, found, err
	}
	if err := ms.recoverCorruptState(); err != nil {
		return nil, false, errors.Wrap(err, "recover corrupt meta shard")
	}
	tree, _, closeSnapshot, releaseLock, err = ms.openCommittedTreeForRead()
	if err != nil {
		return nil, false, err
	}
	defer closeSnapshot()
	defer releaseLock()
	return tree.Get(key)
}

// WriteTx executes a write transaction. The function fn receives the tree
// and may call Put/Delete. After fn returns, the transaction is committed
// by writing dirty pages and flipping the superblock.
func (ms *MetaShard) WriteTx(fn func(tree *pagestore.Tree) error) error {
	// Acquire write lock.
	release, err := ms.acquireStateLock(true)
	if err != nil {
		return errors.Wrap(err, "acquire meta write lock")
	}
	defer release()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if err := ms.reloadCommittedStateLocked(); err != nil {
		if !IsCorruptError(err) {
			return errors.Wrap(err, "reload committed state")
		}
		if err := ms.resetCorruptStateLocked(err); err != nil {
			return errors.Wrap(err, "reset corrupt meta shard")
		}
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
	ms.pager.Flush()
	if err := ms.pager.Close(); err != nil {
		return errors.Wrap(err, "close page file before superblock flip")
	}
	if err := ms.callTestHook("after-page-close"); err != nil {
		return err
	}

	gen++
	sb := pagestore.Superblock{
		Magic:        pagestore.SuperblockMagic,
		Version:      1,
		Generation:   gen,
		RootPage:     tree.RootID(),
		FreelistPage: freelistPage,
		PageCount:    ms.pager.PageCount(),
	}

	slot := "super-a"
	if gen%2 == 0 {
		slot = "super-b"
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

	return nil
}

// ScanPrefix iterates over entries matching the prefix.
func (ms *MetaShard) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) error {
	tree, _, closeSnapshot, releaseLock, err := ms.openCommittedTreeForRead()
	if err != nil {
		return err
	}
	entries, err := scanPrefixEntries(tree, prefix)
	closeSnapshot()
	releaseLock()
	if err == nil || !IsCorruptError(err) {
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
	if err := ms.recoverCorruptState(); err != nil {
		return errors.Wrap(err, "recover corrupt meta shard")
	}
	tree, _, closeSnapshot, releaseLock, err = ms.openCommittedTreeForRead()
	if err != nil {
		return err
	}
	defer closeSnapshot()
	defer releaseLock()
	entries, err = scanPrefixEntries(tree, prefix)
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

// Generation returns the current commit generation.
func (ms *MetaShard) Generation() uint64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.generation
}

// RefreshGeneration reloads the committed superblock and returns its generation.
func (ms *MetaShard) RefreshGeneration() (uint64, error) {
	return ms.RefreshGenerationContext(context.Background())
}

// RefreshGenerationContext reloads the committed superblock and returns its generation.
func (ms *MetaShard) RefreshGenerationContext(ctx context.Context) (uint64, error) {
	release, err := filelock.AcquireWebLockContext(ctx, ms.lockPrefix+"/meta/write", false)
	if err != nil {
		return 0, errors.Wrap(err, "acquire meta read lock")
	}

	if err := ms.reloadCommittedState(); err != nil {
		release()
		if !IsCorruptError(err) {
			return 0, errors.Wrap(err, "reload committed state")
		}
		if err := ms.recoverCorruptState(); err != nil {
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

func (ms *MetaShard) openCommittedTreeForRead() (*pagestore.Tree, uint64, func(), func(), error) {
	releaseLock, err := ms.acquireStateLock(false)
	if err != nil {
		return nil, 0, nil, nil, errors.Wrap(err, "acquire meta read lock")
	}
	if err := ms.reloadCommittedState(); err != nil {
		releaseLock()
		if !IsCorruptError(err) {
			return nil, 0, nil, nil, errors.Wrap(err, "reload committed state")
		}
		if err := ms.recoverCorruptState(); err != nil {
			return nil, 0, nil, nil, errors.Wrap(err, "recover corrupt meta shard")
		}
		releaseLock, err = ms.acquireStateLock(false)
		if err != nil {
			return nil, 0, nil, nil, errors.Wrap(err, "reacquire meta read lock")
		}
		if err := ms.reloadCommittedState(); err != nil {
			releaseLock()
			return nil, 0, nil, nil, errors.Wrap(err, "reload recovered state")
		}
	}
	tree, generation, closeSnapshot := ms.openCommittedSnapshotTree()
	return tree, generation, closeSnapshot, releaseLock, nil
}

func (ms *MetaShard) reloadCommittedState() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	return ms.reloadCommittedStateLocked()
}

func (ms *MetaShard) reloadCommittedStateLocked() error {
	var aBuf [pagestore.SuperblockSize]byte
	var bBuf [pagestore.SuperblockSize]byte
	if err := readSuper(ms.dir, "super-a", aBuf[:]); err != nil {
		return err
	}
	if err := readSuper(ms.dir, "super-b", bBuf[:]); err != nil {
		return err
	}

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
	return nil
}

func (ms *MetaShard) recoverCorruptState() error {
	release, err := ms.acquireStateLock(true)
	if err != nil {
		return errors.Wrap(err, "acquire meta write lock")
	}
	defer release()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if err := ms.reloadCommittedStateLocked(); err == nil {
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

func (ms *MetaShard) acquireStateLock(exclusive bool) (func(), error) {
	release, err := filelock.AcquireWebLock(ms.lockPrefix+"/meta/write", exclusive)
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
