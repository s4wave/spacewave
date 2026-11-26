package packcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// testBlock returns block i of size bytes, keyed by the hash of i.
func testBlock(i, size int) packahead.Block {
	var seed [8]byte
	binary.LittleEndian.PutUint64(seed[:], uint64(i)) //nolint:gosec // test index.
	key := sha256.Sum256(seed[:])
	value := bytes.Repeat(key[:1], size)
	binary.LittleEndian.PutUint64(value, uint64(i)) //nolint:gosec // test index.
	return packahead.Block{Key: key[:], Value: value}
}

// openReady opens a cache in dir and waits for its load.
func openReady(t *testing.T, dir string, opts Options) *Cache {
	t.Helper()
	c := Open(t.Context(), dir, opts)
	if err := c.Ready(t.Context()); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCacheCarriesHotSet fills a cache to several times its limit while a hot
// set is read between fills, and checks that every hot value stays readable
// through each eviction pass while the cold values age out.
func TestCacheCarriesHotSet(t *testing.T) {
	const limit, packBytes, size = 256 << 10, 64 << 10, 1 << 10
	c := openReady(t, t.TempDir(), Options{LimitBytes: limit, Log: packahead.Options{MaxPackBytes: packBytes}})
	defer c.Close()

	// Store the hot set, then fill eight limits of cold values in batches,
	// reading every hot value after each batch.
	hot := make([]packahead.Block, 16)
	for i := range hot {
		hot[i] = testBlock(i, size)
	}
	if err := c.Put(t.Context(), hot); err != nil {
		t.Fatal(err)
	}
	const cold = 8 * limit / size
	for i := 0; i < cold; i += 16 {
		batch := make([]packahead.Block, 16)
		for j := range batch {
			batch[j] = testBlock(len(hot)+i+j, size)
		}
		if err := c.Put(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		for _, b := range hot {
			value, found, err := c.Get(b.Key)
			if err != nil || !found || !bytes.Equal(value, b.Value) {
				t.Fatalf("hot block %x after %d cold: found %v, err %v", b.Key[:4], i, found, err)
			}
		}
	}

	// The evictor works the cache back to its limit, past two full passes.
	deadline := time.Now().Add(5 * time.Second)
	for c.Size() > limit+2*packBytes {
		if time.Now().After(deadline) {
			t.Fatalf("size %d stayed over the limit %d", c.Size(), limit)
		}
		time.Sleep(10 * time.Millisecond)
	}
	counts := c.Counts()
	if counts.Evicted < 2*limit/packBytes || counts.Carried == 0 {
		t.Fatalf("evicted %d packs, carried %d values", counts.Evicted, counts.Carried)
	}
	if _, found, _ := c.Get(testBlock(len(hot), size).Key); found {
		t.Fatal("the first cold block survived eight limits of fills")
	}
	for _, b := range hot {
		if _, found, _ := c.Get(b.Key); !found {
			t.Fatalf("hot block %x was evicted", b.Key[:4])
		}
	}
}

// TestCacheReopenKeepsRecent checks that startup eviction under a lowered
// limit carries the named old values, leaves removed and unknown keys absent,
// and never returns a value that no Put stored under the key.
func TestCacheReopenKeepsRecent(t *testing.T) {
	// Fill eight limits of values before enabling eviction.
	const limit, packBytes, size = 256 << 10, 64 << 10, 1 << 10
	const total, batchSize = 8 * limit / size, 16
	dir := t.TempDir()
	logOpts := packahead.Options{MaxPackBytes: packBytes}
	c := openReady(t, dir, Options{Log: logOpts})
	defer c.Close()
	for i := 0; i < total; i += batchSize {
		batch := make([]packahead.Block, batchSize)
		for j := range batch {
			batch[j] = testBlock(i+j, size)
		}
		if err := c.Put(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
	}
	c.Close()

	// Name the oldest batch, a key to remove, and keys never stored.
	const removedIndex, unknownCount = 2 * batchSize, 4
	removedKey := testBlock(removedIndex, size).Key
	keys := make([][]byte, 0, batchSize+1+unknownCount)
	for i := range batchSize {
		keys = append(keys, testBlock(i, size).Key)
	}
	keys = append(keys, removedKey)
	for i := range unknownCount {
		keys = append(keys, testBlock(1_000_000+i, size).Key)
	}

	// Remove the key after load and before Recent marks its slice.
	removed := make(chan struct{})
	c = openReady(t, dir, Options{
		LimitBytes: limit,
		Log:        logOpts,
		Recent: func(_ context.Context, mark func(keys [][]byte)) {
			<-removed
			mark(keys)
		},
	})
	defer c.Close()
	c.Remove(removedKey)
	close(removed)

	// Wait for eviction to reach the limit within two packs.
	deadline := time.Now().Add(5 * time.Second)
	for c.Size() > limit+2*packBytes {
		if time.Now().After(deadline) {
			t.Fatalf("size %d stayed over the limit %d", c.Size(), limit)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Check every written key and confirm that eviction reached the first cold batch.
	var coldMiss bool
	for i := range total {
		b := testBlock(i, size)
		value, found, err := c.Get(b.Key)
		if err != nil {
			t.Fatalf("read block %d: %v", i, err)
		}
		if found && !bytes.Equal(value, b.Value) {
			t.Fatalf("block %d returned a value that Put did not store under its key", i)
		}
		if i < batchSize && !found {
			t.Fatalf("recent block %d was evicted", i)
		}
		if i == removedIndex && found {
			t.Fatalf("removed block %d was found", i)
		}
		if i >= batchSize && i < 2*batchSize && !found {
			coldMiss = true
		}
	}
	// Check that marking the unknown keys did not make them readable.
	for _, key := range keys[batchSize+1:] {
		if _, found, err := c.Get(key); err != nil || found {
			t.Fatalf("unknown block %x: found %v, err %v", key[:4], found, err)
		}
	}

	// Require both an eviction of a cold value and a carry of a recent value.
	if carried := c.Counts().Carried; !coldMiss || carried == 0 {
		t.Fatalf("old cold block missed: %v, carried %d values", coldMiss, carried)
	}
}

// TestCacheTornOpenPack cuts the open pack inside its last batch, as a crash
// before the batch reached the drive would, and checks that the reopened
// cache misses that batch's values and returns every earlier one intact.
func TestCacheTornOpenPack(t *testing.T) {
	dir := t.TempDir()
	const size = 2 << 10
	c := openReady(t, dir, Options{})
	var batches [][]packahead.Block
	for b := range 3 {
		batch := make([]packahead.Block, 8)
		for j := range batch {
			batch[j] = testBlock(b*8+j, size)
		}
		if err := c.Put(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, batch)
	}
	c.Close()

	// Cut the open pack, the largest one, halfway into its last value.
	var torn string
	var tornSize int64
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(e.Name(), ".open") && info.Size() > tornSize {
			torn, tornSize = filepath.Join(dir, e.Name()), info.Size()
		}
	}
	if torn == "" {
		t.Fatal("no open pack holds the batches")
	}
	if err := os.Truncate(torn, tornSize-size/2); err != nil {
		t.Fatal(err)
	}

	// The reopened cache returns the first two batches and misses the last.
	c = openReady(t, dir, Options{})
	defer c.Close()
	for b, batch := range batches {
		for _, blk := range batch {
			value, found, err := c.Get(blk.Key)
			if err != nil {
				t.Fatal(err)
			}
			if b < 2 && (!found || !bytes.Equal(value, blk.Value)) {
				t.Fatalf("batch %d block %x: found %v, value intact %v", b, blk.Key[:4], found, bytes.Equal(value, blk.Value))
			}
			if b == 2 && found {
				t.Fatalf("torn batch block %x was found", blk.Key[:4])
			}
		}
	}
}
