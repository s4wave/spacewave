package block

import (
	"hash/maphash"
	"sync"
	"sync/atomic"
)

const (
	// decodedBlockPoolMaxShards bounds the shards of a pool.
	decodedBlockPoolMaxShards = 64
	// decodedBlockShardMinCost is the smallest budget a shard gets when the
	// pool is created, so a small pool keeps room for large blocks.
	decodedBlockShardMinCost = 4 << 20
	// decodedBlockProbationDivisor sets the probation queue's share of a
	// shard's budget to one part in this many.
	decodedBlockProbationDivisor = 10
	// decodedBlockMaxFreq caps the lookups an entry remembers, and so the
	// passes of the main queue it survives once lookups stop.
	decodedBlockMaxFreq = 3
	// decodedBlockShardPad separates shards by more than a cache line, so
	// lookups on neighboring shards do not contend.
	decodedBlockShardPad = 128
)

// decodedBlockPool is a byte-budgeted cache of decoded blocks shared by
// scopes. A store is visible to the next lookup and is never refused for being
// new: each World commit replaces the pages from the root to its changed keys,
// and every reader of the new head needs exactly those pages.
//
// Each shard keeps two FIFO queues. A new entry joins the probation queue,
// which holds a tenth of the budget. When it reaches the head of probation it
// moves to the main queue if a lookup found it, and is evicted otherwise, so a
// scan that reads each page once displaces only other probation entries. The
// main queue gives each entry one more pass per remembered lookup. Lookups only
// raise counters under the shard's read lock, and each shard keeps its own
// counters, so lookups on different shards share no cache line.
type decodedBlockPool struct {
	// shards holds the entries by key hash; nil when caching is disabled.
	shards []decodedBlockShard
	// seed hashes keys to shards.
	seed maphash.Seed
	// maxCost is the budget of the whole pool.
	maxCost atomic.Int64
	// nextScope allocates scope key prefixes.
	nextScope atomic.Uint64
	// off bypasses the shards from then on, as a disabled pool does.
	off atomic.Bool
}

// decodedBlockShard is one lock's share of a pool.
type decodedBlockShard struct {
	// mtx guards entries and the queues; lookups take it for reading.
	mtx sync.RWMutex
	// hits and misses count lookups. They share the cache line of mtx, which
	// every lookup writes anyway.
	hits, misses atomic.Uint64
	// entries holds the shard's entries by pool key.
	entries map[string]*decodedBlockPoolEntry
	// probation holds entries not yet found by a lookup at its head.
	probation decodedBlockQueue
	// main holds entries promoted from probation.
	main decodedBlockQueue
	// stores, rejections and evictions count the shard's other events, and
	// costAdded and costEvicted sum their costs.
	stores, rejections, evictions atomic.Uint64
	costAdded, costEvicted        atomic.Int64
	// retained is the cost of the entries held, readable without mtx.
	retained atomic.Int64
	// _ keeps the next shard's lock off this shard's cache lines.
	_ [decodedBlockShardPad]byte
}

// decodedBlockPoolStats sums the counters of a pool's shards.
type decodedBlockPoolStats struct {
	hits, misses, stores, rejections, evictions uint64
	costAdded, costEvicted, retained            int64
}

// decodedBlockPoolEntry is one cached block in a shard queue.
type decodedBlockPoolEntry struct {
	// key is the entry's pool key.
	key string
	// value is the cached block and the epochs it was stored under.
	value decodedBlockCacheEntry
	// cost is the budget the entry is charged.
	cost int64
	// freq counts lookups since the entry last moved. Lookups raise it under
	// the read lock, so it may pass decodedBlockMaxFreq; eviction clamps it.
	freq atomic.Int32
	// queue is the queue holding the entry.
	queue *decodedBlockQueue
	// prev and next link the entry in its queue.
	prev, next *decodedBlockPoolEntry
}

// decodedBlockQueue is a FIFO of entries with their total cost.
type decodedBlockQueue struct {
	// head is the oldest entry and tail the newest.
	head, tail *decodedBlockPoolEntry
	// cost is the total cost of the queued entries.
	cost int64
}

// newDecodedBlockPoolShards returns a pool with one shard per
// decodedBlockShardMinCost of maxCost, up to decodedBlockPoolMaxShards.
func newDecodedBlockPoolShards(maxCost int64) *decodedBlockPool {
	// Size the shards from the budget and make their maps.
	n := int(min(max(maxCost/decodedBlockShardMinCost, 1), decodedBlockPoolMaxShards))
	pool := &decodedBlockPool{shards: make([]decodedBlockShard, n), seed: maphash.MakeSeed()}
	for i := range pool.shards {
		pool.shards[i].entries = make(map[string]*decodedBlockPoolEntry)
	}
	pool.maxCost.Store(maxCost)
	return pool
}

// shard returns the shard holding key.
func (p *decodedBlockPool) shard(key string) *decodedBlockShard {
	return &p.shards[maphash.String(p.seed, key)%uint64(len(p.shards))]
}

// shardBudget returns the budget of each shard.
func (p *decodedBlockPool) shardBudget() int64 {
	return p.maxCost.Load() / int64(len(p.shards))
}

