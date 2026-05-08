package desktop_tray

import (
	"context"
	"sort"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

type desktopTrayRegistration struct {
	resourceID               uint32
	entry                    *DesktopTrayEntry
	attachedActionResourceID uint32
	client                   resource_server.ResourceClientContext
}

// DesktopTray is an in-memory resource-backed desktop tray registry.
type DesktopTray struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	registrations map[uint32]*desktopTrayRegistration
}

// NewDesktopTray creates a new DesktopTray.
func NewDesktopTray() *DesktopTray {
	r := &DesktopTray{
		registrations: make(map[uint32]*desktopTrayRegistration),
	}
	mux := srpc.NewMux()
	_ = SRPCRegisterDesktopTrayResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *DesktopTray) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterDesktopTrayEntry registers one resource-backed tray entry.
func (r *DesktopTray) RegisterDesktopTrayEntry(
	ctx context.Context,
	req *RegisterDesktopTrayEntryRequest,
) (*RegisterDesktopTrayEntryResponse, error) {
	entry := req.GetEntry()
	if entry == nil {
		return nil, ErrDesktopTrayEntryRequired
	}
	if entry.GetId() == "" {
		return nil, ErrDesktopTrayEntryIdRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var duplicate bool
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		duplicate = r.hasEntryIDLocked(entry.GetId(), 0)
	})
	if duplicate {
		return nil, ErrDesktopTrayEntryDuplicate
	}

	entryResource := NewDesktopTrayEntryResource(r)
	reg := &desktopTrayRegistration{
		entry:                    entry.CloneVT(),
		attachedActionResourceID: req.GetAttachedActionResourceId(),
		client:                   client,
	}

	var released bool
	var resourceID uint32
	resourceID, err = client.AddResourceValue(entryResource.GetMux(), entryResource, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			released = true
			if _, ok := r.registrations[resourceID]; !ok {
				return
			}
			delete(r.registrations, resourceID)
			broadcast()
		})
	})
	if err != nil {
		return nil, err
	}

	entryResource.SetResourceID(resourceID)
	reg.resourceID = resourceID
	duplicate = false
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if released {
			return
		}
		if r.hasEntryIDLocked(entry.GetId(), 0) {
			duplicate = true
			return
		}
		r.registrations[resourceID] = reg
		broadcast()
	})
	if duplicate {
		client.ReleaseResource(resourceID)
		return nil, ErrDesktopTrayEntryDuplicate
	}
	if released {
		return nil, resource.ErrClientReleased
	}

	return &RegisterDesktopTrayEntryResponse{
		ResourceId: resourceID,
	}, nil
}

// WatchDesktopTray streams the full ordered tray tree.
func (r *DesktopTray) WatchDesktopTray(
	req *WatchDesktopTrayRequest,
	strm SRPCDesktopTrayResourceService_WatchDesktopTrayStream,
) error {
	ctx := strm.Context()

	for {
		var state *DesktopTrayState
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			state = r.snapshotLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&WatchDesktopTrayResponse{
			State: state,
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

// InvokeDesktopTrayEntry invokes an action entry by id.
func (r *DesktopTray) InvokeDesktopTrayEntry(
	ctx context.Context,
	req *InvokeDesktopTrayEntryRequest,
) (*InvokeDesktopTrayEntryResponse, error) {
	entryID := req.GetEntryId()
	if entryID == "" {
		return nil, ErrDesktopTrayEntryIdRequired
	}

	var reg *desktopTrayRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, candidate := range r.registrations {
			if candidate == nil || candidate.entry == nil {
				continue
			}
			if candidate.entry.GetId() == entryID {
				reg = candidate
				return
			}
		}
	})
	if reg == nil {
		return nil, ErrDesktopTrayEntryNotFound
	}

	entry := reg.entry.CloneVT()
	if entry.GetKind() != DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION || !entry.GetEnabled() {
		return nil, ErrDesktopTrayEntryNotInvokable
	}
	action := entry.GetAction()
	if action == nil || action.GetKind() != DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER {
		return nil, ErrDesktopTrayEntryNotInvokable
	}
	if reg.attachedActionResourceID == 0 {
		return nil, ErrDesktopTrayActionHandlerRequired
	}

	client, err := reg.client.GetAttachedResource(reg.attachedActionResourceID)
	if err != nil {
		return nil, err
	}
	handler := NewSRPCDesktopTrayActionHandlerServiceClient(client)
	_, err = handler.HandleDesktopTrayAction(ctx, &HandleDesktopTrayActionRequest{
		EntryId: entry.GetId(),
		Action:  action,
	})
	if err != nil {
		return nil, err
	}

	return &InvokeDesktopTrayEntryResponse{}, nil
}

func (r *DesktopTray) setEntry(resourceID uint32, entry *DesktopTrayEntry) error {
	var found bool
	var duplicate bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil {
			return
		}
		if r.hasEntryIDLocked(entry.GetId(), resourceID) {
			duplicate = true
			return
		}
		reg.entry = entry.CloneVT()
		found = true
		broadcast()
	})
	if duplicate {
		return ErrDesktopTrayEntryDuplicate
	}
	if !found {
		return ErrDesktopTrayEntryNotFound
	}
	return nil
}

