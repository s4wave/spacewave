package desktop_tray

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// DesktopTrayEntryResource controls one registered desktop tray entry.
type DesktopTrayEntryResource struct {
	mux srpc.Invoker

	tray       *DesktopTray
	resourceID uint32
}

// NewDesktopTrayEntryResource creates a new DesktopTrayEntryResource.
func NewDesktopTrayEntryResource(tray *DesktopTray) *DesktopTrayEntryResource {
	r := &DesktopTrayEntryResource{
		tray: tray,
	}
	mux := srpc.NewMux()
	_ = SRPCRegisterDesktopTrayEntryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *DesktopTrayEntryResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetResourceID returns the owning resource id.
func (r *DesktopTrayEntryResource) GetResourceID() uint32 {
	return r.resourceID
}

// SetResourceID sets the owning resource id.
func (r *DesktopTrayEntryResource) SetResourceID(resourceID uint32) {
	r.resourceID = resourceID
}

// SetDesktopTrayEntry replaces the registered entry.
func (r *DesktopTrayEntryResource) SetDesktopTrayEntry(
	ctx context.Context,
	req *SetDesktopTrayEntryRequest,
) (*SetDesktopTrayEntryResponse, error) {
	entry := req.GetEntry()
	if entry == nil {
		return nil, ErrDesktopTrayEntryRequired
	}
	if entry.GetId() == "" {
		return nil, ErrDesktopTrayEntryIdRequired
	}
	err := r.tray.setEntry(r.resourceID, entry)
	if err != nil {
		return nil, err
	}
	return &SetDesktopTrayEntryResponse{}, nil
}

// SetDesktopTrayEntryActive updates the entry active flag.
func (r *DesktopTrayEntryResource) SetDesktopTrayEntryActive(
	ctx context.Context,
	req *SetDesktopTrayEntryActiveRequest,
) (*SetDesktopTrayEntryActiveResponse, error) {
	err := r.tray.setActive(r.resourceID, req.GetActive())
	if err != nil {
		return nil, err
	}
	return &SetDesktopTrayEntryActiveResponse{}, nil
}

// SetDesktopTrayEntryEnabled updates the entry enabled flag.
func (r *DesktopTrayEntryResource) SetDesktopTrayEntryEnabled(
	ctx context.Context,
	req *SetDesktopTrayEntryEnabledRequest,
) (*SetDesktopTrayEntryEnabledResponse, error) {
	err := r.tray.setEnabled(r.resourceID, req.GetEnabled())
	if err != nil {
		return nil, err
	}
	return &SetDesktopTrayEntryEnabledResponse{}, nil
}

// _ is a type assertion
var _ SRPCDesktopTrayEntryResourceServiceServer = (*DesktopTrayEntryResource)(nil)
