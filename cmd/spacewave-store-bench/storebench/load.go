//go:build !js

package storebench

import (
	"bytes"
	"context"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// hotLinkPredicate is the graph predicate the writes move between written
// objects.
const hotLinkPredicate = "store-bench/link"

// load is the state of one running load.
type load struct {
	// le logs the load.
	le *logrus.Entry
	// eng is the World under load.
	eng world.Engine
	// conf shapes the load.
	conf *LoadConfig
	// vol is the account volume, or nil.
	vol volume.Volume
	// keys are the sampled store object keys.
	keys []string
	// hot are the keys of the written objects.
	hot []string
	// measuring is set while the measured window runs.
	measuring atomic.Bool
	// viewRuns counts view evaluations in the measured window.
	viewRuns atomic.Uint64
	// writeSeq numbers the writes.
	writeSeq atomic.Uint64
	// dropped counts writes skipped in the measured window.
	dropped atomic.Uint64
	// stallDumps counts the goroutine dumps of stalled writes.
	stallDumps atomic.Uint64
	// flight records the execution trace of the window when stall dumps
	// are on, or is nil.
	flight *trace.FlightRecorder

	// mtx guards the fields below.
	mtx sync.Mutex
	// latencies are the measured write latencies.
	latencies []time.Duration
	// errors counts failed writes in the measured window.
	errors uint64
}

// RunLoad drives conf's load on the store's World and returns the report of
// its measured window.
func RunLoad(ctx context.Context, le *logrus.Entry, st *Store, conf *LoadConfig) (*Report, error) {
	// Sample the store keys the views and writes read.
	eng := st.Engine
	l := &load{le: le, eng: eng, conf: conf, vol: st.Volume}
	keys, err := sampleKeys(ctx, eng, conf.SampleKeys, conf.Seed)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("the World holds no objects to read")
	}
	l.keys = keys

	// Name the hot keys the writes rewrite and create them.
	for i := range max(conf.HotKeys, 1) {
		l.hot = append(l.hot, HotKeyPrefix+"hot-"+strconv.Itoa(i))
	}
	le.WithField("sampled-keys", len(keys)).Info("sampled store keys")
	if err := l.createHot(ctx); err != nil {
		return nil, err
	}

	// Start the view watchers and the writers.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for i := range conf.Watchers {
		wg.Go(func() { l.watch(ctx, i) })
	}
	queue := make(chan struct{}, 1024)
	for range max(conf.Writers, 1) {
		wg.Go(func() { l.writer(ctx, queue) })
	}
	wg.Go(func() { l.schedule(ctx, queue) })

	// Warm the caches.
	le.WithField("warmup", conf.Warmup).Info("warming up")
	if err := sleepCtx(ctx, conf.Warmup); err != nil {
		return nil, err
	}

	// Measure the window under a CPU profile, with GC passes running through
	// it when asked.
	var gcErr error
	if conf.GCMark {
		wg.Go(func() { gcErr = l.collectLoop(ctx) })
	}
	report, err := l.measure(ctx)
	cancel()
	wg.Wait()
	if err == nil {
		err = gcErr
	}
	return report, err
}

// writeAllocProfile writes the heap allocation profile to path plus suffix,
// or nothing for an empty path.
func writeAllocProfile(path, suffix string) error {
	// Skip the profile unless asked for one.
	if path == "" {
		return nil
	}

	// Write the profile as of the last GC.
	var buf bytes.Buffer
	if err := pprof.Lookup("allocs").WriteTo(&buf, 0); err != nil {
		return errors.Wrap(err, "write alloc profile")
	}
	return errors.Wrap(os.WriteFile(path+suffix, buf.Bytes(), 0o644), "write alloc profile")
}

