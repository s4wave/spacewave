package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/s4wave/spacewave/net/hash"
)

// evictEntry is an eviction queue entry: its object store and full key.
type evictEntry struct {
	obj string
	key []byte
}

// facts are the local proofs the drop rule reads. They come from key-only
// scans of the object stores, point reads of the saved read generations, and
// point reads of the pack index for the queued, unmarked blocks; no value of
// the eviction queue is read.
type facts struct {
	marked    map[string]struct{}
	evict     map[string][]evictEntry
	hashTypes map[string]hash.HashType
	hot       map[[readKeyLen]byte]struct{}
	// packed holds the queued, unmarked digests whose h row names a live
	// pack.
	packed    map[string]struct{}
	gens      []string
	objStores map[string]string
	// evictBytesStored is each object store's evictable-bytes as stored.
	evictBytesStored map[string]int64
	evictKeys        int64
	badKeys          int
	// otherFacts counts the marked/ and evictable/ rows of write-back
	// stores other than the one rebuilt, by store id.
	otherFacts map[string]int
	// copies holds each copy to a remote by store and target: true once
	// its copy-done/ row is seen.
	copies          map[[2]string]bool
	packsLive       int
	lookups         int64
	hMissing, hDead int64
	took            time.Duration
}

// subKey is one key name the write-back object store writes under its id,
// with the shape of the key after the name.
type subKey struct {
	name string
	// prefix is set for a name that prefixes keys; otherwise the key is the
	// name exactly.
	prefix bool
	// slashes is the number of '/' after a prefix name, or -1 for any.
	slashes int
}

// subKeys are the key names of db/block/store/writeback, relative to its
// object store:
//
//	marked/<hash>                 queue.go:21 markedPrefix
//	queue/<16 hex>/<hash>         queue.go:27 queuePrefix
//	evictable/<16 hex>/<hash>     evict.go:20 evictPrefix
//	evictable-bytes               evict.go:26 evictableBytesKey
//	epoch                         evict.go:23 epochKey
//	queue-count                   pending.go:42 countKey
//	reads/<16 hex>/<16 hex>       reads.go:19 readsPrefix, reads.go:73-75
//	copy/<target>                 copy.go:22 copyCursorPrefix
//	copy-done/<target>            copy.go:25 copyDonePrefix
//	backfill                      writeback.go:36 backfillKey
//	pending/, target, queue-pending  writeback.go:41 formerPrefixes (legacy)
var subKeys = []subKey{
	{markedSeg, true, 0},
	{"queue/", true, 1},
	{evictSeg, true, 1},
	{readsSeg, true, 1},
	{"copy/", true, -1},
	{"copy-done/", true, -1},
	{"pending/", true, -1},
	{evictBytesSeg, false, 0},
	{"epoch", false, 0},
	{"queue-count", false, 0},
	{"backfill", false, 0},
	{"target", false, 0},
	{"queue-pending", false, 0},
}

// isSubKey reports whether rest is a key the write-back store writes.
func isSubKey(rest string) bool {
	for _, k := range subKeys {
		if !k.prefix {
			if rest == k.name {
				return true
			}
			continue
		}
		if !strings.HasPrefix(rest, k.name) {
			continue
		}
		r := rest[len(k.name):]
		if r == "" || strings.HasPrefix(r, "/") || strings.HasSuffix(r, "/") {
			continue
		}
		if k.slashes < 0 || strings.Count(r, "/") == k.slashes {
			return true
		}
	}
	return false
}

// Object store key split results.
const (
	splitOK = iota
	splitNotObj
	splitUnknown
	splitAmbiguous
	// splitForeign is a key of an object store that is not a write-back
	// store: its id has no writeback segment. It holds no drop fact.
	splitForeign
)

// wbSeg is the last segment of every write-back object store id, as
// core/provider/local/id.go BlockStoreWritebackObjectStoreID builds it.
const wbSeg = "writeback"

