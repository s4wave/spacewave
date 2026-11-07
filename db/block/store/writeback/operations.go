package block_store_writeback

import (
	"sync"
	"time"
)

// OpenOperations is when the oldest write and the oldest read operation still
// open on a store began, each zero for none.
//
// A pack sweep compares them with the time a pack left the reads: a write
// that began before may still log a block the pack holds after the sweep read
// the log, and a read operation that began before may hold a block of the
// pack it read without the fallback, and link it later.
type OpenOperations struct {
	// Write is the start of the oldest write in flight: a put, a batch or a
	// root move, open from before the store below writes until the observer
	// has seen it.
	Write time.Time
	// Read is the start of the oldest read operation or stage not yet
	// released.
	Read time.Time
}

// openOp is one open operation.
type openOp struct {
	// at is when it began, in Unix nanoseconds.
	at int64
	// read is set for a read operation or stage, clear for a write.
	read bool
}

// openOps records the open operations of a MarkingStore and every copy of
// it. A nil openOps records none.
type openOps struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// next numbers the last operation begun.
	next uint64
	// open are the open operations by number.
	open map[uint64]openOp
}

// newOpenOps builds an empty record.
func newOpenOps() *openOps {
	return &openOps{open: make(map[uint64]openOp)}
}

// begin records an operation beginning now and returns its number.
func (o *openOps) begin(read bool) uint64 {
	// Number the operation and record its start under the lock.
	if o == nil {
		return 0
	}
	at := time.Now().UnixNano()
	o.mtx.Lock()
	o.next++
	n := o.next
	o.open[n] = openOp{at: at, read: read}
	o.mtx.Unlock()
	return n
}

// end drops operation n.
func (o *openOps) end(n uint64) {
	if o == nil {
		return
	}
	o.mtx.Lock()
	delete(o.open, n)
	o.mtx.Unlock()
}

// oldest returns the start of the oldest open write and read operation.
func (o *openOps) oldest() OpenOperations {
	// Find the earliest of each kind under the lock.
	var write, read int64
	if o == nil {
		return OpenOperations{}
	}
	o.mtx.Lock()
	for _, op := range o.open {
		at := &write
		if op.read {
			at = &read
		}
		if *at == 0 || op.at < *at {
			*at = op.at
		}
	}
	o.mtx.Unlock()

	// Convert them, zero for none.
	var out OpenOperations
	if write != 0 {
		out.Write = time.Unix(0, write)
	}
	if read != 0 {
		out.Read = time.Unix(0, read)
	}
	return out
}

// releaseRead wraps the release of a read operation or stage to drop it
// once, on the first call.
func (o *openOps) releaseRead(n uint64, release func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if release != nil {
				release()
			}
			o.end(n)
		})
	}
}

// OldestOperations returns when the oldest write and read operation still
// open on the store and its copies began.
func (m *MarkingStore) OldestOperations() OpenOperations {
	return m.ops.oldest()
}
