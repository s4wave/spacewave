//go:build js

package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/opfs"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
)

// runVolumeRuntimeWrite persists a block and the metadata needed to find it after remount.
func runVolumeRuntimeWrite(ctx context.Context, c *config) error {
	// Store the block durably.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	ref, _, err := vol.PutBlock(ctx, volumeBlockValue(), &block.PutOpts{RefsKnown: true})
	if err != nil {
		return errors.Wrap(err, "put volume block")
	}
	if _, err := vol.Sync(ctx); err != nil {
		return err
	}

	// Store the metadata and the block reference in one transaction.
	refData, err := ref.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal volume block ref")
	}
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open volume write tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, volumeMetaKey(), volumeMetaValue()); err != nil {
		return errors.Wrap(err, "set volume meta")
	}
	if err := tx.Set(ctx, volumeRefKey(), refData); err != nil {
		return errors.Wrap(err, "set volume block ref")
	}

	// Commit and sync the transaction.
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit volume meta")
	}
	_, err = vol.Sync(ctx)
	return err
}

// volumeKVKey returns the benchmark key for one write iteration.
func volumeKVKey(i int) []byte {
	return []byte("bench/kv/" + strconv.Itoa(i))
}

// volumeKVValue returns the benchmark payload. The batch field carries the size
// in bytes.
func volumeKVValue(c *config) []byte {
	return bytes.Repeat([]byte{0x61}, c.batch)
}

// runVolumeKVWritePerOp commits one key per write transaction. opNanos covers
// every iteration including transaction open and commit.
func runVolumeKVWritePerOp(ctx context.Context, c *config) error {
	// Open the volume's key/value store.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()

	// Commit each key in its own transaction, timing the loop.
	start := time.Now()
	for i := range c.iterations {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open kv write tx")
		}
		if err := tx.Set(ctx, volumeKVKey(i), volumeKVValue(c)); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set kv")
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			return errors.Wrap(err, "commit kv tx")
		}
	}
	c.extra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeKVWriteSingleTx puts all values into one write transaction and
// commits once. opNanos covers every set plus the single commit.
func runVolumeKVWriteSingleTx(ctx context.Context, c *config) error {
	// Open one write transaction on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open kv write tx")
	}
	defer tx.Discard()

	// Set every key and commit once, timing both.
	start := time.Now()
	for i := range c.iterations {
		if err := tx.Set(ctx, volumeKVKey(i), volumeKVValue(c)); err != nil {
			return errors.Wrap(err, "set kv")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit kv tx")
	}
	c.extra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeRuntimeVerify checks saved metadata, referenced block content, and storage totals.
func runVolumeRuntimeVerify(ctx context.Context, c *config) error {
	// Open a read transaction on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open volume read tx")
	}
	defer tx.Discard()

	// Check the metadata and read the block reference.
	meta, found, err := tx.Get(ctx, volumeMetaKey())
	if err != nil {
		return errors.Wrap(err, "get volume meta")
	}
	if !found || !bytes.Equal(meta, volumeMetaValue()) {
		return errors.Errorf("volume meta mismatch found=%v value=%q", found, string(meta))
	}
	refData, found, err := tx.Get(ctx, volumeRefKey())
	if err != nil {
		return errors.Wrap(err, "get volume block ref")
	}
	if !found {
		return errors.New("volume block ref missing")
	}

	// Check the referenced block's content.
	ref := &block.BlockRef{}
	if err := ref.UnmarshalVT(refData); err != nil {
		return errors.Wrap(err, "unmarshal volume block ref")
	}
	data, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "get volume block")
	}
	if !found || !bytes.Equal(data, volumeBlockValue()) {
		return errors.Errorf("volume block mismatch found=%v value=%q", found, string(data))
	}

	// Check that the statistics count the block.
	stats, err := vol.GetStorageStats(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume stats")
	}
	if stats.GetBlockCount() != 1 {
		return errors.Errorf("volume block count=%d want=1", stats.GetBlockCount())
	}
	if stats.GetTotalBytes() < uint64(len(data)) {
		return errors.Errorf("volume total bytes=%d want at least %d", stats.GetTotalBytes(), len(data))
	}
	return nil
}

// runVolumeRuntimeDeleteVerify requires explicit Volume.Delete to remove its subtree.
func runVolumeRuntimeDeleteVerify(ctx context.Context, c *config) error {
	// Delete the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	if err := vol.Delete(); err != nil {
		return err
	}

	// Check that its directory is gone.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	_, err = opfs.GetDirectoryPath(root, strings.Split(c.root+"/volume", "/"), false)
	if !opfs.IsNotFound(err) {
		return errors.Errorf("volume root after delete: %v", err)
	}
	return nil
}

// openVolume opens the browser volume under the scenario's isolated root.
func openVolume(ctx context.Context, c *config) (*volume_browser.Volume, error) {
	return volume_browser.NewVolume(ctx, newVolumeConfig(c))
}

// newVolumeConfig sets the volume name from the scenario's isolated root.
func newVolumeConfig(c *config) *volume_browser.Config {
	return &volume_browser.Config{
		Name:        c.root + "/volume",
		StoreConfig: &store_kvtx.Config{},
	}
}

// volumeMetaKey returns the runtime probe's metadata key.
func volumeMetaKey() []byte {
	return []byte("volume/runtime/meta")
}

// volumeMetaValue returns the runtime probe's expected metadata bytes.
func volumeMetaValue() []byte {
	return []byte("volume-runtime-meta-value")
}

// volumeRefKey returns the key holding the runtime probe's serialized block reference.
func volumeRefKey() []byte {
	return []byte("volume/runtime/block-ref")
}

// volumeBlockValue returns the runtime probe's expected block content.
func volumeBlockValue() []byte {
	return []byte("volume-runtime-block-value")
}
