//go:build js

package resource_debugdb

import (
	"context"
	"runtime"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
)

// suiteRunner manages suite execution against a throw-away engine.
type suiteRunner struct {
	ctx      context.Context
	config   *s4wave_debugdb.BenchmarkConfig
	runner   *BenchmarkRunner
	suites   []string
	results  []*s4wave_debugdb.BenchmarkSuite
	duration time.Duration
}

func newSuiteRunner(ctx context.Context, r *BenchmarkRunner) *suiteRunner {
	suites := []string{
		"blockshard-put-single",
		"blockshard-put-batch",
		"blockshard-get",
		"blockstore-put",
		"blockstore-get",
		"gc-flush",
		"metastore-rw",
	}
	if r.config.GetIncludeWorldSuite() {
		suites = append(suites, "world-tx")
	}
	return &suiteRunner{
		ctx:      ctx,
		config:   r.config,
		runner:   r,
		suites:   suites,
		duration: time.Duration(r.config.GetDurationSeconds()) * time.Second,
	}
}

func (s *suiteRunner) updateProgress(idx int, metric string) {
	pct := uint32(idx * 100 / len(s.suites))
	s.runner.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.runner.progress = s4wave_debugdb.WatchProgressResponse{
			SuiteName:       s.suites[idx],
			SuiteIndex:      uint32(idx),
			SuiteCount:      uint32(len(s.suites)),
			PercentComplete: pct,
			MetricName:      metric,
		}
		broadcast()
	})
}

func (s *suiteRunner) timer(idx int) *SuiteTimer {
	return NewSuiteTimer(s.duration, len(s.suites), idx)
}

