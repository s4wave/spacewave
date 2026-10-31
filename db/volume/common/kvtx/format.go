package kvtx

import (
	"bytes"
	"context"
	"fmt"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
)

// keyFormatVersion is the last format version that changed the key layout.
// Format 6 adds the shared counts and the sharing walk to the graph of format
// 5, which a Volume begins in one commit, as upgradeFormat describes.
const keyFormatVersion = 5

// formatMigration upgrades a store to keyFormatVersion. It must leave the
// store readable by a rerun after a crash at any point.
type formatMigration func(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error

// formatMigrations upgrade a store from the version at their index directly
// to keyFormatVersion, so a store of any age is rewritten once. A new key
// layout retargets every entry and adds one for the previous version.
var formatMigrations = [keyFormatVersion]formatMigration{
	0: migrateBase58Store,
	1: migrateCayleyRefGraph,
	2: thenChildLists(migrateLegacyRefGraph),
	3: migrateChildLists,
	4: migrateYoungLog,
}

// migrateBatchBytes bounds the keys and values one migration commit moves.
const migrateBatchBytes = 32 << 20

// binaryBlockRefTag is the first byte of every marshaled BlockRef: the tag of
// its hash field. Base58 text never contains it.
const binaryBlockRefTag = 0x0a

// upgradeFormat brings the store to store_kvkey.FormatVersion, or to format 5
// when it tracks blocks and shareCounts is unset, and returns the version it
// leaves. A store at that version costs one or two reads. A store newer than
// this binary supports fails with a permanent error, since only a newer binary
// can open it.
//
// Moving to format 6 begins the sharing walk in the commit that records the
// version, or completes the counts at once on a graph that tracks nothing, as
// block_gc.RefGraph.BeginSharing describes. A Volume that tracks blocks moves
// only when asked, because its walk reads every child list and owner edge.
func upgradeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, shareCounts bool) (uint64, error) {
	// A migration's commits are background work, which waits for a
	// checkpoint the store's writes outpaced.
	ctx = block.WithBackground(ctx)

	// Read the stored version. A store without one predates versioning.
	version, err := readFormatVersion(ctx, store, keys)
	if err != nil {
		return 0, err
	}
	if version == store_kvkey.FormatVersion {
		return version, nil
	}
	if version > store_kvkey.FormatVersion {
		return 0, volume.Permanent(fmt.Errorf("store format version %d is newer than supported version %d", version, store_kvkey.FormatVersion))
	}

	// Migrate the key layout; the commit below makes its ordered commits
	// durable.
	if version < keyFormatVersion {
		if err := formatMigrations[version](ctx, store, keys); err != nil {
			return 0, fmt.Errorf("migrate store format version %d to %d: %w", version, keyFormatVersion, err)
		}
	}

	// Choose the version: format 6 when asked or when the graph tracks no
	// block, so no count is missing.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()
	rg := OpenRefGraph(kvtx.NewTxStore(tx))
	next := uint64(keyFormatVersion)
	if !shareCounts {
		tracked, err := rg.TracksBlocks(ctx)
		if err != nil {
			return 0, err
		}
		shareCounts = !tracked
	}
	if shareCounts {
		if _, err := rg.BeginSharing(ctx); err != nil {
			return 0, err
		}
		next = store_kvkey.FormatVersion
	}
	if next == version {
		return version, nil
	}

	// A graph that tracks no block yet is minor-ready from birth, as
	// block_gc.RefGraph.ClaimYoungReady describes. Record the version in a
	// durable commit.
	if _, err := rg.ClaimYoungReady(ctx); err != nil {
		return 0, err
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(next)); err != nil {
		return 0, err
	}
	return next, tx.Commit(ctx)
}

// migrateYoungLog upgrades a store of format version 4, whose keys are those
// of version 5. A build of version 4 may not have logged young records, so
// the store stays not minor-ready until a block pass promotes.
func migrateYoungLog(context.Context, kvtx.Store, *store_kvkey.KVKey) error {
	return nil
}

// NeedsFormatUpgrade reports whether opening a Volume on store, with
// shareCounts as its Config's UpgradeSharedCounts, rewrites its keys or begins
// its sharing walk; a binary that predates the walk must not have it open. An
// empty store's move to format 6 is not counted, since a graph that tracks
// nothing has no count to miss. A newer store is not migrated: opening it
// fails.
func NeedsFormatUpgrade(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, shareCounts bool) (bool, error) {
	version, err := readFormatVersion(ctx, store, keys)
	return version < keyFormatVersion || (shareCounts && version < store_kvkey.FormatVersion), err
}

// readFormatVersion returns the store's format version, or 0 when unset.
func readFormatVersion(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) (uint64, error) {
	// Read the version key in a snapshot.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
	if err != nil || !found {
		return 0, err
	}
	return store_kvkey.ParseFormatVersion(data)
}

// migrateBase58BlockKeys moves each block keyed by the base58 text of its
// marshaled ref to the key of the ref bytes. Every binary key begins with
// binaryBlockRefTag, so binary keys sort before all base58 keys and each batch
// seeks past them, and past the base58 keys earlier batches moved. A batch
// moves its blocks in one commit, so a rerun resumes at the first block still
// keyed in base58.
func migrateBase58BlockKeys(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error {
	prefix := keys.GetBlockFullPrefix()
	for start := append(bytes.Clone(prefix), binaryBlockRefTag+1); start != nil; {
		var err error
		start, err = migrateBase58BlockKeyBatch(ctx, store, keys, prefix, start)
		if err != nil {
			return err
		}
	}
	return nil
}

// keyValue is one key and its value.
type keyValue struct {
	key, value []byte
}

// migrateBase58BlockKeyBatch moves up to migrateBatchBytes of base58 keyed
// blocks at or after start, and returns the key to resume from, or nil when
// none remain.
func migrateBase58BlockKeyBatch(
	ctx context.Context,
	store kvtx.Store,
	keys *store_kvkey.KVKey,
	prefix, start []byte,
) ([]byte, error) {
	// Collect the next batch under the writer.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	moves, more, err := collectMoves(ctx, tx, prefix, start)
	if err != nil || len(moves) == 0 {
		return nil, err
	}

	// Rekey each block by its decoded ref bytes.
	for _, m := range moves {
		ref, err := b58.Decode(string(m.key[len(prefix):]))
		if err != nil {
			return nil, fmt.Errorf("decode block key %q: %w", m.key, err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(ref), m.value); err != nil {
			return nil, err
		}
		if err := tx.Delete(ctx, m.key); err != nil {
			return nil, err
		}
	}
	if err := kvtx.CommitOrdered(ctx, tx); err != nil || !more {
		return nil, err
	}
	return keyAfter(moves[len(moves)-1].key), nil
}

// collectMoves copies up to migrateBatchBytes of the records under prefix at
// or after start, and reports whether more remain.
func collectMoves(ctx context.Context, tx kvtx.Tx, prefix, start []byte) ([]keyValue, bool, error) {
	// Position the sorted iterator at start.
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(start); err != nil {
		return nil, false, err
	}

	// Copy records until the batch fills.
	var moves []keyValue
	var size int
	for ; it.Valid() && size < migrateBatchBytes; it.Next() {
		value, err := it.ValueCopy(nil)
		if err != nil {
			return nil, false, err
		}
		key := bytes.Clone(it.Key())
		moves = append(moves, keyValue{key: key, value: value})
		size += len(key) + len(value)
	}
	return moves, it.Valid(), it.Err()
}
