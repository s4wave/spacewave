package resource_debugdb

import (
	"context"
	"runtime"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	"github.com/sirupsen/logrus"
)

// DebugDbResource implements the DebugDbResourceService.
type DebugDbResource struct {
	// le reports benchmark failures.
	le *logrus.Entry
	// b provides the resource's controller bus.
	b bus.Bus
	// mux serves the resource RPC methods.
	mux srpc.Invoker
}

// NewDebugDbResource creates a new DebugDbResource.
func NewDebugDbResource(le *logrus.Entry, b bus.Bus) *DebugDbResource {
	r := &DebugDbResource{le: le, b: b}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_debugdb.SRPCRegisterDebugDbResourceService(mux, r)
	})
	return r
}

// GetMux returns the rpc mux.
func (r *DebugDbResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetStorageInfo returns information about the current storage backend.
func (r *DebugDbResource) GetStorageInfo(
	_ context.Context,
	_ *s4wave_debugdb.GetStorageInfoRequest,
) (*s4wave_debugdb.GetStorageInfoResponse, error) {
	return &s4wave_debugdb.GetStorageInfoResponse{Info: storageInfo()}, nil
}

// StartBenchmark starts a new benchmark run.
func (r *DebugDbResource) StartBenchmark(
	ctx context.Context,
	req *s4wave_debugdb.StartBenchmarkRequest,
) (*s4wave_debugdb.StartBenchmarkResponse, error) {
	info := storageInfo()

	_, resourceID, err := resource_server.ConstructChildResource(ctx,
		func(subCtx context.Context) (srpc.Invoker, struct{}, func(), error) {
			runner := NewBenchmarkRunner(r.le, req.GetConfig(), info)
			benchResource := NewBenchmarkResource(r.le, runner)
			go runner.Run(subCtx)
			return benchResource.GetMux(), struct{}{}, nil, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_debugdb.StartBenchmarkResponse{ResourceId: resourceID}, nil
}

// storageInfo describes the browser volume and the runtime it runs in.
func storageInfo() *s4wave_debugdb.StorageInfo {
	return &s4wave_debugdb.StorageInfo{
		VolumeType: "hydra/volume/browser",
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

// _ is a type assertion.
var _ s4wave_debugdb.SRPCDebugDbResourceServiceServer = (*DebugDbResource)(nil)
