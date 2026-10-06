//go:build js

package resource_debugdb

import (
	"context"
	"runtime"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	"github.com/sirupsen/logrus"
)

// BenchmarkRunner executes storage benchmark suites against a throw-away volume.
type BenchmarkRunner struct {
	// le reports execution and cleanup failures.
	le *logrus.Entry
	// config fixes the options for this benchmark run.
	config *s4wave_debugdb.BenchmarkConfig
	// info describes the runtime in which the measurements were taken.
	info *s4wave_debugdb.StorageInfo

	// bcast protects completion, results, and progress and wakes subscribers.
	bcast broadcast.Broadcast
	// done records completion of all selected suites.
	done bool
	// results remains immutable after completion.
	results *s4wave_debugdb.BenchmarkResults

	// progress is the latest measurement stage shared with watchers.
	progress s4wave_debugdb.WatchProgressResponse
}

// NewBenchmarkRunner creates a new benchmark runner.
func NewBenchmarkRunner(
	le *logrus.Entry,
	config *s4wave_debugdb.BenchmarkConfig,
	info *s4wave_debugdb.StorageInfo,
) *BenchmarkRunner {
	if config.GetDurationSeconds() == 0 {
		config.DurationSeconds = 10
	}
	return &BenchmarkRunner{
		le:     le,
		config: config,
		info:   info,
	}
}

// Run executes all benchmark suites. Call from a goroutine.
func (r *BenchmarkRunner) Run(ctx context.Context) {
	// Record the run's configuration and start.
	start := time.Now()
	results := &s4wave_debugdb.BenchmarkResults{
		Info:                r.info,
		Config:              r.config,
		StartTimeUnixMillis: uint64(start.UnixMilli()),
	}

	// Open the throw-away volume, deleting it when the run ends.
	vol, deleteVol, err := r.allocateVolume(ctx)
	if err != nil {
		r.le.WithError(err).Warn("benchmark: failed to allocate volume")
		r.finish(results, start)
		return
	}
	defer func() {
		vol.Close()
		if deleteErr := deleteVol(); deleteErr != nil {
			r.le.WithError(deleteErr).Warn("benchmark: failed to delete volume")
		}
	}()

	// Engine suites benchmark the volume's block store without the GC wrapper.
	sr := newSuiteRunner(ctx, r)
	putSuite, engineRefs := sr.runEnginePutSingle(vol)
	results.Suites = append(results.Suites, putSuite)
	results.Suites = append(results.Suites, sr.runEnginePutBatch(vol))
	results.Suites = append(results.Suites, sr.runEngineGet(vol, engineRefs))

	// Block store suites: through the full StoreOps interface (includes GC wrapper).
	gcStore := block_gc.NewGCStoreOps(vol, vol.GetRefGraph())
	results.Suites = append(results.Suites, sr.runBlockStorePut(gcStore))

	// Collect block refs for get benchmark.
	var refs []*block.BlockRef
	for i := range 20 {
		data := make([]byte, 4096)
		data[0] = byte(i)
		runtime.Gosched()
		ref, _, err := gcStore.PutBlock(ctx, data, nil)
		if err != nil {
			break
		}
		refs = append(refs, ref)
	}
	results.Suites = append(results.Suites, sr.runBlockStoreGet(gcStore, refs))

	// GC flush suite.
	results.Suites = append(results.Suites, sr.runGCFlush(gcStore))

	// Meta store suite.
	results.Suites = append(results.Suites, sr.runMetaStoreRW(vol.GetKvtxStore()))

	// Publish the results.
	r.finish(results, start)
}

// finish records the results and broadcasts completion.
func (r *BenchmarkRunner) finish(results *s4wave_debugdb.BenchmarkResults, start time.Time) {
	results.TotalDurationMillis = uint64(time.Since(start).Milliseconds())
	r.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Store the results and wake the watchers.
		r.done = true
		r.results = results
		r.progress.Done = true
		r.progress.PercentComplete = 100
		broadcast()
	})
}

// WatchProgress returns the current progress and a channel for changes.
func (r *BenchmarkRunner) WatchProgress() (s4wave_debugdb.WatchProgressResponse, <-chan struct{}) {
	var prog s4wave_debugdb.WatchProgressResponse
	var ch <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		prog = r.progress
		ch = getWaitCh()
	})
	return prog, ch
}

// GetResults waits for completion and returns the results.
func (r *BenchmarkRunner) GetResults(ctx context.Context) (*s4wave_debugdb.BenchmarkResults, error) {
	for {
		var results *s4wave_debugdb.BenchmarkResults
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if r.done {
				results = r.results
			}
			ch = getWaitCh()
		})
		if results != nil {
			return results, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

// allocateVolume opens a throw-away browser volume under a unique name. The
// caller closes the volume, then calls the returned func to delete it.
func (r *BenchmarkRunner) allocateVolume(ctx context.Context) (*volume_browser.Volume, func() error, error) {
	// Open the volume on the device the browser selects.
	name := "debugdb-bench-" + time.Now().Format("20060102-150405.000000000")
	conf := &volume_browser.Config{Name: name}
	vol, err := volume_browser.NewVolume(ctx, r.le.WithField("volume", name), conf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create benchmark volume")
	}

	// Yield before the benchmark starts timing.
	runtime.Gosched()
	return vol, func() error { return volume_browser.Delete(name) }, nil
}
