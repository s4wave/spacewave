//go:build js

// Package metashard implements a metadata store backed by a B+tree page file
// in OPFS with dual superblocks and transactional commit.
package metashard

import (
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

// MetaShard is a metadata store backed by a single OPFS page file
// with dual superblocks and B+tree page store.
type MetaShard struct {
	dir        js.Value
	lockPrefix string
	pageSize   int
	pager      *OpfsPager

	mu         sync.RWMutex
	rootPage   pagestore.PageID
	generation uint64
	testHook   func(string) error
}

// NewMetaShard opens or creates a meta shard in the given OPFS directory.
func NewMetaShard(dir js.Value, lockPrefix string, pageSize int) (*MetaShard, error) {
	if pageSize == 0 {
		pageSize = pagestore.DefaultPageSize
	}

	pager := NewOpfsPager(dir, "pages.dat", pageSize)

	// Read superblocks.
	aBuf := make([]byte, pagestore.SuperblockSize)
	bBuf := make([]byte, pagestore.SuperblockSize)
	readSuper(dir, "super-a", aBuf)
	readSuper(dir, "super-b", bBuf)

	sb := pagestore.PickSuperblock(aBuf, bBuf)

	rootPage := pagestore.InvalidPage
	var gen uint64
	if sb != nil {
		pager.SetPageCount(sb.PageCount)
		rootPage = sb.RootPage
		gen = sb.Generation
		if err := pager.LoadFreelist(sb.FreelistPage); err != nil {
			return nil, errors.Wrap(err, "load freelist")
		}
	}

	return &MetaShard{
		dir:        dir,
		lockPrefix: lockPrefix,
		pageSize:   pageSize,
		pager:      pager,
		rootPage:   rootPage,
		generation: gen,
	}, nil
}

// Get looks up a key. Returns value, found, error.
func (ms *MetaShard) Get(key []byte) ([]byte, bool, error) {
	tree, _ := ms.OpenCommittedTree()
	return tree.Get(key)
}

// WriteTx executes a write transaction. The function fn receives the tree
// and may call Put/Delete. After fn returns, the transaction is committed
// by writing dirty pages and flipping the superblock.
func (ms *MetaShard) WriteTx(fn func(tree *pagestore.Tree) error) error {
	// Acquire write lock.
	release, err := filelock.AcquireWebLock(ms.lockPrefix+"/meta/write", true)
	if err != nil {
		return errors.Wrap(err, "acquire meta write lock")
	}
	defer release()

	tree, gen := ms.OpenCommittedTree()

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

	ms.mu.Lock()
	ms.rootPage = tree.RootID()
	ms.generation = gen
	ms.mu.Unlock()

	return nil
}

// ScanPrefix iterates over entries matching the prefix.
func (ms *MetaShard) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) error {
	tree, _ := ms.OpenCommittedTree()
	return tree.ScanPrefix(prefix, fn)
}

// Generation returns the current commit generation.
func (ms *MetaShard) Generation() uint64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.generation
}

// OpenCommittedTree opens a tree at the currently committed root.
func (ms *MetaShard) OpenCommittedTree() (*pagestore.Tree, uint64) {
	ms.mu.RLock()
	rootPage := ms.rootPage
	generation := ms.generation
	ms.mu.RUnlock()
	return pagestore.OpenTree(ms.pager, rootPage), generation
}

func (ms *MetaShard) callTestHook(stage string) error {
	if ms.testHook != nil {
		return ms.testHook(stage)
	}
	return nil
}

// readSuper reads a superblock file into buf, ignoring errors.
func readSuper(dir js.Value, name string, buf []byte) {
	f, err := opfs.OpenAsyncFile(dir, name)
	if err != nil {
		return
	}
	f.ReadAt(buf, 0)
}

// writeSuper writes a superblock to OPFS.
func writeSuper(dir js.Value, name string, data []byte) error {
	if !opfs.SyncAvailable() {
		return opfs.WriteFile(dir, name, data)
	}
	f, err := opfs.CreateSyncFile(dir, name)
	if err != nil {
		return err
	}
	f.Truncate(0)
	if _, err := f.WriteAt(data, 0); err != nil {
		f.Close()
		return err
	}
	f.Flush()
	return f.Close()
}
