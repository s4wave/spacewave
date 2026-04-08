package pagestore

// Pager provides page-level I/O for the B+tree.
// Implementations may be in-memory (testing) or OPFS-backed (production).
type Pager interface {
	// PageSize returns the fixed page size.
	PageSize() int
	// ReadPage reads a page by ID into buf.
	ReadPage(id PageID, buf []byte) error
	// WritePage writes buf to the given page ID.
	WritePage(id PageID, buf []byte) error
	// AllocPage returns the next free page ID.
	AllocPage() PageID
	// FreePage returns a page to the free pool.
	FreePage(id PageID)
	// PageCount returns the total number of allocated pages.
	PageCount() uint32
}

// MemPager is an in-memory Pager for testing.
type MemPager struct {
	pageSize int
	pages    map[PageID][]byte
	nextID   PageID
	freed    []PageID
}

// NewMemPager creates an in-memory pager with the given page size.
func NewMemPager(pageSize int) *MemPager {
	return &MemPager{
		pageSize: pageSize,
		pages:    make(map[PageID][]byte),
	}
}

// PageSize returns the page size.
func (m *MemPager) PageSize() int { return m.pageSize }

// ReadPage reads a page by ID.
func (m *MemPager) ReadPage(id PageID, buf []byte) error {
	data, ok := m.pages[id]
	if !ok {
		// Return zeroed page for unwritten pages.
		for i := range buf {
			buf[i] = 0
		}
		return nil
	}
	copy(buf, data)
	return nil
}

// WritePage writes a page.
func (m *MemPager) WritePage(id PageID, buf []byte) error {
	data := make([]byte, len(buf))
	copy(data, buf)
	m.pages[id] = data
	return nil
}

// AllocPage returns the next free page ID.
func (m *MemPager) AllocPage() PageID {
	if len(m.freed) > 0 {
		id := m.freed[len(m.freed)-1]
		m.freed = m.freed[:len(m.freed)-1]
		return id
	}
	id := m.nextID
	m.nextID++
	return id
}

// FreePage returns a page to the free pool.
func (m *MemPager) FreePage(id PageID) {
	m.freed = append(m.freed, id)
	delete(m.pages, id)
}

// PageCount returns the total number of pages allocated.
func (m *MemPager) PageCount() uint32 {
	return uint32(m.nextID)
}
