package volume_rpc_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	rpc_gc "github.com/s4wave/spacewave/db/block/gc/rpc"
	rpc_block "github.com/s4wave/spacewave/db/block/rpc"
	rpc_bucket "github.com/s4wave/spacewave/db/bucket/store/rpc"
	rpc_mqueue "github.com/s4wave/spacewave/db/mqueue/rpc"
	rpc_object "github.com/s4wave/spacewave/db/object/rpc"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// ProxyVolumeController implements a volume controller with a ProxyVolumeController service.
type ProxyVolumeController struct {
	*volume_controller.Controller
	// volumeInfo contains the volume information.
	volumeInfo *volume.VolumeInfo

	// proxyVolumeClient is the client for the ProxyVolume service
	proxyVolumeClient volume_rpc.SRPCProxyVolumeClient
	// blockStoreClient is the client for the BlockStore
	blockStoreClient rpc_block.SRPCBlockStoreClient
	// bucketStoreClient is the client for the BucketStore
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient
	// objectStoreClient is the client for the ObjectStore.
	objectStoreClient rpc_object.SRPCObjectStoreClient
	// mqueueStoreClient is the client for the MqueueStore
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient
	// refGraphClient is the client for the RefGraph
	refGraphClient rpc_gc.SRPCRefGraphClient
}

// NewProxyVolumeController constructs a new ProxyVolumeController.
func NewProxyVolumeController(
	b bus.Bus,
	le *logrus.Entry,
	volumeInfo *volume.VolumeInfo,
	volumeIDAlias []string,
	proxyVolumeClient volume_rpc.SRPCProxyVolumeClient,
	blockStoreClient rpc_block.SRPCBlockStoreClient,
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient,
	objectStoreClient rpc_object.SRPCObjectStoreClient,
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient,
	refGraphClient rpc_gc.SRPCRefGraphClient,
) *ProxyVolumeController {
	return &ProxyVolumeController{
		Controller: volume_controller.NewController(
			le,
			&volume_controller.Config{
				VolumeIdAlias: volumeIDAlias,

				DisableEventBlockRm:     true,
				DisableReconcilerQueues: true,
				DisablePeer:             true,
			},
			b,
			controller.NewInfo(
				ControllerID+"-volume-controller",
				Version,
				"volume controller: "+volumeInfo.GetVolumeId(),
			),
			func(
				ctx context.Context,
				le *logrus.Entry,
			) (volume.Volume, error) {
				return NewProxyVolume(
					volumeInfo,
					proxyVolumeClient,
					blockStoreClient,
					bucketStoreClient,
					objectStoreClient,
					mqueueStoreClient,
					refGraphClient,
				)
			},
		),

		volumeInfo:        volumeInfo,
		proxyVolumeClient: proxyVolumeClient,
		blockStoreClient:  blockStoreClient,
		bucketStoreClient: bucketStoreClient,
		objectStoreClient: objectStoreClient,
		mqueueStoreClient: mqueueStoreClient,
		refGraphClient:    refGraphClient,
	}
}

// NewProxyVolumeControllerWithClient constructs a new ProxyVolumeController with a client and service id prefix.
func NewProxyVolumeControllerWithClient(
	b bus.Bus,
	le *logrus.Entry,
	volumeInfo *volume.VolumeInfo,
	volumeIDAlias []string,
	cc srpc.Client,
	serviceIDPrefix string,
) *ProxyVolumeController {
	return NewProxyVolumeController(
		b,
		le,
		volumeInfo,
		volumeIDAlias,
		volume_rpc.NewSRPCProxyVolumeClientWithServiceID(
			cc,
			serviceIDPrefix+volume_rpc.SRPCProxyVolumeServiceID,
		),
		rpc_block.NewSRPCBlockStoreClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_block.SRPCBlockStoreServiceID,
		),
		rpc_bucket.NewSRPCBucketStoreClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_bucket.SRPCBucketStoreServiceID,
		),
		rpc_object.NewSRPCObjectStoreClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_object.SRPCObjectStoreServiceID,
		),
		rpc_mqueue.NewSRPCMqueueStoreClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_mqueue.SRPCMqueueStoreServiceID,
		),
		rpc_gc.NewSRPCRefGraphClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_gc.SRPCRefGraphServiceID,
		),
	)
}

// GetID returns the volume ID.
func (v *ProxyVolumeController) GetID() string {
	return v.volumeInfo.GetVolumeId()
}

// GetVolumeClient returns the proxy volume client.
func (v *ProxyVolumeController) GetVolumeClient() volume_rpc.SRPCProxyVolumeClient {
	return v.proxyVolumeClient
}

// GetBlockStoreClient returns the block store client.
func (v *ProxyVolumeController) GetBlockStoreClient() rpc_block.SRPCBlockStoreClient {
	return v.blockStoreClient
}

// GetBucketStoreClient returns the bucket store client.
func (v *ProxyVolumeController) GetBucketStoreClient() rpc_bucket.SRPCBucketStoreClient {
	return v.bucketStoreClient
}

// GetObjectStoreClient returns the object store client.
func (v *ProxyVolumeController) GetObjectStoreClient() rpc_object.SRPCObjectStoreClient {
	return v.objectStoreClient
}

// GetMqueueStoreClient returns the store for the message queue.
func (v *ProxyVolumeController) GetMqueueStoreClient() rpc_mqueue.SRPCMqueueStoreClient {
	return v.mqueueStoreClient
}

// _ is a type assertion
var _ volume.Controller = ((*ProxyVolumeController)(nil))
