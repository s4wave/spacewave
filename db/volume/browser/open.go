package volume_browser

import (
	"context"
	"errors"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
	"github.com/s4wave/spacewave/db/volume/paylog"
)

// ControllerID identifies the browser volume controller.
const ControllerID = "hydra/volume/browser"

// volumePrefix holds the Volume's records in the engine's store.
const volumePrefix = "m/"

// graphPrefix held the reference graph records beside the Volume before the
// graph moved into it.
const graphPrefix = "\x03"

// legacyGraphPrefix held the reference graph records of format version 2 beside
// the Volume.
const legacyGraphPrefix = "\x02"

// Volume is the browser volume.
type Volume = volume_kvtx.Volume

// openedDevice is a device the volume owns until it closes it.
type openedDevice interface {
	device.Device

	// Close releases the device, and its lock, once.
	Close() error
}

// deviceOpener opens the device holding the volume name. While another
// opener holds it, it fails with device.ErrHeld.
type deviceOpener func(ctx context.Context, name string) (openedDevice, error)

// openVolume opens the volume named by conf on the device openDev opens,
// creating it when absent; deleteFn deletes its data. The open owns the
// device until the Volume does, so every failed open, by an error, a
// cancelled context or a panic, closes it and releases its lock, and the next
// open can take the device. The failure path closes the index and the device
// only, and writes, truncates or deletes nothing.
func openVolume(ctx context.Context, conf *Config, openDev deviceOpener, deleteFn func() error) (_ *Volume, err error) {
	// Open the device and hold it until the Volume owns it.
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	dev, err := openDev(ctx, conf.GetName())
	if err != nil {
		return nil, err
	}
	var op volume.Opening
	defer op.Settle(&err)
	op.Hold(dev.Close)

	// Open the index and the engine on the device.
	idx, err := logindex.Open(ctx, dev, logindex.Options{})
	if err != nil {
		return nil, err
	}
	op.Hold(func() error { return errors.Join(idx.Close(), dev.Close()) })
	hashType := conf.GetStoreConfig().ResolveHashType()
	s, err := paylog.Open(ctx, dev, idx, hashType)
	if err != nil {
		return nil, err
	}
	closeStore := func() error {
		return errors.Join(s.Close(), dev.Close())
	}
	op.Hold(closeStore)

	// Move the reference graph the engine kept beside the Volume into it.
	err = volume_kvtx.MoveSideGraph(ctx, s.Domain(nil), []byte(volumePrefix), []byte(legacyGraphPrefix), []byte(graphPrefix))
	if err != nil {
		return nil, err
	}

	// Build the Volume in its namespace of the engine. It owns the store from
	// here, including when its build fails.
	stats := func(ctx context.Context) (*volume.StorageStats, error) {
		count, size, err := s.BlockStats(ctx)
		if err != nil {
			return nil, err
		}
		return &volume.StorageStats{BlockCount: count, TotalBytes: size}, nil
	}
	op.Handoff()
	return volume_kvtx.NewVolume(
		ctx, ControllerID, keys, s.Domain([]byte(volumePrefix)), conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), stats, closeStore, deleteFn,
	)
}

// _ is a type assertion: the Volume serves its blocks from the engine only
// through a BlockDomain.
var _ volume_kvtx.BlockDomain = (*paylog.Domain)(nil)
