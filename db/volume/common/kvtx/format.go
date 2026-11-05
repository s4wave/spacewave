package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
)

// migrateBatchBytes bounds the keys and values one migration commit moves.
const migrateBatchBytes = 32 << 20

// binaryBlockRefTag is the first byte of every marshaled BlockRef: the tag of
// its hash field. Base58 text never contains it.
const binaryBlockRefTag = 0x0a

// legacyGraphPrefixes hold the reference graphs of store formats 1 and 2:
// format 1's Cayley quad store and format 2's edge records.
var legacyGraphPrefixes = [][]byte{[]byte("gc/"), []byte("refs/")}

// legacyRootOwnerPrefix begins the owner node of a named root in the
// reference graph of store formats 5 and 6, followed by the base64 of its
// bucket, a slash and the base64 of its name.
const legacyRootOwnerPrefix = "head:"

// legacyForwardKind and legacyTextNode are the record kind of a forward edge
// in the reference graph of store formats 3 to 6, and the byte that begins
// the encoding of a node that is not a block.
const (
	legacyForwardKind byte = 'f'
	legacyTextNode    byte = 0
)

// upgradeFormat brings the store to store_kvkey.FormatVersion and returns the
// version it found. A store at that version costs one read. A store newer than
// this binary supports fails with a permanent error, since only a newer binary
// can open it.
//
// Format 7 keeps liveness in child lists, named roots and count records. An
// empty store starts adopted. Any other store keeps the child lists format 4
// and later wrote, in place under the liveness prefix, gains the named roots
// its reference graph held, and is left unadopted: it keeps no journal and
// frees nothing until an adoption counts its holders. Every other record of
// the old graph stays until a background sweep deletes it, since nothing reads
// it, so the upgrade at open reads the graph's edges once and writes only the
// roots. A crash at any point leaves a store a rerun upgrades again.
func upgradeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) (uint64, error) {
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

	// Start an empty store adopted at the current format.
	empty, err := storeEmpty(ctx, store)
	if err != nil {
		return 0, err
	}
	if empty {
		state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_ADOPTED}
		return version, writeFormat(ctx, store, keys, state)
	}

	// Rekey the blocks of a store that keyed them by base58 text, and delete
	// the graph formats 1 and 2 kept outside the liveness prefix.
	if version == 0 {
		if err := migrateBase58BlockKeys(ctx, store, keys); err != nil {
			return 0, fmt.Errorf("migrate store format version 0 block keys: %w", err)
		}
	}
	if version < 3 {
		for _, prefix := range legacyGraphPrefixes {
			if err := deletePrefix(ctx, store, prefix); err != nil {
				return 0, fmt.Errorf("delete the reference graph of store format version %d: %w", version, err)
			}
		}
	}

	// Carry the named roots of a format 5 or 6 graph into the root table.
	if version >= 5 {
		if err := carryLegacyRoots(ctx, store); err != nil {
			return 0, fmt.Errorf("carry the named roots of store format version %d: %w", version, err)
		}
	}

	// Record the format, unadopted, with the old graph left for its sweep.
	state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, LegacyGraph: true}
	return version, writeFormat(ctx, store, keys, state)
}

