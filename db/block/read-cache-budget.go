package block

import (
	"sync"
	"sync/atomic"
)

const (
	// DefaultReadCacheBudget is the default total of the resident read caches:
	// the sum of the decoded pool, resident spans and resident indexes defaults.
	DefaultReadCacheBudget int64 = 640 << 20
	// readCacheDecodedFloorDivisor and readCacheEncodedFloorDivisor set each
	// class's protected floor to one part in this many of the limit.
	readCacheDecodedFloorDivisor = 4
	readCacheEncodedFloorDivisor = 8
)

// ReadCacheClass is a kind of resident read cache drawing on a budget.
type ReadCacheClass int

const (
	// ReadCacheDecoded holds decoded blocks. The ReadCursor metadata is charged
	// to its decoded entry, so the cursor pool needs no share of its own.
	ReadCacheDecoded ReadCacheClass = iota
	// ReadCacheEncoded holds encoded bytes: resident pack spans and indexes.
	ReadCacheEncoded
	readCacheClasses
)

// ReadCacheMember is a cache that draws on a ReadCacheBudget.
type ReadCacheMember interface {
	// ReadCacheUsed returns the bytes the member holds.
	ReadCacheUsed() int64
	// ReadCacheEvict frees about want bytes of unprotected entries and returns
	// the bytes freed, which is zero when nothing can be evicted. It never
	// evicts an entry a reader is writing or attaching, and runs under no
	// budget lock.
	ReadCacheEvict(want int64) int64
}

// ReadCacheBudget is one byte budget shared by the resident read caches of a
// process. The members' bytes are summed on demand; Reclaim brings the total
// back under the limit by evicting from the class furthest above its floor,
// and never takes a class below its floor, so a busy class cannot starve the
// other. A reader holding a block keeps it through its own reference, so
// eviction never invalidates a block in use: it only releases the cache's
// copy. The behavior is checked by ReadCacheBudget.tla.
type ReadCacheBudget struct {
	// limit is the total byte limit. Zero or negative disables eviction.
	limit atomic.Int64
	// mtx guards members.
	mtx sync.RWMutex
	// members holds the joined caches by class.
	members [readCacheClasses][]*readCacheJoin
	// reclaiming serializes Reclaim.
	reclaiming atomic.Bool
	// evicted counts the bytes Reclaim freed from each class.
	evicted [readCacheClasses]atomic.Int64
}

// readCacheJoin is one joined member.
type readCacheJoin struct {
	m ReadCacheMember
}

// NewReadCacheBudget returns a budget with the byte limit.
func NewReadCacheBudget(limit int64) *ReadCacheBudget {
	b := &ReadCacheBudget{}
	b.limit.Store(limit)
	return b
}

var sharedReadCacheBudget = sync.OnceValue(func() *ReadCacheBudget {
	return NewReadCacheBudget(DefaultReadCacheBudget)
})

// SharedReadCacheBudget returns the process-wide budget behind the shared
// decoded-block pool. Pack stores join it so one setting bounds every
// resident read cache.
func SharedReadCacheBudget() *ReadCacheBudget {
	return sharedReadCacheBudget()
}

// Join adds a member to the class and returns the function that removes it.
func (b *ReadCacheBudget) Join(class ReadCacheClass, m ReadCacheMember) (leave func()) {
	j := &readCacheJoin{m: m}
	b.mtx.Lock()
	b.members[class] = append(b.members[class], j)
	b.mtx.Unlock()
	return func() {
		b.mtx.Lock()
		defer b.mtx.Unlock()
		list := b.members[class]
		for i, x := range list {
			if x == j {
				b.members[class] = append(list[:i:i], list[i+1:]...)
				return
			}
		}
	}
}

// Limit returns the total byte limit.
func (b *ReadCacheBudget) Limit() int64 {
	return b.limit.Load()
}

// SetLimit sets the total byte limit and reclaims down to it.
func (b *ReadCacheBudget) SetLimit(limit int64) {
	b.limit.Store(limit)
	b.Reclaim()
}

// Floor returns the bytes of the class that eviction leaves in place.
func (b *ReadCacheBudget) Floor(class ReadCacheClass) int64 {
	limit := max(b.limit.Load(), 0)
	if class == ReadCacheDecoded {
		return limit / readCacheDecodedFloorDivisor
	}
	return limit / readCacheEncodedFloorDivisor
}

// Used returns the bytes held by the class's members.
func (b *ReadCacheBudget) Used(class ReadCacheClass) int64 {
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	var n int64
	for _, j := range b.members[class] {
		n += j.m.ReadCacheUsed()
	}
	return n
}

// Total returns the bytes held by every class.
func (b *ReadCacheBudget) Total() int64 {
	return b.Used(ReadCacheDecoded) + b.Used(ReadCacheEncoded)
}

// Evicted returns the bytes Reclaim has freed from the class.
func (b *ReadCacheBudget) Evicted(class ReadCacheClass) int64 {
	return b.evicted[class].Load()
}

// Reclaim evicts until the total fits the limit or nothing more may be
// evicted. A caller racing a running Reclaim returns at once: the running one
// rechecks the total after it releases, so the caller's bytes are not missed.
// The caller must hold no member lock.
func (b *ReadCacheBudget) Reclaim() {
	for b.overage() > 0 && b.reclaiming.CompareAndSwap(false, true) {
		left := b.reclaimLocked()
		b.reclaiming.Store(false)

		// Run again only if bytes arrived after the pass; a pass that stopped
		// because every class sits at its floor or is held would repeat itself.
		if b.Total() <= left {
			return
		}
	}
}

// overage returns the bytes beyond the limit, or zero when eviction is off.
func (b *ReadCacheBudget) overage() int64 {
	limit := b.limit.Load()
	if limit <= 0 {
		return 0
	}
	return max(b.Total()-limit, 0)
}

// reclaimLocked evicts from the class furthest above its floor, then the
// other, until the total fits or neither class can give. It returns the total
// it left.
func (b *ReadCacheBudget) reclaimLocked() int64 {
	var stuck [readCacheClasses]bool
	for {
		over := b.overage()
		if over <= 0 {
			return b.Total()
		}

		// Pick the class with the most bytes above its floor.
		class, room := ReadCacheClass(-1), int64(0)
		for c := ReadCacheClass(0); c < readCacheClasses; c++ {
			if r := b.Used(c) - b.Floor(c); !stuck[c] && r > room {
				class, room = c, r
			}
		}
		if class < 0 {
			return b.Total()
		}

		// Ask for the overage, capped at the class's room above its floor.
		freed := b.evict(class, min(over, room))
		if freed <= 0 {
			stuck[class] = true
			continue
		}
		b.evicted[class].Add(freed)
	}
}

// evict asks the class's members for want bytes in turn.
func (b *ReadCacheBudget) evict(class ReadCacheClass, want int64) int64 {
	b.mtx.RLock()
	list := append([]*readCacheJoin(nil), b.members[class]...)
	b.mtx.RUnlock()
	var freed int64
	for _, j := range list {
		if freed >= want {
			break
		}
		freed += j.m.ReadCacheEvict(want - freed)
	}
	return freed
}
