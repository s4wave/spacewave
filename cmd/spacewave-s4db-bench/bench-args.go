//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"slices"
	"sync"
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
	}
}

// sample is one Stats reading.
type sample struct {
	// at is the time since the start.
	at time.Duration
	// runs are the runs per level and reserved the reserved chunks.
	runs     []int
	reserved int64
}

// commit is one measured commit.
type commit struct {
	// at is its end since the start, and dur how long it took.
	at, dur time.Duration
}

// Run opens the file, drives the writes and prints the report.
func (a *BenchArgs) Run(c *cli.Context) error {
	// Open the file.
	ctx := c.Context
	db, err := s4db.Open(a.Path, s4db.Options{CacheBytes: a.CacheBytes})
	if err != nil {
		return err
	}
	defer db.Close()

	// Sample stats while the writers run.
	start := time.Now()
	sctx, stopSampler := context.WithCancel(ctx)
	var samples []sample
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		samples = a.sampleStats(sctx, db, start)
	}()

	// Run the writers, then stop the sampler.
	commits, err := a.write(ctx, db, start)
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
func (a *BenchArgs) write(ctx context.Context, db *s4db.DB, start time.Time) ([]commit, error) {
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
			own, err := a.writer(ctx, db, start, uint64(w), batches) // #nosec G115 -- a small count.
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
func (a *BenchArgs) writer(ctx context.Context, db *s4db.DB, start time.Time, w uint64, batches int) ([]commit, error) {
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
	}
	return out, nil
}

// sampleStats reads Stats every Sample until ctx ends.
func (a *BenchArgs) sampleStats(ctx context.Context, db *s4db.DB, start time.Time) []sample {
	var out []sample
	tick := time.NewTicker(a.Sample)
	defer tick.Stop()
	for {
		st, err := db.Stats(ctx)
		if err == nil {
			out = append(out, sample{at: time.Since(start), runs: st.Runs, reserved: st.Chunks.Reserved})
		}
		select {
		case <-ctx.Done():
			return out
		case <-tick.C:
		}
	}
}
