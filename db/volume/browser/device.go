//go:build js

package volume_browser

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/volume/device"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
)

// openedDevice is a device the volume owns until it closes it.
type openedDevice interface {
	device.Device

	// Close releases the device.
	Close() error
}

// errOPFSUnavailable reports a context where OPFS sync access handles do not
// work, such as a shared worker or a WebKit private window.
var errOPFSUnavailable = errors.New("OPFS sync access handles unavailable")

// openDevice opens the device holding the volume name. An existing IndexedDB
// database keeps the volume there. Otherwise the volume is on OPFS when its
// probe passes and in a new IndexedDB database when it does not. Both devices
// failing is permanent.
func openDevice(ctx context.Context, name string) (openedDevice, error) {
	// Keep a volume already in IndexedDB there.
	idbErr := volume_idb.Available()
	if idbErr == nil {
		exists, err := volume_idb.DatabaseExists(name)
		if err != nil {
			return nil, err
		}
		if exists {
			return volume_idb.OpenDevice(ctx, name)
		}
	}

	// Prefer OPFS, falling back to IndexedDB where OPFS is unavailable.
	dev, err := openOPFS(name)
	if !errors.Is(err, errOPFSUnavailable) {
		return dev, err
	}
	if idbErr != nil {
		return nil, volume.Permanent(errors.Join(err, idbErr))
	}
	return volume_idb.OpenDevice(ctx, name)
}

// openOPFS opens the OPFS directory name, returning errOPFSUnavailable when
// the context cannot use OPFS sync access handles.
func openOPFS(name string) (openedDevice, error) {
	// Open the directory where sync access handles work.
	if !opfs.SyncAvailable() {
		return nil, errOPFSUnavailable
	}
	dev, err := device_opfs.Open(name)
	if opfs.IsSecurity(err) || opfs.IsUnknown(err) {
		return nil, errors.Join(errOPFSUnavailable, err)
	}
	if err != nil {
		return nil, err
	}
	return dev, nil
}

// Delete deletes the data of the volume name from both devices. The volume
// must be closed. A device without the volume, or unavailable in this
// context, has nothing to delete.
func Delete(name string) error {
	// Delete the OPFS directory where OPFS is reachable.
	err := device_opfs.Delete(name)
	if opfs.IsSecurity(err) || opfs.IsUnknown(err) {
		err = nil
	}

	// Delete the IndexedDB database where IndexedDB exists.
	if volume_idb.Available() == nil {
		err = errors.Join(err, volume_idb.DeleteDatabase(name))
	}
	return err
}
