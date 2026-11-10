package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	wb "github.com/s4wave/spacewave/db/block/store/writeback"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/hash"
)

// The drop rule is the next eviction pass of the rebuilt write-back store,
// run offline over its rows: every row it decides by is decoded with the
// function the store or its caller decodes it with, imported, and the drop
// itself is block_store_writeback.EvictDrops. A row of the rebuilt store
// those functions refuse, or one that is no key the store writes, refuses
// the run, as does a pack index row the pack table load refuses or an h row
// of a candidate that does not decode: nothing is dropped and nothing is
// skipped silently. Where the tool cannot run a check the pass runs, it
// keeps more:
//
//   - the remote's packfiles are not read: a block is stored when its h row
//     names a pack the p/ table holds once, as the pack index's locate
//     finds it, and packcheck checks the packs against the bucket;
//   - the read set is the two newest generations as loaded, not turned by
//     the clock, and without the decoded caches' hits, which only keep;
//   - a block in the upload queue (queue/) is kept even without its marker.

// worldKV is the key namespace of the World's block store and object
// stores.
var worldKV = store_kvkey.NewDefaultKVKey()

// evictEntry is one eviction queue entry of the rebuilt store, decoded by
// block_store_writeback.ParseEvictKey.
type evictEntry struct {
	key   []byte
	epoch uint64
	h     *hash.Hash
}

// hashRow is the h row lookup of one drop candidate.
type hashRow struct {
	bk  string
	row []byte
}

// facts are the rebuilt store's rows the drop rule decides by, decoded, and
// the verdicts once decided. Blocks are named by their block key, as the
// block store builds it, so a block row is dropped only when its key is the
// one a tombstone of the entry's hash deletes.
type facts struct {
	// own is the rebuilt store's object store prefix.
	own []byte
	// marked holds the store's marked/ keys, relative to own.
	marked map[string]struct{}
	// markedBlocks holds the block keys of the marked blocks, for the
	// verdict of one with no eviction entry.
	markedBlocks map[string]struct{}
	// evict holds the eviction entries by block key.
	evict     map[string][]evictEntry
	evictKeys int64
	// queued holds the block keys of the upload queue's entries.
	queued map[string]struct{}
	// valueKeys are the keys whose values the rule reads: the read set
	// chunks, the epoch, and evictable-bytes.
	valueKeys [][]byte
	// epoch is the stored eviction epoch, zero without the row.
	epoch  uint64
	chunks []wb.ReadChunk
	// keepRead is the read set the next pass loads.
	keepRead func(*hash.Hash) bool
	gens     []string
	// packNums counts the p/ rows by pack number.
	packNums map[uint32]int
	// hashNum holds the pack number each candidate's h row names.
	hashNum map[string]uint32
	// packed holds the candidates whose h row names a pack the table holds
	// once.
	packed   map[string]struct{}
	verdicts map[string]string
	settled  bool

	// evictBytesStored is each object store's evictable-bytes as stored.
	evictBytesStored map[string]int64
	objStores        map[string]struct{}
	// badKeys counts the rows refused; bad holds the first few reasons.
	badKeys int
	bad     []string
	// otherFacts counts the marked/ and evictable/ rows of write-back
	// stores other than the one rebuilt, by store id.
	otherFacts map[string]int
	// copies holds each copy to a remote by store and target: true once
	// its copy-done/ row is seen.
	copies                   map[[2]string]bool
	packsLive, packsShared   int
	lookups, hMissing, hDead int64
	hShared                  int64
	took                     time.Duration
	walkedN, unwalkedN       int
}

// newFacts returns empty facts for the rebuilt store of o.
func newFacts(o opts) *facts {
	return &facts{
		own:              worldKV.GetObjectStorePrefixByID(o.writebackID()),
		marked:           map[string]struct{}{},
		markedBlocks:     map[string]struct{}{},
		evict:            map[string][]evictEntry{},
		queued:           map[string]struct{}{},
		packNums:         map[uint32]int{},
		hashNum:          map[string]uint32{},
		packed:           map[string]struct{}{},
		verdicts:         map[string]string{},
		evictBytesStored: map[string]int64{},
		objStores:        map[string]struct{}{},
		otherFacts:       map[string]int{},
	}
}

