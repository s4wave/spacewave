package reconciler

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/volume"
)

// Reconciler is a routine executed when the reconciler message queue is filled.
type Reconciler interface {
	// Execute executes the reconciler with the handle.
	// The context will be canceled if the handle becomes invalid.
	// Returning an error triggers a retry.
	// Returning nil permanently exits without retrying.
	Execute(ctx context.Context, handle Handle) error
	// Close releases any resources used by the controller.
	// Error indicates any issue encountered releasing.
	Close() error
}

// Handle is the handle passed to a reconciler controller.
type Handle interface {
	// GetBucketId returns the bucket id.
	GetBucketId() string
	// GetReconcilerId returns the reconciler id.
	GetReconcilerId() string
	// GetBucketHandle returns the handle to the bucket.
	GetBucketHandle() volume.BucketHandle
	// GetVolume returns the volume.
	GetVolume() volume.Volume
	// GetEventQueue returns the reconciler event queue handle.
	GetEventQueue() mqueue.Queue
}

// Controller is implemented by the reconciler controller.
type Controller interface {
	// Controller indicates controller is a controller.
	controller.Controller

	// GetReconciler returns the reconciler instance when ready.
	GetReconciler() Reconciler
	// PushReconcilerHandle pushes the updated reconciler handle, overwriting
	// any other pending handle. This will trigger a restart of the reconciler
	// controller with the new handle.
	PushReconcilerHandle(Handle)
}

// Config is the minimum requirement for a reconciler config object.
type Config interface {
	// Config indicates the config is a config object.
	config.Config

	// GetBucketId returns the bucket id that the reconciler is attached to.
	GetBucketId() string
	// SetBucketId sets the bucket ID field.
	SetBucketId(id string)

	// GetVolumeId returns the volume id that the reconciler is attached to.
	GetVolumeId() string
	// SetVolumeId sets the volume ID field.
	SetVolumeId(id string)

	// GetReconcilerId returns the reconciler id that the reconciler is attached to.
	GetReconcilerId() string
	// SetReconcilerId sets the reconciler ID field.
	SetReconcilerId(id string)
}
