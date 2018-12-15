package hydra_volume

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/store"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a volume with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	bus bus.Bus,
) (Volume, error)

// Volume is a storage device attached to the network.
type Volume interface {
	// KV indicates a volume is a key-value capable store.
	store.KV
	// Peer indicates the volume has a peer identity.
	peer.Peer
	// GetVolumeInfo returns the basic volume information.
	GetVolumeInfo() *VolumeInfo
	// Close closes the volume, returning any errors.
	Close() error
}

// Controller is a volume controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller

	// GetVolume returns the controlled volume.
	// This may wait for the volume to be ready.
	GetVolume(ctx context.Context) (Volume, error)
}