// refuse records a row the rule cannot decide by.
func (f *facts) refuse(format string, a ...any) {
	f.badKeys++
	if len(f.bad) < 10 {
		f.bad = append(f.bad, fmt.Sprintf(format, a...))
	}
}

// refusedErr refuses a World holding a row the rule could not decide by.
func (f *facts) refusedErr() error {
	if f.badKeys == 0 {
		return nil
	}
	return fmt.Errorf("%d rows the drop rule cannot decide by (%s); refusing", f.badKeys, strings.Join(f.bad, "; "))
}

// blockKeyOf returns the block key a tombstone of the block with hash h
// deletes, as the World's block store builds it.
func blockKeyOf(h *hash.Hash) (string, error) {
	if err := h.Validate(); err != nil {
		return "", err
	}
	k, err := block_store_kvtx.BlockKey(worldKV, block.NewBlockRef(h))
	return string(k), err
}

// formerKeys are the write-back store's keys of its layouts before the
// upload queue, which it drops at open and backfills again: blocks they
// name may await upload, so a World holding them is refused.
var formerKeys = []string{"pending/", "target", "queue-pending"}

// addKey takes one object store key, from the scan of every key under
// objsPrefix. It is not safe for concurrent use.
func (f *facts) addKey(key []byte) {
	if bytes.HasPrefix(key, f.own) {
		f.objStores[string(f.own)] = struct{}{}
		f.addOwn(key, key[len(f.own):])
		return
	}
	k := objKeyOf(key)
	switch k.kind {
	case kindBad:
		f.refuse("object store key %q did not split", trunc(key))
	case kindMarked, kindEvict:
		// Any marker or eviction entry of another write-back store refuses
		// the run: the block table is shared, as otherFactsErr says.
		f.otherFacts[k.obj]++
	case kindCopy, kindCopyDone:
		f.noteCopy(k)
	}
}

// addOwn decodes one key of the rebuilt store, rest relative to it.
func (f *facts) addOwn(key, rest []byte) {
	r := string(rest)
	switch {
	case strings.HasPrefix(r, wb.MarkedPrefix):
		// The pass looks markers up by MarkedKey(h): a marker that is no
		// such key of a valid hash is one it would never find.
		h := &hash.Hash{}
		if err := h.ParseFromB58(r[len(wb.MarkedPrefix):]); err != nil {
			f.refuse("marked/ key %q: %v", trunc(key), err)
			return
		}
		bk, err := blockKeyOf(h)
		if err != nil || !bytes.Equal(wb.MarkedKey(h), rest) {
			f.refuse("marked/ key %q is not the marker of its hash", trunc(key))
			return
		}
		f.marked[r] = struct{}{}
		f.markedBlocks[bk] = struct{}{}
	case strings.HasPrefix(r, wb.EvictPrefix):
		epoch, h, err := wb.ParseEvictKey(rest)
		if err != nil {
			f.refuse("evictable/ key %q: %v", trunc(key), err)
			return
		}
		bk, err := blockKeyOf(h)
		if err != nil {
			f.refuse("evictable/ key %q: %v", trunc(key), err)
			return
		}
		f.evict[bk] = append(f.evict[bk], evictEntry{key: bytes.Clone(key), epoch: epoch, h: h})
		f.evictKeys++
	case strings.HasPrefix(r, wb.ReadsPrefix):
		if _, err := wb.ParseReadChunk(rest, nil); err != nil {
			f.refuse("reads/ key %q: %v", trunc(key), err)
			return
		}
		f.valueKeys = append(f.valueKeys, bytes.Clone(key))
	case strings.HasPrefix(r, wb.QueuePrefix):
		_, h, err := wb.ParseQueueKey(rest)
		if err != nil {
			f.refuse("queue/ key %q: %v", trunc(key), err)
			return
		}
		bk, err := blockKeyOf(h)
		if err != nil {
			f.refuse("queue/ key %q: %v", trunc(key), err)
			return
		}
		f.queued[bk] = struct{}{}
	case r == wb.EpochKey, r == wb.EvictableBytesKey:
		f.valueKeys = append(f.valueKeys, bytes.Clone(key))
	case strings.HasPrefix(r, "copy/") && len(r) > len("copy/"):
		f.noteCopy(objKeyInfo{obj: string(f.own), kind: kindCopy, gen: r[len("copy/"):]})
	case strings.HasPrefix(r, "copy-done/") && len(r) > len("copy-done/"):
		f.noteCopy(objKeyInfo{obj: string(f.own), kind: kindCopyDone, gen: r[len("copy-done/"):]})
	case r == "queue-count", r == "backfill":
		// Kept; the pass reads neither.
	default:
		for _, p := range formerKeys {
			if strings.HasPrefix(r, p) {
				f.refuse("key %q of the store's former layout", trunc(key))
				return
			}
		}
		f.refuse("key %q is none the write-back store writes", trunc(key))
	}
}