// runBlockshardPutSingle benchmarks sequential single-entry Engine.Put calls.
func (s *suiteRunner) runBlockshardPutSingle(engine *blockshard.Engine) *s4wave_debugdb.BenchmarkSuite {
	idx := 0
	s.updateProgress(idx, "put-single")
	timer := s.timer(idx)
	m := NewMetricCollector("put-single", "ms")

	i := 0
	for timer.Running() {
		key := []byte("bench-s-" + strconv.Itoa(i))
		val := make([]byte, 4096)
		// Yield before put so the write actor's flush timer can fire in WASM.
		runtime.Gosched()
		m.Start()
		err := engine.Put(s.ctx, []segment.Entry{{Key: key, Value: val}})
		m.Stop()
		if err != nil {
			break
		}
		i++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "blockshard-put-single",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runBlockshardPutBatch benchmarks batched Engine.Put calls.
func (s *suiteRunner) runBlockshardPutBatch(engine *blockshard.Engine) *s4wave_debugdb.BenchmarkSuite {
	idx := 1
	s.updateProgress(idx, "put-batch")
	timer := s.timer(idx)
	m := NewMetricCollector("put-batch-32", "ms")

	batchSize := 32
	round := 0
	for timer.Running() {
		entries := make([]segment.Entry, batchSize)
		for j := range entries {
			entries[j] = segment.Entry{
				Key:   []byte("bench-b-" + strconv.Itoa(round*batchSize+j)),
				Value: make([]byte, 4096),
			}
		}
		runtime.Gosched()
		m.Start()
		err := engine.Put(s.ctx, entries)
		m.Stop()
		if err != nil {
			break
		}
		round++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "blockshard-put-batch",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runBlockshardGet benchmarks Engine.Get for existing keys.
func (s *suiteRunner) runBlockshardGet(engine *blockshard.Engine) *s4wave_debugdb.BenchmarkSuite {
	idx := 2
	s.updateProgress(idx, "get")
	timer := s.timer(idx)
	m := NewMetricCollector("get", "ms")

	i := 0
	for timer.Running() {
		key := []byte("bench-s-" + strconv.Itoa(i%1000))
		m.Start()
		_, _, _ = engine.Get(key)
		m.Stop()
		m.MaybeYield()
		i++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "blockshard-get",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runBlockStorePut benchmarks PutBlock through the full StoreOps interface.
func (s *suiteRunner) runBlockStorePut(store block.StoreOps) *s4wave_debugdb.BenchmarkSuite {
	idx := 3
	s.updateProgress(idx, "putblock")
	timer := s.timer(idx)
	m := NewMetricCollector("putblock-4k", "ms")

	for timer.Running() {
		data := make([]byte, 4096)
		runtime.Gosched()
		m.Start()
		_, _, err := store.PutBlock(s.ctx, data, nil)
		m.Stop()
		if err != nil {
			break
		}
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "blockstore-put",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runBlockStoreGet benchmarks GetBlock through the full StoreOps interface.
func (s *suiteRunner) runBlockStoreGet(store block.StoreOps, refs []*block.BlockRef) *s4wave_debugdb.BenchmarkSuite {
	idx := 4
	s.updateProgress(idx, "getblock")
	timer := s.timer(idx)
	m := NewMetricCollector("getblock", "ms")

	if len(refs) == 0 {
		return &s4wave_debugdb.BenchmarkSuite{
			Name:    "blockstore-get",
			Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
		}
	}

	i := 0
	for timer.Running() {
		ref := refs[i%len(refs)]
		m.Start()
		_, _, err := store.GetBlock(s.ctx, ref)
		m.Stop()
		if err != nil {
			break
		}
		m.MaybeYield()
		i++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "blockstore-get",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runGCFlush benchmarks FlushPending after buffered block puts.
func (s *suiteRunner) runGCFlush(store *block_gc.GCStoreOps) *s4wave_debugdb.BenchmarkSuite {
	idx := 5
	s.updateProgress(idx, "flush-pending")
	timer := s.timer(idx)
	m := NewMetricCollector("flush-pending", "ms")

	for timer.Running() {
		for range 10 {
			data := make([]byte, 4096)
			runtime.Gosched()
			if _, _, err := store.PutBlock(s.ctx, data, nil); err != nil {
				break
			}
		}
		runtime.Gosched()
		m.Start()
		err := store.FlushPending(s.ctx)
		m.Stop()
		if err != nil {
			break
		}
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "gc-flush",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runMetaStoreRW benchmarks kvtx read/write operations.
func (s *suiteRunner) runMetaStoreRW(store kvtx.Store) *s4wave_debugdb.BenchmarkSuite {
	idx := 6
	s.updateProgress(idx, "meta-rw")
	timer := s.timer(idx)
	mWrite := NewMetricCollector("meta-write", "ms")
	mRead := NewMetricCollector("meta-read", "ms")

	i := 0
	for timer.Running() {
		key := []byte("meta-" + strconv.Itoa(i))
		val := []byte("val-" + strconv.Itoa(i))

		mWrite.Start()
		tx, err := store.NewTransaction(s.ctx, true)
		if err != nil {
			break
		}
		if err := tx.Set(s.ctx, key, val); err != nil {
			tx.Discard()
			break
		}
		err = tx.Commit(s.ctx)
		tx.Discard()
		mWrite.Stop()
		if err != nil {
			break
		}

		mRead.Start()
		rtx, err := store.NewTransaction(s.ctx, false)
		if err != nil {
			break
		}
		_, _, err = rtx.Get(s.ctx, key)
		rtx.Discard()
		mRead.Stop()
		if err != nil {
			break
		}

		mWrite.MaybeYield()
		i++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name: "metastore-rw",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{
			mWrite.Build(),
			mRead.Build(),
		},
	}
}

// createBlockshardEngine creates a standalone blockshard engine for direct benchmarks.
func createBlockshardEngine(ctx context.Context, settings *blockshard.Settings) (*blockshard.Engine, func() error, error) {
	root, err := opfs.GetRoot()
	if err != nil {
		return nil, nil, err
	}
	dirName := "debugdb-bench-engine-" + time.Now().Format("150405")
	dir, err := opfs.GetDirectory(root, dirName, true)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create engine directory")
	}
	engine, err := blockshard.NewEngineWithSettings(ctx, dir, "debugdb-bench", settings)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create engine")
	}
	cleanup := func() error {
		engine.Close()
		return opfs.DeleteEntry(root, dirName, true)
	}
	return engine, cleanup, nil
}