// splitObjKey splits an object store key into its store id and the key
// inside it. Write-back store ids hold slashes (p/<provider>/<account>/blk/
// <store>/writeback), so the split is after the key's first writeback
// segment, and the rest must be a key the write-back store writes: a store
// id never holds a writeback segment but its last. A key with no writeback
// segment is splitForeign. A rest that is no such key is splitUnknown, and
// a later writeback segment that also leaves one is splitAmbiguous. Nothing
// is guessed.
func splitObjKey(key []byte) (string, string, int) {
	if !bytes.HasPrefix(key, []byte(objsPrefix)) {
		return "", "", splitNotObj
	}
	s := string(key[len(objsPrefix):])
	first, n := -1, 0
	for i := len(wbSeg); i <= len(s); i++ {
		if (i < len(s) && s[i] != '/') || !isWritebackID(s[:i]) {
			continue
		}
		if first < 0 {
			first = i
		}
		if i < len(s) && isSubKey(s[i+1:]) {
			n++
		}
	}
	switch {
	case first < 0:
		return "", "", splitForeign
	case first == len(s) || !isSubKey(s[first+1:]):
		return "", "", splitUnknown
	case n > 1:
		return "", "", splitAmbiguous
	}
	return s[:first], s[first+1:], splitOK
}

// isWritebackID reports whether id is a write-back object store id: its last
// segment is writeback.
func isWritebackID(id string) bool {
	return id == wbSeg || strings.HasSuffix(id, "/"+wbSeg)
}

// prodAccount is the provider account of production's write-back store:
// its id is p/local/hyperline-backend/blk/<store>/writeback.
const prodAccount = "hyperline-backend"

// prodWritebackID is production's write-back object store id for a block
// store, as core/provider/local/id.go BlockStoreWritebackObjectStoreID
// builds it.
func prodWritebackID(storeID string) string {
	return "p/local/" + prodAccount + "/blk/" + storeID + "/" + wbSeg
}

// writebackID is the write-back object store id the drop rule takes facts
// from.
func (o opts) writebackID() string {
	if o.wbID != "" {
		return o.wbID
	}
	return prodWritebackID(o.storeID)
}

// ownStore reports whether obj is the write-back object store of the block
// store rebuilt: its id exactly. Any other write-back store, of another
// provider, account or block store, is another store's.
func ownStore(o opts, obj string) bool {
	return obj == o.writebackID()
}

// otherFactsErr refuses a World where another write-back store holds a
// marker or an eviction entry: the block table is shared by every store of
// the volume, so the drop rule of one store cannot decide such a block.
func otherFactsErr(f *facts) error {
	if len(f.otherFacts) == 0 {
		return nil
	}
	return fmt.Errorf("write-back stores other than the one rebuilt hold marked/ or evictable/ rows (%v); refusing", f.otherFacts)
}

// noteCopy records a copy cursor or completion row.
func (f *facts) noteCopy(k objKeyInfo) {
	if f.copies == nil {
		f.copies = map[[2]string]bool{}
	}
	id := [2]string{k.obj, k.gen}
	f.copies[id] = f.copies[id] || k.kind == kindCopyDone
}

// unfinishedCopiesErr refuses a World where a write-back copy to a remote
// is under way: a copy/<target> row with no copy-done/<target>. Its blocks
// may not be on the remote yet.
func unfinishedCopiesErr(f *facts) error {
	var open []string
	for id, done := range f.copies {
		if !done {
			open = append(open, id[0]+" copy/"+id[1])
		}
	}
	if len(open) == 0 {
		return nil
	}
	sort.Strings(open)
	return fmt.Errorf("%d write-back copies to a remote are unfinished (copy/ with no copy-done/): %v; refusing", len(open), open)
}

// objKey splits an object store key into its id and the key inside it; ok is
// false for a key outside the object stores or one splitObjKey refuses.
func objKey(key []byte) (string, string, bool) {
	obj, rest, st := splitObjKey(key)
	return obj, rest, st == splitOK
}

// evictSize parses an eviction entry's value, the block size.
func evictSize(v []byte) (int64, bool) {
	n, err := strconv.ParseInt(string(v), 10, 64)
	return n, err == nil && n >= 0
}

