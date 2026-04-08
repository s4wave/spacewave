//go:build js

package volume_opfs

import (
	"context"

	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/block/gc/gcgraph"
	block_gc_wal "github.com/aperturerobotics/hydra/block/gc/wal"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/volume"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/aperturerobotics/hydra/volume/js/opfs/blockshard"
	"github.com/aperturerobotics/hydra/volume/js/opfs/metashard"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the OPFS volume controller.
const ControllerID = "hydra/volume/opfs"

// Version is the version of the OPFS volume implementation.
var Version = semver.MustParse("0.0.1")

// Opfs implements an OPFS-backed volume.
type Opfs = kvtx.Volume

// NewOpfs builds a new OPFS volume, opening or creating the directory tree.
func NewOpfs(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Opfs, error) {
	kk, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	rootPath := conf.GetRootPath()
	lockPrefix := conf.GetLockPrefix()
	if lockPrefix == "" {
		lockPrefix = rootPath
	}

	// Open or create the OPFS directory for this volume.
	opfsRoot, err := opfs.GetRoot()
	if err != nil {
		return nil, errors.Wrap(err, "opfs GetRoot")
	}

	pathParts, _ := unixfs.SplitPath(rootPath)
	volDir, err := opfs.GetDirectoryPath(opfsRoot, pathParts, true)
	if err != nil {
		return nil, errors.Wrap(err, "create volume directory")
	}

	// Block shard engine: sharded SSTable segments with per-shard write actors.
	blocksDir, err := opfs.GetDirectory(volDir, "blocks", true)
	if err != nil {
		return nil, errors.Wrap(err, "create blocks directory")
	}

	blockSettings := &blockshard.Settings{
		ShardCount:        int(conf.GetBlockShardCount()),
		BloomFPR:          conf.GetBlockBloomFpr(),
		CompactionTrigger: int(conf.GetBlockCompactionTrigger()),
		AsyncIO:           conf.GetAsyncIo(),
	}
	blkEngine, err := blockshard.NewEngineWithSettings(ctx, blocksDir, lockPrefix+"/blocks", blockSettings)
	if err != nil {
		return nil, errors.Wrap(err, "create block shard engine")
	}
	blkStore := blockshard.NewBlockStore(blkEngine, conf.GetStoreConfig().GetHashType())

	// Meta page store: single B+tree page file with dual superblocks.
	metaShardCount := conf.GetMetaShardCount()
	if metaShardCount == 0 {
		metaShardCount = 1
	}
	if metaShardCount != 1 {
		return nil, errors.Errorf("meta shard count must be 1, got %d", metaShardCount)
	}
	metaDir, err := opfs.GetDirectory(volDir, "meta", true)
	if err != nil {
		return nil, errors.Wrap(err, "create meta directory")
	}

	meta, err := metashard.NewMetaShard(metaDir, lockPrefix+"/meta", int(conf.GetPageSize()))
	if err != nil {
		return nil, errors.Wrap(err, "create meta shard")
	}
	metaStore := metashard.NewMetaStore(meta)

	var store skvtx.Store = metaStore
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	statsFn := func(ctx context.Context) (*volume.StorageStats, error) {
		count, totalBytes, txErr := blkEngine.LiveStats()
		if txErr != nil {
			return nil, txErr
		}
		return &volume.StorageStats{
			TotalBytes: totalBytes,
			BlockCount: count,
		}, nil
	}

	// GC graph store: own OPFS subdirectory with per-file locking.
	gcDir, err := opfs.GetDirectory(volDir, "gc", true)
	if err != nil {
		return nil, errors.Wrap(err, "create gc directory")
	}
	graphDir, err := opfs.GetDirectory(gcDir, "graph", true)
	if err != nil {
		return nil, errors.Wrap(err, "create gc/graph directory")
	}
	walDir, err := opfs.GetDirectory(gcDir, "wal", true)
	if err != nil {
		return nil, errors.Wrap(err, "create gc/wal directory")
	}

	gcGraph, err := gcgraph.NewGCGraph(graphDir, lockPrefix+"/gc/graph")
	if err != nil {
		return nil, errors.Wrap(err, "create GC graph store")
	}

	// Register volume-context roots.
	if err := gcGraph.AddRoot(ctx, block_gc.NodeGCRoot); err != nil {
		return nil, errors.Wrap(err, "register gcroot")
	}
	if err := gcGraph.AddRoot(ctx, block_gc.NodeUnreferenced); err != nil {
		return nil, errors.Wrap(err, "register unreferenced root")
	}

	// WAL writer with STW and ordering locks.
	stwLockName := lockPrefix + "|gc-stw"
	orderLockName := lockPrefix + "|gc-wal-order"
	walWriter := block_gc_wal.NewWriter(walDir, lockPrefix+"/gc/wal", orderLockName, stwLockName)
	walAppender := block_gc_wal.NewAppender(walWriter)

	vol, err := kvtx.NewVolumeWithBlockStoreAndGC(
		ctx,
		ControllerID,
		kk,
		store,
		blkStore,
		gcGraph,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		statsFn,
		func() error {
			blkEngine.Close()
			return gcGraph.Close()
		},
		func() error {
			// Delete: navigate to the parent, then remove the leaf directory.
			parts, _ := unixfs.SplitPath(rootPath)
			parent := opfsRoot
			for _, p := range parts[:len(parts)-1] {
				var err error
				parent, err = opfs.GetDirectory(parent, p, false)
				if err != nil {
					if opfs.IsNotFound(err) {
						return nil
					}
					return err
				}
			}
			return opfs.DeleteEntry(parent, parts[len(parts)-1], true)
		},
	)
	if err != nil {
		return nil, err
	}

	// Store the WAL appender on the volume so the volume controller
	// and bucket handles can propagate it to GCStoreOps instances.
	vol.SetWALAppender(walAppender)
	vol.SetGCManagerHooks(block_gc.ManagerHooks{
		Graph: gcGraph,
		ReplayWAL: func(ctx context.Context, graph block_gc.CollectorGraph) (int, error) {
			entries, filenames, err := block_gc_wal.ReadWAL(walDir, lockPrefix+"/gc/wal")
			if err != nil {
				return 0, err
			}
			for i, entry := range entries {
				adds := make([]block_gc.RefEdge, len(entry.GetAdds()))
				for j, e := range entry.GetAdds() {
					adds[j] = block_gc.RefEdge{Subject: e.GetSubject(), Object: e.GetObject()}
				}
				removes := make([]block_gc.RefEdge, len(entry.GetRemoves()))
				for j, e := range entry.GetRemoves() {
					removes[j] = block_gc.RefEdge{Subject: e.GetSubject(), Object: e.GetObject()}
				}
				if err := graph.ApplyRefBatch(ctx, adds, removes); err != nil {
					return i, err
				}
				if err := block_gc_wal.DeleteWALEntry(walDir, filenames[i]); err != nil {
					return i, err
				}
			}
			return len(entries), nil
		},
		AcquireSTW: func() (func(), error) {
			return filelock.AcquireWebLock(stwLockName, true)
		},
	})

	return vol, nil
}
