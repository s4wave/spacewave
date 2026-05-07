package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
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
	ref     resource_client.ResourceRef
	service desktop_tray.SRPCDesktopTrayEntryResourceServiceClient
	entry   *desktop_tray.DesktopTrayEntry
}

type desktopTrayPublisher struct {
	resources *resource_client.Client
	trayRef   resource_client.ResourceRef
	tray      desktop_tray.SRPCDesktopTrayResourceServiceClient
	entries   map[string]*desktopTrayEntryRegistration
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
	}, nil
}

func (p *desktopTrayPublisher) Release() {
	for id, entry := range p.entries {
		entry.ref.Release()
		delete(p.entries, id)
	}
	p.trayRef.Release()
	p.resources.Release()
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
		current.ref.Release()
		delete(p.entries, id)
		changed = true
	}
	return changed, nil
}

func (p *desktopTrayPublisher) register(
	ctx context.Context,
	entry *desktop_tray.DesktopTrayEntry,
) (*desktopTrayEntryRegistration, error) {
	resp, err := p.tray.RegisterDesktopTrayEntry(ctx, &desktop_tray.RegisterDesktopTrayEntryRequest{
		Entry: entry,
	})
	if err != nil {
		return nil, errors.Wrap(err, "register desktop tray entry")
	}

	ref := p.resources.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, err
	}
	return &desktopTrayEntryRegistration{
		ref:     ref,
		service: desktop_tray.NewSRPCDesktopTrayEntryResourceServiceClient(client),
		entry:   entry.CloneVT(),
	}, nil
}
