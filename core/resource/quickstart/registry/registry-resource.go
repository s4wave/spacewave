package resource_quickstart_registry

import (
	"cmp"
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/resource/registration"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	"github.com/sirupsen/logrus"
)

// QuickstartRegistryResource provides an in-memory Quickstart registry.
// Plugins register Quickstarts via RegisterQuickstart and watch for changes via WatchQuickstarts.
type QuickstartRegistryResource struct {
	le  *logrus.Entry
	b   bus.Bus
	mux srpc.Invoker

	// generations owns visibility of prepared plugin registrations.
	generations   *registration.Registry
	bcast         *broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_quickstart_registry.QuickstartRegistration
}

// NewQuickstartRegistryResource creates a new QuickstartRegistryResource.
func NewQuickstartRegistryResource(
	le *logrus.Entry,
	b bus.Bus,
	generations *registration.Registry,
) *QuickstartRegistryResource {
	// A standalone registry has its own admission boundary.
	if generations == nil {
		generations = registration.NewRegistry()
	}
	r := &QuickstartRegistryResource{
		le:            le,
		b:             b,
		generations:   generations,
		bcast:         generations.Broadcast(),
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
	// Require the Quickstart identity and display fields before registration.
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

	// Resolve the plugin generation associated with this registration.
	generation, err := registration.FromContext(ctx, reg.GetPluginId())
	if err != nil {
		return nil, err
	}

	// Resolve the resource client that will retain the registration.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Register the Quickstart under the shared generation lock.
	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Reject conflicting Quickstart identities and remove a replaceable registration.
		for id, v := range r.registrations {
			if v.GetQuickstartId() != reg.GetQuickstartId() {
				continue
			}
			if r.generations.CanShareNameLocked(v, generation) {
				continue
			}
			if v.GetPluginId() != reg.GetPluginId() {
				err = ErrQuickstartIdAlreadyRegistered
				return
			}
			if generation != nil || r.generations.GenerationLocked(v) != nil {
				err = ErrQuickstartIdAlreadyRegistered
				return
			}
			r.generations.ForgetLocked(v)
			delete(r.registrations, id)
		}

		// Assign a registration identity and capture its plugin manifest.
		regID = r.nextID
		r.nextID++
		stored := reg.CloneVT()
		stored.RegistrationId = regID
		stored.ManifestRoot = ""
		if generation != nil {
			stored.ManifestRoot = generation.ManifestRoot()
		}

		// Bind the stored Quickstart to its generation and notify watchers.
		if err = r.generations.BindLocked(stored, generation); err != nil {
			return
		}
		r.registrations[regID] = stored
		broadcast()
	})
	if err != nil {
		return nil, err
	}

	// Retain the registration until its client resource is released.
	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.registrations[regID]; ok {
				r.generations.ForgetLocked(r.registrations[regID])
				delete(r.registrations, regID)
				broadcast()
			}
		})
	})
	if err != nil {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.generations.ForgetLocked(r.registrations[regID])
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
		regs = r.getRegistrationsLocked(req.GetInstanceKey())
	})
	return &s4wave_quickstart_registry.ListQuickstartsResponse{Registrations: regs}, nil
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
			regs = r.getRegistrationsLocked(req.GetInstanceKey())
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
	quickstartID, instanceKey string,
) *s4wave_quickstart_registry.QuickstartRegistration {
	var reg *s4wave_quickstart_registry.QuickstartRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.getRegistrationsLocked(instanceKey) {
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
func (r *QuickstartRegistryResource) getRegistrationsLocked(instanceKey string) []*s4wave_quickstart_registry.QuickstartRegistration {
	// Clone the Quickstart registrations visible to the requested instance.
	selected := registration.SelectLocked(r.generations, r.registrations, instanceKey, (*s4wave_quickstart_registry.QuickstartRegistration).GetQuickstartId)
	regs := make([]*s4wave_quickstart_registry.QuickstartRegistration, 0, len(selected))
	for _, reg := range selected {
		regs = append(regs, reg.CloneVT())
	}

	// Order the Quickstart snapshot by identity and registration number.
	slices.SortFunc(regs, func(a, b *s4wave_quickstart_registry.QuickstartRegistration) int {
		if c := cmp.Compare(a.GetQuickstartId(), b.GetQuickstartId()); c != 0 {
			return c
		}
		return cmp.Compare(a.GetRegistrationId(), b.GetRegistrationId())
	})
	return regs
}

// _ is a type assertion
var _ s4wave_quickstart_registry.SRPCQuickstartRegistryResourceServiceServer = (*QuickstartRegistryResource)(nil)
