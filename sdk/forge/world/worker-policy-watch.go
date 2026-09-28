//go:build !tinygo && !goscript

package s4wave_forge_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// workerPolicyWatch owns one plugin-host stream and its Resource references.
type workerPolicyWatch struct {
	stream  plugin_host.SRPCPluginHostResourceService_WatchDevicePolicyClient
	release func()
}

// openWorkerPolicyWatch subscribes to daemon policy through the existing host transport.
func openWorkerPolicyWatch(ctx context.Context, b bus.Bus) (*workerPolicyWatch, error) {
	service := resource.NewSRPCResourceServiceClientWithServiceID(
		bifrost_rpc.NewBusClient(b), bldr_plugin.HostServiceIDPrefix+resource.SRPCResourceServiceServiceID,
	)
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		return nil, errors.Wrap(err, "connect to plugin host")
	}
	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		return nil, err
	}
	host := plugin_host.NewSRPCPluginHostResourceServiceClient(rootClient)
	stream, err := host.WatchDevicePolicy(ctx, &plugin_host.WatchDevicePolicyRequest{})
	if err != nil {
		rootRef.Release()
		client.Release()
		return nil, err
	}
	return &workerPolicyWatch{stream: stream, release: func() {
		stream.Close()
		rootRef.Release()
		client.Release()
	}}, nil
}

// Recv decodes one complete policy snapshot and its enrolled Device identity.
func (w *workerPolicyWatch) Recv() (*device_policy.DevicePolicy, string, error) {
	update, err := w.stream.Recv()
	if err != nil {
		return nil, "", err
	}
	policy := &device_policy.DevicePolicy{}
	if err := policy.UnmarshalVT(update.GetPolicy()); err != nil {
		return nil, "", errors.Wrap(err, "decode daemon policy")
	}
	if policy.GetRevision() != update.GetRevision() {
		return nil, "", errors.New("device policy revision mismatch")
	}
	return policy, update.GetDeviceObjectKey(), nil
}

// Close releases the host stream and its root Resource references.
func (w *workerPolicyWatch) Close() { w.release() }
