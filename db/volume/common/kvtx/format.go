package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"strings"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
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
// Format 9 keeps liveness in child lists, named roots, count records with
// zero records for blocks at zero, the ripe queue, cascade records, the
// journal and fill records; a format 7 or 8 store opens unadopted, as
// unadoptFormat describes. Format 8 kept a count record only for a block
// with holders, a queue entry for one without, and an index of the journal's
// up pairs; format 7 kept fill marks instead of fill records. An empty store
// starts adopted. Any other store keeps the child lists format 4
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
		return 0, volume.Permanent(errors.Errorf("store format version %d is newer than supported version %d", version, store_kvkey.FormatVersion))
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

	// Leave a format 7 or 8 store unadopted with its records as they are:
	// its counts and queue use the old encoding, and a format 7 fill wrote a
	// fill mark and gave its block's children no holder, so the adoption
	// counts every list again and clears the old records.
	if version == 7 || version == 8 {
		return version, unadoptFormat(ctx, store, keys)
	}

	// Rekey the blocks of a store that keyed them by base58 text, and delete
	// the graph formats 1 and 2 kept outside the liveness prefix.
	if version == 0 {
		if err := migrateBase58BlockKeys(ctx, store, keys); err != nil {
			return 0, errors.Wrap(err, "migrate store format version 0 block keys")
		}
	}
	if version < 3 {
		for _, prefix := range legacyGraphPrefixes {
			if err := deletePrefix(ctx, store, prefix); err != nil {
				return 0, errors.Wrapf(err, "delete the reference graph of store format version %d", version)
			}
		}
	}

	// Carry the named roots of a format 5 or 6 graph into the root table.
	if version >= 5 {
		if err := carryLegacyRoots(ctx, store); err != nil {
			return 0, errors.Wrapf(err, "carry the named roots of store format version %d", version)
		}
	}

	// Record the format, unadopted, with the old graph left for its sweep and
	// its child lists unverified: the old graph committed a large edge set in
	// slices, so a crash or a failed slice could leave a list without a child
	// its block names.
	state := &block_gc.LivenessState{
		Adoption:        block_gc.Adoption_ADOPTION_UNADOPTED,
		LegacyGraph:     true,
		UnverifiedLists: true,
	}
	return version, writeFormat(ctx, store, keys, state)
}

// unadoptFormat returns a format 7 or 8 store's liveness state to
// unadopted, keeping its journal position and its legacy flags, and records
// the current format, in one durable commit. It drops the release in
// progress and the hold mark, which the adoption's recount replaces, and the
// fields format 9 retired.
func unadoptFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey) error {
	// Read the state.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	state, err := block_gc.NewRecords(tx, livenessPrefix).GetState(ctx)
	tx.Discard()
	if err != nil {
		return err
	}

	// Write it back unadopted, with no run.
	return writeFormat(ctx, store, keys, &block_gc.LivenessState{
		Adoption:        block_gc.Adoption_ADOPTION_UNADOPTED,
		NextSeq:         state.GetNextSeq(),
		Cursor:          state.GetCursor(),
		NetOffset:       state.GetNetOffset(),
		LegacyGraph:     state.GetLegacyGraph(),
		UnverifiedLists: state.GetUnverifiedLists(),
	})
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
	// Scan the text node forward edges in runs, each read in its own short
	// snapshot after the last key the run before it reached, and write the
	// root owner edges each run finds before reading the next.
	prefix := append(bytes.Clone(livenessPrefix), legacyForwardKind, legacyTextNode)
	var after []byte
	for {
		// Read the next run of edges.
		roots, last, err := readLegacyRoots(ctx, store, prefix, after, legacyRootRun)
		if err != nil || last == nil {
			return err
		}
		after = last

		// Write the roots it found.
		if len(roots) == 0 {
			continue
		}
		wtx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		rec := block_gc.NewRecords(wtx, livenessPrefix)
		for _, root := range roots {
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
	}
}

// legacyRootRun bounds the forward edges one read of carryLegacyRoots scans.
const legacyRootRun = 4096

// legacyRoot is a named root an old graph's root owner edge holds.
type legacyRoot struct {
	// bucketID and name name the root.
	bucketID, name string
	// ref is the block it holds.
	ref *block.BlockRef
}

// readLegacyRoots reads up to limit forward edge keys under prefix after the
// key after, nil for the first, in one snapshot, and returns the root owner
// edges among them and the last key it read, nil when none remain.
func readLegacyRoots(ctx context.Context, store kvtx.Store, prefix, after []byte, limit int) ([]legacyRoot, []byte, error) {
	// Position a sorted iterator past after.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := seekPastKey(it, after); err != nil {
		return nil, nil, err
	}

	// Collect the root owner edges of the run.
	var roots []legacyRoot
	var last []byte
	for n := 0; it.Valid() && n < limit; it.Next() {
		key := it.Key()
		last = bytes.Clone(key)
		n++
		if bucketID, name, ref, ok := parseLegacyRootEdge(key[len(prefix)-1:]); ok {
			roots = append(roots, legacyRoot{bucketID: bucketID, name: name, ref: ref})
		}
	}
	return roots, last, it.Err()
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
	start := append(bytes.Clone(prefix), binaryBlockRefTag+1)
	return rekeyPrefix(ctx, store, prefix, start, func(key []byte) ([]byte, error) {
		ref, err := b58.Decode(string(key[len(prefix):]))
		if err != nil {
			return nil, errors.Wrapf(err, "decode block key %q", key)
		}
		return keys.GetBlockKey(ref), nil
	})
}

// deletePrefix deletes every key under prefix, a bounded batch per durable
// commit, so a rerun resumes with the keys left.
func deletePrefix(ctx context.Context, store kvtx.Store, prefix []byte) error {
	var after []byte
	for {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		var n int
		n, after, err = deletePrefixBatch(ctx, tx, prefix, after, legacySweepKeys)
		if err == nil && n != 0 {
			err = kvtx.CommitOrdered(ctx, tx)
		}
		tx.Discard()
		if err != nil || n < legacySweepKeys {
			return err
		}
	}
}

// seekPastKey positions it at its first key when after is nil, and otherwise
// at the first key after after.
func seekPastKey(it kvtx.Iterator, after []byte) error {
	if after == nil {
		it.Next()
		return nil
	}
	return it.Seek(keyAfter(after))
}

// keyAfter returns the least key after key.
func keyAfter(key []byte) []byte {
	return append(bytes.Clone(key), 0)
}
