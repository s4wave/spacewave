//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bbolt"
	bbolt_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// volumeRepairEdgeBatch bounds the edges one ref graph transaction removes.
const volumeRepairEdgeBatch = 4096

// volumeRepairKeyBatch bounds the keys one delete transaction removes.
const volumeRepairKeyBatch = 50000

// volumeCompactTxBytes bounds the bytes one compaction copy transaction holds.
const volumeCompactTxBytes = 64 << 20

// volumeRepairResult counts the changes one repair made.
type volumeRepairResult struct {
	// edgesReleased counts the direct bucket block edges removed.
	edgesReleased uint64
	// nodesSwept counts the orphaned graph nodes removed.
	nodesSwept uint64
	// blocksSwept counts the swept nodes that named a block.
	blocksSwept uint64
	// proofKeysDeleted counts the local completion proof keys removed.
	proofKeysDeleted uint64
	// bytesBefore is the file size before the repair.
	bytesBefore uint64
	// bytesAfter is the file size after the repair and any compaction.
	bytesAfter uint64
}

// newDebugVolumeRepairCommand builds the offline volume repair.
func newDebugVolumeRepairCommand() *cli.Command {
	// Bind the compaction flag.
	var compact bool
	return &cli.Command{
		Name:      "volume-repair",
		Usage:     "release blocks leaked into bucket ownership and optionally compact the file",
		ArgsUsage: "<volume-file>",
		Description: "Rewrites a stopped bbolt volume in place. A bucket that owns named roots " +
			"retains its blocks through them, so its direct block edges are left over from " +
			"interrupted or superseded writes: the repair removes them, sweeps every block no " +
			"longer owned, and deletes the local completion proofs the volume graph replaced. " +
			"With --compact it then copies the volume into a fresh file and swaps it in. " +
			"Stop the daemon and keep a copy (cp -c clones it on APFS) before running it.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "compact",
				Usage:       "copy the repaired volume into a fresh file to return freed pages to the filesystem",
				Destination: &compact,
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected one volume file argument")
			}
			return runDebugVolumeRepair(c.Context, c.Args().First(), compact, c.String("output"))
		},
	}
}

// runDebugVolumeRepair repairs a stopped volume file and prints what changed.
func runDebugVolumeRepair(ctx context.Context, path string, compact bool, outputFormat string) error {
	// Record the file size before any change.
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	res := &volumeRepairResult{bytesBefore: uint64(fi.Size())} //nolint:gosec // Stat sizes are never negative.

	// Refuse a volume another process has open before holding it
	// exclusively, which would otherwise wait for that process to exit.
	probe, err := bbolt.Open(path, 0o400, &bbolt.Options{ReadOnly: true, Exclusive: true, Timeout: volumeOpenTimeout})
	if errors.Is(err, bbolt_errors.ErrTimeout) {
		return errors.Errorf("%s is open in another process; stop the daemon first", path)
	}
	if err != nil {
		return errors.Wrap(err, "open volume")
	}
	if err := probe.Close(); err != nil {
		return err
	}

	// Release the leaked history and its proofs, and compact when asked.
	le := logrus.NewEntry(logrus.New())
	if err := repairVolume(ctx, le, path, compact, fi.Mode().Perm(), res); err != nil {
		return err
	}
	fi, err = os.Stat(path)
	if err != nil {
		return err
	}
	res.bytesAfter = uint64(fi.Size()) //nolint:gosec // Stat sizes are never negative.

	// Emit structured output for scripts.
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var more bool
		writeJSONStringField(ms, &more, "path", path)
		writeJSONUint64Field(ms, &more, "edgesReleased", res.edgesReleased)
		writeJSONUint64Field(ms, &more, "nodesSwept", res.nodesSwept)
		writeJSONUint64Field(ms, &more, "blocksSwept", res.blocksSwept)
		writeJSONUint64Field(ms, &more, "proofKeysDeleted", res.proofKeysDeleted)
		writeJSONUint64Field(ms, &more, "bytesBefore", res.bytesBefore)
		writeJSONUint64Field(ms, &more, "bytesAfter", res.bytesAfter)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	// Print the changes for people.
	writeFields(os.Stdout, [][2]string{
		{"Volume", path},
		{"Bucket Edges Released", strconv.FormatUint(res.edgesReleased, 10)},
		{"Nodes Swept", strconv.FormatUint(res.nodesSwept, 10)},
		{"Blocks Swept", strconv.FormatUint(res.blocksSwept, 10)},
		{"Proof Keys Deleted", strconv.FormatUint(res.proofKeysDeleted, 10)},
		{"File Before", humanize.IBytes(res.bytesBefore)},
		{"File After", humanize.IBytes(res.bytesAfter)},
	})
	return nil
}

