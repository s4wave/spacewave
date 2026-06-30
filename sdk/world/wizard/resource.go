package s4wave_wizard

import (
	"cmp"
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// WizardRegistryResource implements ObjectWizardRegistryResourceService.
type WizardRegistryResource struct {
	mux   srpc.Mux
	state *wizardRegistryState
}

type wizardRegistryState struct {
	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*ObjectWizard
}

var defaultWizardRegistryState = newWizardRegistryState()

func newWizardRegistryState() *wizardRegistryState {
	return &wizardRegistryState{
		nextID:        1,
		registrations: make(map[uint32]*ObjectWizard),
	}
}

// NewWizardRegistryResource creates a new WizardRegistryResource.
func NewWizardRegistryResource() *WizardRegistryResource {
	r := &WizardRegistryResource{state: defaultWizardRegistryState}
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
	r.state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, v := range r.state.registrations {
			if v.GetTypeId() == wizard.GetTypeId() {
				err = ErrWizardAlreadyRegistered
				return
			}
		}
		regID = r.state.nextID
		r.state.nextID++
		stored := wizard.CloneVT()
		stored.RegistrationId = regID
		r.state.registrations[regID] = stored
		broadcast()
	})
	if err != nil {
		return nil, err
	}

	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.state.registrations[regID]; ok {
				delete(r.state.registrations, regID)
				broadcast()
			}
		})
	})
	if err != nil {
		r.state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			delete(r.state.registrations, regID)
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
	r.state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		wizards = r.getWizardsLocked()
	})
	return &ListWizardsResponse{Wizards: wizards}, nil
}

// WatchWizards streams all registered object wizards.
func (r *WizardRegistryResource) WatchWizards(
	_ *WatchWizardsRequest,
	strm SRPCObjectWizardRegistryResourceService_WatchWizardsStream,
) error {
	return broadcast.WatchBroadcastWithEqual(
		strm.Context(),
		&r.state.bcast,
		func() *WatchWizardsResponse {
			return &WatchWizardsResponse{Wizards: r.getWizardsLocked()}
		},
		func(resp *WatchWizardsResponse) error {
			return strm.Send(resp)
		},
		func(_, _ *WatchWizardsResponse) bool {
			return false
		},
	)
}

func (r *WizardRegistryResource) getWizardsLocked() []*ObjectWizard {
	wizards := make([]*ObjectWizard, 0, len(ObjectWizards)+len(r.state.registrations))
	seen := make(map[string]struct{})
	for _, wizard := range ObjectWizards {
		if wizard.GetTypeId() == "" {
			continue
		}
		seen[wizard.GetTypeId()] = struct{}{}
		wizards = append(wizards, wizard.CloneVT())
	}
	regs := make([]*ObjectWizard, 0, len(r.state.registrations))
	for _, wizard := range r.state.registrations {
		regs = append(regs, wizard.CloneVT())
	}
	slices.SortFunc(regs, func(a, b *ObjectWizard) int {
		if n := cmp.Compare(a.GetTypeId(), b.GetTypeId()); n != 0 {
			return n
		}
		return cmp.Compare(a.GetRegistrationId(), b.GetRegistrationId())
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

var _ SRPCObjectWizardRegistryResourceServiceServer = (*WizardRegistryResource)(nil)
