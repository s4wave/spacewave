package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

type desktopTrayEntryRegistration struct {
	ref                      resource_client.ResourceRef
	service                  desktop_tray.SRPCDesktopTrayEntryResourceServiceClient
	entry                    *desktop_tray.DesktopTrayEntry
	attachedActionResourceID uint32
}

type desktopTrayPublisher struct {
	resources      *resource_client.Client
	trayRef        resource_client.ResourceRef
	tray           desktop_tray.SRPCDesktopTrayResourceServiceClient
	entries        map[string]*desktopTrayEntryRegistration
	actionHandlers map[string]desktop_tray.SRPCDesktopTrayActionHandlerServiceServer
}

func newHostDesktopTrayPublisher(ctx context.Context, b bus.Bus) (*desktopTrayPublisher, error) {
	rpcClient := bifrost_rpc.NewBusClient(b)
	resourceService := bldr_resource.NewSRPCResourceServiceClientWithServiceID(
		rpcClient,
		bldr_plugin.HostServiceIDPrefix+bldr_resource.SRPCResourceServiceServiceID,
	)
	resources, err := resource_client.NewClient(ctx, resourceService)
	if err != nil {
		return nil, err
	}

	rootRef := resources.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, err
	}

	hostService := sdk_plugin_host.NewSRPCPluginHostResourceServiceClient(rootClient)
	resp, err := hostService.AccessDesktopTray(ctx, &sdk_plugin_host.AccessDesktopTrayRequest{})
	rootRef.Release()
	if err != nil {
		resources.Release()
		return nil, err
	}

	trayRef := resources.CreateResourceReference(resp.GetResourceId())
	trayClient, err := trayRef.GetClient()
	if err != nil {
		trayRef.Release()
		resources.Release()
		return nil, err
	}
	return &desktopTrayPublisher{
		resources: resources,
		trayRef:   trayRef,
		tray:      desktop_tray.NewSRPCDesktopTrayResourceServiceClient(trayClient),
		entries:   make(map[string]*desktopTrayEntryRegistration),
		actionHandlers: map[string]desktop_tray.SRPCDesktopTrayActionHandlerServiceServer{
			"apply-update": &applyUpdateTrayActionHandler{bus: b},
		},
	}, nil
}

func (p *desktopTrayPublisher) Release(ctx context.Context) error {
	var releaseErr error
	for id, entry := range p.entries {
		if err := p.releaseEntry(ctx, entry); err != nil && releaseErr == nil {
			releaseErr = err
		}
		delete(p.entries, id)
	}
	p.trayRef.Release()
	p.resources.Release()
	return releaseErr
}

func (p *desktopTrayPublisher) Publish(
	ctx context.Context,
	state *desktop_runtime.DesktopRuntimeState,
) (bool, error) {
	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	next := make(map[string]*desktop_tray.DesktopTrayEntry, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.GetId() == "" {
			continue
		}
		next[entry.GetId()] = entry.CloneVT()
	}

	var changed bool
	for id, entry := range next {
		current := p.entries[id]
		if current == nil {
			registered, err := p.register(ctx, entry)
			if err != nil {
				return changed, err
			}
			p.entries[id] = registered
			changed = true
			continue
		}
		if current.usesAttachedActionHandler() != entryUsesAttachedHandler(entry) {
			if err := p.releaseEntry(ctx, current); err != nil {
				return changed, err
			}
			delete(p.entries, id)
			registered, err := p.register(ctx, entry)
			if err != nil {
				return changed, err
			}
			p.entries[id] = registered
			changed = true
			continue
		}
		if current.entry.EqualVT(entry) {
			continue
		}
		_, err := current.service.SetDesktopTrayEntry(ctx, &desktop_tray.SetDesktopTrayEntryRequest{
			Entry: entry,
		})
		if err != nil {
			return changed, err
		}
		current.entry = entry.CloneVT()
		changed = true
	}

	for id, current := range p.entries {
		if next[id] != nil {
			continue
		}
		if err := p.releaseEntry(ctx, current); err != nil {
			return changed, err
		}
		delete(p.entries, id)
		changed = true
	}
	return changed, nil
}

func (p *desktopTrayPublisher) register(
	ctx context.Context,
	entry *desktop_tray.DesktopTrayEntry,
) (*desktopTrayEntryRegistration, error) {
	attachedActionResourceID, err := p.attachActionHandler(ctx, entry)
	if err != nil {
		return nil, err
	}
	resp, err := p.tray.RegisterDesktopTrayEntry(ctx, &desktop_tray.RegisterDesktopTrayEntryRequest{
		Entry:                    entry,
		AttachedActionResourceId: attachedActionResourceID,
	})
	if err != nil {
		if detachErr := p.detachActionHandler(ctx, attachedActionResourceID); detachErr != nil {
			return nil, errors.Errorf(
				"register desktop tray entry: %v; detach desktop tray action handler: %v",
				err,
				detachErr,
			)
		}
		return nil, errors.Wrap(err, "register desktop tray entry")
	}

	ref := p.resources.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		if detachErr := p.detachActionHandler(ctx, attachedActionResourceID); detachErr != nil {
			return nil, errors.Errorf(
				"open desktop tray entry resource: %v; detach desktop tray action handler: %v",
				err,
				detachErr,
			)
		}
		return nil, err
	}
	return &desktopTrayEntryRegistration{
		ref:                      ref,
		service:                  desktop_tray.NewSRPCDesktopTrayEntryResourceServiceClient(client),
		entry:                    entry.CloneVT(),
		attachedActionResourceID: attachedActionResourceID,
	}, nil
}

func (p *desktopTrayPublisher) attachActionHandler(
	ctx context.Context,
	entry *desktop_tray.DesktopTrayEntry,
) (uint32, error) {
	if !entryUsesAttachedHandler(entry) {
		return 0, nil
	}
	handler := p.actionHandlers[entry.GetId()]
	if handler == nil {
		return 0, errors.Errorf("desktop tray action handler missing for %s", entry.GetId())
	}
	mux := srpc.NewMux()
	if err := desktop_tray.SRPCRegisterDesktopTrayActionHandlerService(mux, handler); err != nil {
		return 0, err
	}
	return p.resources.AttachRawInvoker(ctx, "desktop-tray-action-"+entry.GetId(), mux)
}

func (p *desktopTrayPublisher) releaseEntry(
	ctx context.Context,
	entry *desktopTrayEntryRegistration,
) error {
	entry.ref.Release()
	return p.detachActionHandler(ctx, entry.attachedActionResourceID)
}

func (p *desktopTrayPublisher) detachActionHandler(ctx context.Context, resourceID uint32) error {
	if resourceID == 0 {
		return nil
	}
	return p.resources.DetachResource(ctx, resourceID)
}

func (r *desktopTrayEntryRegistration) usesAttachedActionHandler() bool {
	return r.attachedActionResourceID != 0
}

func entryUsesAttachedHandler(entry *desktop_tray.DesktopTrayEntry) bool {
	return entry.GetAction().GetKind() == desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER
}
