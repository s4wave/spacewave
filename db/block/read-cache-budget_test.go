package block

import (
	"strconv"
	"sync"
	"testing"
)

// fakeReadCache is a member holding fixed-size entries, some protected.
type fakeReadCache struct {
	mtx       sync.Mutex
	entryCost int64
	entries   []int
	held      map[int]bool
	next      int
}

func newFakeReadCache(entryCost int64) *fakeReadCache {
	return &fakeReadCache{entryCost: entryCost, held: map[int]bool{}}
}

// add admits n entries and returns their ids.
func (f *fakeReadCache) add(n int) []int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	var ids []int
	for range n {
		f.next++
		f.entries = append(f.entries, f.next)
		ids = append(ids, f.next)
	}
	return ids
}

func (f *fakeReadCache) setHeld(id int, held bool) {
	f.mtx.Lock()
	f.held[id] = held
	f.mtx.Unlock()
}

func (f *fakeReadCache) has(id int) bool {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	for _, e := range f.entries {
		if e == id {
			return true
		}
	}
	return false
}

func (f *fakeReadCache) ReadCacheUsed() int64 {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return int64(len(f.entries)) * f.entryCost
}

// ReadCacheEvict drops the oldest entries that are not held.
func (f *fakeReadCache) ReadCacheEvict(want int64) int64 {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	var freed int64
	kept := f.entries[:0]
	for _, id := range f.entries {
		if freed < want && !f.held[id] {
			freed += f.entryCost
			continue
		}
		kept = append(kept, id)
	}
	f.entries = kept
	return freed
}

// TestReadCacheBudget walks the states of ReadCacheBudget.tla: admission and
// eviction across two classes, floors, held entries, a disabled budget, and a
// member that leaves.
func TestReadCacheBudget(t *testing.T) {
	const limit = 800
	b := NewReadCacheBudget(limit)
	dec, enc := newFakeReadCache(10), newFakeReadCache(10)
	leaveDec := b.Join(ReadCacheDecoded, dec)
	b.Join(ReadCacheEncoded, enc)
	decFloor, encFloor := b.Floor(ReadCacheDecoded), b.Floor(ReadCacheEncoded)
	if decFloor != 200 || encFloor != 100 {
		t.Fatalf("floors %d %d", decFloor, encFloor)
	}

	// Fill encoded alone past the limit: it yields to the limit.
	enc.add(100)
	b.Reclaim()
	if got := b.Total(); got != limit {
		t.Fatalf("total %d after encoded fill, want %d", got, limit)
	}

	// Decoded demand takes bytes from encoded, never below the encoded floor.
	dec.add(60)
	b.Reclaim()
	if got := b.Total(); got != limit {
		t.Fatalf("total %d after decoded demand, want %d", got, limit)
	}
	if got := b.Used(ReadCacheDecoded); got != 600 {
		t.Fatalf("decoded %d, want 600", got)
	}
	dec.add(60)
	b.Reclaim()
	if got := b.Used(ReadCacheEncoded); got < encFloor {
		t.Fatalf("encoded %d fell below floor %d", got, encFloor)
	}
	if got := b.Total(); got != limit {
		t.Fatalf("total %d with decoded flood, want %d", got, limit)
	}

	// Encoded demand takes back from decoded, never below the decoded floor.
	enc.add(200)
	b.Reclaim()
	if got := b.Used(ReadCacheDecoded); got < decFloor {
		t.Fatalf("decoded %d fell below floor %d", got, decFloor)
	}

	// A held entry survives, and the budget stays over its limit by at most
	// the held bytes until the hold releases.
	ids := dec.add(1)
	dec.setHeld(ids[0], true)
	for _, e := range append([]int(nil), dec.entries...) {
		dec.setHeld(e, true)
	}
	enc.add(50)
	b.Reclaim()
	for _, id := range dec.entries {
		if !dec.has(id) {
			t.Fatal("held entry evicted")
		}
	}
	if b.Used(ReadCacheDecoded) < decFloor {
		t.Fatal("held decoded entries evicted")
	}
	for _, e := range append([]int(nil), dec.entries...) {
		dec.setHeld(e, false)
	}
	b.Reclaim()
	if got := b.Total(); got > limit {
		t.Fatalf("total %d still over %d after holds released", got, limit)
	}

	// A member that left is no longer asked to evict or counted.
	leaveDec()
	if got := b.Used(ReadCacheDecoded); got != 0 {
		t.Fatalf("left member counted: %d", got)
	}

	// A disabled budget evicts nothing.
	off := NewReadCacheBudget(0)
	e2 := newFakeReadCache(10)
	off.Join(ReadCacheEncoded, e2)
	e2.add(100)
	off.Reclaim()
	if e2.ReadCacheUsed() != 1000 {
		t.Fatal("disabled budget evicted")
	}
}

