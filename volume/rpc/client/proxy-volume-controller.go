package volume_rpc_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	rpc_block "github.com/aperturerobotics/hydra/block/rpc"
	rpc_bucket "github.com/aperturerobotics/hydra/bucket/rpc"
	rpc_mqueue "github.com/aperturerobotics/hydra/mqueue/rpc"
	rpc_object "github.com/aperturerobotics/hydra/object/rpc"
	"github.com/aperturerobotics/hydra/volume"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	rpc_volume "github.com/aperturerobotics/hydra/volume/rpc"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// ProxyVolumeController implements a volume controller with a ProxyVolumeController service.
type ProxyVolumeController struct {
	*volume_controller.Controller
	// volumeInfo contains the volume information.
	volumeInfo *volume.VolumeInfo

	// proxyVolumeClient is the client for the ProxyVolume service
	proxyVolumeClient rpc_volume.SRPCProxyVolumeClient
	// blockStoreClient is the client for the BlockStore
	blockStoreClient rpc_block.SRPCBlockStoreClient
	// bucketStoreClient is the client for the BucketStore
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient
	// objectStoreClient is the client for the ObjectStore.
	objectStoreClient rpc_object.SRPCObjectStoreClient
	// mqueueStoreClient is the client for the MqueueStore
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient
}

// NewProxyVolumeController constructs a new ProxyVolumeController.
func NewProxyVolumeController(
	b bus.Bus,
	le *logrus.Entry,
	volumeInfo *volume.VolumeInfo,
	volumeIDAlias []string,
	proxyVolumeClient rpc_volume.SRPCProxyVolumeClient,
	blockStoreClient rpc_block.SRPCBlockStoreClient,
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient,
	objectStoreClient rpc_object.SRPCObjectStoreClient,
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient,
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
					ctx,
					volumeInfo,
					proxyVolumeClient,
					blockStoreClient,
					bucketStoreClient,
					objectStoreClient,
					mqueueStoreClient,
				)
			},
		),

		volumeInfo:        volumeInfo,
		proxyVolumeClient: proxyVolumeClient,
		blockStoreClient:  blockStoreClient,
		bucketStoreClient: bucketStoreClient,
		objectStoreClient: objectStoreClient,
		mqueueStoreClient: mqueueStoreClient,
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
		rpc_volume.NewSRPCProxyVolumeClientWithServiceID(
			cc,
			serviceIDPrefix+rpc_volume.SRPCProxyVolumeServiceID,
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
	)
}

// GetID returns the volume ID.
func (v *ProxyVolumeController) GetID() string {
	return v.volumeInfo.GetVolumeId()
}

// GetVolumeClient returns the proxy volume client.
func (v *ProxyVolumeController) GetVolumeClient() rpc_volume.SRPCProxyVolumeClient {
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