// Object store key kinds.
const (
	kindOther = iota
	kindBad
	kindMarked
	kindEvict
	kindReads
	kindBytes
	kindQueue
	// kindForeign is a key of an object store that is not a write-back
	// store; it is kept and holds no drop fact.
	kindForeign
	// kindCopy and kindCopyDone are a copy to a remote's cursor and its
	// completion mark; gen holds the target.
	kindCopy
	kindCopyDone
)

// objKeyInfo is one object store key, parsed.
type objKeyInfo struct {
	ok       bool
	obj      string
	kind     int
	digest   string
	hashType hash.HashType
	gen      string
}

// objKeyOf parses an object store key: its store, and its kind with the
// block digest of a marker or eviction entry, or the generation of a read
// set chunk.
func objKeyOf(key []byte) objKeyInfo {
	obj, rest, st := splitObjKey(key)
	switch st {
	case splitNotObj:
		return objKeyInfo{}
	case splitUnknown, splitAmbiguous:
		return objKeyInfo{ok: true, kind: kindBad}
	case splitForeign:
		return objKeyInfo{ok: true, kind: kindForeign}
	}
	k := objKeyInfo{ok: true, obj: obj}
	switch {
	case strings.HasPrefix(rest, markedSeg):
		d, ok := digestOfB58(rest[len(markedSeg):])
		if !ok {
			k.kind = kindBad
			return k
		}
		k.kind, k.digest = kindMarked, d
	case strings.HasPrefix(rest, evictSeg):
		parts := strings.SplitN(rest[len(evictSeg):], "/", 2)
		if len(parts) != 2 || !isStamp(parts[0]) {
			// The store's parseEvictKey (evict.go:140) refuses an
			// epoch that is not 16 hex digits.
			k.kind = kindBad
			return k
		}
		h := &hash.Hash{}
		if err := h.ParseFromB58(parts[1]); err != nil || len(h.GetHash()) == 0 {
			k.kind = kindBad
			return k
		}
		k.kind, k.digest, k.hashType = kindEvict, string(h.GetHash()), h.GetHashType()
	case strings.HasPrefix(rest, readsSeg):
		// reads/<16 hex generation>/<16 hex save time>, reads.go:73-75.
		gen, at, ok := strings.Cut(rest[len(readsSeg):], "/")
		if !ok || !isStamp(gen) || !isStamp(at) {
			k.kind = kindBad
			return k
		}
		k.kind, k.gen = kindReads, gen
	case rest == evictBytesSeg:
		k.kind = kindBytes
	case strings.HasPrefix(rest, "queue/"):
		k.kind = kindQueue
	case strings.HasPrefix(rest, "copy/"):
		k.kind, k.gen = kindCopy, rest[len("copy/"):]
	case strings.HasPrefix(rest, "copy-done/"):
		k.kind, k.gen = kindCopyDone, rest[len("copy-done/"):]
	}
	return k
}

