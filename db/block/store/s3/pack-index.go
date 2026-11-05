//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"golang.org/x/sync/errgroup"
)

// The local KV a PackStore keeps its pack index in holds these rows:
//
//	e/{pack id}   the PackfileEntry of the pack, as the entries/ object holds it
//	p/{pack id}   the pack's number and whether its keys are all in h rows
//	h{digest tail} the number of a pack that holds the block, 4 bytes
//	t/{pack id}   the raw kvfile index tail of the pack, as its reader read it
//	n/next        the lowest pack number not yet given, 4 bytes
//
// Pack numbers grow by one for each pack the index learns and are never reused,
// so an h row of a removed pack names a number no pack holds and reads as a
// miss. A remove records the next number, since the removed packs may have
// held the highest ones and the p/ rows alone would give them out again after
// a restart, pointing their stale h rows at packs that lack the blocks. The number table is the only place the id strings live: a block costs
// its key and 4 bytes, not a 20 byte id.
//
// Invariants:
//   - A pack with an indexed p/ row has every block key in h rows, written in
//     a transaction that committed no later than the row.
//   - An e/ row is written in one transaction with a p/ row of its pack, so
//     neither is written without the other. Marking the pack indexed later
//     rewrites its p/ row alone.
//   - The h row key is the last indexKeyBytes bytes of the block key, the end
//     of its hash digest: 128 bits, so two blocks share a row only by a
//     collision no one will find. It halves the bytes the index costs.
//   - An h row names the number of the last pack that wrote it, so a block
//     written twice is found in one pack, and the lookups fall back to the
//     filters when that pack has left the manifest. The background indexer
//     never replaces a row that names a pack the table holds, so a listed pack
//     indexed while a tail merge replaces it cannot shadow the merged pack.
//   - A t/ row holds the tail bytes of a pack id, which never change, so a
//     reader loads it instead of reading the tail again; removing the pack
//     removes its row.
const (
	indexEntryPrefix = "e/"
	indexPackPrefix  = "p/"
	indexHashPrefix  = "h"
	indexTailPrefix  = "t/"
	indexNextKey     = "n/next"
	// indexKeyBytes is the bytes of a block key an h row key keeps.
	indexKeyBytes = 16

	// indexBatchKeys bounds the keys inspected by one background write transaction.
	indexBatchKeys = 2048
	// indexFlushEntries bounds the entries read from the listing that wait for
	// their write transaction.
	indexFlushEntries = 32
)

// PackIndexPrefix returns the key prefix of the pack index of a block store in
// the account volume. The backend and the offline index build use the same one.
//
// The prefix is "pi" and 8 bytes of the store id's hash, as every row key
// repeats it.
func PackIndexPrefix(blockStoreID string) []byte {
	sum := sha256.Sum256([]byte(blockStoreID))
	return append([]byte("pi"), sum[:8]...)
}

// packRow is the p/ row of a pack.
type packRow struct {
	num     uint32
	indexed bool
}

func (r packRow) marshal() []byte {
	b := binary.BigEndian.AppendUint32(nil, r.num)
	if r.indexed {
		return append(b, 1)
	}
	return append(b, 0)
}

func unmarshalPackRow(b []byte) (packRow, error) {
	if len(b) != 5 {
		return packRow{}, errors.Errorf("pack row of %d bytes", len(b))
	}
	return packRow{num: binary.BigEndian.Uint32(b), indexed: b[4] == 1}, nil
}

// packIndex is the entry cache and block location index of a PackStore in a
// local KV store. The KV store is the durable copy; the pack table is kept in
// memory too, as a read needs it for every lookup.
type packIndex struct {
	store kvtx.Store

	// mtx guards the fields below. It is held for map operations only, never
	// across a KV transaction.
	mtx sync.RWMutex
	// rows is the pack table by pack id.
	rows map[string]packRow
	// ids is the pack table by pack number; "" is a removed pack.
	ids []string
	// pending holds the persisted packs whose keys are not all indexed.
	pending map[string]*packfile.PackfileEntry
	// buffered holds the registered entries waiting for their transaction.
	buffered []*packfile.PackfileEntry
	// work wakes the background indexer when pending grows.
	work chan struct{}
	// version counts the changes to rows, so Indexed answers stay valid
	// while it holds. Changed with mtx held.
	version atomic.Uint64
}

