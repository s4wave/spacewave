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
	"github.com/s4wave/spacewave/db/opfs"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	"github.com/sirupsen/logrus"
)

// BenchmarkRunner executes storage benchmark suites against a throw-away volume.
type BenchmarkRunner struct {
	le     *logrus.Entry
	config *s4wave_debugdb.BenchmarkConfig
	info   *s4wave_debugdb.StorageInfo

	bcast   broadcast.Broadcast
	done    bool
	results *s4wave_debugdb.BenchmarkResults

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
	if info.GetBlockShardCount() == 0 {
		info.BlockShardCount = blockshard.DefaultShardCount
	}
	if info.GetPageSize() == 0 {
		info.PageSize = pagestore.DefaultPageSize
	}
	return &BenchmarkRunner{
		le:     le,
		config: config,
		info:   info,
	}
}

// Run executes all benchmark suites. Call from a goroutine.
func (r *BenchmarkRunner) Run(ctx context.Context) {
	start := time.Now()
	results := &s4wave_debugdb.BenchmarkResults{
		Info:                r.info,
		Config:              r.config,
		StartTimeUnixMillis: uint64(start.UnixMilli()),
	}

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

	sr := newSuiteRunner(ctx, r)

	// Blockshard suites: direct engine benchmarks.
	engine, engineCleanup, err := createBlockshardEngine(ctx, &blockshard.Settings{
		ShardCount: int(r.info.GetBlockShardCount()),
		AsyncIO:    r.info.GetAsyncIo(),
	})
	if err != nil {
		r.le.WithError(err).Warn("benchmark: failed to create blockshard engine")
		r.finish(results, start)
		return
	}
	results.Suites = append(results.Suites, sr.runBlockshardPutSingle(engine))
	results.Suites = append(results.Suites, sr.runBlockshardPutBatch(engine))
	results.Suites = append(results.Suites, sr.runBlockshardGet(engine))
	if err := engineCleanup(); err != nil {
		r.le.WithError(err).Warn("benchmark: failed to cleanup blockshard engine")
	}

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

	r.finish(results, start)
}

// finish records the results and broadcasts completion.
func (r *BenchmarkRunner) finish(results *s4wave_debugdb.BenchmarkResults, start time.Time) {
	results.TotalDurationMillis = uint64(time.Since(start).Milliseconds())
	r.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
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

// allocateVolume creates a throw-away OPFS volume for benchmarking.
func (r *BenchmarkRunner) allocateVolume(ctx context.Context) (*volume_opfs.Opfs, func() error, error) {
	rootPath := "debugdb-bench-" + time.Now().Format("20060102-150405")
	conf := &volume_opfs.Config{
		RootPath:        rootPath,
		BlockShardCount: r.info.GetBlockShardCount(),
		PageSize:        r.info.GetPageSize(),
		AsyncIo:         r.info.GetAsyncIo(),
	}

	le := r.le.WithField("volume", rootPath)
	vol, err := volume_opfs.NewOpfs(ctx, le, conf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create benchmark volume")
	}

	deleteVol := func() error {
		root, err := opfs.GetRoot()
		if err != nil {
			return err
		}
		return opfs.DeleteEntry(root, rootPath, true)
	}

	runtime.Gosched()
	return vol, deleteVol, nil
}
