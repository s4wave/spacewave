package s4wave_wizard

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// WizardRegistryResource implements ObjectWizardRegistryResourceService.
type WizardRegistryResource struct {
	mux srpc.Mux
}

// NewWizardRegistryResource creates a new WizardRegistryResource.
func NewWizardRegistryResource() *WizardRegistryResource {
	r := &WizardRegistryResource{}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterObjectWizardRegistryResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for the resource.
func (r *WizardRegistryResource) GetMux() srpc.Mux {
	return r.mux
}

// ListWizards returns all registered object wizards.
func (r *WizardRegistryResource) ListWizards(
	ctx context.Context,
	req *ListWizardsRequest,
) (*ListWizardsResponse, error) {
	return &ListWizardsResponse{Wizards: ObjectWizards}, nil
}

// _ is a type assertion
var _ SRPCObjectWizardRegistryResourceServiceServer = (*WizardRegistryResource)(nil)
