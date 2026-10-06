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
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/logindex"
	"github.com/s4wave/spacewave/db/volume/paylog"
	"github.com/s4wave/spacewave/db/volume/refgraph"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the browser volume controller.
const ControllerID = "hydra/volume/browser"

// Version is the version of the browser volume implementation.
var Version = controller.MustParseVersion("0.0.1")

// metadataPrefix holds the Volume metadata in the engine's store, apart from
// the garbage collection graph, which refgraph keeps under its own prefix.
const metadataPrefix = "m/"

// Volume is the browser volume.
type Volume = volume_kvtx.Volume

// NewVolume opens the volume named by conf on its device, creating it when
// absent.
func NewVolume(ctx context.Context, le *logrus.Entry, conf *Config) (*Volume, error) {
	// Check the config and open the device.
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	st, err := openDevice(ctx, conf.GetName())
	if err != nil {
		return nil, err
	}

	// Open the index and the engine on the device.
	idx, err := logindex.Open(ctx, st.dev, logindex.Options{})
	if err != nil {
		return nil, errors.Join(err, st.dev.Close())
	}
	hashType := conf.GetStoreConfig().ResolveHashType()
	s, err := paylog.Open(ctx, st.dev, idx, hashType)
	if err != nil {
		return nil, errors.Join(err, st.dev.Close())
	}
	closeStore := func() error {
		return errors.Join(s.Close(), st.dev.Close())
	}

	// Metadata and the garbage collection graph have disjoint namespaces.
	var store kvtx.Store = kvtx_prefixer.NewPrefixer(s, []byte(metadataPrefix))
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}
	graph := refgraph.NewGraph(s)
	for _, node := range []string{block_gc.NodeGCRoot, block_gc.NodeUnreferenced} {
		if err := graph.AddRoot(ctx, node); err != nil {
			return nil, errors.Join(err, closeStore())
		}
	}

	// Build the Volume and wire the collector hooks.
	stats := func(ctx context.Context) (*volume.StorageStats, error) {
		count, size, err := s.BlockStats(ctx)
		if err != nil {
			return nil, err
		}
		return &volume.StorageStats{BlockCount: count, TotalBytes: size}, nil
	}
	vol, err := volume_kvtx.NewVolumeWithBlockStoreAndGC(
		ctx, ControllerID, keys, store, s, graph, conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), stats, closeStore,
		st.remove,
	)
	if err != nil {
		return nil, errors.Join(err, closeStore())
	}
	vol.SetWALAppender(s)
	vol.SetGCManagerHooks(block_gc.ManagerHooks{
		Graph:       graph,
		ReplayWAL:   s.ReplayWAL,
		AcquireSTW:  func() (func(), error) { return s.AcquireSTW(ctx) },
		Maintenance: s.Maintenance,
	})
	return vol, nil
}