// addValue takes the value of one of valueKeys; null reports a NULL value,
// and blob whether the value is a blob or NULL.
func (f *facts) addValue(key, value []byte, blob bool) {
	if !blob {
		f.refuse("value of %q is not a blob", trunc(key))
		return
	}
	rest := key[len(f.own):]
	switch r := string(rest); {
	case strings.HasPrefix(r, wb.ReadsPrefix):
		chunk, err := wb.ParseReadChunk(rest, value)
		if err != nil {
			f.refuse("reads/ key %q: %v", trunc(key), err)
			return
		}
		f.chunks = append(f.chunks, chunk)
	case r == wb.EpochKey:
		n, err := wb.ParseEpoch(value)
		if err != nil {
			f.refuse("epoch %q: %v", trunc(value), err)
			return
		}
		f.epoch = n
	case r == wb.EvictableBytesKey:
		n, _ := evictSize(value)
		f.evictBytesStored[string(f.own)] = n
	}
}

// settle loads the read set from the chunks read, as the next pass loads
// it, and returns the h rows of the drop candidates: the blocks with an
// entry the pass walks and no marker, whose drop needs the remote to hold
// them.
func (f *facts) settle() []hashRow {
	wb.SortReadChunks(f.chunks)
	f.keepRead = provider_local.LoadedReadKeep(f.chunks)
	type gen struct {
		chunks, keys int
	}
	gens := map[time.Time]*gen{}
	var order []time.Time
	for _, c := range f.chunks {
		g := gens[c.Generation]
		if g == nil {
			g = &gen{}
			gens[c.Generation] = g
			order = append(order, c.Generation)
		}
		g.chunks++
		g.keys += len(c.Keys) / readKeyLen
	}
	for i, t := range order {
		state := "aged out"
		if i >= len(order)-2 {
			state = "loaded"
		}
		f.gens = append(f.gens, fmt.Sprintf("gen %s: %d chunks, %d keys, %s", t.UTC().Format(time.RFC3339Nano), gens[t].chunks, gens[t].keys, state))
	}

	pass := wb.PassEpoch(f.epoch)
	var rows []hashRow
	for bk, es := range f.evict {
		if !walked(es, pass) {
			continue
		}
		if _, marked := f.marked[string(wb.MarkedKey(es[0].h))]; marked {
			continue
		}
		rows = append(rows, hashRow{bk: bk, row: block_store_s3.HashRowKey(es[0].h)})
	}
	sort.Slice(rows, func(i, j int) bool { return bytes.Compare(rows[i].row, rows[j].row) < 0 })
	f.settled = true
	return rows
}

// walked reports whether the pass at epoch walks any of the entries.
func walked(es []evictEntry, epoch uint64) bool {
	for _, e := range es {
		if wb.EvictWalks(e.epoch, epoch) {
			return true
		}
	}
	return false
}

// addPackRow takes the value of one p/ row of the pack index.
func (f *facts) addPackRow(key, value []byte) {
	num, err := block_store_s3.ParsePackRow(value)
	if err != nil {
		f.refuse("pack index p/ row %q: %v", trunc(key), err)
		return
	}
	f.packNums[num]++
}

