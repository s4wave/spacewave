package resource_world

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// GraphPathQueryResource wraps bounded graph path query results.
type GraphPathQueryResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	pageSize uint32

	mu         sync.Mutex
	objectKeys []string
	quads      []world.GraphQuad
	offset     int
	closed     bool
}

// NewGraphPathQueryResource creates a graph path query resource.
func NewGraphPathQueryResource(
	le *logrus.Entry,
	b bus.Bus,
	result *world.GraphPathQueryResult,
	pageSize uint32,
) *GraphPathQueryResource {
	if result == nil {
		result = &world.GraphPathQueryResult{}
	}
	if pageSize == 0 {
		pageSize = uint32(len(result.ObjectKeys))
		if pageSize == 0 {
			pageSize = 1
		}
	}
	queryResource := &GraphPathQueryResource{
		le:         le,
		b:          b,
		pageSize:   pageSize,
		objectKeys: result.ObjectKeys,
		quads:      result.Quads,
	}
	mux := srpc.NewMux()
	_ = s4wave_world.SRPCRegisterGraphPathQueryResourceService(mux, queryResource)
	queryResource.mux = mux
	return queryResource
}

// GetMux returns the rpc mux.
func (r *GraphPathQueryResource) GetMux() srpc.Invoker {
	return r.mux
}

// Next returns the next page of path query results.
func (r *GraphPathQueryResource) Next(ctx context.Context, req *s4wave_world.NextGraphPathQueryRequest) (*s4wave_world.NextGraphPathQueryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return &s4wave_world.NextGraphPathQueryResponse{Done: true}, nil
	}
	if r.offset >= len(r.objectKeys) {
		quads := append([]world.GraphQuad(nil), r.quads...)
		r.quads = nil
		return &s4wave_world.NextGraphPathQueryResponse{
			Quads: graphQuadsToProto(quads),
			Done:  true,
		}, nil
	}

	end := min(r.offset+int(r.pageSize), len(r.objectKeys))
	objectKeys := append([]string(nil), r.objectKeys[r.offset:end]...)
	done := end >= len(r.objectKeys)
	r.offset = end

	var quads []world.GraphQuad
	if done {
		quads = append([]world.GraphQuad(nil), r.quads...)
		r.quads = nil
	}
	return &s4wave_world.NextGraphPathQueryResponse{
		ObjectKeys: objectKeys,
		Quads:      graphQuadsToProto(quads),
		Done:       done,
	}, nil
}

// Close closes the graph path query resource.
func (r *GraphPathQueryResource) Close(ctx context.Context, req *s4wave_world.CloseGraphPathQueryRequest) (*s4wave_world.CloseGraphPathQueryResponse, error) {
	r.mu.Lock()
	r.objectKeys = nil
	r.quads = nil
	r.closed = true
	r.mu.Unlock()
	return &s4wave_world.CloseGraphPathQueryResponse{}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCGraphPathQueryResourceServiceServer = (*GraphPathQueryResource)(nil)
