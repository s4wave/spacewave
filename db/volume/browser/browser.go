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
	"errors"

	"github.com/aperturerobotics/controllerbus/controller"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/logindex"
	"github.com/s4wave/spacewave/db/volume/paylog"
)

// ControllerID identifies the browser volume controller.
const ControllerID = "hydra/volume/browser"

// Version is the version of the browser volume implementation.
var Version = controller.MustParseVersion("0.0.1")

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

// NewVolume opens the volume named by conf on its device, creating it when
// absent.
func NewVolume(ctx context.Context, conf *Config) (*Volume, error) {
	// Check the config and open the device.
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	dev, err := openDevice(ctx, conf.GetName())
	if err != nil {
		return nil, err
	}

	// Open the index and the engine on the device.
	idx, err := logindex.Open(ctx, dev, logindex.Options{})
	if err != nil {
		return nil, errors.Join(err, dev.Close())
	}
	hashType := conf.GetStoreConfig().ResolveHashType()
	s, err := paylog.Open(ctx, dev, idx, hashType)
	if err != nil {
		return nil, errors.Join(err, dev.Close())
	}
	closeStore := func() error {
		return errors.Join(s.Close(), dev.Close())
	}

	// Move the reference graph the engine kept beside the Volume into it.
	err = volume_kvtx.MoveSideGraph(ctx, s.Domain(nil), []byte(volumePrefix), []byte(legacyGraphPrefix), []byte(graphPrefix))
	if err != nil {
		return nil, errors.Join(err, closeStore())
	}

	// Build the Volume in its namespace of the engine.
	stats := func(ctx context.Context) (*volume.StorageStats, error) {
		count, size, err := s.BlockStats(ctx)
		if err != nil {
			return nil, err
		}
		return &volume.StorageStats{BlockCount: count, TotalBytes: size}, nil
	}
	vol, err := volume_kvtx.NewVolume(
		ctx, ControllerID, keys, s.Domain([]byte(volumePrefix)), conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), stats, closeStore,
		func() error { return Delete(conf.GetName()) },
	)
	if err != nil {
		return nil, errors.Join(err, closeStore())
	}
	return vol, nil
}

// _ is a type assertion: the Volume serves its blocks from the engine only
// through a BlockDomain.
var _ volume_kvtx.BlockDomain = (*paylog.Domain)(nil)
