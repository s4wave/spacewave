//go:build js

// Package metashard implements a metadata store backed by a B+tree page file
// in OPFS with dual superblocks and transactional commit.
package metashard

import (
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/aperturerobotics/hydra/volume/js/opfs/pagestore"
	"github.com/pkg/errors"
)

// MetaShard is a metadata store backed by a single OPFS page file
// with dual superblocks and B+tree page store.
type MetaShard struct {
	dir        js.Value
	lockPrefix string
	pageSize   int
	pager      *OpfsPager
	tree       *pagestore.Tree
	generation uint64
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

	var tree *pagestore.Tree
	var gen uint64
	if sb != nil {
		pager.SetPageCount(sb.PageCount)
		tree = pagestore.OpenTree(pager, sb.RootPage)
		gen = sb.Generation
	} else {
		tree = pagestore.NewTree(pager)
	}

	return &MetaShard{
		dir:        dir,
		lockPrefix: lockPrefix,
		pageSize:   pageSize,
		pager:      pager,
		tree:       tree,
		generation: gen,
	}, nil
}

// Get looks up a key. Returns value, found, error.
func (ms *MetaShard) Get(key []byte) ([]byte, bool, error) {
	return ms.tree.Get(key)
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

	// Execute mutations.
	if err := fn(ms.tree); err != nil {
		return err
	}

	// Flush dirty pages (pager writes through).
	// Commit: flip superblock.
	ms.generation++
	sb := pagestore.Superblock{
		Magic:        pagestore.SuperblockMagic,
		Version:      1,
		Generation:   ms.generation,
		RootPage:     ms.tree.RootID(),
		FreelistPage: pagestore.InvalidPage,
		PageCount:    ms.pager.PageCount(),
	}

	slot := "super-a"
	if ms.generation%2 == 0 {
		slot = "super-b"
	}
	var sbBuf [pagestore.SuperblockSize]byte
	pagestore.EncodeSuperblock(sbBuf[:], &sb)

	if err := writeSuper(ms.dir, slot, sbBuf[:]); err != nil {
		return errors.Wrap(err, "write superblock")
	}

	return nil
}

// ScanPrefix iterates over entries matching the prefix.
func (ms *MetaShard) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) error {
	return ms.tree.ScanPrefix(prefix, fn)
}

// Generation returns the current commit generation.
func (ms *MetaShard) Generation() uint64 {
	return ms.generation
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
