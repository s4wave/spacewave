//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- served only on the --pprof address.
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db"
)

// BenchArgs drive uniform commits into one s4db file and report commit
// latency while the engine's flushes and merges run.
type BenchArgs struct {
	// Path is the database file.
	Path string
	// Ops is the number of key writes.
	Ops int
	// Keys is the key space the writes draw from uniformly; a draw of a
	// written key overwrites it.
	Keys int
	// Batch is the keys per commit.
	Batch int
	// Writers is the number of concurrent writers.
	Writers int
	// ValueBytes is the inline value length.
	ValueBytes int
	// LargeEvery writes a LargeBytes value every this many keys, so value
	// chunks fill and the cleaner has work; zero writes none.
	LargeEvery int
	// LargeBytes is the length of a large value.
	LargeBytes int
	// CacheBytes is the engine's memory budget.
	CacheBytes int
	// Sample is the interval between Stats samples.
	Sample time.Duration
	// Seed seeds the key draws.
	Seed uint64
	// ReportPath is where the JSON report goes; empty prints it.
	ReportPath string
	// Progress is the interval between progress lines on stderr; zero
	// prints none.
	Progress time.Duration
	// Pprof is the address serving the Go profiles; empty serves none.
	Pprof string
}

// BuildFlags binds the flags.
func (a *BenchArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "path", Usage: "database file", Destination: &a.Path, Required: true},
		&cli.IntFlag{Name: "ops", Usage: "key writes", Destination: &a.Ops, Value: 50_000_000},
		&cli.IntFlag{Name: "keys", Usage: "key space the writes draw from", Destination: &a.Keys, Value: 50_000_000},
		&cli.IntFlag{Name: "batch", Usage: "keys per commit", Destination: &a.Batch, Value: 1000},
		&cli.IntFlag{Name: "writers", Usage: "concurrent writers", Destination: &a.Writers, Value: 4},
		&cli.IntFlag{Name: "value-bytes", Usage: "inline value length", Destination: &a.ValueBytes, Value: 100},
		&cli.IntFlag{Name: "large-every", Usage: "a large value every this many keys", Destination: &a.LargeEvery, Value: 50},
		&cli.IntFlag{Name: "large-bytes", Usage: "large value length", Destination: &a.LargeBytes, Value: 4096},
		&cli.IntFlag{Name: "cache-bytes", Usage: "engine memory budget", Destination: &a.CacheBytes, Value: 256 << 20},
		&cli.DurationFlag{Name: "sample", Usage: "stats sample interval", Destination: &a.Sample, Value: 100 * time.Millisecond},
		&cli.Uint64Flag{Name: "seed", Usage: "seeds the key draws", Destination: &a.Seed, Value: 1},
		&cli.StringFlag{Name: "report", Usage: "write the JSON report here instead of printing it", Destination: &a.ReportPath},
		&cli.DurationFlag{Name: "progress", Usage: "interval between progress lines on stderr, zero for none", Destination: &a.Progress, Value: 30 * time.Second},
		&cli.StringFlag{Name: "pprof", Usage: "serve the Go profiles on this address, such as localhost:6060", Destination: &a.Pprof},
	}
}

// sample is one Stats reading.
type sample struct {
	// at is the time since the start.
	at time.Duration
	// runs are the runs per level and reserved the reserved chunks.
	runs     []int
	reserved int64
	// fileBytes is the file's size, which falls when a release truncates it.
	fileBytes int64
}

// commit is one measured commit.
type commit struct {
	// at is its end since the start, and dur how long it took.
	at, dur time.Duration
}

// progress counts the committed work for the progress lines.
type progress struct {
	// keys is the keys committed, and commits the commits.
	keys, commits atomic.Int64
}

// Run opens the file, drives the writes and prints the report.
func (a *BenchArgs) Run(c *cli.Context) error {
	// Serve the profiles when asked.
	ctx := c.Context
	if a.Pprof != "" {
		srv := &http.Server{Addr: a.Pprof, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = srv.ListenAndServe() }()
		defer srv.Close()
	}

	// Open the file.
	db, err := s4db.Open(a.Path, s4db.Options{CacheBytes: a.CacheBytes})
	if err != nil {
		return err
	}
	defer db.Close()

	// Sample stats while the writers run.
	start := time.Now()
	sctx, stopSampler := context.WithCancel(ctx)
	var prog progress
	var samples []sample
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		samples = a.sampleStats(sctx, db, start, &prog)
	}()

	// Run the writers, then stop the sampler.
	commits, err := a.write(ctx, db, start, &prog)
	stopSampler()
	<-sampled
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	// Read the engine's final report.
	stats, err := db.Stats(ctx)
	if err != nil {
		return err
	}

	// Class each commit by the window it ended in and report.
	report := a.classify(samples, commits)
	report.Seconds = elapsed.Seconds()
	report.Ops = a.Ops
	report.Commits = len(commits)
	report.BackgroundWaitMax = stats.BackgroundWaitMax
	report.Stats = stats
	return a.emit(report)
}