func (r *DesktopTray) setActive(resourceID uint32, active bool) error {
	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil {
			return
		}
		if reg.entry.GetActive() == active {
			found = true
			return
		}
		reg.entry.Active = active
		found = true
		broadcast()
	})
	if !found {
		return ErrDesktopTrayEntryNotFound
	}
	return nil
}

func (r *DesktopTray) setEnabled(resourceID uint32, enabled bool) error {
	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil {
			return
		}
		if reg.entry.GetEnabled() == enabled {
			found = true
			return
		}
		reg.entry.Enabled = enabled
		found = true
		broadcast()
	})
	if !found {
		return ErrDesktopTrayEntryNotFound
	}
	return nil
}

func (r *DesktopTray) snapshot() *DesktopTrayState {
	var state *DesktopTrayState
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = r.snapshotLocked()
	})
	return state
}

func (r *DesktopTray) snapshotLocked() *DesktopTrayState {
	regs := make([]*desktopTrayRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		if reg == nil || reg.entry == nil {
			continue
		}
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(i, j int) bool {
		left := regs[i].entry
		right := regs[j].entry
		leftPath := strings.Join(left.GetPath(), "\x00")
		rightPath := strings.Join(right.GetPath(), "\x00")
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		if left.GetGroup() != right.GetGroup() {
			return left.GetGroup() < right.GetGroup()
		}
		if left.GetOrder() != right.GetOrder() {
			return left.GetOrder() < right.GetOrder()
		}
		if left.GetId() != right.GetId() {
			return left.GetId() < right.GetId()
		}
		return regs[i].resourceID < regs[j].resourceID
	})

	entries := make([]*DesktopTrayEntry, 0, len(regs))
	iconState := DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL
	var statusText string
	for _, reg := range regs {
		entry := reg.entry.CloneVT()
		entries = append(entries, entry)
		if entry.GetIconState() > iconState {
			iconState = entry.GetIconState()
		}
		if statusText == "" && entry.GetId() == "title" {
			statusText = desktopTrayTitleStatusText(entry.GetLabel())
		}
	}
	return &DesktopTrayState{
		Entries:    entries,
		IconState:  iconState,
		StatusText: statusText,
	}
}

func desktopTrayTitleStatusText(label string) string {
	return strings.TrimPrefix(label, "Spacewave: ")
}

func (r *DesktopTray) hasEntryIDLocked(entryID string, exceptResourceID uint32) bool {
	for resourceID, reg := range r.registrations {
		if resourceID == exceptResourceID || reg == nil || reg.entry == nil {
			continue
		}
		if reg.entry.GetId() == entryID {
			return true
		}
	}
	return false
}

// _ is a type assertion
var _ SRPCDesktopTrayResourceServiceServer = ((*DesktopTray)(nil))
