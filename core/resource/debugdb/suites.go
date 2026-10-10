//go:build js

package resource_debugdb

import (
	"context"
	"encoding/binary"
	"runtime"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
)

// suiteRunner manages suite execution against a throw-away volume.
type suiteRunner struct {
	// ctx bounds all operations in this run.
	ctx context.Context
	// runner owns shared progress and final results.
	runner *BenchmarkRunner
	// suites fixes the selected suite names and order.
	suites []string
	// duration is the total requested benchmark duration.
	duration time.Duration
}

// newSuiteRunner fixes the selected suites and their duration budget.
func newSuiteRunner(ctx context.Context, r *BenchmarkRunner) *suiteRunner {
	suites := []string{
		"engine-put-single",
		"engine-put-batch",
		"engine-get",
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
		runner:   r,
		suites:   suites,
		duration: time.Duration(r.config.GetDurationSeconds()) * time.Second,
	}
}

// updateProgress publishes the current suite and metric to watchers.
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

// timer starts the proportional time budget for one suite.
func (s *suiteRunner) timer(idx int) *SuiteTimer {
	return NewSuiteTimer(s.duration, len(s.suites), idx)
}

// runEnginePutSingle benchmarks durable sequential block writes and retains a
// bounded existing-ref corpus for the engine get suite.
func (s *suiteRunner) runEnginePutSingle(store block.StoreOps) (*s4wave_debugdb.BenchmarkSuite, []*block.BlockRef) {
	// Publish the suite and start its time budget.
	idx := 0
	s.updateProgress(idx, "put-single")
	timer := s.timer(idx)
	m := NewMetricCollector("put-single", "ms")

	// Write and sync one block at a time, retaining refs for the get suite.
	const maxRetainedRefs = 1000
	refs := make([]*block.BlockRef, 0, maxRetainedRefs)
	var id uint64
	for timer.Running() {
		data := engineBenchmarkBlock('s', id)
		runtime.Gosched()
		m.Start()
		ref, _, err := store.PutBlock(s.ctx, data, nil)
		if err == nil {
			_, err = store.Sync(s.ctx)
		}
		m.Stop()
		if err != nil {
			break
		}
		if len(refs) < maxRetainedRefs {
			refs = append(refs, ref)
		}
		id++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "engine-put-single",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}, refs
}

// runEnginePutBatch benchmarks durable batches through the engine block store.
func (s *suiteRunner) runEnginePutBatch(store block.StoreOps) *s4wave_debugdb.BenchmarkSuite {
	// Publish the suite and start its time budget.
	idx := 1
	s.updateProgress(idx, "put-batch")
	timer := s.timer(idx)
	m := NewMetricCollector("put-batch-32", "ms")

	// Write and sync batches of 32 blocks.
	batchSize := 32
	var id uint64
	for timer.Running() {
		entries := make([]*block.PutBatchEntry, batchSize)
		var buildErr error
		for j := range entries {
			data := engineBenchmarkBlock('b', id)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				buildErr = err
				break
			}
			entries[j] = &block.PutBatchEntry{Ref: ref, Data: data}
			id++
		}
		if buildErr != nil {
			break
		}
		runtime.Gosched()
		m.Start()
		_, err := store.PutBlockBatch(s.ctx, entries)
		if err == nil {
			_, err = store.Sync(s.ctx)
		}
		m.Stop()
		if err != nil {
			break
		}
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "engine-put-batch",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// runEngineGet benchmarks engine block reads from the retained existing corpus.
func (s *suiteRunner) runEngineGet(store block.StoreOps, refs []*block.BlockRef) *s4wave_debugdb.BenchmarkSuite {
	// Publish the suite and start its time budget.
	idx := 2
	s.updateProgress(idx, "get")
	timer := s.timer(idx)
	m := NewMetricCollector("get", "ms")

	// Without retained refs there is nothing to read.
	if len(refs) == 0 {
		return &s4wave_debugdb.BenchmarkSuite{
			Name:    "engine-get",
			Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
		}
	}

	// Read the retained blocks round robin.
	i := 0
	for timer.Running() {
		ref := refs[i%len(refs)]
		m.Start()
		_, found, err := store.GetBlock(s.ctx, ref)
		m.Stop()
		if err != nil || !found {
			break
		}
		m.MaybeYield()
		i++
	}

	return &s4wave_debugdb.BenchmarkSuite{
		Name:    "engine-get",
		Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
	}
}

// engineBenchmarkBlock builds one unique deterministic benchmark payload.
func engineBenchmarkBlock(family byte, id uint64) []byte {
	data := make([]byte, 4096)
	data[0] = family
	binary.BigEndian.PutUint64(data[1:], id)
	return data
}

// runBlockStorePut benchmarks PutBlock through the full StoreOps interface.
func (s *suiteRunner) runBlockStorePut(store block.StoreOps) *s4wave_debugdb.BenchmarkSuite {
	// Publish the suite and start its time budget.
	idx := 3
	s.updateProgress(idx, "putblock")
	timer := s.timer(idx)
	m := NewMetricCollector("putblock-4k", "ms")

	// Write one block at a time.
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
	// Publish the suite and start its time budget.
	idx := 4
	s.updateProgress(idx, "getblock")
	timer := s.timer(idx)
	m := NewMetricCollector("getblock", "ms")

	// Without retained refs there is nothing to read.
	if len(refs) == 0 {
		return &s4wave_debugdb.BenchmarkSuite{
			Name:    "blockstore-get",
			Metrics: []*s4wave_debugdb.BenchmarkMetric{m.Build()},
		}
	}

	// Read the retained blocks round robin.
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
	// Publish the suite and start its time budget.
	idx := 5
	s.updateProgress(idx, "flush-pending")
	timer := s.timer(idx)
	m := NewMetricCollector("flush-pending", "ms")

	// Buffer ten puts, then time the flush.
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
	// Publish the suite and start its time budget.
	idx := 6
	s.updateProgress(idx, "meta-rw")
	timer := s.timer(idx)
	mWrite := NewMetricCollector("meta-write", "ms")
	mRead := NewMetricCollector("meta-read", "ms")

	// Time a write transaction, then a read of the same key.
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
