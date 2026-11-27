package prom_plugin

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/promql"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	promstorage "github.com/s4wave/spacewave/plugin/prom/storage"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// handler serves the prom/target and prom/dashboard ObjectTypes to the core
// plugin.
type handler struct {
	// le is the logger.
	le *logrus.Entry
	// b is the plugin's bus.
	b bus.Bus
	// window is the time between a Device's commits in one Space.
	window time.Duration
	// promql evaluates every query this plugin serves.
	promql *promql.Engine

	// mtx guards groups.
	mtx sync.Mutex
	// groups holds the commit group of each World engine, by engine ID.
	groups map[string]*commitGroup
}

// newHandler constructs a handler on b that commits every window.
func newHandler(le *logrus.Entry, b bus.Bus, window time.Duration) *handler {
	return &handler{
		le:     le,
		b:      b,
		window: window,
		promql: newQueryEngine(),
		groups: make(map[string]*commitGroup),
	}
}

// InvokeObjectType serves the Space's PromQL queries on the attached engine,
// with a target's scrape process or a Dashboard's edits.
func (h *handler) InvokeObjectType(
	ctx context.Context,
	req *s4wave_objecttype_registry.InvokeObjectTypeRequest,
) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	// Require a target or a Dashboard and the engine it lives in.
	typeID := req.GetTypeId()
	if typeID != s4wave_prom.TargetTypeID && typeID != s4wave_prom.DashboardTypeID {
		return nil, errors.Errorf("prom plugin: unhandled object type %s", req.GetTypeId())
	}
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, errors.New("prom plugin: attached engine resource is required")
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Open the engine and learn which World and Device it serves.
	engine, err := sdk_world_engine.NewAttachedEngine(ctx, req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	info, err := engine.GetEngineInfo(ctx)
	if err != nil {
		engine.Release()
		return nil, errors.Wrap(err, "prom plugin: engine info")
	}

	// Serve the query service with the target's scrape process or the
	// Dashboard's edits.
	query := &queryService{engine: h.promql, eng: engine, q: promstorage.NewQueryable(engine, worldTargets{eng: engine})}
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		if err := s4wave_prom.SRPCRegisterPromQueryResourceService(mux, query); err != nil {
			return err
		}
		if typeID == s4wave_prom.DashboardTypeID {
			return s4wave_prom.SRPCRegisterPromDashboardResourceService(mux, &dashboardService{eng: engine, objectKey: req.GetObjectKey()})
		}
		return s4wave_process.SRPCRegisterPersistentExecutionService(mux, &targetProcess{
			le:        h.le.WithField("object-key", req.GetObjectKey()),
			b:         h.b,
			eng:       engine,
			group:     h.group(info.GetEngineInfo().GetEngineId()),
			objectKey: req.GetObjectKey(),
			peerID:    info.GetSessionPeerId(),
		})
	})
	resourceID, err := resourceCtx.AddResource(mux, engine.Release)
	if err != nil {
		engine.Release()
		return nil, err
	}
	return &s4wave_objecttype_registry.InvokeObjectTypeResponse{ResourceId: resourceID}, nil
}

// group returns the commit group of the World engine engineID.
func (h *handler) group(engineID string) *commitGroup {
	// Create the group on first use.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	g := h.groups[engineID]
	if g == nil {
		g = newCommitGroup(h.le.WithField("engine-id", engineID), h.window)
		h.groups[engineID] = g
	}
	return g
}

// _ is a type assertion.
var _ s4wave_objecttype_registry.SRPCObjectTypeHandlerServiceServer = (*handler)(nil)
