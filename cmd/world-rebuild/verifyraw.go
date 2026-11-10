package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"slices"
	"strconv"
	"sync"
	"time"
)

// rowSet is an order-free digest of a set of rows: the count, and two sums
// of a seeded 64-bit hash of each row, so the readers can add in any order.
type rowSet struct {
	n    int64
	a, b uint64
}

// rowHasher hashes rows for a rowSet; one per goroutine.
type rowHasher struct{ a, b maphash.Hash }

func newRowHasher(sa, sb maphash.Seed) *rowHasher {
	h := &rowHasher{}
	h.a.SetSeed(sa)
	h.b.SetSeed(sb)
	return h
}

// Value type tags, hashed with each row so a NULL value and an empty blob,
// equal as bytes, differ. scanTable gives a NULL value as nil and any other
// as a non-nil slice.
var (
	tagNull = []byte{'n'}
	tagBlob = []byte{'b'}
)

// valueTag returns the type tag of a value scanTable gave.
func valueTag(v []byte) []byte {
	if v == nil {
		return tagNull
	}
	return tagBlob
}

// add adds the row of the fields to s, each length-prefixed.
func (h *rowHasher) add(s *rowSet, fields ...[]byte) {
	h.a.Reset()
	h.b.Reset()
	var l [8]byte
	for _, f := range fields {
		binary.LittleEndian.PutUint64(l[:], uint64(len(f)))
		_, _ = h.a.Write(l[:])
		_, _ = h.a.Write(f)
		_, _ = h.b.Write(l[:])
		_, _ = h.b.Write(f)
	}
	s.n++
	s.a += h.a.Sum64()
	s.b += h.b.Sum64()
}

func (s *rowSet) merge(t rowSet) { s.n += t.n; s.a += t.a; s.b += t.b }

// rawTable opens path raw and walks its kv table's b-tree.
func rawTable(db *sql.DB, path string, readers int) (*rawFile, []uint64, error) {
	table, stmt, err := kvTable(db)
	if err != nil {
		return nil, nil, err
	}
	root, err := rawSource(db, path, table, stmt)
	if err != nil {
		return nil, nil, err
	}
	r, err := openRaw(path)
	if err != nil {
		return nil, nil, err
	}
	leaves, _, err := r.tableLeaves(root, readers)
	if err != nil {
		r.Close()
		return nil, nil, fmt.Errorf("%s: walk the table b-tree: %w", path, err)
	}
	return r, leaves, nil
}