func newPackIndex(store kvtx.Store) *packIndex {
	return &packIndex{
		store:   store,
		rows:    make(map[string]packRow),
		pending: make(map[string]*packfile.PackfileEntry),
		work:    make(chan struct{}, 1),
	}
}

// loadReaders is the number of read transactions that read cached entries at
// once. Each entry is a value of its own in the KV store, one random read,
// so reading many one at a time waits on the disk once per pack.
const loadReaders = 16

// readChunk is the number of entries one read transaction reads in readAll,
// so no snapshot stays open across the whole table.
const readChunk = 1024

// load registers the pack table and reads the entries a lookup cannot place,
// and returns the number of packs the table holds with those entries. Those
// are the unindexed packs, and the indexed packs that share a number with
// another pack: an index written before the table loaded gave out numbers
// that indexed packs held, so an h row of a shared number may name a pack
// that lacks the block. Publishing every pack of a shared number lets the
// filters find the block in whichever pack holds it. The other indexed packs'
// entries stay in the KV store until a lookup places a key in one. A failed
// load registers nothing that a retry would not register again, so the caller
// may call it until it succeeds.
func (x *packIndex) load(ctx context.Context) (int, []*packfile.PackfileEntry, error) {
	// Read the pack table, which names every pack that may have a cached
	// entry, and register it.
	rows, ids, nextNum, err := x.loadTable(ctx)
	if err != nil {
		return 0, nil, err
	}
	x.mtx.Lock()
	for _, id := range ids {
		x.registerRowLocked(id, rows[id])
	}
	for uint32(len(x.ids)) < nextNum { //nolint:gosec // 2^32 packs cannot exist.
		x.ids = append(x.ids, "")
	}
	x.mtx.Unlock()

	// Count the packs holding each number.
	holders := make(map[uint32]int, len(ids))
	for _, id := range ids {
		holders[rows[id].num]++
	}

	// Choose the packs a lookup cannot place: the unindexed packs and the
	// packs sharing a number.
	var unplaced []string
	for _, id := range ids {
		if row := rows[id]; !row.indexed || holders[row.num] > 1 {
			unplaced = append(unplaced, id)
		}
	}

	// Read their entries. A pack removed since the table read has no entry;
	// forget its row.
	read, err := x.readEntries(ctx, unplaced)
	if err != nil {
		return 0, nil, err
	}
	x.forgetMissing(unplaced, read)

	// Register them, queueing the unindexed ones for the indexer.
	for _, entry := range read {
		x.register(entry.GetId(), rows[entry.GetId()], entry)
	}
	return len(ids), read, nil
}

