// Command bench drives one key-value engine through a workload shaped like a
// Spacewave volume and prints its costs. Each run uses one engine in a fresh
// process, so heap and resident memory belong to that engine alone.
//
// Phases: fill, close and reopen, random point reads, prefix scans, and a
// churn phase that deletes half the block keys and writes as many new ones.
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// engine opens one key-value engine in a directory.
type engine struct {
	// name selects the engine on the command line.
	name string
	// open opens or creates the engine's files under dir.
	open func(ctx context.Context, dir string) (store, error)
}

// store is an open engine.
type store interface {
	kvtx.Store
	// Close flushes and closes the engine.
	Close() error
}

// syncer is a store that can make ordered commits durable.
type syncer interface {
	Sync(ctx context.Context) error
}

// workload is the generated key set and value sizes.
type workload struct {
	// keys holds every key in insertion order.
	keys [][]byte
	// sizes holds each key's value size.
	sizes []int
	// objects is the number of distinct object prefixes among kv keys.
	objects int
}

// main runs the fill, cold read, churn, and compaction phases against one
// engine and prints their measurements.
func main() {
	// Parse the engine, directory, and workload size.
	engineName := flag.String("engine", "bolt", "engine: "+engineNames())
	dir := flag.String("dir", "", "directory for the engine's files")
	n := flag.Int("n", 200000, "number of keys")
	profile := flag.String("values", "small", "value profile: small (kv-heavy, 100 B) or blocks (mixed up to 256 KiB)")

	// Parse the commit and phase options.
	batch := flag.Int("batch", 128, "operations per commit")
	ordered := flag.Bool("ordered", false, "commit with write ordering and Sync every 32 commits")
	reads := flag.Int("reads", 50000, "random point reads")
	churn := flag.Bool("churn", true, "run the churn phase")
	flag.Parse()

	// Resolve the engine and prepare an empty directory.
	var eng *engine
	for i := range engines {
		if engines[i].name == *engineName {
			eng = &engines[i]
		}
	}
	if eng == nil || *dir == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := os.RemoveAll(*dir); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		panic(err)
	}

	// Generate the workload.
	ctx := context.Background()
	w := newWorkload(*n, *profile)
	fmt.Printf("engine %s  keys %d  values %s  batch %d  ordered %v  live %s\n",
		eng.name, *n, *profile, *batch, *ordered, mib(w.liveBytes()))

	// Fill the engine.
	s, err := eng.open(ctx, *dir)
	if err != nil {
		panic(err)
	}

	// Write every key and report throughput and write amplification.
	io0 := ioWritten()
	start := time.Now()
	lat := w.write(ctx, s, 0, len(w.keys), *batch, *ordered)
	fill := time.Since(start)
	syncStore(ctx, s)
	written := ioWritten() - io0
	fmt.Printf("fill        %8.2f s  %9.0f keys/s  %8.1f MiB/s  commit %s  written %s (%.2fx live)\n",
		fill.Seconds(), float64(*n)/fill.Seconds(), float64(w.liveBytes())/(1<<20)/fill.Seconds(),
		pct(lat), mib(written), float64(written)/float64(w.liveBytes()))
	fmt.Printf("after fill  disk %s  heap %s  rss %s\n", mib(diskUsage(*dir)), mib(heap()), mib(rss()))

	// Close, drop the page cache for the engine's files, and reopen.
	if err := s.Close(); err != nil {
		panic(err)
	}
	dropCache(*dir)
	runtime.GC()
	start = time.Now()
	s, err = eng.open(ctx, *dir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("reopen      %8.1f ms  heap %s  rss %s\n", ms(time.Since(start)), mib(heap()), mib(rss()))

	// Read random existing keys, first with a cold page cache.
	rng := rand.New(rand.NewPCG(1, 2))
	rlat := make([]time.Duration, 0, *reads)
	start = time.Now()
	for range *reads {
		i := rng.IntN(len(w.keys))
		t0 := time.Now()
		tx, err := s.NewTransaction(ctx, false)
		if err != nil {
			panic(err)
		}
		val, found, err := tx.Get(ctx, w.keys[i])
		if err != nil || !found || len(val) != w.sizes[i] {
			panic(fmt.Sprintf("read %d: found %v len %d want %d err %v", i, found, len(val), w.sizes[i], err))
		}
		tx.Discard()
		rlat = append(rlat, time.Since(t0))
	}
	el := time.Since(start)
	fmt.Printf("read        %8.0f gets/s  %s  heap %s  rss %s\n", float64(*reads)/el.Seconds(), pct(rlat), mib(heap()), mib(rss()))

	// Scan object prefixes, as World state reads do.
	slat := make([]time.Duration, 0, 2000)
	entries := 0
	for range 2000 {
		prefix := objectPrefix(rng.IntN(w.objects))
		t0 := time.Now()
		tx, err := s.NewTransaction(ctx, false)
		if err != nil {
			panic(err)
		}
		it := tx.Iterate(ctx, prefix, true, false)
		for it.Next() {
			if _, err := it.Value(); err != nil {
				panic(err)
			}
			entries++
		}
		if err := it.Err(); err != nil {
			panic(err)
		}
		it.Close()
		tx.Discard()
		slat = append(slat, time.Since(t0))
	}
	fmt.Printf("scan        %d entries  %s\n", entries, pct(slat))

	// Delete half the block keys, then write as many new ones.
	if *churn {
		io0 = ioWritten()
		start = time.Now()
		deleted := w.deleteHalf(ctx, s, *batch, *ordered)
		syncStore(ctx, s)
		fmt.Printf("delete      %8.2f s  %d keys  disk %s  live %s\n", time.Since(start).Seconds(), deleted, mib(diskUsage(*dir)), mib(w.liveBytes()))
		first := len(w.keys)
		w.extend(deleted, *profile)
		start = time.Now()
		w.write(ctx, s, first, len(w.keys), *batch, *ordered)
		syncStore(ctx, s)
		written = ioWritten() - io0
		fmt.Printf("rewrite     %8.2f s  disk %s  live %s  written %s\n", time.Since(start).Seconds(), mib(diskUsage(*dir)), mib(w.liveBytes()), mib(written))
		if c, ok := s.(interface{ Compact() error }); ok {
			start = time.Now()
			if err := c.Compact(); err != nil {
				panic(err)
			}
			fmt.Printf("compact     %8.2f s  disk %s\n", time.Since(start).Seconds(), mib(diskUsage(*dir)))
		}
		if err := s.Close(); err != nil {
			panic(err)
		}
		fmt.Printf("closed      disk %s  live %s  space amp %.2fx\n", mib(diskUsage(*dir)), mib(w.liveBytes()), float64(diskUsage(*dir))/float64(w.liveBytes()))
		return
	}
	if err := s.Close(); err != nil {
		panic(err)
	}
}