// isStamp reports whether s is a write-back key's time stamp or epoch: 16
// hex digits, a big-endian uint64.
func isStamp(s string) bool {
	if len(s) != 16 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// loadFacts reads the markers, eviction queue and read generations of every
// object store, and checks each queued, unmarked block against the pack index.
func loadFacts(o opts, world *sql.DB, table string, packs *sql.DB) (*facts, error) {
	start := time.Now()
	f := &facts{
		marked:           map[string]struct{}{},
		evict:            map[string][]evictEntry{},
		hashTypes:        map[string]hash.HashType{},
		hot:              map[[readKeyLen]byte]struct{}{},
		packed:           map[string]struct{}{},
		objStores:        map[string]string{},
		evictBytesStored: map[string]int64{},
		otherFacts:       map[string]int{},
	}

	// Key-only scan of the object stores. Values are read below only for
	// the few read generation chunks and evictable-bytes keys.
	gens := map[string]map[string][][]byte{}
	var bytesKeys [][]byte
	var seen int64
	live.setPhase("object store keys", func() string {
		return fmt.Sprintf("%d keys, %d marked, %d queued", seen, len(f.marked), f.evictKeys)
	})
	// Each key is parsed by objKeyOf, which needs no state, then applied.
	apply := func(key []byte, k objKeyInfo) {
		seen++
		if !k.ok {
			return
		}
		obj := k.obj
		switch k.kind {
		case kindBad:
			f.badKeys++
			return
		case kindForeign:
			return
		case kindCopy, kindCopyDone:
			f.noteCopy(k)
		case kindMarked, kindEvict, kindReads:
			if !ownStore(o, obj) {
				if k.kind != kindReads {
					f.otherFacts[obj]++
				}
				return
			}
		}
		if in, ok := f.objStores[obj]; ok {
			obj = in
		} else {
			f.objStores[obj] = obj
		}
		switch k.kind {
		case kindMarked:
			f.marked[k.digest] = struct{}{}
		case kindEvict:
			f.hashTypes[k.digest] = k.hashType
			f.evict[k.digest] = append(f.evict[k.digest], evictEntry{obj: obj, key: bytes.Clone(key)})
			f.evictKeys++
		case kindReads:
			if gens[obj] == nil {
				gens[obj] = map[string][][]byte{}
			}
			gens[obj][k.gen] = append(gens[obj][k.gen], bytes.Clone(key))
		case kindBytes:
			bytesKeys = append(bytesKeys, bytes.Clone(key))
		}
	}
	var err error
	if o.rawFacts {
		// The offline source's key index, read raw by several readers that
		// parse each key, then apply it through the lock.
		var mu sync.Mutex
		err = rawIndexKeys(o, world, o.world, table, []byte(objsPrefix), func(key []byte) error {
			k := objKeyOf(key)
			mu.Lock()
			apply(key, k)
			mu.Unlock()
			return nil
		})
	} else {
		err = scanKeys(world, table, []byte(objsPrefix), prefixEnd([]byte(objsPrefix)), o.chunk, o.pause, func(key []byte) error {
			apply(key, objKeyOf(key))
			return nil
		})
	}
	if err != nil {
		return nil, fmt.Errorf("scan object store keys: %w", err)
	}
	if f.badKeys != 0 {
		// An unparsed marker or read set key would leave its block
		// droppable: nothing is decided on such a World.
		return nil, fmt.Errorf("%d object store keys under %s did not parse as write-back store keys; refusing", f.badKeys, objsPrefix)
	}
	if err := otherFactsErr(f); err != nil {
		return nil, err
	}
	if err := unfinishedCopiesErr(f); err != nil {
		return nil, err
	}

	// Point reads: the chunks of the two newest read generations of each
	// object store, as readSet.load keeps, and the stored evictable-bytes.
	var readKeys [][]byte
	for obj, g := range gens {
		var starts []string
		for s := range g {
			starts = append(starts, s)
		}
		sort.Strings(starts)
		if len(starts) > 2 {
			starts = starts[len(starts)-2:]
		}
		for _, s := range starts {
			raw, _ := hex.DecodeString(s)
			at := time.Unix(0, int64(binary.BigEndian.Uint64(raw))).UTC()
			f.gens = append(f.gens, fmt.Sprintf("%s gen %s: %d chunks", obj, at.Format(time.RFC3339), len(g[s])))
			readKeys = append(readKeys, g[s]...)
		}
	}
	live.setPhase("read set values", func() string { return fmt.Sprintf("%d hot keys", len(f.hot)) })
	q := "SELECT value FROM " + table + " WHERE key = ?"
	err = pointReads(world, q, len(readKeys), 1, func(i int, st *sql.Stmt) error {
		var v []byte
		if err := st.QueryRow(readKeys[i]).Scan(&v); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Dropped since the key scan; its generation is gone.
				return nil
			}
			return err
		}
		for ; len(v) >= readKeyLen; v = v[readKeyLen:] {
			f.hot[[readKeyLen]byte(v[:readKeyLen])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read set values: %w", err)
	}
	err = pointReads(world, q, len(bytesKeys), 1, func(i int, st *sql.Stmt) error {
		var v []byte
		if err := st.QueryRow(bytesKeys[i]).Scan(&v); err != nil {
			return err
		}
		obj, _, _ := objKey(bytesKeys[i])
		n, _ := evictSize(v)
		f.evictBytesStored[obj] = n
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("evictable-bytes: %w", err)
	}

	// Pack index: live pack numbers from the p/ table (a few thousand
	// rows), then a point read of the h row of each queued, unmarked block,
	// in row key order for locality.
	ptable, _, err := kvTable(packs)
	if err != nil {
		return nil, fmt.Errorf("pack index table: %w", err)
	}
	pre := packIndexPrefix(o.storeID)
	livePacks := map[uint32]struct{}{}
	pp := append(bytes.Clone(pre), "p/"...)
	live.setPhase("pack table", func() string { return fmt.Sprintf("%d packs", len(livePacks)) })
	err = scan(packs, ptable, pp, prefixEnd(pp), o.chunk, o.pause, true, func(_ []byte, _ int64, v []byte) error {
		if len(v) >= 4 {
			livePacks[binary.BigEndian.Uint32(v)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan pack table: %w", err)
	}
	f.packsLive = len(livePacks)

	type lookup struct {
		row    []byte
		digest string
	}
	var looks []lookup
	for d := range f.evict {
		if _, marked := f.marked[d]; marked {
			continue
		}
		rk := packRowKey(d, f.hashTypes[d])
		row := append(append(bytes.Clone(pre), 'h'), rk[:]...)
		looks = append(looks, lookup{row: row, digest: d})
	}
	sort.Slice(looks, func(i, j int) bool { return bytes.Compare(looks[i].row, looks[j].row) < 0 })
	live.setPhase("pack index h rows", func() string {
		return fmt.Sprintf("%d of %d looked up, %d packed", f.lookups, len(looks), len(f.packed))
	})
	getH := func(i int, st *sql.Stmt) ([]byte, error) {
		var v []byte
		err := st.QueryRow(looks[i].row).Scan(&v)
		return v, err
	}
	if o.rawFacts {
		// Read the offline pack index raw, whole, and pick out the h rows
		// looked for: one pass instead of a point read per block.
		vals, err := rawLookups(o, packs, o.packs, ptable, len(looks), func(i int) []byte { return looks[i].row })
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
	err = pointReads(packs, "SELECT value FROM "+ptable+" WHERE key = ?", len(looks), o.chunk, func(i int, st *sql.Stmt) error {
		f.lookups++
		v, err := getH(i, st)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			f.hMissing++
			return nil
		case err != nil:
			return err
		case len(v) < 4:
			f.hMissing++
			return nil
		}
		if _, ok := livePacks[binary.BigEndian.Uint32(v)]; !ok {
			f.hDead++
			return nil
		}
		f.packed[looks[i].digest] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pack index h rows: %w", err)
	}
	f.took = time.Since(start)
	return f, nil
}

// Block verdicts.
const (
	vDrop       = "drop: queued, unmarked, packed, not hot"
	vHot        = "keep: queued and packed, hot by read set"
	vMarked     = "keep: pending upload (marked)"
	vUnqueued   = "keep: not in eviction queue"
	vUnpacked   = "keep: queued, no live pack row"
	vUnparsed   = "keep: block key did not parse"
	vMarkedEvic = "keep: marked and also queued (anomaly)"
)

// verdict applies the drop rule to one block digest.
func (f *facts) verdict(d string) string {
	_, marked := f.marked[d]
	_, queued := f.evict[d]
	switch {
	case marked && queued:
		return vMarkedEvic
	case marked:
		return vMarked
	case !queued:
		return vUnqueued
	}
	if _, ok := f.packed[d]; !ok {
		return vUnpacked
	}
	var hk [readKeyLen]byte
	copy(hk[:], d)
	if _, ok := f.hot[hk]; ok {
		return vHot
	}
	return vDrop
}

// blockVerdict applies the drop rule to a block key.
func (f *facts) blockVerdict(key []byte) string {
	d, ok := digestOfBlockKey(key)
	if !ok {
		return vUnparsed
	}
	return f.verdict(d)
}

// evictDigest returns the digest an eviction entry key names.
func evictDigest(key []byte) (string, bool) {
	_, rest, ok := objKey(key)
	if !ok || !strings.HasPrefix(rest, evictSeg) {
		return "", false
	}
	parts := strings.SplitN(rest[len(evictSeg):], "/", 2)
	if len(parts) != 2 {
		return "", false
	}
	return digestOfB58(parts[1])
}
