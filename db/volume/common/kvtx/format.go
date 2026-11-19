package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"slices"
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
// Format 10 keeps liveness in child lists, named roots, count records with
// zero records for blocks at zero, the ripe queue, and the up and down logs.
// A format 7, 8 or 9 store opens unadopted, as unadoptFormat describes, so
// an adoption counts every holder again before any free. Format 9 kept a
// journal, cascade records and fill records instead of the logs; format 8
// kept a count record only for a block with holders, a queue entry for one
// without, and an index of the journal's up pairs; format 7 kept fill marks
// instead of fill records. An empty store starts adopted, with the frees
// switch as frees sets it. Any other store keeps the child lists format 4
// and later wrote, in place under the liveness prefix, gains the named roots
// its reference graph held, and is left unadopted: it keeps no logs and
// frees nothing until an adoption counts its holders. Every other record of
// the old graph stays until a background sweep deletes it, since nothing
// reads it, so the upgrade at open reads the graph's edges once and writes
// only the roots. A crash at any point leaves a store a rerun upgrades
// again.
func upgradeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, frees bool) (uint64, error) {
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

	// Start an empty store adopted at the current format, with frees as
	// configured: it has no holder to count again.
	empty, err := storeEmpty(ctx, store)
	if err != nil {
		return 0, err
	}
	if empty {
		state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_ADOPTED, Frees: frees}
		return version, writeFormat(ctx, store, keys, state, nil)
	}

	// Leave a format 7, 8 or 9 store unadopted: its counts and queue
	// describe holders under an older protocol, and a format 7 fill wrote a
	// fill mark and gave its block's children no holder, so the adoption
	// counts every list again and clears the old records.
	if version >= 7 {
		return version, unadoptFormat(ctx, store, keys, version)
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
	// its old generation Open: the old graph committed a large edge set in
	// slices, so a crash or a failed slice could leave a list without a child
	// its block names.
	state := &block_gc.LivenessState{
		Adoption:      block_gc.Adoption_ADOPTION_UNADOPTED,
		ObsoleteKinds: legacyGraphKinds,
		Generation:    block_gc.Generation_GENERATION_OPEN,
	}
	return version, writeFormat(ctx, store, keys, state, nil)
}

// unadoptFormat returns a format 7, 8 or 9 store's liveness state to
// unadopted and records the current format. It keeps the generation, so a
// store upgraded mid-sweep finishes its sweep first, the frees switch, and
// the kinds a background sweep still owes, and adds the kinds format 10 no
// longer reads to them, so the sweep deletes them in slices: the old
// reference graph's, since an older format recorded that it remained in a
// field format 10 no longer reads, and block_gc.Format9Kinds. It drops the
// release in progress, the log positions and the fields later formats
// retired, which the adoption's recount replaces.
//
// A format 9 store first has its zero count records carried to format 10's
// encoding in slices, as Records.CarryFormat9Counts describes, and its pop
// mark moved in the final commit, since a Land of the unadopted Volume
// still reads a count to learn whether the free parked its block. Formats 7
// and 8 wrote no record either reads.
//
// It deletes only collector metadata, never a block, a block's bytes or a
// child list: the format 9 kinds go to the background sweep, and the
// adoption clears the counts, queue and logs it replaces.
func unadoptFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, version uint64) error {
	// Carry the zero count records of a format 9 store, in slices.
	if version == 9 {
		var after []byte
		for more := true; more; {
			tx, err := store.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			after, more, err = block_gc.NewRecords(tx, livenessPrefix).CarryFormat9Counts(ctx, after, obsoleteSweepKeys)
			if err == nil {
				err = kvtx.CommitOrdered(ctx, tx)
			}
			tx.Discard()
			if err != nil {
				return err
			}
		}
	}

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

	// Write it back unadopted, with no run, moving a format 9 pop mark.
	obsolete := bytes.Clone(state.GetObsoleteKinds())
	for _, kind := range slices.Concat(legacyGraphKinds, block_gc.Format9Kinds) {
		if bytes.IndexByte(obsolete, kind) < 0 {
			obsolete = append(obsolete, kind)
		}
	}
	next := &block_gc.LivenessState{
		Adoption:      block_gc.Adoption_ADOPTION_UNADOPTED,
		Generation:    state.GetGeneration(),
		Frees:         state.GetFrees(),
		ObsoleteKinds: obsolete,
	}
	return writeFormat(ctx, store, keys, next, func(rec *block_gc.Records) error {
		if version != 9 {
			return nil
		}
		return rec.CarryFormat9PopMark(ctx)
	})
}