// measure runs the measured window and builds its report.
func (l *load) measure(ctx context.Context) (*Report, error) {
	// Record the counters and allocations at the start of the window.
	cache := block.NewDecodedBlockCache()
	cache0 := cache.Snapshot()
	var mem0 runtime.MemStats
	runtime.ReadMemStats(&mem0)
	pub0 := l.publicationStats()
	if err := writeAllocProfile(l.conf.AllocProfilePath, ".start"); err != nil {
		return nil, err
	}

	// Profile the CPU and start counting the window's writes and views.
	var prof bytes.Buffer
	if err := pprof.StartCPUProfile(&prof); err != nil {
		return nil, errors.Wrap(err, "start cpu profile")
	}
	if fr := l.startFlight(); fr != nil {
		defer fr.Stop()
	}
	l.measuring.Store(true)
	start := time.Now()
	l.le.WithField("duration", l.conf.Duration).Info("measuring")

	// Run the window, then stop measuring and read the counters again.
	werr := sleepCtx(ctx, l.conf.Duration)
	l.measuring.Store(false)
	elapsed := time.Since(start)
	pprof.StopCPUProfile()

	// Read the counters and allocations at the end of the window.
	cache1 := cache.Snapshot()
	var mem1 runtime.MemStats
	runtime.ReadMemStats(&mem1)
	pub1 := l.publicationStats()
	if werr != nil {
		return nil, werr
	}
	if err := writeAllocProfile(l.conf.AllocProfilePath, ""); err != nil {
		return nil, err
	}

	// Keep the raw profile and attribute its CPU.
	if l.conf.ProfilePath != "" {
		if err := os.WriteFile(l.conf.ProfilePath, prof.Bytes(), 0o644); err != nil {
			return nil, errors.Wrap(err, "write cpu profile")
		}
	}
	cpu, err := attributeCPU(prof.Bytes())
	if err != nil {
		return nil, err
	}

	// Build the report from the window's samples and counter changes.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	report := &Report{
		Label:         l.conf.Label,
		DurationMs:    uint64(elapsed / time.Millisecond), // #nosec G115 -- elapsed is positive.
		Watchers:      uint32(l.conf.Watchers),            // #nosec G115 -- a flag bounds the count.
		WriteRate:     l.conf.WriteRate,
		Writes:        uint64(len(l.latencies)),
		WriteErrors:   l.errors,
		WriteLatency:  summarize(l.latencies),
		ViewRuns:      l.viewRuns.Load(),
		Cache:         cacheStats(cache0, cache1),
		Gc:            gcStats(&mem0, &mem1),
		Cpu:           cpu,
		WritesDropped: l.dropped.Load(),
		Writers:       uint32(max(l.conf.Writers, 1)), // #nosec G115 -- a flag bounds the count.
		Publications:  publicationDelta(pub0, pub1),
	}
	return report, nil
}

// schedule queues writes at the configured rate. A full queue drops the write,
// so a slow store shows as dropped writes rather than a slower schedule.
func (l *load) schedule(ctx context.Context, queue chan<- struct{}) {
	// Skip writing when the rate is zero.
	if l.conf.WriteRate <= 0 {
		return
	}

	// Queue one write per tick until the load ends.
	tick := time.NewTicker(time.Duration(float64(time.Second) / l.conf.WriteRate))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		select {
		case queue <- struct{}{}:
		default:
			if l.measuring.Load() {
				l.dropped.Add(1)
			}
		}
	}
}

// writer runs queued writes until the load ends.
func (l *load) writer(ctx context.Context, queue <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-queue:
		}
		l.write(ctx)
	}
}

// write commits one World write and records its times when measuring.
func (l *load) write(ctx context.Context) {
	// Run the write, dumping the goroutines when it stalls.
	seq := l.writeSeq.Add(1)
	stall := l.watchStall(seq)
	start := time.Now()
	err := l.commitWrite(ctx, seq)
	elapsed := time.Since(start)
	if stall != nil {
		stall.Stop()
	}

	// Stop quietly when the load ends.
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		l.le.WithError(err).Warn("write failed")
	}
	if !l.measuring.Load() {
		return
	}

	// Record the write.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if err != nil {
		l.errors++
		return
	}
	l.latencies = append(l.latencies, elapsed)
}

// commitWrite commits write seq: it reads sampled store objects, stores a new
// body in one written object, and moves that object's graph link to the next.
func (l *load) commitWrite(ctx context.Context, seq uint64) error {
	// Open the write.
	tx, err := l.eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Read store objects as a server write does before it changes state.
	for i := range l.conf.WriteReads {
		key := l.keys[(seq*uint64(l.conf.WriteReads)+uint64(i))%uint64(len(l.keys))]
		if err := readObject(ctx, tx, key); err != nil {
			return err
		}
	}

	// Store a new body in the written object.
	n := uint64(len(l.hot))
	key := l.hot[seq%n]
	body := make([]byte, l.conf.BodyBytes)
	copy(body, strconv.FormatUint(seq, 10))
	_, _, err = world.AccessWorldObject(ctx, tx, key, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&body), true)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "write object body")
	}

	// Move its link from the previous written object to the next one.
	next, prev := l.hot[(seq+1)%n], l.hot[(seq+n-1)%n]
	if prev != next {
		if err := tx.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys(key, hotLinkPredicate, prev, "")); err != nil {
			return errors.Wrap(err, "delete graph link")
		}
	}
	if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, hotLinkPredicate, next, "")); err != nil {
		return errors.Wrap(err, "set graph link")
	}
	return tx.Commit(ctx)
}

