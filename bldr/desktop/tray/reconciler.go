package desktop_tray

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

type reconciledDesktopTrayEntry struct {
	ref                      resource_client.ResourceRef
	service                  SRPCDesktopTrayEntryResourceServiceClient
	attachedActionResourceID uint32
}

// ReconcileDesktopTray mirrors source tray snapshots into a target tray resource.
func ReconcileDesktopTray(
	ctx context.Context,
	source SRPCDesktopTrayResourceServiceClient,
	target SRPCDesktopTrayResourceServiceClient,
	targetResources *resource_client.Client,
) error {
	r := &desktopTrayReconciler{
		source:          source,
		target:          target,
		targetResources: targetResources,
		entries:         make(map[string]*reconciledDesktopTrayEntry),
	}
	defer r.releaseAll(context.Background())

	strm, err := source.WatchDesktopTray(ctx, &WatchDesktopTrayRequest{})
	if err != nil {
		return err
	}

	for {
		resp, err := strm.Recv()
		if err != nil {
			return err
		}
		if err := r.apply(ctx, resp.GetState()); err != nil {
			return err
		}
	}
}

type desktopTrayReconciler struct {
	source          SRPCDesktopTrayResourceServiceClient
	target          SRPCDesktopTrayResourceServiceClient
	targetResources *resource_client.Client
	entries         map[string]*reconciledDesktopTrayEntry
}

func (r *desktopTrayReconciler) apply(ctx context.Context, state *DesktopTrayState) error {
	next := make(map[string]*DesktopTrayEntry)
	for _, entry := range state.GetEntries() {
		if entry.GetId() == "" {
			continue
		}
		next[entry.GetId()] = entry.CloneVT()
	}

	for id, entry := range next {
		current := r.entries[id]
		if current == nil {
			registered, err := r.register(ctx, entry)
			if err != nil {
				return err
			}
			r.entries[id] = registered
			continue
		}
		if current.usesAttachedActionHandler() != entryUsesAttachedHandler(entry) {
			r.releaseEntry(ctx, current)
			registered, err := r.register(ctx, entry)
			if err != nil {
				return err
			}
			r.entries[id] = registered
			continue
		}
		_, err := current.service.SetDesktopTrayEntry(ctx, &SetDesktopTrayEntryRequest{
			Entry: entry,
		})
		if err != nil {
			return err
		}
	}

	for id, current := range r.entries {
		if next[id] != nil {
			continue
		}
		r.releaseEntry(ctx, current)
		delete(r.entries, id)
	}
	return nil
}

func (r *desktopTrayReconciler) register(
	ctx context.Context,
	entry *DesktopTrayEntry,
) (*reconciledDesktopTrayEntry, error) {
	var attachedActionResourceID uint32
	if entryUsesAttachedHandler(entry) {
		handler := &forwardingDesktopTrayActionHandler{
			source:  r.source,
			entryID: entry.GetId(),
		}
		mux := srpc.NewMux()
		if err := SRPCRegisterDesktopTrayActionHandlerService(mux, handler); err != nil {
			return nil, err
		}
		var err error
		attachedActionResourceID, err = r.targetResources.AttachRawInvoker(
			ctx,
			"desktop-tray-action-"+entry.GetId(),
			mux,
		)
		if err != nil {
			return nil, err
		}
	}

	resp, err := r.target.RegisterDesktopTrayEntry(ctx, &RegisterDesktopTrayEntryRequest{
		Entry:                    entry,
		AttachedActionResourceId: attachedActionResourceID,
	})
	if err != nil {
		if attachedActionResourceID != 0 {
			_ = r.targetResources.DetachResource(ctx, attachedActionResourceID)
		}
		return nil, err
	}

	ref := r.targetResources.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		if attachedActionResourceID != 0 {
			_ = r.targetResources.DetachResource(ctx, attachedActionResourceID)
		}
		return nil, err
	}
	return &reconciledDesktopTrayEntry{
		ref:                      ref,
		service:                  NewSRPCDesktopTrayEntryResourceServiceClient(client),
		attachedActionResourceID: attachedActionResourceID,
	}, nil
}

func (r *reconciledDesktopTrayEntry) usesAttachedActionHandler() bool {
	return r.attachedActionResourceID != 0
}

func entryUsesAttachedHandler(entry *DesktopTrayEntry) bool {
	return entry.GetAction().GetKind() == DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER
}

func (r *desktopTrayReconciler) releaseAll(ctx context.Context) {
	for id, entry := range r.entries {
		r.releaseEntry(ctx, entry)
		delete(r.entries, id)
	}
}

func (r *desktopTrayReconciler) releaseEntry(ctx context.Context, entry *reconciledDesktopTrayEntry) {
	entry.ref.Release()
	if entry.attachedActionResourceID != 0 {
		_ = r.targetResources.DetachResource(ctx, entry.attachedActionResourceID)
	}
}

type forwardingDesktopTrayActionHandler struct {
	source  SRPCDesktopTrayResourceServiceClient
	entryID string
}

func (h *forwardingDesktopTrayActionHandler) HandleDesktopTrayAction(
	ctx context.Context,
	req *HandleDesktopTrayActionRequest,
) (*HandleDesktopTrayActionResponse, error) {
	_, err := h.source.InvokeDesktopTrayEntry(ctx, &InvokeDesktopTrayEntryRequest{
		EntryId: h.entryID,
	})
	if err != nil {
		return nil, err
	}
	return &HandleDesktopTrayActionResponse{}, nil
}

var _ SRPCDesktopTrayActionHandlerServiceServer = (*forwardingDesktopTrayActionHandler)(nil)