// readEntries reads the cached entries of ids in parallel, each reader in its
// own read transaction, skipping a pack with no entry. The entries keep the
// order of ids.
func (x *packIndex) readEntries(ctx context.Context, ids []string) ([]*packfile.PackfileEntry, error) {
	read := make([]*packfile.PackfileEntry, len(ids))
	var next atomic.Int64
	eg, egCtx := errgroup.WithContext(ctx)
	for range min(loadReaders, len(ids)) {
		eg.Go(func() error {
			// Read this worker's share through one snapshot.
			tx, err := x.store.NewTransaction(egCtx, false)
			if err != nil {
				return err
			}
			defer tx.Discard()
			for i := int(next.Add(1) - 1); i < len(ids); i = int(next.Add(1) - 1) {
				entry, err := getEntry(egCtx, tx, ids[i])
				if err != nil {
					return err
				}
				read[i] = entry
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(read, func(entry *packfile.PackfileEntry) bool { return entry == nil }), nil
}

// getEntry reads the cached entry of pack id, or nil when it has none or it
// does not decode to that pack.
func getEntry(ctx context.Context, tx kvtx.Tx, id string) (*packfile.PackfileEntry, error) {
	value, found, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
	if err != nil || !found {
		return nil, errors.Wrap(err, "read cached entry")
	}
	entry := &packfile.PackfileEntry{}
	if entry.UnmarshalVT(value) != nil || entry.GetId() != id {
		return nil, nil
	}
	return entry, nil
}

// forgetMissing forgets the rows of the packs in ids that read has no entry
// for, which the table named before they were removed.
func (x *packIndex) forgetMissing(ids []string, read []*packfile.PackfileEntry) {
	if len(read) == len(ids) {
		return
	}
	found := make(map[string]bool, len(read))
	for _, entry := range read {
		found[entry.GetId()] = true
	}
	for _, id := range ids {
		if found[id] {
			continue
		}
		x.mtx.RLock()
		row, ok := x.rows[id]
		x.mtx.RUnlock()
		if ok {
			x.forgetRow(id, row)
		}
	}
}

// readAll reads the entry of every pack the table holds that skip does not
// skip, a chunk at a time, and passes each chunk to publish. It forgets the
// rows of the packs with no entry.
func (x *packIndex) readAll(ctx context.Context, skip func(id string) bool, publish func([]*packfile.PackfileEntry)) error {
	var ids []string
	x.eachPack(func(id string) {
		if !skip(id) {
			ids = append(ids, id)
		}
	})
	for chunk := range slices.Chunk(ids, readChunk*loadReaders) {
		read, err := x.readEntries(ctx, chunk)
		if err != nil {
			return err
		}
		x.forgetMissing(chunk, read)
		publish(read)
	}
	return nil
}

// eachPack calls fn with the id of every pack the table holds. fn must not
// call back into the index.
func (x *packIndex) eachPack(fn func(id string)) {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	for id := range x.rows {
		fn(id)
	}
}

// loadTable reads the pack table, the ids of its packs in key order and the
// next pack number the index recorded, 0 when it recorded none. It
// reads only the table, whose rows are a few bytes each, and no entry: listing
// the entries' keys would read every entry's pages, bloom filter included,
// once here and again in load, and load already skips a pack with no entry.
func (x *packIndex) loadTable(ctx context.Context) (map[string]packRow, []string, uint32, error) {
	// Hold one read snapshot while listing the cached pack table.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, 0, err
	}
	defer tx.Discard()

	// Read the next number the removes recorded.
	next, err := readNext(ctx, tx)
	if err != nil {
		return nil, nil, 0, err
	}

	// Read the pack table in key order.
	rows := make(map[string]packRow)
	var ids []string
	err = tx.ScanPrefix(ctx, []byte(indexPackPrefix), func(key, value []byte) error {
		// Decode the pack row and retain its stable identity.
		row, err := unmarshalPackRow(value)
		if err != nil {
			return err
		}
		id := string(key[len(indexPackPrefix):])
		rows[id] = row
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return nil, nil, 0, errors.Wrap(err, "read pack table")
	}
	return rows, ids, next, nil
}

// readNext reads the recorded next pack number, 0 when none is recorded.
func readNext(ctx context.Context, tx kvtx.Tx) (uint32, error) {
	value, ok, err := tx.Get(ctx, []byte(indexNextKey))
	if err != nil || !ok || len(value) != 4 {
		return 0, err
	}
	return binary.BigEndian.Uint32(value), nil
}

// register adds a persisted pack to the table.
func (x *packIndex) register(id string, row packRow, entry *packfile.PackfileEntry) {
	x.mtx.Lock()
	defer x.mtx.Unlock()
	x.registerRowLocked(id, row)
	if !row.indexed {
		x.pending[id] = entry
	}
}

// registerRowLocked adds a pack's row to the table. Call with mtx held.
func (x *packIndex) registerRowLocked(id string, row packRow) {
	x.rows[id] = row
	x.version.Add(1)
	for int(row.num) >= len(x.ids) {
		x.ids = append(x.ids, "")
	}
	x.ids[row.num] = id
}

// forgetRow drops a pack's row from the in-memory table, keeping its number
// taken.
func (x *packIndex) forgetRow(id string, row packRow) {
	// Forget only the exact row this load observed.
	x.mtx.Lock()
	defer x.mtx.Unlock()
	if x.rows[id] != row {
		return
	}
	delete(x.rows, id)
	x.version.Add(1)
	if int(row.num) < len(x.ids) && x.ids[row.num] == id {
		x.ids[row.num] = ""
	}
}

// nextRow numbers a new pack. Call with mtx held.
func (x *packIndex) nextRow(id string, indexed bool) packRow {
	row := packRow{num: uint32(len(x.ids)), indexed: indexed} //nolint:gosec // 2^32 packs cannot exist.
	x.ids = append(x.ids, id)
	x.rows[id] = row
	x.version.Add(1)
	return row
}

// addListed buffers an entry read from the listing, and persists the buffer
// once it holds indexFlushEntries entries. The pack is indexed in the
// background.
func (x *packIndex) addListed(ctx context.Context, entry *packfile.PackfileEntry) error {
	x.mtx.Lock()
	x.buffered = append(x.buffered, entry)
	full := len(x.buffered) >= indexFlushEntries
	x.mtx.Unlock()
	if !full {
		return nil
	}
	return x.flush(ctx)
}

// flush persists the buffered entries as unindexed packs.
func (x *packIndex) flush(ctx context.Context) error {
	x.mtx.Lock()
	buffered := x.buffered
	x.buffered = nil
	x.mtx.Unlock()
	if len(buffered) == 0 {
		return nil
	}

	// Number the packs and write their rows.
	type added struct {
		entry *packfile.PackfileEntry
		row   packRow
	}
	rows := make([]added, 0, len(buffered))
	x.mtx.Lock()
	for _, entry := range buffered {
		id := entry.GetId()
		if _, ok := x.rows[id]; ok {
			continue
		}
		rows = append(rows, added{entry: entry, row: x.nextRow(id, false)})
	}
	x.mtx.Unlock()
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, a := range rows {
		if err := putPackRows(ctx, tx, a.entry, a.row); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Queue the packs for the indexer.
	x.mtx.Lock()
	for _, a := range rows {
		x.pending[a.entry.GetId()] = a.entry
	}
	x.mtx.Unlock()
	x.wake()
	return nil
}

// putPackRows writes the e/ and p/ rows of a pack.
func putPackRows(ctx context.Context, tx kvtx.Tx, entry *packfile.PackfileEntry, row packRow) error {
	data, err := entry.MarshalVT()
	if err != nil {
		return err
	}
	id := entry.GetId()
	if err := tx.Set(ctx, []byte(indexEntryPrefix+id), data); err != nil {
		return err
	}
	return tx.Set(ctx, []byte(indexPackPrefix+id), row.marshal())
}

// addWritten indexes a pack this store wrote, with its entry and every block
// key, in transactions of at most indexBatchKeys keys. Each transaction holds
// the store's writer, so a large pack lets the writers that queued behind one
// slice go before the next, rather than holding them for every key.
//
// The first transaction writes the e/ row and an unindexed p/ row with the
// first slice, so the pack's number is durable before any h row names it: a
// number taken only in memory would be given out again after a restart, and
// h rows left by an interrupted index would place their blocks in a pack that
// lacks them. The last slice marks the row indexed. A pack interrupted
// between them loads as unindexed and the background indexer finishes it.
// The pack is not in the manifest until this returns, so no lookup trusts its
// row early. A pack of at most indexBatchKeys keys takes one transaction.
func (x *packIndex) addWritten(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error {
	// Number the pack, indexed in memory: the manifest does not hold it yet.
	x.mtx.Lock()
	row := x.nextRow(entry.GetId(), true)
	x.mtx.Unlock()

	// Write the slices, the rows with the first and the marker with the last.
	value := binary.BigEndian.AppendUint32(nil, row.num)
	for start := 0; start == 0 || start < len(keys); start += indexBatchKeys {
		end := min(start+indexBatchKeys, len(keys))
		stored := row
		stored.indexed = end == len(keys)
		if err := x.writeSlice(ctx, entry, stored, start == 0, keys[start:end], value); err != nil {
			return err
		}
	}
	return nil
}

// writeSlice writes the h rows of keys naming value, and the pack's row, with
// its entry when first is set, in one transaction.
func (x *packIndex) writeSlice(ctx context.Context, entry *packfile.PackfileEntry, row packRow, first bool, keys [][]byte, value []byte) error {
	// Open the transaction, which queues behind waiting writers.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write the pack's rows: all of them first, the indexed marker last.
	switch {
	case first:
		err = putPackRows(ctx, tx, entry, row)
	case row.indexed:
		err = tx.Set(ctx, []byte(indexPackPrefix+entry.GetId()), row.marshal())
	}
	if err != nil {
		return err
	}

	// Write the slice's h rows and commit.
	for _, key := range keys {
		if err := tx.Set(ctx, hashRowKey(key), value); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// hashRowKey returns the h row key of a block key.
func hashRowKey(key []byte) []byte {
	return append([]byte(indexHashPrefix), key[max(len(key)-indexKeyBytes, 0):]...)
}

// namesLivePack reports whether the h row at rowKey names a pack other than
// num that the table still holds.
func (x *packIndex) namesLivePack(ctx context.Context, tx kvtx.Tx, rowKey []byte, num uint32) (bool, error) {
	// Read the row; a missing row or one of this pack is free to write.
	value, ok, err := tx.Get(ctx, rowKey)
	if err != nil || !ok || len(value) != 4 {
		return false, err
	}
	at := binary.BigEndian.Uint32(value)
	if at == num {
		return false, nil
	}

	// Check the number it names against the table.
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	return int(at) < len(x.ids) && x.ids[at] != "", nil
}

// remove drops packs from the table and the cache. Their h rows stay: no pack
// holds their number again.
func (x *packIndex) remove(ctx context.Context, ids []string) error {
	// Nothing to remove.
	if len(ids) == 0 {
		return nil
	}

	// Forget the packs in memory and note the next number, which no removed
	// pack can reach.
	x.mtx.Lock()
	for _, id := range ids {
		if row, ok := x.rows[id]; ok {
			x.ids[row.num] = ""
		}
		delete(x.rows, id)
		delete(x.pending, id)
	}
	next := uint32(len(x.ids)) //nolint:gosec // 2^32 packs cannot exist.
	x.version.Add(1)
	x.mtx.Unlock()

	// Record the next number, keeping a higher one a concurrent remove wrote.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	stored, err := readNext(ctx, tx)
	if err != nil {
		return err
	}
	if next > stored {
		if err := tx.Set(ctx, []byte(indexNextKey), binary.BigEndian.AppendUint32(nil, next)); err != nil {
			return err
		}
	}

	// Delete the packs' rows.
	for _, id := range ids {
		if err := tx.Delete(ctx, []byte(indexEntryPrefix+id)); err != nil {
			return err
		}
		if err := tx.Delete(ctx, []byte(indexPackPrefix+id)); err != nil {
			return err
		}
		if err := tx.Delete(ctx, []byte(indexTailPrefix+id)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Get returns the stored index tail of a pack, as the packfile store's index
// cache.
func (x *packIndex) Get(ctx context.Context, packID string) ([]byte, bool, error) {
	// Read the row, copying it out of the transaction.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(indexTailPrefix+packID))
	if err != nil || !found {
		return nil, false, err
	}
	return bytes.Clone(data), true, nil
}

// Set stores the index tail of a pack, as the packfile store's index cache.
func (x *packIndex) Set(ctx context.Context, packID string, data []byte) error {
	// Write the row in its own transaction.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(indexTailPrefix+packID), bytes.Clone(data)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// cacheTail stores the index tail of a pack that its writer or the indexer
// already holds, so the pack's first reader skips the remote tail read. The
// row is a cache: a failed write only leaves that read in place. A pack
// removed meanwhile may keep the row; pack ids are never reused, so no reader
// finds it.
func (x *packIndex) cacheTail(ctx context.Context, packID string, tail []byte) {
	_ = x.Set(ctx, packID, tail)
}

// Locate returns the pack that holds a block key. A row that names a removed
// pack reports found with an empty id: the block may sit in an older pack the
// row shadowed, so the lookup must test every filter rather than end.
func (x *packIndex) Locate(ctx context.Context, key []byte) (string, bool, error) {
	// Read the key's h row.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return "", false, err
	}
	defer tx.Discard()
	value, ok, err := tx.Get(ctx, hashRowKey(key))
	if err != nil || !ok || len(value) != 4 {
		return "", false, err
	}

	// Name its pack, or none when the pack was removed.
	num := binary.BigEndian.Uint32(value)
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	if int(num) >= len(x.ids) {
		return "", true, nil
	}
	return x.ids[num], true, nil
}

// locate reads the rows of keys in one snapshot, sets placed for each key
// that has one, and returns the id of the pack each row names: "" for a key
// with no row, a nil key, or a row naming a removed pack.
func (x *packIndex) locate(ctx context.Context, keys [][]byte, placed []bool) ([]string, error) {
	// Read every row in one snapshot.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Name the pack of each key the index places.
	ids := make([]string, len(keys))
	for i, key := range keys {
		if key == nil {
			continue
		}
		value, ok, err := tx.Get(ctx, hashRowKey(key))
		if err != nil {
			return nil, err
		}
		if !ok || len(value) != 4 {
			continue
		}
		placed[i] = true
		num := binary.BigEndian.Uint32(value)
		x.mtx.RLock()
		if int(num) < len(x.ids) {
			ids[i] = x.ids[num]
		}
		x.mtx.RUnlock()
	}
	return ids, nil
}

// locateAll reports for each key whether its row names a pack the table
// holds.
func (x *packIndex) locateAll(ctx context.Context, keys [][]byte) ([]bool, error) {
	ids, err := x.locate(ctx, keys, make([]bool, len(keys)))
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(keys))
	for i, id := range ids {
		out[i] = id != ""
	}
	return out, nil
}

// Indexed reports whether every key of the pack is in h rows.
func (x *packIndex) Indexed(id string) bool {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	return x.rows[id].indexed
}

// Version changes whenever Indexed may answer differently for some pack.
func (x *packIndex) Version() uint64 {
	return x.version.Load()
}

// wake signals the background indexer.
func (x *packIndex) wake() {
	select {
	case x.work <- struct{}{}:
	default:
	}
}

// pendingIDs returns the packs to index.
func (x *packIndex) pendingIDs() []string {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	ids := make([]string, 0, len(x.pending))
	for id := range x.pending {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// pendingEntry returns the entry of a pack to index, or nil once it is done or
// removed.
func (x *packIndex) pendingEntry(id string) *packfile.PackfileEntry {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	return x.pending[id]
}

// writeKeys writes h rows of the pack for the keys scan yields, in
// transactions of indexBatchKeys inspected keys with pause after each, then marks
// the pack indexed. Preserved rows count toward the bound: checking a row still
// holds the transaction. scan may reuse the key slice it passes. It returns the
// keys written, excluding preserved rows.
func (x *packIndex) writeKeys(ctx context.Context, id string, scan func(func(key []byte) error) error, pause func(context.Context) error) (uint64, error) {
	// Snapshot the pack's stable number before opening any KV transaction.
	x.mtx.RLock()
	row, ok := x.rows[id]
	x.mtx.RUnlock()
	if !ok {
		return 0, nil
	}
	value := binary.BigEndian.AppendUint32(nil, row.num)

	// Keep at most one bounded batch open and discard it on every exit.
	var tx kvtx.Tx
	var inTx int
	var total uint64
	defer func() {
		if tx != nil {
			tx.Discard()
		}
	}()
	commit := func() error {
		// Release the completed batch before giving foreground work its turn.
		if tx == nil {
			return nil
		}
		err := tx.Commit(ctx)
		tx.Discard()
		tx, inTx = nil, 0
		if err != nil {
			return err
		}
		return pause(ctx)
	}
	err := scan(func(key []byte) error {
		// Open the batch's transaction.
		if tx == nil {
			var err error
			tx, err = x.store.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
		}

		// Keep a row that names another live pack, which wrote the block
		// later or replaced this one.
		rowKey := hashRowKey(key)
		live, err := x.namesLivePack(ctx, tx, rowKey, row.num)
		if err != nil {
			return err
		}

		// Write only replaceable rows, but yield after every inspected batch.
		if !live {
			if err := tx.Set(ctx, rowKey, value); err != nil {
				return err
			}
			total++
		}
		inTx++
		if inTx >= indexBatchKeys {
			return commit()
		}
		return nil
	})
	if err != nil {
		return total, err
	}
	if err := commit(); err != nil {
		return total, err
	}

	// Open the final marker transaction after every inspected batch is durable.
	tx, err = x.store.NewTransaction(ctx, true)
	if err != nil {
		return total, err
	}
	row.indexed = true

	// A pack removed during the scan needs no indexed marker.
	x.mtx.RLock()
	entry := x.pending[id]
	x.mtx.RUnlock()
	if entry == nil {
		return total, nil
	}

	// Record the completed index and its entry in one durable transaction.
	if err := putPackRows(ctx, tx, entry, row); err != nil {
		return total, err
	}
	if err := tx.Commit(ctx); err != nil {
		return total, err
	}

	// Publish only a pack still in the table and retire its pending work.
	x.mtx.Lock()
	if _, ok := x.rows[id]; ok {
		x.rows[id] = row
		x.version.Add(1)
	}
	delete(x.pending, id)
	x.mtx.Unlock()
	return total, nil
}
