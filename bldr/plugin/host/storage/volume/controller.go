package bldr_plugin_host_storage_volume

import (
	"context"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver/v4"
)

// ControllerID is the controller id.
const ControllerID = "bldr/plugin/host/storage/volume"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "plugin host storage volume"

// Controller is the session controller.
type Controller struct {
	*bus.BusController[*Config]

	volProm *promise.PromiseContainer[volume.Controller]
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
				volProm:       promise.NewPromiseContainer[volume.Controller](),
			}, nil
		},
	)
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
// Retry will NOT re-construct the controller, just re-start Execute.
func (c *Controller) Execute(ctx context.Context) error {
	// Start the storage volume on the plugin host.
	storageVolumeID := c.GetConfig().GetStorageVolumeId()
	hostVolumeID := "sv-" + storageVolumeID

	hostVolumeConf := c.GetConfig().GetVolumeConfig().CloneVT()
	if hostVolumeConf == nil {
		hostVolumeConf = &volume_controller.Config{}
	}

	// Use an ID as an alias so we can access it via rpc.
	hostVolumeConf.VolumeIdAlias = []string{hostVolumeID}
	hostVolumeConf.DisablePeer = true
	hostVolumeConf.DisableReconcilerQueues = true
	hostVolumeConf.DisableEventBlockRm = true

	// Host storage ID defaults
	hostStorageID := c.GetConfig().GetStorageId()
	if hostStorageID == "" {
		hostStorageID = "default"
	}

	// Start via the plugin host.
	hostStorageVolumeConf := &storage_volume.Config{
		StorageId:       hostStorageID,
		StorageVolumeId: storageVolumeID,
		VolumeConfig:    hostVolumeConf,
	}
	hostStorageVolumeCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, hostStorageVolumeConf), false)
	if err != nil {
		return err
	}
	hostStorageVolumeServiceID := storageVolumeID + "/" + volume_rpc.SRPCAccessVolumesServiceID
	hostStorageVolumeRpcServerConf := &volume_rpc_server.Config{
		ServiceId:        hostStorageVolumeServiceID,
		VolumeIdList:     []string{hostVolumeID},
		ExposePrivateKey: true,
	}
	hostStorageVolumeRpcServerCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, hostStorageVolumeRpcServerConf), false)
	if err != nil {
		return err
	}
	hostConfigSet := &plugin_host_configset.Config{
		ConfigSet: map[string]*configset_proto.ControllerConfig{
			hostVolumeID + "-vol": hostStorageVolumeCtrlConf,
			hostVolumeID + "-srv": hostStorageVolumeRpcServerCtrlConf,
		},
	}
	_, _, hostConfigSetRef, err := loader.WaitExecControllerRunning(ctx, c.GetBus(), resolver.NewLoadControllerWithConfig(hostConfigSet), nil)
	if err != nil {
		return err
	}
	defer hostConfigSetRef.Release()

	// Construct the volume rpc client controller.
	volumeRpcClientConf := &volume_rpc_client.Config{
		ServiceId:     bldr_plugin.HostServiceIDPrefix + hostStorageVolumeServiceID,
		VolumeIdList:  []string{hostVolumeID},
		LoadOnStartup: true,
		VolumeAliases: map[string]*volume_rpc_client.VolumeAliases{
			hostVolumeID: {
				From: c.GetConfig().GetVolumeConfig().GetVolumeIdAlias(),
			},
		},
	}
	rpcClient, _, rpcClientRef, err := loader.WaitExecControllerRunningTyped[*volume_rpc_client.Controller](ctx, c.GetBus(), resolver.NewLoadControllerWithConfig(volumeRpcClientConf), nil)
	if err != nil {
		return err
	}
	defer rpcClientRef.Release()

	volCtrl, relVolCtrl, err := rpcClient.LoadProxyVolume(ctx, hostVolumeID)
	if err != nil {
		return err
	}
	defer relVolCtrl()

	c.volProm.SetResult(volCtrl, nil)
	defer c.volProm.SetPromise(nil)

	// Wait for context cancel to release everything.
	<-ctx.Done()
	return context.Canceled
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// GetVolume returns the controlled volume.
// This may wait for the volume to be ready.
func (c *Controller) GetVolume(ctx context.Context) (volume.Volume, error) {
	volCtrl, err := c.volProm.Await(ctx)
	if err != nil {
		return nil, err
	}
	return volCtrl.GetVolume(ctx)
}

// BuildBucketAPI builds an API handle for the bucket ID in the volume.
// Returns the handle & a release function, or (nil, nil, err).
func (c *Controller) BuildBucketAPI(ctx context.Context, bucketID string) (bucket.BucketHandle, func(), error) {
	volCtrl, err := c.volProm.Await(ctx)
	if err != nil {
		return nil, nil, err
	}
	return volCtrl.BuildBucketAPI(ctx, bucketID)
}

// _ is a type assertion
var (
	_ controller.Controller = ((*Controller)(nil))
	_ volume.Controller     = ((*Controller)(nil))
)
