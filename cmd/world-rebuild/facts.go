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

// objKey splits an object store key into its id and the key inside it.
func objKey(key []byte) (string, string, bool) {
	if !bytes.HasPrefix(key, []byte(objsPrefix)) {
		return "", "", false
	}
	rest := string(key[len(objsPrefix):])
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// evictSize parses an eviction entry's value, the block size.
func evictSize(v []byte) (int64, bool) {
	n, err := strconv.ParseInt(string(v), 10, 64)
	return n, err == nil && n >= 0
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
	scanObjs := func(fn func(key []byte) error) error {
		return scanKeys(world, table, []byte(objsPrefix), prefixEnd([]byte(objsPrefix)), o.chunk, o.pause, fn)
	}
	if o.rawFacts {
		// The offline source's key index, read raw by several readers,
		// one key at a time through the lock.
		scanObjs = func(fn func(key []byte) error) error {
			var mu sync.Mutex
			return rawIndexKeys(o, world, o.world, table, []byte(objsPrefix), func(key []byte) error {
				mu.Lock()
				defer mu.Unlock()
				return fn(bytes.Clone(key))
			})
		}
	}
	err := scanObjs(func(key []byte) error {
		seen++
		obj, rest, ok := objKey(key)
		if !ok {
			return nil
		}
		if in, ok := f.objStores[obj]; ok {
			obj = in
		} else {
			f.objStores[obj] = obj
		}
		switch {
		case strings.HasPrefix(rest, markedSeg):
			d, ok := digestOfB58(rest[len(markedSeg):])
			if !ok {
				f.badKeys++
				return nil
			}
			f.marked[d] = struct{}{}
		case strings.HasPrefix(rest, evictSeg):
			parts := strings.SplitN(rest[len(evictSeg):], "/", 2)
			if len(parts) != 2 {
				f.badKeys++
				return nil
			}
			h := &hash.Hash{}
			if err := h.ParseFromB58(parts[1]); err != nil || len(h.GetHash()) == 0 {
				f.badKeys++
				return nil
			}
			d := string(h.GetHash())
			f.hashTypes[d] = h.GetHashType()
			f.evict[d] = append(f.evict[d], evictEntry{obj: obj, key: bytes.Clone(key)})
			f.evictKeys++
		case strings.HasPrefix(rest, readsSeg):
			r := rest[len(readsSeg):]
			if len(r) < 16 {
				f.badKeys++
				return nil
			}
			if gens[obj] == nil {
				gens[obj] = map[string][][]byte{}
			}
			gens[obj][r[:16]] = append(gens[obj][r[:16]], bytes.Clone(key))
		case rest == evictBytesSeg:
			bytesKeys = append(bytesKeys, bytes.Clone(key))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan object store keys: %w", err)
	}
	if f.badKeys != 0 {
		fmt.Printf("warning: %d object store keys did not parse; their blocks are kept\n", f.badKeys)
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
