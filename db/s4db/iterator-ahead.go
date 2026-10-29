package s4db

import (
	"bytes"
	"slices"
	"sync"
	"sync/atomic"
)

// aheadStreak is how many consecutive stored values of the index tree a
// caller reads before its iterator reads ahead, so a point lookup through an
// iterator reads one value.
const aheadStreak = 4

// aheadBytes bounds the stored values read ahead and not yet taken.
const aheadBytes = 8 << 20

// aheadReads is the number of reads ahead that run at once.
const aheadReads = 16

// aheadQueue bounds the reads ahead queued for the readers.
const aheadQueue = 1024

// aheadRead is one stored value read ahead of an iterator.
type aheadRead struct {
	// v is the value to read.
	v value
	// gen is the generation that scheduled the read; a later one skips it.
	gen uint64
	// done is closed once b and err are set.
	done chan struct{}
	// b is the value read.
	b []byte
	// err is the read error.
	err error
}

// valueAhead reads the stored values after an iterator's position in the
// index tree with up to aheadReads concurrent reads, so a scan that takes
// every value waits on the disk's throughput rather than on one read's
// latency per value. It starts once the caller has read aheadStreak tree
// values in a row. Values the overlay shadows are read and never taken; they
// hold budget until the next restart.
//
// Its methods run on the iterator's goroutine. Its readers start on demand
// and exit once the queue is empty, so an iterator dropped without Close
// leaves none behind. A reader only reads the file, and a read that
// outlives the iterator's snapshot is never taken.
type valueAhead struct {
	// p reads values.
	p *pager
	// prefix bounds the keys to read ahead.
	prefix []byte
	// reverse reads ahead in descending order.
	reverse bool
	// streak counts the consecutive tree values read without read-ahead.
	streak int
	// cur is positioned after the last value scheduled, when started.
	cur cursor
	// started is set once cur is positioned.
	started bool
	// reads holds the scheduled values by file offset.
	reads map[uint64]*aheadRead
	// last is the read taken last, so the caller may take it again.
	last *aheadRead
	// held is the length of the scheduled values not yet taken.
	held int
	// gen is the current generation; reset and restart advance it.
	gen atomic.Uint64
	// jobs queues reads for the readers.
	jobs chan *aheadRead
	// running counts the readers.
	running atomic.Int32
	// wg tracks the readers.
	wg sync.WaitGroup
}

// take returns the stored value v at key, from a read ahead when one is
// scheduled. tree is the iterator's tree cursor.
func (a *valueAhead) take(tree *cursor, key []byte, v value) ([]byte, error) {
	// Take a value read ahead, and schedule more in its place.
	if a.last != nil && a.last.v.ref == v.ref {
		return a.last.b, a.last.err
	}
	if r, ok := a.reads[v.ref.off]; ok && r.v.ref == v.ref {
		delete(a.reads, v.ref.off)
		a.held -= int(v.ref.n)
		a.fill()
		<-r.done
		a.last = r
		return r.b, r.err
	}

	// Restart from the tree after a run of its values; a value of the
	// overlay reads alone.
	fromTree := tree.valid() && bytes.Equal(tree.key(), key) && tree.value().ref == v.ref
	if fromTree {
		a.streak++
	}
	if fromTree && a.streak >= aheadStreak {
		a.restart(tree)
	}
	return a.p.readValue(v)
}

// reset drops the scheduled reads, as after a seek.
func (a *valueAhead) reset() {
	a.gen.Add(1)
	a.streak, a.started, a.reads, a.last, a.held = 0, false, nil, nil, 0
}

// restart drops the scheduled reads and schedules the values after the
// tree cursor's entry.
func (a *valueAhead) restart(tree *cursor) {
	// Position a copy of the tree cursor after its entry.
	a.gen.Add(1)
	a.reads, a.last, a.held = make(map[uint64]*aheadRead), nil, 0
	a.cur = cursor{p: tree.p, root: tree.root, path: slices.Clone(tree.path)}
	a.cur.step(a.reverse)
	a.started = true
	if a.jobs == nil {
		a.jobs = make(chan *aheadRead, aheadQueue)
	}
	a.fill()
}

// fill schedules the stored values after cur until the budget or the queue
// is full or the keys leave the prefix.
func (a *valueAhead) fill() {
	gen := a.gen.Load()
	for a.started && a.held < aheadBytes && len(a.jobs) < aheadQueue && a.cur.valid() {
		// Stop past the prefix.
		key := a.cur.key()
		if !bytes.HasPrefix(key, a.prefix) {
			c := bytes.Compare(key, a.prefix)
			if (!a.reverse && c > 0) || (a.reverse && c < 0) {
				return
			}
		}

		// Schedule a stored value, starting a reader when fewer than
		// aheadReads run.
		v := a.cur.value()
		if v.isRef && a.reads[v.ref.off] == nil {
			r := &aheadRead{v: v, gen: gen, done: make(chan struct{})}
			a.reads[v.ref.off] = r
			a.held += int(v.ref.n)
			a.jobs <- r
			if a.claim() {
				a.wg.Add(1)
				go a.read()
			}
		}
		a.cur.step(a.reverse)
	}
}

// claim counts one more reader unless aheadReads run.
func (a *valueAhead) claim() bool {
	for {
		n := a.running.Load()
		if n >= aheadReads {
			return false
		}
		if a.running.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// read runs one reader until the queue is empty. A reader that finds it
// empty checks it again after leaving the count, so a read queued while it
// left is either seen here or starts a new reader in fill.
func (a *valueAhead) read() {
	defer a.wg.Done()
	for {
		select {
		case r := <-a.jobs:
			if r.gen == a.gen.Load() {
				r.b, r.err = a.p.readValue(r.v)
			}
			close(r.done)
		default:
			a.running.Add(-1)
			if len(a.jobs) == 0 || !a.claim() {
				return
			}
		}
	}
}

// close drops the scheduled reads and waits for the readers, so no read
// outlives the iterator.
func (a *valueAhead) close() {
	a.reset()
	a.wg.Wait()
}
