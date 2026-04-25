//go:build !js

package resource_debugdb

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	"github.com/sirupsen/logrus"
)

// BenchmarkRunner executes storage benchmark suites.
// On non-WASM platforms this is a stub that returns an error.
type BenchmarkRunner struct {
	bcast    broadcast.Broadcast
	done     bool
	results  *s4wave_debugdb.BenchmarkResults
	progress s4wave_debugdb.WatchProgressResponse
}

// NewBenchmarkRunner creates a new benchmark runner.
func NewBenchmarkRunner(
	_ *logrus.Entry,
	_ *s4wave_debugdb.BenchmarkConfig,
	_ *s4wave_debugdb.StorageInfo,
) *BenchmarkRunner {
	return &BenchmarkRunner{}
}

// Run is a no-op on non-WASM platforms.
func (r *BenchmarkRunner) Run(_ context.Context) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.done = true
		r.results = &s4wave_debugdb.BenchmarkResults{}
		r.progress.Done = true
		broadcast()
	})
}

// WatchProgress returns the current progress.
func (r *BenchmarkRunner) WatchProgress() (s4wave_debugdb.WatchProgressResponse, <-chan struct{}) {
	var prog s4wave_debugdb.WatchProgressResponse
	var ch <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		prog = r.progress
		ch = getWaitCh()
	})
	return prog, ch
}

// GetResults returns an error on non-WASM platforms.
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

// ErrNotSupported is returned when benchmarking is not available.
var ErrNotSupported = errors.New("storage benchmarks only available in browser")
