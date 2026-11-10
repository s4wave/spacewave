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
	packsLive        int
	lookups          int64
	hMissing, hDead  int64
	took             time.Duration
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
// <store>/writeback), so the split is at the one '/' that follows a
// writeback segment and after which the rest is a key the write-back store
// writes. No such '/' is splitUnknown if the key has a writeback segment,
// else splitForeign; more than one is splitAmbiguous. Nothing is guessed.
func splitObjKey(key []byte) (string, string, int) {
	if !bytes.HasPrefix(key, []byte(objsPrefix)) {
		return "", "", splitNotObj
	}
	s := string(key[len(objsPrefix):])
	at, n := -1, 0
	for i := len(wbSeg); i < len(s); i++ {
		if s[i] != '/' || !isWritebackID(s[:i]) || !isSubKey(s[i+1:]) {
			continue
		}
		at = i
		n++
	}
	switch {
	case n == 1:
		return s[:at], s[at+1:], splitOK
	case n > 1:
		return "", "", splitAmbiguous
	case s == wbSeg || strings.HasPrefix(s, wbSeg+"/") || strings.Contains(s, "/"+wbSeg+"/") || strings.HasSuffix(s, "/"+wbSeg):
		return "", "", splitUnknown
	}
	return "", "", splitForeign
}

// isWritebackID reports whether id is a write-back object store id: its last
// segment is writeback.
func isWritebackID(id string) bool {
	return id == wbSeg || strings.HasSuffix(id, "/"+wbSeg)
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
		if len(parts) != 2 {
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
		r := rest[len(readsSeg):]
		if len(r) < 16 {
			k.kind = kindBad
			return k
		}
		k.kind, k.gen = kindReads, r[:16]
	case rest == evictBytesSeg:
		k.kind = kindBytes
	case strings.HasPrefix(rest, "queue/"):
		k.kind = kindQueue
	}
	return k
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