// addHashRow takes the h row of a candidate: found is false for none.
func (f *facts) addHashRow(c hashRow, value []byte, found bool) {
	f.lookups++
	if !found {
		f.hMissing++
		return
	}
	num, ok := block_store_s3.HashRowPack(value)
	if !ok {
		f.refuse("pack index h row %x of %d bytes did not decode", c.row, len(value))
		return
	}
	f.hashNum[c.bk] = num
}

// Block verdicts.
const (
	vDrop       = "drop: queued, unmarked, packed, not hot"
	vHot        = "keep: queued and packed, hot by read set"
	vMarked     = "keep: pending upload (marked)"
	vUnqueued   = "keep: not in eviction queue"
	vUnpacked   = "keep: queued, no live pack row"
	vMarkedEvic = "keep: marked and also queued (anomaly)"
	vUnwalked   = "keep: queued at or after the next pass's epoch"
	vQueued     = "keep: queued and packed, in the upload queue"
	vShared     = "keep: queued, h row names a pack number two p/ rows hold"
)

// decide decides every queued block, once the p/ rows and the candidates'
// h rows are in: the drop is block_store_writeback.EvictDrops of whether
// a marker exists, the remote holds the block, and the read set or the
// upload queue keeps it. The block is held: a verdict applies to a block
// row present.
func (f *facts) decide() {
	for _, n := range f.packNums {
		f.packsLive++
		if n > 1 {
			f.packsShared++
		}
	}
	pass := wb.PassEpoch(f.epoch)
	for bk, es := range f.evict {
		h := es[0].h
		if !walked(es, pass) {
			f.unwalkedN++
			f.verdicts[bk] = vUnwalked
			continue
		}
		f.walkedN++
		_, marked := f.marked[string(wb.MarkedKey(h))]
		stored, shared := false, false
		if num, ok := f.hashNum[bk]; ok {
			switch f.packNums[num] {
			case 0:
				f.hDead++
			case 1:
				stored = true
				f.packed[bk] = struct{}{}
			default:
				shared = true
				f.hShared++
			}
		}
		hot := f.keepRead(h)
		_, queued := f.queued[bk]
		keep := func() bool { return hot || queued }
		v := vDrop
		switch {
		case wb.EvictDrops(true, marked, stored, keep):
		case marked:
			v = vMarkedEvic
		case shared:
			v = vShared
		case !stored:
			v = vUnpacked
		case hot:
			v = vHot
		default:
			v = vQueued
		}
		f.verdicts[bk] = v
	}
}

// verdict applies the drop rule to a block row's key.
func (f *facts) verdict(key []byte) string {
	if v, ok := f.verdicts[string(key)]; ok {
		return v
	}
	if _, ok := f.markedBlocks[string(key)]; ok {
		return vMarked
	}
	return vUnqueued
}

// drops reports whether the rule drops the block row with key.
func (f *facts) drops(key []byte) bool {
	return f.verdicts[string(key)] == vDrop
}

// factsErr refuses a World the rule cannot decide.
func factsErr(f *facts) error {
	if err := f.refusedErr(); err != nil {
		return err
	}
	if err := otherFactsErr(f); err != nil {
		return err
	}
	return unfinishedCopiesErr(f)
}

