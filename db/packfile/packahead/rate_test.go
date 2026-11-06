package packahead

import (
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// TestPackAheadRate writes at be-5's measured rate, 940 blocks a second in
// 60-block batches of about 3.3 KB from eight writers, for
// PACKAHEAD_RATE_SECONDS seconds into PACKAHEAD_RATE_DIR or a temporary
// directory, and logs receipt latency, syncs a second and bytes written. It
// is a measurement on a real disk, so it runs only when asked.
func TestPackAheadRate(t *testing.T) {
	seconds, _ := strconv.Atoi(os.Getenv("PACKAHEAD_RATE_SECONDS"))
	if seconds <= 0 {
		t.Skip("set PACKAHEAD_RATE_SECONDS to measure")
	}
	dir := t.TempDir()
	if parent := os.Getenv("PACKAHEAD_RATE_DIR"); parent != "" {
		var err error
		if dir, err = os.MkdirTemp(parent, "packahead-rate-"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	ctx := t.Context()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Each writer sends one 60-block batch every 8 x 60 / 940 seconds.
	const writers, batch, rate = 8, 60, 940
	interval := time.Second * writers * batch / rate
	stop := time.Now().Add(time.Duration(seconds) * time.Second)
	var mtx sync.Mutex
	var latencies []time.Duration
	eg, egCtx := errgroup.WithContext(ctx)
	for w := range writers {
		eg.Go(func() error {
			tick := time.NewTicker(interval)
			defer tick.Stop()
			<-time.After(interval * time.Duration(w) / writers)
			for n := 0; time.Now().Before(stop); n++ {
				blocks := make([]Block, batch)
				for i := range blocks {
					blocks[i] = testBlock(t, (w<<40)|(n*batch+i), 3300)
				}
				started := time.Now()
				if err := l.Append(egCtx, blocks); err != nil {
					return err
				}
				mtx.Lock()
				latencies = append(latencies, time.Since(started))
				mtx.Unlock()
				<-tick.C
			}
			return nil
		})
	}
	began := time.Now()
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(began)

	// Report the receipt latencies and the disk work.
	slices.Sort(latencies)
	at := func(q float64) time.Duration { return latencies[int(q*float64(len(latencies)-1))] }
	stats := l.GetStats()
	t.Logf("batches %d over %s; receipt p50 %s p99 %s max %s; syncs %.1f/s; %.1f MB/s; packs %d",
		len(latencies), elapsed.Round(time.Millisecond), at(0.5), at(0.99), latencies[len(latencies)-1],
		float64(stats.Syncs)/elapsed.Seconds(), float64(stats.LocalBytes)/elapsed.Seconds()/1e6, stats.Packs)
}
