package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"
)

// indexLocal is the payload an index b-tree cell keeps on its page.
func (r *rawFile) indexLocal(p int64) int64 {
	u := int64(r.usable)
	x := (u-12)*64/255 - 23
	if p <= x {
		return p
	}
	m := (u-12)*32/255 - 23
	k := m + (p-m)%(u-4)
	if k <= x {
		return k
	}
	return m
}

// indexCell decodes the key of one index cell at off of page p: a record
// of a blob key and an integer rowid. spilled reports a cell whose payload
// goes on to overflow pages; the key may then be incomplete.
func (r *rawFile) indexCell(p []byte, off int) (key []byte, spilled bool, err error) {
	if off >= r.usable {
		return nil, false, fmt.Errorf("cell offset %d out of range", off)
	}
	size, k := varint(p[off:r.usable])
	if k == 0 {
		return nil, false, fmt.Errorf("cell size did not decode")
	}
	start := off + k
	local := r.indexLocal(int64(size))
	if start+int(local) > r.usable {
		return nil, false, fmt.Errorf("cell payload out of range")
	}
	b := p[start : start+int(local)]
	hs, k1 := varint(b)
	if k1 == 0 || hs < 2 || int(hs) > len(b) {
		return nil, false, fmt.Errorf("record header did not decode")
	}
	t, k2 := varint(b[k1:hs])
	if k2 == 0 || t < 12 || t%2 != 0 {
		return nil, false, fmt.Errorf("index key is serial type %d, want a blob", t)
	}
	kl := int64(t-12) / 2
	if int64(hs)+kl > int64(len(b)) {
		return b[hs:], true, nil
	}
	return b[hs : int64(hs)+kl], local < int64(size), nil
}

