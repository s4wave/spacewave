package s4wave_flowgraph_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// FlowgraphRunTypeID is the type identifier for Flowgraph runs.
const FlowgraphRunTypeID = s4wave_flowgraph.FlowgraphRunTypeID

// FlowgraphRunType exposes a run as an approved local Space process.
var FlowgraphRunType = objecttype.NewObjectType(FlowgraphRunTypeID, FlowgraphRunFactory)

// RunResource drives a run for the lifetime of its execution stream.
// The Space process binding retains approval and recreates the stream on restart.
type RunResource struct {
	// le reports run-controller errors.
	le *logrus.Entry
	// b resolves the run's World engine.
	b bus.Bus
	// conf identifies the run and the granting World engine.
	conf *flowgraph_run.Config
}

// FlowgraphRunFactory exposes execution against the granting World engine.
func FlowgraphRunFactory(ctx context.Context, le *logrus.Entry, b bus.Bus, engine world.Engine, ws world.WorldState, key string) (srpc.Invoker, func(), error) {
	// Require the World capability that grants writes to the run.
	if ws == nil || engine == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Serve execution through the existing process-binding protocol.
	resource := &RunResource{le: le, b: b, conf: flowgraph_run.NewConfig(objecttype.EngineIDFromContext(ctx), key)}
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
	})
	return mux, func() {}, nil
}

// Execute watches the durable run and its Tasks until the binding stream ends.
func (r *RunResource) Execute(_ *s4wave_process.ExecuteRequest, stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream) error {
	// Give each binding generation its own controller and release its engine.
	ctrl := flowgraph_run.NewController(r.le, r.b, r.conf)
	defer ctrl.Close()

	// Report execution and retain the stream while the controller watches.
	if err := stream.Send(&s4wave_process.ExecuteStatus{State: s4wave_process.ExecutionState_ExecutionState_RUNNING}); err != nil {
		return err
	}
	return ctrl.Execute(stream.Context())
}

// _ is a type assertion.
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*RunResource)(nil)