// TestReadCacheBudgetPool checks the decoded pool against a budget it shares
// with an encoded member: the pool serves hits within its share and yields to
// the limit as the encoded side fills.
func TestReadCacheBudgetPool(t *testing.T) {
	const limit = 10000
	b := NewReadCacheBudget(limit)
	pool := newDecodedBlockPoolShards(limit)
	pool.budget = b
	b.Join(ReadCacheDecoded, pool)
	enc := newFakeReadCache(100)
	b.Join(ReadCacheEncoded, enc)

	// Fill the pool with hot entries, then the encoded side to the limit.
	for i := range 50 {
		key := strconv.Itoa(i)
		pool.put(key, decodedBlockCacheEntry{}, 100)
		pool.get(key)
		pool.get(key)
	}
	enc.add(100)
	b.Reclaim()
	if got := b.Total(); got > limit {
		t.Fatalf("total %d over limit %d", got, limit)
	}
	if got := b.Used(ReadCacheDecoded); got < b.Floor(ReadCacheDecoded) {
		t.Fatalf("decoded %d below its floor", got)
	}
	hits := 0
	for i := range 50 {
		if _, ok := pool.get(strconv.Itoa(i)); ok {
			hits++
		}
	}
	if hits == 0 {
		t.Fatal("decoded pool served no hits")
	}

	// A store past the limit reclaims through the pool itself.
	for i := range 500 {
		pool.put("n"+strconv.Itoa(i), decodedBlockCacheEntry{}, 100)
	}
	if got := b.Total(); got > limit {
		t.Fatalf("total %d over limit %d after stores", got, limit)
	}
	if b.Evicted(ReadCacheEncoded) == 0 {
		t.Fatal("encoded side never yielded")
	}
	checkDecodedBlockPool(t, pool)
}

// BenchmarkReadCacheBudgetProduction drives the decoded pool and a span-like
// encoded member at the production setting: a 1536 MiB budget, 16 KiB decoded
// entries read with a skewed key distribution, and 1 MiB encoded spans fetched
// on every decoded miss. It reports the decoded hit rate and the largest total
// seen, which stays within the budget.
func BenchmarkReadCacheBudgetProduction(b *testing.B) {
	const (
		limit     = 1536 << 20
		entryCost = 16 << 10
		spanCost  = 1 << 20
		keys      = 400000
	)
	budget := NewReadCacheBudget(limit)
	pool := newDecodedBlockPoolShards(limit)
	pool.budget = budget
	budget.Join(ReadCacheDecoded, pool)
	spans := newFakeReadCache(spanCost)
	budget.Join(ReadCacheEncoded, spans)

	var hits, reads, peak int64
	rng := uint64(88172645463325252)
	names := make([]string, keys)
	for i := range names {
		names[i] = strconv.Itoa(i)
	}
	for i := 0; i < b.N; i++ {
		// A cheap xorshift with a square skew toward low keys.
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		f := float64(rng%1000003) / 1000003
		key := names[int(f*f*float64(keys))]
		reads++
		if _, ok := pool.get(key); ok {
			hits++
			continue
		}
		spans.add(1)
		budget.Reclaim()
		pool.put(key, decodedBlockCacheEntry{}, entryCost)
		if t := budget.Total(); t > peak {
			peak = t
		}
	}
	b.ReportMetric(float64(hits)/float64(reads), "hit-rate")
	b.ReportMetric(float64(peak)/(1<<20), "peak-MiB")
	b.ReportMetric(float64(budget.Used(ReadCacheEncoded))/(1<<20), "encoded-MiB")
	b.ReportMetric(float64(budget.Used(ReadCacheDecoded))/(1<<20), "decoded-MiB")
	if peak > limit {
		b.Fatalf("peak %d over limit %d", peak, limit)
	}
}
