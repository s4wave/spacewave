package desktop_tray

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

type reconciledDesktopTrayEntry struct {
	ref     resource_client.ResourceRef
	service SRPCDesktopTrayEntryResourceServiceClient
}

// ReconcileDesktopTray mirrors source tray snapshots into a target tray resource.
func ReconcileDesktopTray(
	ctx context.Context,
	source SRPCDesktopTrayResourceServiceClient,
	target SRPCDesktopTrayResourceServiceClient,
	targetResources *resource_client.Client,
) error {
	r := &desktopTrayReconciler{
		target:          target,
		targetResources: targetResources,
		entries:         make(map[string]*reconciledDesktopTrayEntry),
	}
	defer r.releaseAll()

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
		current.ref.Release()
		delete(r.entries, id)
	}
	return nil
}

func (r *desktopTrayReconciler) register(
	ctx context.Context,
	entry *DesktopTrayEntry,
) (*reconciledDesktopTrayEntry, error) {
	resp, err := r.target.RegisterDesktopTrayEntry(ctx, &RegisterDesktopTrayEntryRequest{
		Entry: entry,
	})
	if err != nil {
		return nil, err
	}

	ref := r.targetResources.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, err
	}
	return &reconciledDesktopTrayEntry{
		ref:     ref,
		service: NewSRPCDesktopTrayEntryResourceServiceClient(client),
	}, nil
}

func (r *desktopTrayReconciler) releaseAll() {
	for id, entry := range r.entries {
		entry.ref.Release()
		delete(r.entries, id)
	}
}
