package block

import (
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
)

// TestDecodedBlockPoolStoreVisible checks that a store is visible at once and
// that the pool refuses only an entry larger than a shard.
func TestDecodedBlockPoolStoreVisible(t *testing.T) {
	// Store an entry and find it with no wait.
	pool := newDecodedBlockPoolShards(1000)
	if !pool.put("a", decodedBlockCacheEntry{}, 10) {
		t.Fatal("store refused")
	}
	if _, ok := pool.get("a"); !ok {
		t.Fatal("stored entry missing")
	}

	// Refuse an entry over the shard budget and count it.
	if pool.put("big", decodedBlockCacheEntry{}, 1001) {
		t.Fatal("stored an entry over budget")
	}
	if got := pool.stats().rejections; got != 1 {
		t.Fatalf("rejections = %d, want 1", got)
	}
	checkDecodedBlockPool(t, pool)
}

// TestDecodedBlockPoolBudget checks that stores past the budget evict the
// oldest unread entries and keep the newest.
func TestDecodedBlockPoolBudget(t *testing.T) {
	// Store ten times the budget in entries read once each.
	pool := newDecodedBlockPoolShards(1000)
	for i := range 1000 {
		key := strconv.Itoa(i)
		pool.put(key, decodedBlockCacheEntry{}, 10)
		if _, ok := pool.get(key); !ok {
			t.Fatalf("entry %d missing after its store", i)
		}
	}

	// The pool holds its budget and the newest entry.
	if got := pool.stats().retained; got > 1000 {
		t.Fatalf("retained %d, budget 1000", got)
	}
	if _, ok := pool.get("999"); !ok {
		t.Fatal("newest entry evicted")
	}
	if _, ok := pool.get("0"); ok {
		t.Fatal("oldest entry kept")
	}
	checkDecodedBlockPool(t, pool)
}

// TestDecodedBlockPoolScanResistance checks that a scan of entries read once
// does not evict entries read before it.
func TestDecodedBlockPoolScanResistance(t *testing.T) {
	// Store half the budget in entries that are read.
	pool := newDecodedBlockPoolShards(1000)
	for i := range 50 {
		key := "hot " + strconv.Itoa(i)
		pool.put(key, decodedBlockCacheEntry{}, 10)
		pool.get(key)
	}

	// Scan many times the budget in entries never read.
	for i := range 1000 {
		pool.put("scan "+strconv.Itoa(i), decodedBlockCacheEntry{}, 10)
	}

	// Every read entry survives.
	for i := range 50 {
		if _, ok := pool.get("hot " + strconv.Itoa(i)); !ok {
			t.Fatalf("hot entry %d evicted by the scan", i)
		}
	}
	checkDecodedBlockPool(t, pool)
}

// TestDecodedBlockPoolHotEntriesAge checks that entries read often in the past
// leave once reads stop, so they cannot hold the pool against new entries.
func TestDecodedBlockPoolHotEntriesAge(t *testing.T) {
	// Fill the pool with entries read many times.
	pool := newDecodedBlockPoolShards(1000)
	for i := range 100 {
		key := "old " + strconv.Itoa(i)
		pool.put(key, decodedBlockCacheEntry{}, 10)
		for range 20 {
			pool.get(key)
		}
	}

	// Store and read new entries until the old ones are gone.
	for i := range 1000 {
		key := "new " + strconv.Itoa(i)
		pool.put(key, decodedBlockCacheEntry{}, 10)
		pool.get(key)
	}
	for i := range 100 {
		if _, ok := pool.get("old " + strconv.Itoa(i)); ok {
			t.Fatalf("old entry %d outlived ten budgets of read entries", i)
		}
	}
	checkDecodedBlockPool(t, pool)
}

// TestDecodedBlockPoolUpdateDeleteClear checks the cost accounting of an
// update in place, a delete and a clear.
func TestDecodedBlockPoolUpdateDeleteClear(t *testing.T) {
	// Replace an entry with a larger one.
	pool := newDecodedBlockPoolShards(1000)
	pool.put("a", decodedBlockCacheEntry{}, 10)
	pool.put("a", decodedBlockCacheEntry{}, 30)
	if got := pool.stats().retained; got != 30 {
		t.Fatalf("retained %d after update, want 30", got)
	}
	checkDecodedBlockPool(t, pool)

	// Delete it.
	pool.del("a")
	if _, ok := pool.get("a"); ok {
		t.Fatal("deleted entry found")
	}
	if got := pool.stats().retained; got != 0 {
		t.Fatalf("retained %d after delete, want 0", got)
	}
	checkDecodedBlockPool(t, pool)

	// Clear a pool of many entries.
	for i := range 50 {
		pool.put(strconv.Itoa(i), decodedBlockCacheEntry{}, 10)
	}
	pool.clear()
	if got := pool.stats().retained; got != 0 {
		t.Fatalf("retained %d after clear, want 0", got)
	}
	checkDecodedBlockPool(t, pool)
}

// TestDecodedBlockPoolRandom runs random operations, from several goroutines
// on a sharded pool, and checks the pool's accounting after each round.
func TestDecodedBlockPoolRandom(t *testing.T) {
	for _, maxCost := range []int64{1000, 64 << 20} {
		pool := newDecodedBlockPoolShards(maxCost)
		budget := pool.shardBudget()
		for round := range 20 {
			// Run each goroutine's operations on keys the others also use.
			var wg sync.WaitGroup
			for g := range 8 {
				wg.Go(func() {
					rng := rand.New(rand.NewPCG(uint64(round), uint64(g)))
					for range 2000 {
						key := strconv.Itoa(rng.IntN(300))
						switch op := rng.IntN(10); {
						case op < 5:
							pool.get(key)
						case op < 9:
							pool.put(key, decodedBlockCacheEntry{}, 1+rng.Int64N(budget/8))
						default:
							pool.del(key)
						}
					}
				})
			}
			wg.Wait()
			checkDecodedBlockPool(t, pool)
		}
	}
}

// checkDecodedBlockPool checks that every shard is within its budget, that
// its queues hold exactly its entries with their costs, and that the pool's
// retained cost is their sum.
func checkDecodedBlockPool(t *testing.T, pool *decodedBlockPool) {
	// Check each shard under its lock.
	t.Helper()
	budget := pool.shardBudget()
	var retained int64
	for i := range pool.shards {
		s := &pool.shards[i]
		s.mtx.Lock()
		queued := 0
		for _, q := range []*decodedBlockQueue{&s.probation, &s.main} {
			// Walk the queue forward, checking links, membership and cost.
			var cost int64
			var prev *decodedBlockPoolEntry
			for e := q.head; e != nil; e = e.next {
				if e.prev != prev || e.queue != q || s.entries[e.key] != e {
					t.Fatalf("shard %d: entry %q is mislinked", i, e.key)
				}
				cost += e.cost
				prev = e
				queued++
			}
			if prev != q.tail || cost != q.cost {
				t.Fatalf("shard %d: queue tail or cost %d, want %d", i, q.cost, cost)
			}
			retained += cost
		}
		if queued != len(s.entries) {
			t.Fatalf("shard %d: %d entries queued, %d mapped", i, queued, len(s.entries))
		}
		if total := s.probation.cost + s.main.cost; total > budget {
			t.Fatalf("shard %d: cost %d over budget %d", i, total, budget)
		}
		s.mtx.Unlock()
	}

	// The pool's retained counter matches the queues.
	if got := pool.stats().retained; got != retained {
		t.Fatalf("retained %d, queues hold %d", got, retained)
	}
}
