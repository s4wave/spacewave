package s4wave_flowgraph_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// FlowgraphTypeID is the type identifier for authored Flowgraphs.
const FlowgraphTypeID = s4wave_flowgraph.FlowgraphTypeID

// FlowgraphType exposes independently addressed Flowgraph objects.
var FlowgraphType = objecttype.NewObjectType(s4wave_flowgraph.FlowgraphTypeID, FlowgraphFactory)

// FlowgraphFactory serves a graph through the granting World capability.
func FlowgraphFactory(ctx context.Context, _ *logrus.Entry, _ bus.Bus, engine world.Engine, ws world.WorldState, key string) (srpc.Invoker, func(), error) {
	// Require the granted World state and a decodable graph body.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	if _, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, key); err != nil {
		return nil, nil, err
	}

	// Expose the stateless Resource without a separate background lifetime.
	resource := s4wave_flowgraph.NewFlowgraphResource(ws, engine, key)
	return resource.GetMux(), func() {}, nil
}
