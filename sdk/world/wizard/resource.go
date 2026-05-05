package s4wave_wizard

import (
	"context"
	"sort"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// WizardRegistryResource implements ObjectWizardRegistryResourceService.
type WizardRegistryResource struct {
	mux srpc.Mux

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*ObjectWizard
}

// NewWizardRegistryResource creates a new WizardRegistryResource.
func NewWizardRegistryResource() *WizardRegistryResource {
	r := &WizardRegistryResource{
		nextID:        1,
		registrations: make(map[uint32]*ObjectWizard),
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterObjectWizardRegistryResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for the resource.
func (r *WizardRegistryResource) GetMux() srpc.Mux {
	return r.mux
}

// RegisterWizard registers a plugin-provided object wizard.
func (r *WizardRegistryResource) RegisterWizard(
	ctx context.Context,
	req *RegisterWizardRequest,
) (*RegisterWizardResponse, error) {
	wizard := req.GetWizard()
	if wizard == nil {
		return nil, ErrWizardRequired
	}
	if wizard.GetTypeId() == "" {
		return nil, ErrWizardTypeIDRequired
	}
	if wizard.GetPluginId() == "" {
		return nil, ErrWizardPluginIDRequired
	}
	if wizard.GetDisplayName() == "" {
		return nil, ErrWizardNameRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetTypeId() == wizard.GetTypeId() {
				err = ErrWizardAlreadyRegistered
				return
			}
		}
		regID = r.nextID
		r.nextID++
		stored := wizard.CloneVT()
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

	return &RegisterWizardResponse{ResourceId: resourceID}, nil
}

// ListWizards returns all registered object wizards.
func (r *WizardRegistryResource) ListWizards(
	ctx context.Context,
	req *ListWizardsRequest,
) (*ListWizardsResponse, error) {
	var wizards []*ObjectWizard
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		wizards = r.getWizardsLocked()
	})
	return &ListWizardsResponse{Wizards: wizards}, nil
}

// WatchWizards streams all registered object wizards.
func (r *WizardRegistryResource) WatchWizards(
	req *WatchWizardsRequest,
	strm SRPCObjectWizardRegistryResourceService_WatchWizardsStream,
) error {
	ctx := strm.Context()

	for {
		var wizards []*ObjectWizard
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			wizards = r.getWizardsLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&WatchWizardsResponse{Wizards: wizards}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

func (r *WizardRegistryResource) getWizardsLocked() []*ObjectWizard {
	wizards := make([]*ObjectWizard, 0, len(ObjectWizards)+len(r.registrations))
	seen := make(map[string]struct{})
	for _, wizard := range ObjectWizards {
		if wizard.GetTypeId() == "" {
			continue
		}
		seen[wizard.GetTypeId()] = struct{}{}
		wizards = append(wizards, wizard.CloneVT())
	}
	regs := make([]*ObjectWizard, 0, len(r.registrations))
	for _, wizard := range r.registrations {
		regs = append(regs, wizard.CloneVT())
	}
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].GetTypeId() == regs[j].GetTypeId() {
			return regs[i].GetRegistrationId() < regs[j].GetRegistrationId()
		}
		return regs[i].GetTypeId() < regs[j].GetTypeId()
	})
	for _, wizard := range regs {
		if _, ok := seen[wizard.GetTypeId()]; ok {
			continue
		}
		seen[wizard.GetTypeId()] = struct{}{}
		wizards = append(wizards, wizard)
	}
	return wizards
}

// _ is a type assertion
var _ SRPCObjectWizardRegistryResourceServiceServer = (*WizardRegistryResource)(nil)
