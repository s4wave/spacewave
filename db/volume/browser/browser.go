//go:build js

// Package volume_browser implements the browser Volume: the paylog engine
// over the logindex index on an OPFS or IndexedDB device.
//
// The device holds an exclusive Web Lock from open until close, so one
// runtime host owns the volume and the default in-process coordinator serves
// its leases and watches.
package volume_browser

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/volume"
)

// Version is the version of the browser volume implementation.
var Version = controller.MustParseVersion("0.0.1")

// NewVolume opens the volume named by conf on its device, creating it when
// absent. A failed open releases the device for the next.
func NewVolume(ctx context.Context, conf *Config) (*Volume, error) {
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	return openVolume(ctx, conf, openDevice, func() error { return Delete(conf.GetName()) })
}
