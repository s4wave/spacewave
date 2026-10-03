package s4wave_wizard

import (
	"cmp"
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/resource/registration"
)

// WizardRegistryResource implements ObjectWizardRegistryResourceService.
// Plugins register object wizards through their prepared registration
// generation; listings select the wizards visible to one installation.
type WizardRegistryResource struct {
	mux srpc.Mux

	// generations owns visibility of prepared plugin registrations.
	generations   *registration.Registry
	bcast         *broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*ObjectWizard
}

// NewWizardRegistryResource creates a new WizardRegistryResource.
func NewWizardRegistryResource(generations *registration.Registry) *WizardRegistryResource {
	// A standalone registry has its own admission boundary.
	if generations == nil {
		generations = registration.NewRegistry()
	}
	r := &WizardRegistryResource{
		generations:   generations,
		bcast:         generations.Broadcast(),
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
	// Require the wizard identity and display name before registration.
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

	// Locate the plugin registration generation associated with the wizard.
	generation, err := registration.FromContext(ctx, wizard.GetPluginId())
	if err != nil {
		return nil, err
	}

	// Access the resource client that retains the wizard registration.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Independent installations may each register the same type id.
	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Reject a wizard type that conflicts with the plugin generation.
		for _, v := range r.registrations {
			if v.GetTypeId() == wizard.GetTypeId() && !r.generations.CanShareNameLocked(v, generation) {
				err = ErrWizardAlreadyRegistered
				return
			}
		}

		// Bind the cloned wizard to its generation and publish the registration.
		regID = r.nextID
		r.nextID++
		stored := wizard.CloneVT()
		stored.RegistrationId = regID
		if err = r.generations.BindLocked(stored, generation); err != nil {
			return
		}
		r.registrations[regID] = stored
		broadcast()
	})
	if err != nil {
		return nil, err
	}

	// Tie the wizard registration to the resource client lifetime.
	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.releaseRegistration(regID)
	})
	if err != nil {
		r.releaseRegistration(regID)
		return nil, err
	}

	return &RegisterWizardResponse{ResourceId: resourceID}, nil
}

// releaseRegistration removes one registration and its generation binding.
func (r *WizardRegistryResource) releaseRegistration(regID uint32) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Remove the retained wizard and generation binding, then notify watchers.
		stored, ok := r.registrations[regID]
		if !ok {
			return
		}
		r.generations.ForgetLocked(stored)
		delete(r.registrations, regID)
		broadcast()
	})
}

// ListWizards returns the object wizards visible to the requested installation.
func (r *WizardRegistryResource) ListWizards(
	ctx context.Context,
	req *ListWizardsRequest,
) (*ListWizardsResponse, error) {
	var wizards []*ObjectWizard
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		wizards = r.getWizardsLocked(req.GetInstanceKey())
	})
	return &ListWizardsResponse{Wizards: wizards}, nil
}

// WatchWizards streams the object wizards visible to the requested installation.
func (r *WizardRegistryResource) WatchWizards(
	req *WatchWizardsRequest,
	strm SRPCObjectWizardRegistryResourceService_WatchWizardsStream,
) error {
	return broadcast.WatchBroadcastWithEqual(
		strm.Context(),
		r.bcast,
		func() *WatchWizardsResponse {
			return &WatchWizardsResponse{Wizards: r.getWizardsLocked(req.GetInstanceKey())}
		},
		func(resp *WatchWizardsResponse) error {
			return strm.Send(resp)
		},
		func(a, b *WatchWizardsResponse) bool {
			return a.EqualVT(b)
		},
	)
}

// getWizardsLocked returns the built-in wizards followed by the visible plugin
// wizards sorted by type id. Built-in type ids take precedence. The caller
// holds bcast.
func (r *WizardRegistryResource) getWizardsLocked(instanceKey string) []*ObjectWizard {
	// Select visible plugin wizards in stable type order.
	selected := registration.SelectLocked(r.generations, r.registrations, instanceKey, (*ObjectWizard).GetTypeId)
	slices.SortFunc(selected, func(a, b *ObjectWizard) int {
		return cmp.Compare(a.GetTypeId(), b.GetTypeId())
	})

	// Collect built-in wizards and reserve their type identifiers.
	wizards := make([]*ObjectWizard, 0, len(ObjectWizards)+len(selected))
	builtin := make(map[string]struct{}, len(ObjectWizards))
	for _, wizard := range ObjectWizards {
		if wizard.GetTypeId() == "" {
			continue
		}
		builtin[wizard.GetTypeId()] = struct{}{}
		wizards = append(wizards, wizard.CloneVT())
	}

	// Append visible plugin wizards whose types have no built-in definition.
	for _, wizard := range selected {
		if _, ok := builtin[wizard.GetTypeId()]; ok {
			continue
		}
		wizards = append(wizards, wizard.CloneVT())
	}
	return wizards
}

var _ SRPCObjectWizardRegistryResourceServiceServer = (*WizardRegistryResource)(nil)
