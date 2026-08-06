package s4wave_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ObjectTypeRegistrations retains a plugin's host ObjectType registrations.
type ObjectTypeRegistrations struct {
	client *resource_client.Client
	refs   []resource_client.ResourceRef
}

// RegisterObjectTypes registers ObjectTypes served by the running plugin.
// The returned registrations must be released when the plugin stops serving them.
func RegisterObjectTypes(
	ctx context.Context,
	b bus.Bus,
	typeIDs ...string,
) (*ObjectTypeRegistrations, error) {
	resourceService := resource.NewSRPCResourceServiceClientWithServiceID(
		bifrost_rpc.NewBusClient(b),
		bldr_plugin.HostServiceIDPrefix+resource.SRPCResourceServiceServiceID,
	)
	client, err := resource_client.NewClient(ctx, resourceService)
	if err != nil {
		return nil, errors.Wrap(err, "connect to plugin host resource")
	}

	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		return nil, err
	}
	defer rootRef.Release()

	service := sdk_plugin_host.NewSRPCPluginHostResourceServiceClient(rootClient)
	refs := make([]resource_client.ResourceRef, 0, len(typeIDs))
	for _, typeID := range typeIDs {
		resp, err := service.RegisterObjectType(ctx, &sdk_plugin_host.RegisterObjectTypeRequest{
			TypeId: typeID,
		})
		if err != nil {
			releaseObjectTypeRefs(refs)
			client.Release()
			return nil, errors.Wrapf(err, "register object type %q", typeID)
		}
		if resp.GetResourceId() == 0 {
			releaseObjectTypeRefs(refs)
			client.Release()
			return nil, errors.Errorf("register object type %q returned zero resource id", typeID)
		}
		refs = append(refs, client.CreateResourceReference(resp.GetResourceId()))
	}
	if _, err := service.CompleteInitialCapabilityRegistration(
		ctx,
		&sdk_plugin_host.CompleteInitialCapabilityRegistrationRequest{},
	); err != nil {
		releaseObjectTypeRefs(refs)
		client.Release()
		return nil, errors.Wrap(err, "complete initial capability registration")
	}

	return &ObjectTypeRegistrations{client: client, refs: refs}, nil
}

// Release releases all ObjectType registrations and the host resource client.
func (r *ObjectTypeRegistrations) Release() {
	if r == nil {
		return
	}
	releaseObjectTypeRefs(r.refs)
	r.refs = nil
	if r.client != nil {
		r.client.Release()
		<-r.client.Done()
		r.client = nil
	}
}

func releaseObjectTypeRefs(refs []resource_client.ResourceRef) {
	for _, ref := range refs {
		ref.Release()
	}
}
