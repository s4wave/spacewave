package plan9fs

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
)

// errBadFid is returned when a fid is not found.
var errBadFid = errors.New("fid not found")

// errFidInUse is returned when a fid is already in use.
var errFidInUse = errors.New("fid already in use")

// Fid is an active 9p file identifier bound to an FSHandle.
type Fid struct {
	id     uint32
	handle *unixfs.FSHandle
	uid    uint32
	opened bool
}

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
	t.mu.Lock()
	fids := t.fids
	t.fids = make(map[uint32]*Fid)
	t.qidPaths = make(map[*unixfs.FSHandle]uint64)
	t.mu.Unlock()
	for _, f := range fids {
		f.handle.Release()
	}
}

// AllocQIDPath allocates a unique QID path for an FSHandle.
// Returns the same path if the handle was already seen.
func (t *FidTable) AllocQIDPath(h *unixfs.FSHandle) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.qidPaths[h]; ok {
		return p
	}
	p := t.qidPath.Add(1)
	t.qidPaths[h] = p
	return p
}
