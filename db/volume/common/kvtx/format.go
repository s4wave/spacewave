package kvtx

import (
	"bytes"
	"context"
	"fmt"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// formatMigration upgrades a store to store_kvkey.FormatVersion. It must
// leave the store readable by a rerun after a crash at any point.
type formatMigration func(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error

// formatMigrations upgrade a store from the version at their index directly
// to store_kvkey.FormatVersion, so a store of any age is rewritten once. A new
// format version retargets every entry and adds one for the previous version.
var formatMigrations = [store_kvkey.FormatVersion]formatMigration{
	0: migrateBase58BlockKeys,
}

// migrateBatchBytes bounds the keys and values one migration commit moves.
const migrateBatchBytes = 32 << 20

// binaryBlockRefTag is the first byte of every marshaled BlockRef: the tag of
// its hash field. Base58 text never contains it.
const binaryBlockRefTag = 0x0a

// upgradeFormat brings the store to store_kvkey.FormatVersion. A store at the
// current version costs one read.
func upgradeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error {
	// Read the stored version. A store without one predates versioning.
	version, err := readFormatVersion(ctx, store, keys)
	if err != nil {
		return err
	}
	if version == store_kvkey.FormatVersion {
		return nil
	}
	if version > store_kvkey.FormatVersion {
		return fmt.Errorf("store format version %d is newer than supported version %d", version, store_kvkey.FormatVersion)
	}

	// Migrate, then record the version in a durable commit that also makes
	// the migration's ordered commits durable.
	if err := formatMigrations[version](ctx, store, keys); err != nil {
		return fmt.Errorf("migrate store format version %d to %d: %w", version, store_kvkey.FormatVersion, err)
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// NeedsFormatUpgrade reports whether opening a Volume on store migrates it.
func NeedsFormatUpgrade(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) (bool, error) {
	version, err := readFormatVersion(ctx, store, keys)
	return version != store_kvkey.FormatVersion, err
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
// seeks past them. A batch moves its blocks in one commit, so a rerun resumes
// at the first block still keyed in base58.
func migrateBase58BlockKeys(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error {
	prefix := keys.GetBlockFullPrefix()
	start := append(bytes.Clone(prefix), binaryBlockRefTag+1)
	for {
		more, err := migrateBase58BlockKeyBatch(ctx, store, keys, prefix, start)
		if err != nil || !more {
			return err
		}
	}
}

// blockMove is one block to rekey and its value.
type blockMove struct {
	key, value []byte
}

// migrateBase58BlockKeyBatch moves up to migrateBatchBytes of base58 keyed
// blocks, and reports whether more remain.
func migrateBase58BlockKeyBatch(
	ctx context.Context,
	store kvtx.Store,
	keys *store_kvkey.KVKey,
	prefix, start []byte,
) (bool, error) {
	// Collect the next batch under the writer.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	moves, more, err := collectBlockMoves(ctx, tx, prefix, start)
	if err != nil || len(moves) == 0 {
		return false, err
	}

	// Rekey each block by its decoded ref bytes.
	for _, m := range moves {
		ref, err := b58.Decode(string(m.key[len(prefix):]))
		if err != nil {
			return false, fmt.Errorf("decode block key %q: %w", m.key, err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(ref), m.value); err != nil {
			return false, err
		}
		if err := tx.Delete(ctx, m.key); err != nil {
			return false, err
		}
	}
	return more, kvtx.CommitOrdered(ctx, tx)
}

// collectBlockMoves copies up to migrateBatchBytes of the blocks at or after
// start, and reports whether more remain.
func collectBlockMoves(ctx context.Context, tx kvtx.Tx, prefix, start []byte) ([]blockMove, bool, error) {
	// Position the sorted iterator at the first base58 key.
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(start); err != nil {
		return nil, false, err
	}

	// Copy blocks until the batch fills.
	var moves []blockMove
	var size int
	for ; it.Valid() && size < migrateBatchBytes; it.Next() {
		value, err := it.ValueCopy(nil)
		if err != nil {
			return nil, false, err
		}
		key := bytes.Clone(it.Key())
		moves = append(moves, blockMove{key: key, value: value})
		size += len(key) + len(value)
	}
	return moves, it.Valid(), it.Err()
}
