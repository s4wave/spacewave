package resource_quickstart_registry

import (
	"context"
	"sort"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// QuickstartRegistryResource provides an in-memory Quickstart registry.
// Plugins register Quickstarts via RegisterQuickstart and watch for changes via WatchQuickstarts.
type QuickstartRegistryResource struct {
	le  *logrus.Entry
	b   bus.Bus
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_quickstart_registry.QuickstartRegistration
}

// NewQuickstartRegistryResource creates a new QuickstartRegistryResource.
func NewQuickstartRegistryResource(
	le *logrus.Entry,
	b bus.Bus,
) *QuickstartRegistryResource {
	r := &QuickstartRegistryResource{
		le:            le,
		b:             b,
		nextID:        1,
		registrations: make(map[uint32]*s4wave_quickstart_registry.QuickstartRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_quickstart_registry.SRPCRegisterQuickstartRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *QuickstartRegistryResource) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterQuickstart registers a Quickstart from a plugin.
func (r *QuickstartRegistryResource) RegisterQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.RegisterQuickstartRequest,
) (*s4wave_quickstart_registry.RegisterQuickstartResponse, error) {
	reg := req.GetRegistration()
	if reg == nil {
		return nil, ErrRegistrationRequired
	}
	if reg.GetQuickstartId() == "" {
		return nil, ErrQuickstartIdRequired
	}
	if reg.GetPluginId() == "" {
		return nil, ErrPluginIdRequired
	}
	if reg.GetName() == "" {
		return nil, ErrNameRequired
	}
	if reg.GetDescription() == "" {
		return nil, ErrDescriptionRequired
	}
	if reg.GetCategory() == "" {
		return nil, ErrCategoryRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetQuickstartId() == reg.GetQuickstartId() {
				err = ErrQuickstartIdAlreadyRegistered
				return
			}
		}
		regID = r.nextID
		r.nextID++
		stored := reg.CloneVT()
		stored.RegistrationId = regID
		r.registrations[regID] = stored
		broadcast()
	})
	if err != nil {
		return nil, err
	}

	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.registrations[regID]; ok {
				delete(r.registrations, regID)
				broadcast()
			}
		})
	})
	if err != nil {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			delete(r.registrations, regID)
			broadcast()
		})
		return nil, err
	}

	return &s4wave_quickstart_registry.RegisterQuickstartResponse{ResourceId: resourceID}, nil
}

// ListQuickstarts returns all registered Quickstarts.
func (r *QuickstartRegistryResource) ListQuickstarts(
	ctx context.Context,
	req *s4wave_quickstart_registry.ListQuickstartsRequest,
) (*s4wave_quickstart_registry.ListQuickstartsResponse, error) {
	var regs []*s4wave_quickstart_registry.QuickstartRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked()
	})
	return &s4wave_quickstart_registry.ListQuickstartsResponse{Registrations: regs}, nil
}

// ExecuteQuickstart runs a registered Quickstart seed handler against a mounted Space.
func (r *QuickstartRegistryResource) ExecuteQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.ExecuteQuickstartRequest,
) (*s4wave_quickstart_registry.ExecuteQuickstartResponse, error) {
	quickstartID := req.GetQuickstartId()
	if quickstartID == "" {
		return nil, ErrQuickstartIdRequired
	}
	if req.GetSpaceResourceId() == 0 {
		return nil, ErrSpaceResourceIdRequired
	}
	reg := r.LookupRegistration(quickstartID)
	if reg == nil {
		return nil, ErrQuickstartNotRegistered
	}
	if r.b == nil {
		return nil, ErrQuickstartExecutionUnavailable
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	spaceValue, err := resourceCtx.GetResourceValue(req.GetSpaceResourceId())
	if err != nil {
		return nil, err
	}
	spaceResource, ok := spaceValue.(*resource_space.SpaceResource)
	if !ok || spaceResource == nil {
		return nil, ErrSpaceResourceRequired
	}

	resources, err := s4wave_plugin.ConnectPluginResources(ctx, r.b, reg.GetPluginId())
	if err != nil {
		return nil, errors.Wrap(err, "connect to quickstart plugin")
	}
	defer resources.Release()

	lookupOp := world.BuildLookupWorldOpFunc(r.b, r.le, spaceResource.GetWorldEngineID())
	engineInfo := &s4wave_world.EngineInfo{
		EngineId: spaceResource.GetWorldEngineID(),
		BucketId: spaceResource.GetWorldEngineBucketID(),
	}
	engineResource := resource_world.NewEngineResource(
		r.le,
		r.b,
		spaceResource.GetWorldEngine(),
		lookupOp,
		engineInfo,
	)
	engineMux := engineResource.GetMux()
	engineRootMux := srpc.NewMux(engineMux)
	engineResourceServer := resource_server.NewResourceServer(engineMux)
	if err := engineResourceServer.Register(engineRootMux); err != nil {
		return nil, err
	}
	engineResourceID, err := resources.Client.AttachResource(ctx, "quickstart-world-engine", engineRootMux)
	if err != nil {
		return nil, errors.Wrap(err, "attach quickstart world engine")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, engineResourceID)
	}()

	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return nil, errors.Wrap(err, "get plugin root client")
	}
	defer rootRef.Release()

	handler := s4wave_quickstart_registry.NewSRPCQuickstartHandlerServiceClient(rootClient)
	resp, err := handler.SeedQuickstart(ctx, &s4wave_quickstart_registry.SeedQuickstartRequest{
		QuickstartId:     quickstartID,
		EngineResourceId: engineResourceID,
	})
	if err != nil {
		return nil, err
	}

	return &s4wave_quickstart_registry.ExecuteQuickstartResponse{
		IndexPath: resp.GetIndexPath(),
		PluginIds: mergePluginIDs(
			reg.GetRequiredPluginIds(),
			resp.GetPluginIds(),
		),
	}, nil
}

// WatchQuickstarts streams all registered Quickstarts.
func (r *QuickstartRegistryResource) WatchQuickstarts(
	req *s4wave_quickstart_registry.WatchQuickstartsRequest,
	strm s4wave_quickstart_registry.SRPCQuickstartRegistryResourceService_WatchQuickstartsStream,
) error {
	ctx := strm.Context()

	for {
		var regs []*s4wave_quickstart_registry.QuickstartRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_quickstart_registry.WatchQuickstartsResponse{
			Registrations: regs,
		}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// LookupRegistration finds a registration by quickstartID.
func (r *QuickstartRegistryResource) LookupRegistration(
	quickstartID string,
) *s4wave_quickstart_registry.QuickstartRegistration {
	var reg *s4wave_quickstart_registry.QuickstartRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetQuickstartId() == quickstartID {
				reg = v.CloneVT()
				break
			}
		}
	})
	return reg
}

func mergePluginIDs(lists ...[]string) []string {
	var ids []string
	seen := make(map[string]struct{})
	for _, list := range lists {
		for _, id := range list {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *QuickstartRegistryResource) getRegistrationsLocked() []*s4wave_quickstart_registry.QuickstartRegistration {
	regs := make([]*s4wave_quickstart_registry.QuickstartRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		regs = append(regs, reg.CloneVT())
	}
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].GetQuickstartId() == regs[j].GetQuickstartId() {
			return regs[i].GetRegistrationId() < regs[j].GetRegistrationId()
		}
		return regs[i].GetQuickstartId() < regs[j].GetQuickstartId()
	})
	return regs
}

// _ is a type assertion
var _ s4wave_quickstart_registry.SRPCQuickstartRegistryResourceServiceServer = (*QuickstartRegistryResource)(nil)