// createHot creates the written objects the World does not hold yet, so the
// graph links of the first writes have both endpoints.
func (l *load) createHot(ctx context.Context) error {
	// Open one write for every missing object.
	tx, err := l.eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Create each missing object with an empty body.
	for _, key := range l.hot {
		_, _, err := world.AccessWorldObject(ctx, tx, key, true, func(bcs *block.Cursor) error {
			var body []byte
			bcs.SetBlock(byteslice.NewByteSlice(&body), true)
			return nil
		})
		if err != nil {
			return errors.Wrap(err, "create "+key)
		}
	}
	return tx.Commit(ctx)
}

// watch runs view watcher i until the load ends. Its view reads a fixed set
// of sampled store objects and, for a share of the watchers, a written one.
func (l *load) watch(ctx context.Context, i int) {
	// Choose the objects the view reads.
	rng := rand.New(rand.NewPCG(l.conf.Seed, uint64(i))) // #nosec G404 G115 -- seeded load choice; i is a watcher index.
	keys := make([]string, 0, l.conf.ViewReads+1)
	for range l.conf.ViewReads {
		keys = append(keys, l.keys[rng.IntN(len(l.keys))])
	}
	if rng.Float64() < l.conf.HotViewFraction {
		keys = append(keys, l.hot[rng.IntN(len(l.hot))])
	}

	// Watch the view until the load ends.
	opts := world.WatchViewOptions{MinInterval: l.conf.ViewInterval}
	err := world.WatchViewWith(ctx, l.eng, opts, func(ctx context.Context, ws world.WorldState) error {
		if l.measuring.Load() {
			l.viewRuns.Add(1)
		}
		for _, key := range keys {
			if err := readObject(ctx, ws, key); err != nil {
				return err
			}
		}
		return nil
	})
	if ctx.Err() == nil {
		l.le.WithError(err).Warn("view watcher stopped")
	}
}

// readObject reads the root of the object key, which may not exist.
func readObject(ctx context.Context, ws world.WorldState, key string) error {
	// Look up the object, then read its root.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		return err
	}
	_, _, err = obj.GetRootRef(ctx)
	return err
}

// sampleKeys returns up to n store object keys chosen uniformly by seed,
// skipping the objects the load writes.
func sampleKeys(ctx context.Context, eng world.Engine, n int, seed uint64) ([]string, error) {
	// Read the World in a read transaction.
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Reservoir sample the object keys.
	rng := rand.New(rand.NewPCG(seed, 0)) // #nosec G404 -- seeded sample; no secret.
	keys := make([]string, 0, n)
	it := tx.IterateObjects(ctx, "", false)
	defer it.Close()
	seen := 0
	for it.Next() {
		key := it.Key()
		if strings.HasPrefix(key, HotKeyPrefix) {
			continue
		}
		seen++
		if len(keys) < n {
			keys = append(keys, key)
		} else if j := rng.IntN(seen); j < n {
			keys[j] = key
		}
	}
	return keys, it.Err()
}

// cacheStats returns the decoded block cache change from a to b.
func cacheStats(a, b block.DecodedBlockCacheSnapshot) *CacheStats {
	out := &CacheStats{
		MaxCost:      b.MaxCost,
		Hits:         b.Hits - a.Hits,
		Misses:       b.Misses - a.Misses,
		Evictions:    b.Evictions - a.Evictions,
		RetainedCost: b.RetainedCost,
	}
	if lookups := out.Hits + out.Misses; lookups > 0 {
		out.HitRate = float64(out.Hits) / float64(lookups)
	}
	return out
}

// publicationStats returns the account volume's publication counters, or
// zero when the volume does not publish.
func (l *load) publicationStats() volume_kvtx.PublicationStats {
	vol, ok := l.vol.(*volume_kvtx.Volume)
	if !ok {
		return volume_kvtx.PublicationStats{}
	}
	return vol.GetPublicationStats()
}

// publicationDelta returns the publication queue change from a to b.
func publicationDelta(a, b volume_kvtx.PublicationStats) *PublicationStats {
	out := &PublicationStats{
		Accepted:        b.Accepted - a.Accepted,
		PhysicalCommits: b.PhysicalCommits - a.PhysicalCommits,
	}
	if out.PhysicalCommits > 0 {
		out.PerCommit = float64(out.Accepted) / float64(out.PhysicalCommits)
	}
	return out
}

// gcStats returns the garbage collector change from a to b.
func gcStats(a, b *runtime.MemStats) *GcStats {
	return &GcStats{
		Cycles:         b.NumGC - a.NumGC,
		PauseTotalUs:   (b.PauseTotalNs - a.PauseTotalNs) / 1000,
		HeapAllocBytes: b.HeapAlloc,
		AllocBytes:     b.TotalAlloc - a.TotalAlloc,
	}
}

// sleepCtx waits for d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	// Return at whichever comes first.
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