// repairVolume holds the volume exclusively, releases its leaked bucket edges,
// sweeps the blocks they held, deletes the obsolete local proofs and, with
// compact, replaces the file with a compacted copy.
func repairVolume(ctx context.Context, le *logrus.Entry, path string, compact bool, mode os.FileMode, res *volumeRepairResult) error {
	// Hold the volume exclusively without creating or rewriting its peer key.
	// A daemon started meanwhile waits for the repair to end.
	vol, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{
		Path:          path,
		NoGenerateKey: true,
		NoWriteKey:    true,
		Exclusive:     true,
	})
	if err != nil {
		return errors.Wrap(err, "open volume")
	}
	defer vol.Close()

	// No process holds a reader lease while the daemon is stopped, so every
	// reader pin is abandoned.
	le.Info("reaping abandoned reader pins")
	if err := vol.ReapRootPins(ctx); err != nil {
		return errors.Wrap(err, "reap reader pins")
	}

	// Remove the direct block edges of each bucket that retains named roots.
	rg := vol.GetRefGraph()
	res.edgesReleased, err = releaseBucketBlockEdges(ctx, le, rg)
	if err != nil {
		return err
	}

	// Sweep the orphans until a pass finds none, since each sweep orphans the
	// children of the nodes it removes.
	for {
		nodes, err := rg.GetUnreferencedNodes(ctx)
		if err != nil {
			return err
		}
		le.Infof("sweeping %d orphaned nodes", len(nodes))
		swept, err := vol.SweepUnreferenced(ctx, rg, nodes)
		for _, node := range swept {
			if _, ok := block_gc.ParseBlockIRI(node); ok {
				res.blocksSwept++
			}
		}
		res.nodesSwept += uint64(len(swept))
		if err != nil {
			return errors.Wrap(err, "sweep orphans")
		}
		if len(swept) == 0 {
			break
		}
	}

	// Delete the local proofs the volume graph replaced.
	db := volume_bolt.GetBoltDB(vol)
	if db == nil {
		return errors.New("volume is not bolt-backed")
	}
	res.proofKeysDeleted, err = deleteLocalProofKeys(le, db)
	if err != nil || !compact {
		return err
	}

	// Compact while the volume is still held, so no process opens the file
	// before the compacted copy replaces it.
	le.Info("compacting volume")
	return compactVolume(db, path, mode)
}

