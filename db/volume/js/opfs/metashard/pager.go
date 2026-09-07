//go:build js

package metashard

import (
	"io"
	"slices"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

// OpfsPager implements pagestore.Pager backed by a single OPFS file.
// Pages are stored at offset = pageID * pageSize.
type OpfsPager struct {
	// dir contains the page file.
	dir js.Value
	// filename identifies the page file within dir.
	filename string
	// pgSize is the fixed byte size of each page.
	pgSize int
	// pageCount is the allocation high-water mark.
	pageCount uint32
	// freed contains page IDs available for reuse.
	freed []pagestore.PageID
	// syncFile is opened lazily for synchronous writes.
	syncFile *opfs.SyncFile
	// asyncFile is opened lazily for asynchronous reads or writes.
	asyncFile *opfs.AsyncFile
	// freelistPages holds the pages encoding the current free-page chain.
	freelistPages []pagestore.PageID
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
//
// In the async path the AsyncFile handle is opened lazily and cached on the
// pager so subsequent ReadPage calls reuse it. Opening a fresh handle on every
// page read costs an awaited getFileHandle round-trip per call, which turns
// each B+tree traversal into O(depth) avoidable Promise hops.
func (p *OpfsPager) ReadPage(id pagestore.PageID, buf []byte) error {
	// Reuse an existing synchronous handle when available.
	clear(buf)
	off := int64(id) * int64(p.pgSize)
	if p.syncFile != nil {
		n, err := p.syncFile.ReadAt(buf[:p.pgSize], off)
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "read page")
		}
		if n != p.pgSize {
			return errors.Errorf("short read page %d: got %d want %d", id, n, p.pgSize)
		}
		return nil
	}

	// Retain one asynchronous handle across this pager's reads.
	if p.asyncFile == nil {
		f, err := opfs.OpenAsyncFile(p.dir, p.filename)
		if err != nil {
			return errors.Wrap(err, "open page file for read")
		}
		p.asyncFile = f
	}

	// Require one complete page from the asynchronous handle.
	n, err := p.asyncFile.ReadAt(buf[:p.pgSize], off)
	if err != nil && err != io.EOF {
		return errors.Wrap(err, "read page")
	}
	if n != p.pgSize {
		return errors.Errorf("short read page %d: got %d want %d", id, n, p.pgSize)
	}
	return nil
}

// WritePage writes a page. Uses a sync handle when preferred, async otherwise.
func (p *OpfsPager) WritePage(id pagestore.PageID, buf []byte) error {
	// Retain a writable handle for the selected runtime driver.
	if opfs.PreferSyncAccessHandles() {
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

	// The asynchronous driver buffers writes until its handle closes.
	if p.asyncFile == nil {
		f, err := opfs.CreateAsyncFile(p.dir, p.filename)
		if err != nil {
			return errors.Wrap(err, "open page file for async write")
		}
		p.asyncFile = f
	}
	off := int64(id) * int64(p.pgSize)
	_, err := p.asyncFile.WriteAt(buf[:p.pgSize], off)
	return err
}

// AllocPage returns the next free page ID.
func (p *OpfsPager) AllocPage() pagestore.PageID {
	// Reuse freed pages before extending the allocation high-water mark.
	if len(p.freed) > 0 {
		id := p.freed[len(p.freed)-1]
		p.freed = p.freed[:len(p.freed)-1]
		return id
	}

	// Reserve the next page beyond all previous allocations.
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

// Close closes any open page-file handles.
//
// Both asyncFile and syncFile may be populated at once: ReadPage opens
// asyncFile lazily and WritePage opens syncFile when sync access is
// available. Close releases both and reports the first error.
func (p *OpfsPager) Close() error {
	var firstErr error
	if p.asyncFile != nil {
		if err := p.asyncFile.Close(); err != nil {
			firstErr = err
		}
		p.asyncFile = nil
	}
	if p.syncFile != nil {
		if err := p.syncFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.syncFile = nil
	}
	return firstErr
}

// LoadFreelist restores the free-page state from the committed freelist chain.
func (p *OpfsPager) LoadFreelist(root pagestore.PageID) error {
	// Replace the previous allocation state with the committed chain.
	p.freed = nil
	p.freelistPages = nil
	if root == pagestore.InvalidPage {
		return nil
	}

	// Follow and decode every page in the committed chain.
	buf := make([]byte, p.pgSize)
	pageID := root
	for pageID != pagestore.InvalidPage {
		if err := p.ReadPage(pageID, buf); err != nil {
			return errors.Wrap(err, "read freelist page")
		}
		nextPage, ids, err := pagestore.DecodeFreelistPage(buf)
		if err != nil {
			return errors.Wrap(err, "decode freelist page")
		}
		p.freelistPages = append(p.freelistPages, pageID)
		p.freed = append(p.freed, ids...)
		pageID = nextPage
	}
	return nil
}

// PersistFreelist writes the current free-page state to freelist pages.
// Returns the root freelist page ID, or InvalidPage if the freelist is empty.
func (p *OpfsPager) PersistFreelist() (pagestore.PageID, error) {
	// Recycle the previous chain before allocating its replacement.
	if len(p.freelistPages) > 0 {
		p.freed = append(p.freed, p.freelistPages...)
		p.freelistPages = nil
	}
	if len(p.freed) == 0 {
		return pagestore.InvalidPage, nil
	}

	// Require room for at least one free-page ID per chain page.
	capacity := pagestore.FreelistPageCapacity(p.pgSize)
	if capacity < 1 {
		return pagestore.InvalidPage, errors.New("page size too small for freelist")
	}

	// Allocate the replacement chain beyond all existing pages.
	freed := slices.Clone(p.freed)
	pageCount := (len(freed) + capacity - 1) / capacity
	pages := make([]pagestore.PageID, pageCount)
	for i := range pages {
		pages[i] = pagestore.PageID(p.pageCount)
		p.pageCount++
	}

	// Write the chain backward so each link points to an assigned page.
	buf := make([]byte, p.pgSize)
	off := 0
	for i, page := range slices.Backward(pages) {
		nextPage := pagestore.InvalidPage
		if i+1 < len(pages) {
			nextPage = pages[i+1]
		}
		clear(buf)
		written := pagestore.EncodeFreelistPage(buf, nextPage, freed[off:])
		if written == 0 {
			return pagestore.InvalidPage, errors.New("freelist page wrote zero entries")
		}
		if err := p.WritePage(page, buf); err != nil {
			return pagestore.InvalidPage, errors.Wrap(err, "write freelist page")
		}
		off += written
	}

	// Retain the chain for reuse by the next commit.
	p.freelistPages = pages
	return pages[0], nil
}

// _ is a type assertion.
var _ pagestore.Pager = (*OpfsPager)(nil)
