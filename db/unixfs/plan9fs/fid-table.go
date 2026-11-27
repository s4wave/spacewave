package plan9fs

import (
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
)

// FidTable is a thread-safe table of active fids.
type FidTable struct {
	mu       sync.Mutex
	fids     map[uint32]*Fid
	qidPath  atomic.Uint64
	qidPaths map[*unixfs.FSHandle]uint64
}

// NewFidTable creates a new FidTable.
func NewFidTable() *FidTable {
	return &FidTable{
		fids:     make(map[uint32]*Fid),
		qidPaths: make(map[*unixfs.FSHandle]uint64),
	}
}

// Get returns the fid with the given id.
func (t *FidTable) Get(id uint32) (*Fid, error) {
	// Look up the fid under the table lock.
	t.mu.Lock()
	f, ok := t.fids[id]
	t.mu.Unlock()
	if !ok {
		return nil, errBadFid
	}
	return f, nil
}

// Add adds a new fid to the table.
func (t *FidTable) Add(id uint32, f *Fid) error {
	// Register the fid unless its identifier is already in use.
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.fids[id]; ok {
		return errFidInUse
	}
	t.fids[id] = f
	return nil
}

// Remove removes and returns the fid with the given id.
func (t *FidTable) Remove(id uint32) (*Fid, error) {
	// Detach the fid under the table lock.
	t.mu.Lock()
	f, ok := t.fids[id]
	if ok {
		delete(t.fids, id)
	}
	t.mu.Unlock()
	if !ok {
		return nil, errBadFid
	}
	return f, nil
}

// ReleaseAll releases all fids and their FSHandles.
func (t *FidTable) ReleaseAll() {
	// Detach the session fids and release their filesystem handles.
	t.mu.Lock()
	fids := t.fids
	t.fids = make(map[uint32]*Fid)
	t.qidPaths = make(map[*unixfs.FSHandle]uint64)
	t.mu.Unlock()
	for _, f := range fids {
		f.handle.Release()
	}
}

// NewQID allocates a QID for a newly created entry without an open handle.
func (t *FidTable) NewQID(kind uint8) QID {
	return QID{Type: kind, Version: 0, Path: t.qidPath.Add(1)}
}

// AllocQIDPath allocates a unique QID path for an FSHandle.
// Returns the same path if the handle was already seen.
func (t *FidTable) AllocQIDPath(h *unixfs.FSHandle) uint64 {
	// Reuse or allocate the QID path under the table lock.
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.qidPaths[h]; ok {
		return p
	}
	p := t.qidPath.Add(1)
	t.qidPaths[h] = p
	return p
}