// indexKeys calls fn, from readers goroutines, with every key of the index
// b-tree rooted at root that has the prefix, reading its pages raw: the
// interior pages level by level, then the leaf pages in page order. Index
// interior cells hold entries too, so their keys are passed as well. A key
// with the prefix whose cell spills to overflow pages is an error.
func (r *rawFile) indexKeys(root int64, prefix []byte, readers int, fn func(key []byte) error) error {
	var mu sync.Mutex
	check := func(key []byte, spilled bool) error {
		if !bytes.HasPrefix(key, prefix) && !(spilled && bytes.HasPrefix(prefix, key)) {
			return nil
		}
		if spilled {
			return fmt.Errorf("index key %q spills to overflow pages", trunc(key))
		}
		return fn(key)
	}
	hi := prefixEnd(prefix)
	level := []int64{root}
	for depth := 0; ; depth++ {
		if len(level) == 0 {
			return nil
		}
		if depth > 30 {
			return fmt.Errorf("index b-tree deeper than 30")
		}
		buf := alignedBuf(r.pageSize)
		if err := r.page(level[0], buf); err != nil {
			return err
		}
		leaf := buf[0] == 0x0a
		if !leaf && buf[0] != 0x02 {
			return fmt.Errorf("index page %d is type %#x", level[0], buf[0])
		}
		var next []int64
		var g errgroup.Group
		for w := range readers {
			g.Go(func() error {
				b := alignedBuf(r.pageSize)
				var kids []int64
				for i := w; i < len(level); i += readers {
					pg := level[i]
					if err := r.page(pg, b); err != nil {
						return err
					}
					want := byte(0x02)
					if leaf {
						want = 0x0a
					}
					if b[0] != want {
						return fmt.Errorf("index page %d at depth %d is type %#x, want %#x", pg, depth, b[0], want)
					}
					h := 12
					if leaf {
						h = 8
					}
					n := int(binary.BigEndian.Uint16(b[3:]))
					// The child left of a cell holds the keys between the
					// cell before (or the page's low end) and the cell: it
					// is read only when that span meets the prefix's range.
					// A spilled key bounds nothing.
					var lower []byte
					lowerKnown := false
					for c := range n {
						at := h + 2*c
						if at+2 > r.usable {
							return fmt.Errorf("index page %d: cell %d out of range", pg, c)
						}
						off := int(binary.BigEndian.Uint16(b[at:]))
						if !leaf {
							if off+4 > r.usable {
								return fmt.Errorf("index page %d: cell %d out of range", pg, c)
							}
							off += 4
						}
						key, spilled, err := r.indexCell(b, off)
						if err != nil {
							return fmt.Errorf("index page %d cell %d: %w", pg, c, err)
						}
						if !leaf {
							below := lowerKnown && hi != nil && bytes.Compare(lower, hi) >= 0
							above := !spilled && bytes.Compare(key, prefix) <= 0
							if !below && !above {
								kids = append(kids, int64(binary.BigEndian.Uint32(b[off-4:])))
							}
							lower, lowerKnown = key, !spilled
						}
						if err := check(key, spilled); err != nil {
							return err
						}
					}
					if !leaf && !(lowerKnown && hi != nil && bytes.Compare(lower, hi) >= 0) {
						kids = append(kids, int64(binary.BigEndian.Uint32(b[8:])))
					}
				}
				mu.Lock()
				next = append(next, kids...)
				mu.Unlock()
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		if leaf {
			return nil
		}
		for _, pg := range next {
			if pg < 2 || pg > r.pages {
				return fmt.Errorf("index child page %d out of range", pg)
			}
		}
		slices.Sort(next)
		level = next
	}
}

// keyIndexRoot returns the root page of the table's PRIMARY KEY autoindex.
func keyIndexRoot(db *sql.DB, table string) (int64, error) {
	var root int64
	err := db.QueryRow("SELECT rootpage FROM sqlite_master WHERE type = 'index' AND name = ? AND tbl_name = ?", "sqlite_autoindex_"+table+"_1", table).Scan(&root)
	return root, err
}

// blockDigestsRaw is blockDigests read raw from the key index.
func blockDigestsRaw(o opts, db *sql.DB, path, table, stmt string) (map[string]bool, error) {
	if _, err := rawSource(db, path, table, stmt); err != nil {
		return nil, err
	}
	root, err := keyIndexRoot(db, table)
	if err != nil {
		return nil, fmt.Errorf("key index root: %w", err)
	}
	r, err := openRaw(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	readers := max(o.readers, 1)
	var mu sync.Mutex
	present := map[string]bool{}
	err = r.indexKeys(root, []byte(blockPrefix), readers, func(key []byte) error {
		mu.Lock()
		present[string(key)] = true
		mu.Unlock()
		return nil
	})
	return present, err
}

// rawIndexKeys calls fn, from several goroutines, with each key of the
// source's key index that has the prefix, read raw. The key is valid only
// during the call.
func rawIndexKeys(o opts, db *sql.DB, path, table string, prefix []byte, fn func(key []byte) error) error {
	_, stmt, err := kvTable(db)
	if err != nil {
		return err
	}
	if _, err := rawSource(db, path, table, stmt); err != nil {
		return err
	}
	root, err := keyIndexRoot(db, table)
	if err != nil {
		return fmt.Errorf("key index root: %w", err)
	}
	r, err := openRaw(path)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.indexKeys(root, prefix, max(o.readers, 1), fn)
}

// rawLookups reads the table of an offline file raw and returns the value
// of each of the n keys key(i) names, nil where the table has none.
func rawLookups(o opts, db *sql.DB, path, table string, n int, key func(i int) []byte) ([][]byte, error) {
	_, stmt, err := kvTable(db)
	if err != nil {
		return nil, err
	}
	root, err := rawSource(db, path, table, stmt)
	if err != nil {
		return nil, err
	}
	want := make(map[string]int, n)
	for i := range n {
		want[string(key(i))] = i
	}
	r, err := openRaw(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	readers := max(o.readers, 1)
	leaves, _, err := r.tableLeaves(root, readers)
	if err != nil {
		return nil, err
	}
	vals := make([][]byte, n)
	err = r.scanTable(leaves, readers, nil, nil, func(int) (rawVisit, func() error) {
		return func(_ int64, k []byte, _ int64, value func() ([]byte, error)) error {
			i, ok := want[string(k)]
			if !ok {
				return nil
			}
			v, err := value()
			if err != nil {
				return err
			}
			// Keys are unique, so each slot has one writer.
			vals[i] = v
			return nil
		}, nil
	})
	return vals, err
}