// stats sums the shards' counters.
func (p *decodedBlockPool) stats() decodedBlockPoolStats {
	var st decodedBlockPoolStats
	for i := range p.shards {
		s := &p.shards[i]
		st.hits += s.hits.Load()
		st.misses += s.misses.Load()
		st.stores += s.stores.Load()
		st.rejections += s.rejections.Load()
		st.evictions += s.evictions.Load()
		st.costAdded += s.costAdded.Load()
		st.costEvicted += s.costEvicted.Load()
		st.retained += s.retained.Load()
	}
	return st
}

// get returns the entry for key and records the lookup.
func (p *decodedBlockPool) get(key string) (decodedBlockCacheEntry, bool) {
	// Find the entry and raise its count under the read lock.
	s := p.shard(key)
	s.mtx.RLock()
	e := s.entries[key]
	var value decodedBlockCacheEntry
	if e != nil {
		if e.freq.Load() < decodedBlockMaxFreq {
			e.freq.Add(1)
		}
		value = e.value
	}
	s.mtx.RUnlock()

	// Count the hit or miss.
	if e == nil {
		s.misses.Add(1)
		return decodedBlockCacheEntry{}, false
	}
	s.hits.Add(1)
	return value, true
}

// put stores value under key with cost, evicting older entries to stay within
// the shard's budget. It returns false only when cost exceeds that budget.
func (p *decodedBlockPool) put(key string, value decodedBlockCacheEntry, cost int64) bool {
	// Refuse an entry larger than a whole shard.
	s := p.shard(key)
	budget := p.shardBudget()
	if cost > budget {
		s.rejections.Add(1)
		return false
	}

	// Replace an existing entry in place, or append a new one to probation.
	s.mtx.Lock()
	e := s.entries[key]
	if e != nil {
		e.value = value
		e.queue.cost += cost - e.cost
		s.retained.Add(cost - e.cost)
		e.cost = cost
	} else {
		e = &decodedBlockPoolEntry{key: key, value: value, cost: cost}
		s.entries[key] = e
		s.probation.push(e)
		s.retained.Add(cost)
	}

	// Evict until the shard fits its budget, keeping the stored entry.
	var evicted, evictedCost int64
	for s.probation.cost+s.main.cost > budget {
		victim := s.evictOne(budget, e)
		if victim == nil {
			break
		}
		evicted++
		evictedCost += victim.cost
	}
	s.mtx.Unlock()

	// Count the store and the evictions.
	s.stores.Add(1)
	s.costAdded.Add(cost)
	if evicted != 0 {
		s.retained.Add(-evictedCost)
		s.evictions.Add(uint64(evicted))
		s.costEvicted.Add(evictedCost)
	}
	return true
}

// evictOne evicts one entry other than keep and returns it, or nil when keep
// is the only entry. Entries found by a lookup move on instead of leaving, so
// the loop ends within decodedBlockMaxFreq passes over the shard. The caller
// must hold mtx for writing.
func (s *decodedBlockShard) evictOne(budget int64, keep *decodedBlockPoolEntry) *decodedBlockPoolEntry {
	for {
		// Take from probation while it is over its share, or when main has
		// nothing else, and never the entry being stored.
		fromProbation := s.probation.cost > budget/decodedBlockProbationDivisor
		e := s.main.oldestExcept(keep)
		if fromProbation || e == nil {
			if p := s.probation.oldestExcept(keep); p != nil {
				e, fromProbation = p, true
			} else {
				fromProbation = false
			}
		}
		if e == nil {
			return nil
		}
		e.queue.remove(e)

		// A probation entry found by a lookup moves to main; a main entry
		// found by a lookup gets another pass with one lookup fewer.
		if f := min(e.freq.Load(), decodedBlockMaxFreq); f > 0 {
			if fromProbation {
				e.freq.Store(0)
			} else {
				e.freq.Store(f - 1)
			}
			s.main.push(e)
			continue
		}
		delete(s.entries, e.key)
		return e
	}
}

// del removes the entry for key.
func (p *decodedBlockPool) del(key string) {
	// Unlink the entry and release its cost.
	s := p.shard(key)
	s.mtx.Lock()
	e := s.entries[key]
	if e != nil {
		delete(s.entries, key)
		e.queue.remove(e)
		s.retained.Add(-e.cost)
	}
	s.mtx.Unlock()
}

// clear removes every entry.
func (p *decodedBlockPool) clear() {
	for i := range p.shards {
		s := &p.shards[i]
		s.mtx.Lock()
		s.retained.Add(-(s.probation.cost + s.main.cost))
		s.entries = make(map[string]*decodedBlockPoolEntry)
		s.probation, s.main = decodedBlockQueue{}, decodedBlockQueue{}
		s.mtx.Unlock()
	}
}

// push appends e as the newest entry.
func (q *decodedBlockQueue) push(e *decodedBlockPoolEntry) {
	e.queue, e.prev, e.next = q, q.tail, nil
	if q.tail != nil {
		q.tail.next = e
	} else {
		q.head = e
	}
	q.tail = e
	q.cost += e.cost
}

// oldestExcept returns the oldest entry other than keep, or nil.
func (q *decodedBlockQueue) oldestExcept(keep *decodedBlockPoolEntry) *decodedBlockPoolEntry {
	if e := q.head; e != keep {
		return e
	}
	return keep.next
}

// remove unlinks e from the queue.
func (q *decodedBlockQueue) remove(e *decodedBlockPoolEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		q.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		q.tail = e.prev
	}
	e.queue, e.prev, e.next = nil, nil, nil
	q.cost -= e.cost
}