// releaseBucketBlockEdges removes each direct bucket block edge of the buckets
// that own named roots, orphaning the blocks no root still reaches. A bucket
// without named roots owns its blocks directly, so its edges stay.
func releaseBucketBlockEdges(ctx context.Context, le *logrus.Entry, rg block_gc.RefGraphOps) (uint64, error) {
	// List the buckets under the permanent root.
	roots, err := rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		return 0, err
	}

	// Collect the block edges of each bucket with named roots.
	var removes []block_gc.RefEdge
	for _, root := range roots {
		// Skip nodes that are not buckets.
		if _, ok := block_gc.ParseBucketIRI(root); !ok {
			continue
		}
		children, err := rg.GetOutgoingRefs(ctx, root)
		if err != nil {
			return 0, err
		}

		// Keep the edges of a bucket that retains its blocks directly.
		named := slices.ContainsFunc(children, func(child string) bool {
			return strings.HasPrefix(child, volume_kvtx.RootOwnerPrefix)
		})
		if !named {
			continue
		}

		// Release the bucket's direct block edges.
		var released int
		for _, child := range children {
			if _, ok := block_gc.ParseBlockIRI(child); ok {
				removes = append(removes, block_gc.RefEdge{Subject: root, Object: child})
				released++
			}
		}
		le.Infof("releasing %d direct block edges of %s", released, root)
	}

	// Remove the edges in bounded transactions, which marks their orphans.
	for chunk := range slices.Chunk(removes, volumeRepairEdgeBatch) {
		if err := rg.ApplyRefBatch(ctx, nil, chunk); err != nil {
			return 0, errors.Wrap(err, "release bucket edges")
		}
	}
	return uint64(len(removes)), nil
}

// deleteLocalProofKeys deletes the RetainWorld completion proofs kept in object
// stores. A volume that retains roots holds the proofs in its graph and never
// reads these keys; the next retention pass records what it still needs.
func deleteLocalProofKeys(le *logrus.Entry, db *bbolt.DB) (uint64, error) {
	// Match the proof segment within the object store keys.
	conf := kvkey.DefaultConfig()
	prefix := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix())
	segment := []byte("/" + sobject_world_engine.LocalProofKeyPrefix)
	bucketName := []byte("hydra")

	// Delete the matching keys in bounded transactions.
	var deleted uint64
	start := prefix
	for start != nil {
		err := db.Update(func(tx *bbolt.Tx) error {
			// Skip a volume without the store bucket.
			b := tx.Bucket(bucketName)
			if b == nil {
				start = nil
				return nil
			}

			// Collect a bounded batch, then continue after its last key.
			var keys [][]byte
			cur := b.Cursor()
			k, _ := cur.Seek(start)
			for ; k != nil && bytes.HasPrefix(k, prefix) && len(keys) < volumeRepairKeyBatch; k, _ = cur.Next() {
				if bytes.Contains(k, segment) {
					keys = append(keys, bytes.Clone(k))
				}
			}
			start = nil
			if k != nil && bytes.HasPrefix(k, prefix) {
				start = bytes.Clone(k)
			}

			// Delete the batch outside the cursor walk.
			for _, key := range keys {
				if err := b.Delete(key); err != nil {
					return err
				}
			}
			deleted += uint64(len(keys))
			return nil
		})
		if err != nil {
			return deleted, errors.Wrap(err, "delete proof keys")
		}
	}
	le.Infof("deleted %d local proof keys", deleted)
	return deleted, nil
}

// compactVolume copies src, the held volume at path, into a fresh file and
// renames it over path, returning the pages bbolt freed to the filesystem.
// Opens waiting for src follow the rename once src closes.
func compactVolume(src *bbolt.DB, path string, mode os.FileMode) error {
	// Refuse to overwrite the output of an interrupted compaction.
	dstPath := path + ".compact"
	if _, err := os.Stat(dstPath); err == nil {
		return errors.Errorf("%s exists; remove it after checking it is stale", dstPath)
	} else if !os.IsNotExist(err) {
		return err
	}

	// Copy every bucket into the fresh file.
	dst, err := bbolt.Open(dstPath, mode, &bbolt.Options{FreelistType: bbolt.FreelistMapType})
	if err != nil {
		return errors.Wrap(err, "create compacted volume")
	}
	if err := bbolt.Compact(dst, src, volumeCompactTxBytes); err != nil {
		_ = dst.Close()
		_ = os.Remove(dstPath)
		return errors.Wrap(err, "compact volume")
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(dstPath)
		return err
	}

	// Swap the copy in and drop the lock files its open created.
	if err := os.Rename(dstPath, path); err != nil {
		return err
	}
	for _, suffix := range []string{"-lock", "-lock-coord"} {
		if err := os.Remove(dstPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
