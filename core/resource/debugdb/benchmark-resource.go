package resource_debugdb

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	"github.com/sirupsen/logrus"
)

// BenchmarkResource wraps a BenchmarkRunner as a DebugDbBenchmarkService resource.
type BenchmarkResource struct {
	le     *logrus.Entry
	runner *BenchmarkRunner
	mux    srpc.Invoker
}

// NewBenchmarkResource creates a new BenchmarkResource.
func NewBenchmarkResource(le *logrus.Entry, runner *BenchmarkRunner) *BenchmarkResource {
	r := &BenchmarkResource{le: le, runner: runner}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_debugdb.SRPCRegisterDebugDbBenchmarkService(mux, r)
	})
	return r
}

// GetMux returns the rpc mux.
func (r *BenchmarkResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchProgress streams benchmark progress updates.
func (r *BenchmarkResource) WatchProgress(
	_ *s4wave_debugdb.WatchProgressRequest,
	strm s4wave_debugdb.SRPCDebugDbBenchmarkService_WatchProgressStream,
) error {
	ctx := strm.Context()
	for {
		prog, ch := r.runner.WatchProgress()
		if err := strm.Send(&prog); err != nil {
			return err
		}
		if prog.Done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// GetResults returns the full benchmark results after completion.
func (r *BenchmarkResource) GetResults(
	ctx context.Context,
	_ *s4wave_debugdb.GetResultsRequest,
) (*s4wave_debugdb.GetResultsResponse, error) {
	results, err := r.runner.GetResults(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_debugdb.GetResultsResponse{Results: results}, nil
}

// _ is a type assertion.
var _ s4wave_debugdb.SRPCDebugDbBenchmarkServiceServer = ((*BenchmarkResource)(nil))