// newWorkload generates n keys. Seven in ten are block keys, a prefix and 32
// random bytes; the rest are object fields that share an object prefix.
func newWorkload(n int, profile string) *workload {
	w := &workload{objects: max(1, n*3/10/8)}
	w.extend(n, profile)
	return w
}

// extend appends n new keys with values from profile.
func (w *workload) extend(n int, profile string) {
	rng := rand.New(rand.NewPCG(uint64(len(w.keys)), 7))
	for i := range n {
		if i%10 < 7 {
			key := make([]byte, 2+32)
			copy(key, "b/")
			for j := 2; j < len(key); j += 8 {
				binary.LittleEndian.PutUint64(key[j:], rng.Uint64())
			}
			w.keys = append(w.keys, key)
			w.sizes = append(w.sizes, blockSize(rng, profile))
			continue
		}
		obj := rng.IntN(w.objects)
		key := binary.BigEndian.AppendUint64(objectPrefix(obj), rng.Uint64())
		w.keys = append(w.keys, key)
		w.sizes = append(w.sizes, 100+rng.IntN(200))
	}
}

// blockSize draws a block payload size from profile.
func blockSize(rng *rand.Rand, profile string) int {
	if profile == "small" {
		return 100
	}
	switch r := rng.IntN(10); {
	case r < 4:
		return 256
	case r < 7:
		return 4096
	case r < 9:
		return 32 << 10
	default:
		return 256 << 10
	}
}

// objectPrefix returns the key prefix of one World object's fields.
func objectPrefix(obj int) []byte {
	return fmt.Appendf(nil, "w/%08x/", obj)
}

