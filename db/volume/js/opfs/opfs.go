//go:build js

package volume_opfs

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/controller"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	coord_opfs "github.com/s4wave/spacewave/db/coord/opfs"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/opfs"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
	"github.com/s4wave/spacewave/db/volume/refgraph"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the OPFS volume controller.
const ControllerID = "hydra/volume/opfs"

// Version identifies the immutable OPFS volume implementation.
var Version = controller.MustParseVersion("0.0.2")

// Opfs implements the existing Volume interface over one immutable storage engine.
type Opfs = volume_kvtx.Volume

// NewOpfs opens a compatible volume or creates a new empty one.
// Incompatible saved data remains intact beside a current-format replacement.
func NewOpfs(ctx context.Context, le *logrus.Entry, conf *Config) (*Opfs, error) {
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	opfs.InstallRemoteDriverFromGlobal()
	root, err := opfs.GetRoot()
	if err != nil {
		if opfs.IsSecurity(err) || opfs.IsUnknown(err) {
			return nil, volume.Permanent(err)
		}
		return nil, err
	}
	dir, rootPath, err := openRuntimeRoot(ctx, le, root, conf)
	if err != nil {
		return nil, err
	}
	lockPrefix := conf.GetLockPrefix()
	if lockPrefix == "" {
		lockPrefix = conf.GetRootPath()
	}
	if rootPath != conf.GetRootPath() {
		lockPrefix += recoverySuffix
	}
	// Serialize first identity creation across every runtime mounting this volume.
	backend := engine.NewBrowserBackend(opfs.DefaultDriver, dir, lockPrefix)
	releaseInit, err := backend.Lock(ctx, "initialize", true)
	if err != nil {
		return nil, errors.Join(err, backend.Close())
	}
	defer releaseInit()
	e, err := engine.Open(ctx, backend)
	if err != nil {
		return nil, err
	}
	blocks := engine.NewBlockStore(ctx, e, conf.GetStoreConfig().ResolveHashType())
	closeStore := func() error {
		return errors.Join(blocks.Close(), e.Close())
	}

	// Metadata, GC records, and block locations have disjoint ordered namespaces.
	var store kvtx.Store = e.MetadataStore()
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}
	graph := refgraph.NewGraph(e)
	for _, node := range []string{block_gc.NodeGCRoot, block_gc.NodeUnreferenced} {
		if err := graph.AddRoot(ctx, node); err != nil {
			_ = closeStore()
			return nil, err
		}
	}
	stats := func(ctx context.Context) (*volume.StorageStats, error) {
		count, size, err := e.BlockStats(ctx)
		if err != nil {
			return nil, err
		}
		return &volume.StorageStats{BlockCount: count, TotalBytes: size}, nil
	}
	vol, err := volume_kvtx.NewVolumeWithBlockStoreAndGC(
		ctx, ControllerID, keys, store, blocks, graph, conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), stats, closeStore,
		func() error { return deleteRuntimeRoot(root, rootPath) },
	)
	if err != nil {
		_ = closeStore()
		return nil, errors.Join(errors.New("initialize OPFS volume identity"), err)
	}
	vol.Coordinator = coord_opfs.NewCoordinator(e, lockPrefix, coord_inmem.ForVolume(vol.GetID()))
	vol.SetWALAppender(e)
	vol.SetGCManagerHooks(block_gc.ManagerHooks{
		Graph:       graph,
		ReplayWAL:   e.ReplayWAL,
		AcquireSTW:  func() (func(), error) { return e.AcquireSTW(ctx) },
		Maintenance: e.Maintenance,
	})
	return vol, nil
}
