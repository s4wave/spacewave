//go:build js

package metashard

import (
	"io"
	"syscall/js"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
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
	// Files are opened lazily on first write.
	syncFile      *opfs.SyncFile
	asyncFile     *opfs.AsyncFile
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
func (p *OpfsPager) ReadPage(id pagestore.PageID, buf []byte) error {
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
	// Try async read first (no lock needed for sealed pages).
	f, err := opfs.OpenAsyncFile(p.dir, p.filename)
	if err != nil {
		return errors.Wrap(err, "open page file for read")
	}
	n, err := f.ReadAt(buf[:p.pgSize], off)
	if err != nil && err != io.EOF {
		return errors.Wrap(err, "read page")
	}
	if n != p.pgSize {
		return errors.Errorf("short read page %d: got %d want %d", id, n, p.pgSize)
	}
	return nil
}

// WritePage writes a page. Uses a sync handle when available, async otherwise.
func (p *OpfsPager) WritePage(id pagestore.PageID, buf []byte) error {
	if opfs.SyncAvailable() {
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
	if p.asyncFile != nil {
		err := p.asyncFile.Close()
		p.asyncFile = nil
		return err
	}
	if p.syncFile != nil {
		err := p.syncFile.Close()
		p.syncFile = nil
		return err
	}
	return nil
}

// LoadFreelist restores the free-page state from the committed freelist chain.
func (p *OpfsPager) LoadFreelist(root pagestore.PageID) error {
	p.freed = nil
	p.freelistPages = nil
	if root == pagestore.InvalidPage {
		return nil
	}

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
	if len(p.freelistPages) > 0 {
		p.freed = append(p.freed, p.freelistPages...)
		p.freelistPages = nil
	}
	if len(p.freed) == 0 {
		return pagestore.InvalidPage, nil
	}

	capacity := pagestore.FreelistPageCapacity(p.pgSize)
	if capacity < 1 {
		return pagestore.InvalidPage, errors.New("page size too small for freelist")
	}

	freed := append([]pagestore.PageID(nil), p.freed...)
	pageCount := (len(freed) + capacity - 1) / capacity
	pages := make([]pagestore.PageID, pageCount)
	for i := range pages {
		pages[i] = pagestore.PageID(p.pageCount)
		p.pageCount++
	}

	buf := make([]byte, p.pgSize)
	off := 0
	for i := len(pages) - 1; i >= 0; i-- {
		nextPage := pagestore.InvalidPage
		if i+1 < len(pages) {
			nextPage = pages[i+1]
		}
		clear(buf)
		written := pagestore.EncodeFreelistPage(buf, nextPage, freed[off:])
		if written == 0 {
			return pagestore.InvalidPage, errors.New("freelist page wrote zero entries")
		}
		if err := p.WritePage(pages[i], buf); err != nil {
			return pagestore.InvalidPage, errors.Wrap(err, "write freelist page")
		}
		off += written
	}

	p.freelistPages = pages
	return pages[0], nil
}

// _ is a type assertion.
var _ pagestore.Pager = (*OpfsPager)(nil)
