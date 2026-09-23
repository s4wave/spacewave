package plugin_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

const cloudBlockStoreServiceIDPrefix = "space/cloud-block-store/"

const cloudBlockStoreForwarderControllerID = ControllerID + "/cloud-block-store-forwarder"

// CloudBlockStoreForwarder forwards the mounted Space bucket to the plugin host.
type CloudBlockStoreForwarder struct {
	// le is the logger.
	le *logrus.Entry
	// b is the controller bus.
	b bus.Bus
	// spaceID is the Space identifier used for the forwarded service id.
	spaceID string
	// bucketID is the mounted Space world bucket id.
	bucketID string
	// pluginID is the host plugin id that receives the bucket config.
	pluginID string
	// access retains the mounted Space's authenticated read-through storage.
	access world.AccessWorldStateFunc
}

// NewCloudBlockStoreForwarder constructs a cloud block store forwarding controller.
func NewCloudBlockStoreForwarder(
	le *logrus.Entry,
	b bus.Bus,
	spaceID string,
	bucketID string,
	pluginID string,
	access world.AccessWorldStateFunc,
) *CloudBlockStoreForwarder {
	return &CloudBlockStoreForwarder{
		le:       le,
		b:        b,
		spaceID:  spaceID,
		bucketID: bucketID,
		pluginID: pluginID,
		access:   access,
	}
}

// GetControllerInfo returns information about the controller.
func (c *CloudBlockStoreForwarder) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		cloudBlockStoreForwarderControllerID,
		Version,
		"forwards Space cloud block store to plugin host",
	)
}

// Execute executes the controller goroutine.
func (c *CloudBlockStoreForwarder) Execute(ctx context.Context) error {
	if c.bucketID == "" {
		return nil
	}
	if c.pluginID == "" {
		return errors.New("host_plugin_id is required when world_bucket_id is set")
	}
	serviceID := cloudBlockStoreServiceID(c.spaceID)
	le := c.le.
		WithField("bucket-id", c.bucketID).
		WithField("service-id", serviceID).
		WithField("plugin-id", c.pluginID)

	// Each call borrows the current mounted store. Looking up its local bucket
	// would discard Session read-through and strand artifacts built on a peer.
	invoker := srpc.InvokerFunc(func(requestService, method string, stream srpc.Stream) (bool, error) {
		if requestService != serviceID {
			return false, nil
		}
		var handled bool
		err := c.access(stream.Context(), nil, func(cursor *bucket_lookup.Cursor) error {
			readOnly := block_store.NewStoreReadThrough(func() block.StoreOps { return cursor.GetBlockStore() }, nil, false)
			handler := block_rpc.NewSRPCBlockStoreHandler(block_rpc_server.NewBlockStore(readOnly), serviceID)
			var err error
			handled, err = handler.InvokeMethod(requestService, method, stream)
			return err
		})
		return handled, err
	})
	server := bifrost_rpc.NewRpcServiceController(c.GetControllerInfo(),
		bifrost_rpc.NewRpcServiceBuilder(invoker), nil, false, nil, []string{serviceID}, nil)
	serverRelease, err := c.b.AddController(ctx, server, nil)
	if err != nil {
		return errors.Wrap(err, "start cloud block store rpc server")
	}
	defer serverRelease()

	bucketConf, err := bucket.NewConfig(c.bucketID, 1, nil)
	if err != nil {
		return err
	}

	hostRpcConf := &block_store_rpc.Config{
		BlockStoreId:  c.bucketID,
		ServiceId:     bldr_plugin.PluginServiceID(c.pluginID, serviceID),
		ReadOnly:      true,
		BucketIds:     []string{c.bucketID},
		LookupOnStart: true,
	}
	hostBucketConf := &block_store_bucket.Config{
		BlockStoreId:  c.bucketID,
		BucketConfig:  bucketConf,
		BucketStoreId: c.bucketID,
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
			"plugin-space-cloud-block-store-rpc/" + c.bucketID:    hostRpcCtrlConf,
			"plugin-space-cloud-block-store-bucket/" + c.bucketID: hostBucketCtrlConf,
		},
	}
	_, _, hostConfigSetRef, err := loader.WaitExecControllerRunning(
		ctx,
		c.b,
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

// HandleDirective asks if the handler can resolve the directive.
func (c *CloudBlockStoreForwarder) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases resources used by the controller.
func (c *CloudBlockStoreForwarder) Close() error {
	return nil
}

func cloudBlockStoreServiceID(spaceID string) string {
	return cloudBlockStoreServiceIDPrefix + spaceID
}

// _ is a type assertion
var _ controller.Controller = (*CloudBlockStoreForwarder)(nil)
