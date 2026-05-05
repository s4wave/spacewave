package plugin_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	"github.com/s4wave/spacewave/db/bucket"
)

const cloudBlockStoreServiceIDPrefix = "space/cloud-block-store/"

// runCloudBlockStoreForwarding forwards the mounted Space bucket to the plugin host.
func (c *Controller) runCloudBlockStoreForwarding(ctx context.Context) error {
	conf := c.GetConfig()
	bucketID := conf.GetWorldBucketId()
	if bucketID == "" {
		return nil
	}
	pluginID := conf.GetHostPluginId()
	if pluginID == "" {
		return errors.New("host_plugin_id is required when world_bucket_id is set")
	}
	serviceID := cloudBlockStoreServiceID(conf.GetSpaceId())
	le := c.GetLogger().
		WithField("bucket-id", bucketID).
		WithField("service-id", serviceID).
		WithField("plugin-id", pluginID)

	serverConf := &block_store_rpc_server.Config{
		BlockStoreId: bucketID,
		ServiceId:    serviceID,
	}
	_, _, serverRef, err := loader.WaitExecControllerRunning(
		ctx,
		c.GetBus(),
		resolver.NewLoadControllerWithConfig(serverConf),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "start cloud block store rpc server")
	}
	defer serverRef.Release()

	bucketConf, err := bucket.NewConfig(bucketID, 1, nil, nil)
	if err != nil {
		return err
	}

	hostRpcConf := &block_store_rpc.Config{
		BlockStoreId:  bucketID,
		ServiceId:     bldr_plugin.PluginServiceID(pluginID, serviceID),
		ReadOnly:      true,
		BucketIds:     []string{bucketID},
		LookupOnStart: true,
	}
	hostBucketConf := &block_store_bucket.Config{
		BlockStoreId:  bucketID,
		BucketConfig:  bucketConf,
		BucketStoreId: bucketID,
	}
	hostRpcCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, hostRpcConf), false)
	if err != nil {
		return err
	}
	hostBucketCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, hostBucketConf), false)
	if err != nil {
		return err
	}

	hostConfigSet := &plugin_host_configset.Config{
		ConfigSet: map[string]*configset_proto.ControllerConfig{
			"plugin-space-cloud-block-store-rpc/" + bucketID:    hostRpcCtrlConf,
			"plugin-space-cloud-block-store-bucket/" + bucketID: hostBucketCtrlConf,
		},
	}
	_, _, hostConfigSetRef, err := loader.WaitExecControllerRunning(
		ctx,
		c.GetBus(),
		resolver.NewLoadControllerWithConfig(hostConfigSet),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "apply cloud block store host configset")
	}
	defer hostConfigSetRef.Release()

	le.Info("forwarding Space cloud block store to plugin host")
	<-ctx.Done()
	return ctx.Err()
}

func cloudBlockStoreServiceID(spaceID string) string {
	return cloudBlockStoreServiceIDPrefix + spaceID
}