// writeFormat writes the liveness state and the current format version in one
// durable commit.
func writeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, state *block_gc.LivenessState) error {
	// Open the write transaction.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write the state and the version, and commit.
	if err := block_gc.NewRecords(tx, livenessPrefix).SetState(ctx, state); err != nil {
		return err
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// storeEmpty reports whether store holds no key at all.
func storeEmpty(ctx context.Context, store kvtx.Store) (bool, error) {
	// Open a snapshot.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	// Look for any key.
	it := tx.Iterate(ctx, nil, false, false)
	defer it.Close()
	return !it.Next(), it.Err()
}

// carryLegacyRoots writes a root table entry for each named root the
// reference graph of a format 5 or 6 store held: a forward edge from a root
// owner node to a block. Each batch of entries commits on its own and is
// written again by a rerun, so the carry is safe to repeat.
func carryLegacyRoots(ctx context.Context, store kvtx.Store) error {
	// Read the root owner edges in one snapshot.
	type legacyRoot struct {
		bucketID, name string
		ref            *block.BlockRef
	}
	var roots []legacyRoot
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	prefix := append(bytes.Clone(livenessPrefix), legacyForwardKind, legacyTextNode)
	err = tx.ScanPrefix(ctx, prefix, func(key, _ []byte) error {
		bucketID, name, ref, ok := parseLegacyRootEdge(key[len(prefix)-1:])
		if ok {
			roots = append(roots, legacyRoot{bucketID: bucketID, name: name, ref: ref})
		}
		return nil
	})
	tx.Discard()
	if err != nil {
		return err
	}

	// Write them in bounded batches.
	for len(roots) != 0 {
		n := min(len(roots), 4096)
		wtx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		rec := block_gc.NewRecords(wtx, livenessPrefix)
		for _, root := range roots[:n] {
			if err := rec.SetRoot(ctx, root.bucketID, root.name, &block_gc.RootEntry{Ref: block_gc.BlockKey(root.ref)}); err != nil {
				wtx.Discard()
				return err
			}
		}
		err = kvtx.CommitOrdered(ctx, wtx)
		wtx.Discard()
		if err != nil {
			return err
		}
		roots = roots[n:]
	}
	return nil
}

// parseLegacyRootEdge parses the subject and object of a forward edge record
// of the old graph, enc being the record key after its kind byte, and returns
// the root it names when the subject is a root owner node and the object a
// block.
func parseLegacyRootEdge(enc []byte) (bucketID, name string, ref *block.BlockRef, ok bool) {
	// Read the subject, a text node.
	if len(enc) == 0 || enc[0] != legacyTextNode {
		return "", "", nil, false
	}
	n, size := binary.Uvarint(enc[1:])
	if size <= 0 || uint64(len(enc)-1-size) < n { //nolint:gosec // size > 0 and at most len(enc)-1.
		return "", "", nil, false
	}
	subject := string(enc[1+size : 1+size+int(n)]) //nolint:gosec // n fits the key.
	object := enc[1+size+int(n):]                  //nolint:gosec // n fits the key.

	// Split the owner node into its bucket and name.
	rest, isOwner := strings.CutPrefix(subject, legacyRootOwnerPrefix)
	encBucket, encName, cut := strings.Cut(rest, "/")
	if !isOwner || !cut {
		return "", "", nil, false
	}
	bucket, err := base64.RawURLEncoding.DecodeString(encBucket)
	if err != nil {
		return "", "", nil, false
	}
	rootName, err := base64.RawURLEncoding.DecodeString(encName)
	if err != nil {
		return "", "", nil, false
	}

	// Read the object, a block.
	ref, tail, err := block_gc.ParseBlockKey(object)
	if err != nil || len(tail) != 0 {
		return "", "", nil, false
	}
	return string(bucket), string(rootName), ref, true
}

// NeedsFormatUpgrade reports whether opening a Volume on store rewrites its
// format, which a binary that predates format 7 must not have open. A newer
// store is not migrated: opening it fails.
func NeedsFormatUpgrade(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) (bool, error) {
	version, err := readFormatVersion(ctx, store, keys)
	return version < store_kvkey.FormatVersion, err
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

// deletePrefix deletes every key under prefix, a bounded batch per durable
// commit, so a rerun resumes with the keys left.
func deletePrefix(ctx context.Context, store kvtx.Store, prefix []byte) error {
	for {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		n, err := deletePrefixBatch(ctx, tx, prefix, legacySweepKeys)
		if err == nil && n != 0 {
			err = kvtx.CommitOrdered(ctx, tx)
		}
		tx.Discard()
		if err != nil || n < legacySweepKeys {
			return err
		}
	}
}

// keyAfter returns the least key after key.
func keyAfter(key []byte) []byte {
	return append(bytes.Clone(key), 0)
}