// write sets keys [from, to) in commits of batch operations.
func (w *workload) write(ctx context.Context, s store, from, to, batch int, ordered bool) []time.Duration {
	// Fill values with random bytes so compressing engines gain nothing.
	val := make([]byte, 256<<10)
	vrng := rand.New(rand.NewPCG(uint64(from), 3))
	for i := 0; i < len(val); i += 8 {
		binary.LittleEndian.PutUint64(val[i:], vrng.Uint64())
	}
	var lat []time.Duration
	commits := 0
	for lo := from; lo < to; lo += batch {
		hi := min(lo+batch, to)
		t0 := time.Now()
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			panic(err)
		}
		for i := lo; i < hi; i++ {
			if w.sizes[i] < 0 {
				continue
			}
			off := (i * 4099) % (len(val) - w.sizes[i] + 1)
			if err := tx.Set(ctx, w.keys[i], val[off:off+w.sizes[i]]); err != nil {
				panic(err)
			}
		}
		commit(ctx, s, tx, ordered, &commits)
		lat = append(lat, time.Since(t0))
	}
	return lat
}

// deleteHalf deletes every other block key and returns the count.
func (w *workload) deleteHalf(ctx context.Context, s store, batch int, ordered bool) int {
	// Choose every other block key.
	var victims []int
	for i, k := range w.keys {
		if k[0] == 'b' && w.sizes[i] >= 0 && i%2 == 0 {
			victims = append(victims, i)
		}
	}

	// Delete them in batches.
	commits := 0
	for lo := 0; lo < len(victims); lo += batch {
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			panic(err)
		}
		for _, i := range victims[lo:min(lo+batch, len(victims))] {
			if err := tx.Delete(ctx, w.keys[i]); err != nil {
				panic(err)
			}
			w.sizes[i] = -1
		}
		commit(ctx, s, tx, ordered, &commits)
	}

	// Drop deleted keys so later reads and live sizes skip them.
	keep := 0
	for i := range w.keys {
		if w.sizes[i] >= 0 {
			w.keys[keep], w.sizes[keep] = w.keys[i], w.sizes[i]
			keep++
		}
	}
	w.keys, w.sizes = w.keys[:keep], w.sizes[:keep]
	return len(victims)
}

// commit commits tx durably, or with write ordering and a Sync every 32
// commits when ordered is set.
func commit(ctx context.Context, s store, tx kvtx.Tx, ordered bool, commits *int) {
	// Commit in the requested mode.
	var err error
	if ordered {
		err = kvtx.CommitOrdered(ctx, tx)
	} else {
		err = tx.Commit(ctx)
	}
	if err != nil {
		panic(err)
	}

	// Flush ordered commits every 32 commits.
	*commits++
	if ordered && *commits%32 == 0 {
		syncStore(ctx, s)
	}
}

// syncStore makes ordered commits durable when the store supports it.
func syncStore(ctx context.Context, s store) {
	if sy, ok := s.(syncer); ok {
		if err := sy.Sync(ctx); err != nil {
			panic(err)
		}
	}
}

// liveBytes sums the key and value bytes of the live keys.
func (w *workload) liveBytes() int64 {
	var n int64
	for i, k := range w.keys {
		if w.sizes[i] >= 0 {
			n += int64(len(k) + w.sizes[i])
		}
	}
	return n
}

// diskUsage returns the bytes the file system allocated under dir.
func diskUsage(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			total += allocated(path)
		}
		return nil
	})
	return total
}

// dropCache evicts the page cache for every file under dir where supported.
func dropCache(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			evict(path)
		}
		return nil
	})
}

// heap returns the live Go heap after a collection.
func heap() int64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.HeapInuse)
}

// pct formats the median and 99th percentile of lat.
func pct(lat []time.Duration) string {
	// Nothing was measured.
	if len(lat) == 0 {
		return "-"
	}

	// Sort a copy and read the quantiles.
	s := slices.Clone(lat)
	slices.Sort(s)
	p := func(q float64) time.Duration { return s[int(q*float64(len(s)-1))] }
	return fmt.Sprintf("p50 %s p99 %s", p(0.5).Round(time.Microsecond), p(0.99).Round(time.Microsecond))
}

// mib formats a byte count in MiB.
func mib(n int64) string {
	return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
}

// ms formats a duration in milliseconds.
func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

// engineNames lists the selectable engines.
func engineNames() string {
	var names []string
	for _, e := range engines {
		names = append(names, e.name)
	}
	return fmt.Sprint(names)
}