// loadFacts reads the object store keys of the World, the values of the
// rebuilt store's read set, epoch and evictable-bytes, the pack table, and
// the h rows of the drop candidates, and decides the rule.
func loadFacts(o opts, world *sql.DB, table string, packs *sql.DB) (*facts, error) {
	start := time.Now()
	f := newFacts(o)

	// Key-only scan of the object stores.
	var seen int64
	live.setPhase("object store keys", func() string {
		return fmt.Sprintf("%d keys, %d marked, %d queued", seen, len(f.marked), f.evictKeys)
	})
	var err error
	if o.rawFacts {
		// The offline source's key index, read raw by several readers;
		// each key is taken through the lock.
		var mu sync.Mutex
		err = rawIndexKeys(o, world, o.world, table, []byte(objsPrefix), func(key []byte) error {
			mu.Lock()
			defer mu.Unlock()
			seen++
			f.addKey(key)
			return nil
		})
	} else {
		err = scanKeys(world, table, []byte(objsPrefix), prefixEnd([]byte(objsPrefix)), o.chunk, o.pause, func(key []byte) error {
			seen++
			f.addKey(key)
			return nil
		})
	}
	if err != nil {
		return nil, fmt.Errorf("scan object store keys: %w", err)
	}
	if err := factsErr(f); err != nil {
		return nil, err
	}

	// Every value of the rebuilt store is a blob or NULL, as the store
	// writes them: typeof reads the record header, not the value.
	var notBlob int64
	q0 := "SELECT count(*) FROM " + table + " WHERE key >= ? AND key < ? AND typeof(value) NOT IN ('blob', 'null')"
	if err := world.QueryRow(q0, f.own, prefixEnd(f.own)).Scan(&notBlob); err != nil {
		return nil, fmt.Errorf("rebuilt store value types: %w", err)
	}
	if notBlob > 0 {
		f.refuse("%d values of the rebuilt store are not blobs", notBlob)
		return nil, f.refusedErr()
	}

	// Point reads of the values the rule reads. A key gone since the scan
	// is refused: the source has no writer.
	live.setPhase("rule values", func() string { return fmt.Sprintf("%d values", len(f.valueKeys)) })
	q := "SELECT value, typeof(value) FROM " + table + " WHERE key = ?"
	err = pointReads(world, q, len(f.valueKeys), 1, func(i int, st *sql.Stmt) error {
		key := f.valueKeys[i]
		var v []byte
		var ty string
		if err := st.QueryRow(key).Scan(&v, &ty); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				f.refuse("%q vanished since the key scan", trunc(key))
				return nil
			}
			return err
		}
		f.addValue(key, v, ty == "blob" || ty == "null")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rule values: %w", err)
	}
	if err := f.refusedErr(); err != nil {
		return nil, err
	}
	rows := f.settle()

	// The pack table, then the h row of each candidate, in row key order
	// for locality.
	ptable, _, err := kvTable(packs)
	if err != nil {
		return nil, fmt.Errorf("pack index table: %w", err)
	}
	pre := block_store_s3.PackIndexPrefix(o.storeID)
	pp := append(bytes.Clone(pre), block_store_s3.PackRowPrefix...)
	live.setPhase("pack table", func() string { return fmt.Sprintf("%d packs", len(f.packNums)) })
	err = scan(packs, ptable, pp, prefixEnd(pp), o.chunk, o.pause, true, func(key []byte, _ int64, v []byte) error {
		f.addPackRow(key, v)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan pack table: %w", err)
	}
	full := func(i int) []byte { return append(bytes.Clone(pre), rows[i].row...) }
	live.setPhase("pack index h rows", func() string {
		return fmt.Sprintf("%d of %d looked up", f.lookups, len(rows))
	})
	getH := func(i int, st *sql.Stmt) ([]byte, error) {
		var v []byte
		err := st.QueryRow(full(i)).Scan(&v)
		return v, err
	}
	if o.rawFacts {
		// Read the offline pack index raw, whole, and pick out the h rows
		// looked for: one pass instead of a point read per block.
		vals, err := rawLookups(o, packs, o.packs, ptable, len(rows), full)
		if err != nil {
			return nil, fmt.Errorf("pack index h rows: %w", err)
		}
		getH = func(i int, _ *sql.Stmt) ([]byte, error) {
			if vals[i] == nil {
				return nil, sql.ErrNoRows
			}
			return vals[i], nil
		}
	}
	err = pointReads(packs, "SELECT value FROM "+ptable+" WHERE key = ?", len(rows), o.chunk, func(i int, st *sql.Stmt) error {
		v, err := getH(i, st)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			f.addHashRow(rows[i], nil, false)
		case err != nil:
			return err
		default:
			f.addHashRow(rows[i], v, true)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pack index h rows: %w", err)
	}
	if err := f.refusedErr(); err != nil {
		return nil, err
	}
	f.decide()
	f.took = time.Since(start)
	return f, nil
}