// writeFormat writes the liveness state, with what also, nil for nothing,
// writes, and the current format version in one durable commit.
func writeFormat(ctx context.Context, store kvtx.Store, keys *store_kvkey.KVKey, state *block_gc.LivenessState, also func(rec *block_gc.Records) error) error {
	// Open the write transaction.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write the state, the rest and the version, and commit.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	if err := rec.SetState(ctx, state); err != nil {
		return err
	}
	if also != nil {
		if err := also(rec); err != nil {
			return err
		}
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
// owner node to a block. It scans the text node forward edges in runs, each
// read in its own short snapshot, and writes the root owner edges each run
// finds in a commit of their own before reading the next. A rerun writes the
// entries again, so the carry is safe to repeat.
func carryLegacyRoots(ctx context.Context, store kvtx.Store) error {
	prefix := append(bytes.Clone(livenessPrefix), legacyForwardKind, legacyTextNode)
	parse := func(key []byte) (legacyRoot, bool) {
		return parseLegacyRootEdge(key[len(prefix)-1:])
	}
	return scanRuns(ctx, store, prefix, parse, func(roots []legacyRoot) error {
		// Write the roots the run found.
		if len(roots) == 0 {
			return nil
		}
		wtx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()
		rec := block_gc.NewRecords(wtx, livenessPrefix)
		for _, root := range roots {
			if err := rec.SetRoot(ctx, root.bucketID, root.name, &block_gc.RootEntry{Ref: block_gc.BlockKey(root.ref)}); err != nil {
				return err
			}
		}
		return kvtx.CommitOrdered(ctx, wtx)
	})
}

// legacyRoot is a named root an old graph's root owner edge holds.
type legacyRoot struct {
	// bucketID and name name the root.
	bucketID, name string
	// ref is the block it holds.
	ref *block.BlockRef
}

// parseLegacyRootEdge parses the subject and object of a forward edge record
// of the old graph, enc being the record key after its kind byte, and returns
// the root it names when the subject is a root owner node and the object a
// block.
func parseLegacyRootEdge(enc []byte) (legacyRoot, bool) {
	// Read the subject, a text node.
	if len(enc) == 0 || enc[0] != legacyTextNode {
		return legacyRoot{}, false
	}
	n, size := binary.Uvarint(enc[1:])
	if size <= 0 || uint64(len(enc)-1-size) < n { //nolint:gosec // size > 0 and at most len(enc)-1.
		return legacyRoot{}, false
	}
	subject := string(enc[1+size : 1+size+int(n)]) //nolint:gosec // n fits the key.
	object := enc[1+size+int(n):]                  //nolint:gosec // n fits the key.

	// Split the owner node into its bucket and name.
	rest, isOwner := strings.CutPrefix(subject, legacyRootOwnerPrefix)
	encBucket, encName, cut := strings.Cut(rest, "/")
	if !isOwner || !cut {
		return legacyRoot{}, false
	}
	bucket, err := base64.RawURLEncoding.DecodeString(encBucket)
	if err != nil {
		return legacyRoot{}, false
	}
	rootName, err := base64.RawURLEncoding.DecodeString(encName)
	if err != nil {
		return legacyRoot{}, false
	}

	// Read the object, a block.
	ref, tail, err := block_gc.ParseBlockKey(object)
	if err != nil || len(tail) != 0 {
		return legacyRoot{}, false
	}
	return legacyRoot{bucketID: string(bucket), name: string(rootName), ref: ref}, true
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
		n, after, err = deletePrefixBatch(ctx, tx, prefix, after, obsoleteSweepKeys)
		if err == nil && n != 0 {
			err = kvtx.CommitOrdered(ctx, tx)
		}
		tx.Discard()
		if err != nil || n < obsoleteSweepKeys {
			return err
		}
	}
}

// scanRun bounds the keys one snapshot of scanRuns reads.
const scanRun = 4096

// scanRuns calls fn with each run of the keys under prefix, in key order, up
// to scanRun keys a run, each run read in its own short snapshot of store
// after the last key the run before reached. parse names the item a key
// holds; a key it rejects counts toward its run and is left out of it.
func scanRuns[T any](ctx context.Context, store kvtx.Store, prefix []byte, parse func(key []byte) (T, bool), fn func(run []T) error) error {
	var after []byte
	for {
		run, last, err := readRun(ctx, store, prefix, after, parse)
		if err != nil || last == nil {
			return err
		}
		after = last
		if err := fn(run); err != nil {
			return err
		}
	}
}

// readRun reads up to scanRun keys under prefix after the key after, nil for
// the first, in one snapshot, and returns the items parse names among them
// and the last key, nil when none remain.
func readRun[T any](ctx context.Context, store kvtx.Store, prefix, after []byte, parse func(key []byte) (T, bool)) ([]T, []byte, error) {
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

	// Parse the keys of the run.
	var run []T
	var last []byte
	for n := 0; it.Valid() && n < scanRun; it.Next() {
		last = bytes.Clone(it.Key())
		n++
		if item, ok := parse(last); ok {
			run = append(run, item)
		}
	}
	return run, last, it.Err()
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