// runVerifyRaw is verify read raw: it reads the source and the copy page by
// page, both at once, each with o.readers goroutines, and compares the set
// of rows the drop rule keeps from the source with the set the copy holds,
// as order-free digests of every key and whole value. evictable-bytes is
// checked against the sum of the copy's eviction entries. Last it reads the
// copy's key index through SQLite in key order and checks it holds each
// table row's key and rowid once, so a lookup by key finds every row.
// A fault names no key: run verify -read sqlite for that.
func runVerifyRaw(o opts) error {
	start := time.Now()
	src, err := openRO(o.world)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := openRO(o.out)
	if err != nil {
		return err
	}
	defer dst.Close()
	packs, err := openRO(o.packs)
	if err != nil {
		return err
	}
	defer packs.Close()
	table, stmt, err := kvTable(src)
	if err != nil {
		return err
	}
	dtable, _, err := kvTable(dst)
	if err != nil {
		return err
	}
	o.rawFacts = true
	f, err := loadFacts(o, src, table, packs)
	if err != nil {
		return err
	}
	present, err := readBlockDigests(o, src, o.world, table, stmt)
	if err != nil {
		return err
	}
	p := planDrops(f, present)
	var av int64
	if err := dst.QueryRow("PRAGMA auto_vacuum").Scan(&av); err != nil {
		return err
	}
	readers := max(o.readers, 1)
	sr, sleaves, err := rawTable(src, o.world, readers)
	if err != nil {
		return err
	}
	defer sr.Close()
	dr, dleaves, err := rawTable(dst, o.out, readers)
	if err != nil {
		return err
	}
	defer dr.Close()
	fmt.Printf("verify raw: facts, block keys and b-trees read in %s\n", time.Since(start).Round(time.Second))

	// Read both files at once.
	sa, sb := maphash.MakeSeed(), maphash.MakeSeed()
	var mu sync.Mutex
	var want, got, gotIdx rowSet
	var dropped int64
	wantBytes := map[string]bool{}
	gotBytes := map[string][]byte{}
	sums := evictSums{}
	var srcErr, dstErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srcErr = sr.scanTable(sleaves, readers, nil, nil, func(int) (rawVisit, func() error) {
			h := newRowHasher(sa, sb)
			var s rowSet
			var drop int64
			return func(_ int64, key []byte, _ int64, value func() ([]byte, error)) error {
					// The rule, computed again: a dropped block, or an
					// eviction entry of one.
					if bytes.HasPrefix(key, []byte(blockPrefix)) {
						if d, ok := digestOfBlockKey(key); ok && f.verdict(d) == vDrop {
							drop++
							return nil
						}
					} else if _, skip := p.skipEvict[string(key)]; skip {
						drop++
						return nil
					}
					if obj, rest, ok := objKey(key); ok && rest == evictBytesSeg {
						mu.Lock()
						wantBytes[obj] = true
						mu.Unlock()
						return nil
					}
					v, err := value()
					if err != nil {
						return fmt.Errorf("source value of %q: %w", trunc(key), err)
					}
					h.add(&s, key, v, valueTag(v))
					return nil
				}, func() error {
					mu.Lock()
					want.merge(s)
					dropped += drop
					mu.Unlock()
					return nil
				}
		})
	}()
	go func() {
		defer wg.Done()
		dstErr = dr.scanTable(dleaves, readers, nil, nil, func(int) (rawVisit, func() error) {
			h := newRowHasher(sa, sb)
			var s, idx rowSet
			ws := evictSums{}
			var rid [8]byte
			return func(rowid int64, key []byte, _ int64, value func() ([]byte, error)) error {
					binary.BigEndian.PutUint64(rid[:], uint64(rowid))
					h.add(&idx, key, rid[:])
					v, err := value()
					if err != nil {
						return fmt.Errorf("copy value of %q: %w", trunc(key), err)
					}
					if obj, rest, ok := objKey(key); ok && rest == evictBytesSeg {
						mu.Lock()
						gotBytes[obj] = v
						mu.Unlock()
						return nil
					}
					ws.add(key, v)
					h.add(&s, key, v, valueTag(v))
					return nil
				}, func() error {
					mu.Lock()
					got.merge(s)
					gotIdx.merge(idx)
					for k, n := range ws {
						sums[k] += n
					}
					mu.Unlock()
					return nil
				}
		})
	}()
	wg.Wait()
	if srcErr != nil {
		return fmt.Errorf("source: %w", srcErr)
	}
	if dstErr != nil {
		return fmt.Errorf("copy: %w", dstErr)
	}
	scanned := time.Since(start)

	// Compare.
	var faults []string
	fault := func(s string, a ...any) { faults = append(faults, fmt.Sprintf(s, a...)) }
	if av != 2 {
		fault("auto_vacuum is %d, want 2", av)
	}
	if got.n != want.n {
		fault("copy holds %d rows besides evictable-bytes, the rule keeps %d of the source", got.n, want.n)
	} else if got != want {
		fault("copy's %d rows differ from the %d the rule keeps (row digests differ)", got.n, want.n)
	}
	for _, obj := range sortedKeys(wantBytes) {
		v, ok := gotBytes[obj]
		if !ok {
			fault("%s evictable-bytes missing from the copy", obj)
			continue
		}
		if w := strconv.FormatInt(sums[obj], 10); string(v) != w {
			fault("%s evictable-bytes is %q, the kept entries sum to %s", obj, v, w)
		}
	}
	for obj := range gotBytes {
		if !wantBytes[obj] {
			fault("%s evictable-bytes in the copy, not the source", obj)
		}
	}

	// The key index holds each table row's key and rowid, in strictly
	// increasing key order.
	var idx rowSet
	h := newRowHasher(sa, sb)
	rows, err := dst.Query("SELECT key, rowid FROM " + dtable + " INDEXED BY sqlite_autoindex_" + dtable + "_1 ORDER BY key")
	if err != nil {
		return err
	}
	var last []byte
	var rid [8]byte
	ordered := true
	for rows.Next() {
		var key []byte
		var rowid int64
		if err := rows.Scan(&key, &rowid); err != nil {
			rows.Close()
			return err
		}
		if idx.n > 0 && bytes.Compare(key, last) <= 0 {
			ordered = false
		}
		last = key
		binary.BigEndian.PutUint64(rid[:], uint64(rowid))
		h.add(&idx, key, rid[:])
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !ordered {
		fault("the key index is not in strictly increasing key order")
	}
	if idx != gotIdx {
		fault("the key index holds %d entries, the table %d rows, or their keys and rowids differ", idx.n, gotIdx.n)
	}

	el := time.Since(start)
	fmt.Printf("verify raw: %d rows equal, %d left out; %s of pages read in %s, all checks in %s\n",
		got.n, dropped, gb((sr.pages+dr.pages)*int64(sr.pageSize)), scanned.Round(time.Second), el.Round(time.Second))
	if len(faults) != 0 {
		slices.Sort(faults)
		for _, s := range faults {
			fmt.Println("  FAULT", s)
		}
		return fmt.Errorf("verify failed: %d faults", len(faults))
	}
	fmt.Println("verify: ok")
	return nil
}
