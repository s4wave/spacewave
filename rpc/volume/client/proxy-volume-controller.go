package rpc_volume_client

import (
	"context"
	"errors"

	rpc_block "github.com/aperturerobotics/bldr/rpc/block"
	rpc_bucket "github.com/aperturerobotics/bldr/rpc/bucket"
	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	rpc_object "github.com/aperturerobotics/bldr/rpc/object"
	rpc_volume "github.com/aperturerobotics/bldr/rpc/volume"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/ccontainer"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// ProxyVolumeController implements a volume controller with a ProxyVolumeController service.
type ProxyVolumeController struct {
	// le is the logger
	le *logrus.Entry
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
	// volumeInfo contains the volume information.
	volumeInfo *volume.VolumeInfo
	// volume contains the volume instance
	volume *ccontainer.CContainer[*ProxyVolume]
}

// NewProxyVolumeController constructs a new ProxyVolumeController.
func NewProxyVolumeController(
	le *logrus.Entry,
	volumeInfo *volume.VolumeInfo,
	proxyVolumeClient rpc_volume.SRPCProxyVolumeClient,
	blockStoreClient rpc_block.SRPCBlockStoreClient,
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient,
	objectStoreClient rpc_object.SRPCObjectStoreClient,
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient,
) *ProxyVolumeController {
	return &ProxyVolumeController{
		le:                le,
		proxyVolumeClient: proxyVolumeClient,
		blockStoreClient:  blockStoreClient,
		bucketStoreClient: bucketStoreClient,
		objectStoreClient: objectStoreClient,
		mqueueStoreClient: mqueueStoreClient,
		volumeInfo:        volumeInfo,
		volume:            ccontainer.NewCContainer[*ProxyVolume](nil),
	}
}

// GetID returns the volume ID.
func (v *ProxyVolumeController) GetID() string {
	return v.volumeInfo.GetVolumeId()
}

// GetControllerInfo returns information about the controller.
func (v *ProxyVolumeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID+"-volume-controller",
		Version,
		"volume controller: "+v.GetID(),
	)
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

// GetVolume returns the controlled volume.
// This may wait for the volume to be ready.
func (v *ProxyVolumeController) GetVolume(ctx context.Context) (volume.Volume, error) {
	vol, err := v.volume.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return vol, nil
}

// BuildBucketAPI builds an API handle for the bucket ID in the volume.
// The handles are valid while ctx is valid.
func (v *ProxyVolumeController) BuildBucketAPI(
	ctx context.Context,
	bucketID string,
) (volume.BucketHandle, error) {
	return nil, errors.New("TODO proxy volume controller build bucket api")
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (v *ProxyVolumeController) Execute(ctx context.Context) error {
	// Lookup the volume information.
	v.le.Debug("volume constructed, initializing")
	proxyVolume, err := NewProxyVolume(
		ctx,
		v.volumeInfo,
		v.proxyVolumeClient,
		v.blockStoreClient,
		v.bucketStoreClient,
		v.objectStoreClient,
		v.mqueueStoreClient,
	)
	if err != nil {
		return err
	}

	// note: proxyVolume.Execute() is no-op, don't bother calling it.
	v.le.Info("volume ready")

	v.volume.SetValue(proxyVolume)
	<-ctx.Done()
	v.volume.SetValue(nil)

	return err
}

// HandleDirective asks if the handler can resolve the directive.
func (v *ProxyVolumeController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	// TODO: resolve volume related directives
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (v *ProxyVolumeController) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Controller = ((*ProxyVolumeController)(nil))
