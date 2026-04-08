//go:build js

package metashard

import (
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/pagestore"
	"github.com/pkg/errors"
)

// OpfsPager implements pagestore.Pager backed by a single OPFS file.
// Pages are stored at offset = pageID * pageSize.
type OpfsPager struct {
	dir       js.Value
	filename  string
	pgSize    int
	pageCount uint32
	freed     []pagestore.PageID
	// syncFile is opened lazily on first write.
	syncFile *opfs.SyncFile
}

// NewOpfsPager creates a pager backed by an OPFS file.
func NewOpfsPager(dir js.Value, filename string, pageSize int) *OpfsPager {
	return &OpfsPager{
		dir:      dir,
		filename: filename,
		pgSize:   pageSize,
	}
}

// SetPageCount sets the initial page count (from superblock recovery).
func (p *OpfsPager) SetPageCount(count uint32) {
	p.pageCount = count
}

// PageSize returns the page size.
func (p *OpfsPager) PageSize() int { return p.pgSize }

// ReadPage reads a page by ID.
func (p *OpfsPager) ReadPage(id pagestore.PageID, buf []byte) error {
	off := int64(id) * int64(p.pgSize)
	// Try async read first (no lock needed for sealed pages).
	f, err := opfs.OpenAsyncFile(p.dir, p.filename)
	if err != nil {
		if opfs.IsNotFound(err) {
			for i := range buf {
				buf[i] = 0
			}
			return nil
		}
		return errors.Wrap(err, "open page file for read")
	}
	_, err = f.ReadAt(buf[:p.pgSize], off)
	return err
}

// WritePage writes a page. Uses a sync file handle (opened lazily).
func (p *OpfsPager) WritePage(id pagestore.PageID, buf []byte) error {
	if p.syncFile == nil {
		f, err := opfs.CreateSyncFile(p.dir, p.filename)
		if err != nil {
			return errors.Wrap(err, "open page file for write")
		}
		p.syncFile = f
	}
	off := int64(id) * int64(p.pgSize)
	_, err := p.syncFile.WriteAt(buf[:p.pgSize], off)
	return err
}

// AllocPage returns the next free page ID.
func (p *OpfsPager) AllocPage() pagestore.PageID {
	if len(p.freed) > 0 {
		id := p.freed[len(p.freed)-1]
		p.freed = p.freed[:len(p.freed)-1]
		return id
	}
	id := pagestore.PageID(p.pageCount)
	p.pageCount++
	return id
}

// FreePage returns a page to the free pool.
func (p *OpfsPager) FreePage(id pagestore.PageID) {
	p.freed = append(p.freed, id)
}

// PageCount returns the total number of allocated pages.
func (p *OpfsPager) PageCount() uint32 { return p.pageCount }

// Flush flushes the sync file handle if open.
func (p *OpfsPager) Flush() {
	if p.syncFile != nil {
		p.syncFile.Flush()
	}
}

// Close closes the sync file handle if open.
func (p *OpfsPager) Close() error {
	if p.syncFile != nil {
		err := p.syncFile.Close()
		p.syncFile = nil
		return err
	}
	return nil
}

// _ is a type assertion.
var _ pagestore.Pager = (*OpfsPager)(nil)
