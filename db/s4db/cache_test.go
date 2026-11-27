package s4db

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
)

// TestBlockCacheReaders reads the cache from several goroutines while the
// test inserts past the budget and drops runs. A read takes no lock, so it
// must return the bytes stored under its key or nothing, and afterwards the
// readers' map must hold exactly the blocks the items hold, within the
// budget. Under the race detector the test also fails when a read touches
// what a writer changes under the lock.
func TestBlockCacheReaders(t *testing.T) {
	// Each block's bytes name its key.
	const runs, offs = 8, 64
	c := newBlockCache(64 << 10)
	name := func(k cacheKey) uint64 { return k.run<<32 | k.off }
	block := func(k cacheKey) []byte {
		b := make([]byte, 1<<10)
		binary.LittleEndian.PutUint64(b, name(k))
		return b
	}

	// Read every key in turn until the writer is done.
	var wrong atomic.Int64
	var wg sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for i := uint64(0); ; i++ {
				select {
				case <-done:
					return
				default:
				}
				k := cacheKey{run: i % runs, off: i / runs % offs}
				if b := c.get(k); b != nil && binary.LittleEndian.Uint64(b) != name(k) {
					wrong.Add(1)
				}
			}
		})
	}

	// Insert eight times the budget over and over, dropping a run now and
	// then.
	for i := range uint64(40000) {
		k := cacheKey{run: i % runs, off: i / runs % offs}
		c.put(k, block(k))
		if i%257 == 0 {
			c.drop(i / 257 % runs)
		}
	}
	close(done)
	wg.Wait()
	if n := wrong.Load(); n != 0 {
		t.Fatalf("%d reads returned another key's block", n)
	}

	// The readers' map holds the items' blocks and no other.
	var blocks, bytes int64
	c.blocks.Range(func(k, v any) bool {
		key := k.(cacheKey)
		it := c.items[key.run][key.off]
		if it == nil || it.block != v.(*cacheBlock) {
			t.Errorf("readers see run %d offset %d, which the items do not hold", key.run, key.off)
		}
		blocks++
		bytes += int64(len(v.(*cacheBlock).data))
		return true
	})
	var items int64
	for _, run := range c.items {
		items += int64(len(run))
	}
	if blocks != items || bytes != c.bytes || c.bytes > c.budget || blocks == 0 {
		t.Fatalf("readers see %d blocks of %d bytes, the items hold %d of %d, budget %d", blocks, bytes, items, c.bytes, c.budget)
	}
}