// write runs the writers until Ops keys are written and returns every
// commit's timing.
func (a *BenchArgs) write(ctx context.Context, db *s4db.DB, start time.Time, prog *progress) ([]commit, error) {
	// Give each writer its share of the batches and collect their commits.
	batches := (a.Ops + a.Batch - 1) / a.Batch
	var mtx sync.Mutex
	var all []commit
	var firstErr error

	// Run the writers.
	var wg sync.WaitGroup
	for w := range a.Writers {
		wg.Add(1)
		go func() {
			// Write this writer's batches and merge its commits in.
			defer wg.Done()
			own, err := a.writer(ctx, db, start, uint64(w), batches, prog) // #nosec G115 -- a small count.
			mtx.Lock()
			defer mtx.Unlock()
			all = append(all, own...)
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}()
	}
	wg.Wait()

	// Order the commits by their end.
	slices.SortFunc(all, func(x, y commit) int { return int(x.at - y.at) })
	return all, firstErr
}

// writer commits the batches numbered w, w+Writers, ... below batches.
func (a *BenchArgs) writer(ctx context.Context, db *s4db.DB, start time.Time, w uint64, batches int, prog *progress) ([]commit, error) {
	// Draw keys from the writer's own stream.
	rng := rand.New(rand.NewPCG(a.Seed, w)) // #nosec G404 -- a benchmark's key draws.
	small := make([]byte, a.ValueBytes)
	large := make([]byte, a.LargeBytes)
	var out []commit
	for b := int(w); b < batches; b += a.Writers { // #nosec G115 -- a small count.
		// Write one batch.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return out, err
		}
		for i := range a.Batch {
			// Draw the key and pick the value.
			key := binary.BigEndian.AppendUint64(nil, uint64(rng.IntN(a.Keys))) // #nosec G115 -- a key index.
			val := small
			if a.LargeEvery != 0 && (b*a.Batch+i)%a.LargeEvery == 0 {
				val = large
			}
			binary.BigEndian.PutUint64(val, uint64(b*a.Batch+i)) // #nosec G115 -- a key index.
			if err := tx.Set(ctx, key, val); err != nil {
				tx.Discard()
				return out, err
			}
		}

		// Time the commit.
		began := time.Now()
		if err := tx.Commit(ctx); err != nil {
			return out, errors.Wrap(err, "commit")
		}
		end := time.Now()
		out = append(out, commit{at: end.Sub(start), dur: end.Sub(began)})
		prog.keys.Add(int64(a.Batch))
		prog.commits.Add(1)
	}
	return out, nil
}

// sampleStats reads Stats every Sample until ctx ends, printing a progress
// line every Progress.
func (a *BenchArgs) sampleStats(ctx context.Context, db *s4db.DB, start time.Time, prog *progress) []sample {
	// Read a sample each tick until ctx ends.
	var out []sample
	var printed time.Duration
	tick := time.NewTicker(a.Sample)
	defer tick.Stop()
	for {
		// Keep the sample, and print a line when one is due.
		st, err := db.Stats(ctx)
		if err == nil {
			at := time.Since(start)
			out = append(out, sample{at: at, runs: st.Runs, reserved: st.Chunks.Reserved, fileBytes: st.FileBytes})
			if a.Progress > 0 && at-printed >= a.Progress {
				printed = at
				a.printProgress(at, prog, st)
			}
		}
		select {
		case <-ctx.Done():
			return out
		case <-tick.C:
		}
	}
}

// printProgress writes one progress line: the work committed, the engine's
// shape, and the Go heap.
func (a *BenchArgs) printProgress(at time.Duration, prog *progress, st s4db.Stats) {
	// Read the heap and print the line.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(os.Stderr, "%s keys=%d/%d commits=%d runs=%v memtable=%d file=%d free=%d pending=%d reserved=%d heap=%d sys=%d gc=%d\n",
		at.Truncate(time.Second), prog.keys.Load(), a.Ops, prog.commits.Load(), st.Runs, st.MemtableBytes,
		st.FileBytes, st.FreeBytes, st.PendingBytes, st.Chunks.Reserved, ms.HeapInuse, ms.Sys, ms.NumGC)
}
