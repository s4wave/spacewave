package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// rawSource checks that a source can be read raw and returns its kv table's
// root page. The raw reader sees the main file only, so a WAL holding frames
// would be missed: the source must have none. openRO has already refused a
// file a live connection holds.
func rawSource(db *sql.DB, path, table, stmt string) (int64, error) {
	if !hasRowid(stmt) {
		return 0, fmt.Errorf("%s: -read raw needs a rowid table", path)
	}
	if fi, err := os.Stat(path + "-wal"); err == nil && fi.Size() > 0 {
		return 0, fmt.Errorf("%s-wal holds %d bytes the main file lacks: -read raw reads the main file only; checkpoint it (PRAGMA wal_checkpoint(TRUNCATE)) or use -read sqlite", path, fi.Size())
	}
	var root int64
	if err := db.QueryRow("SELECT rootpage FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&root); err != nil {
		return 0, err
	}
	return root, nil
}

// tableLeaves walks the table b-tree rooted at root through its interior
// pages and returns a bitmap of its leaf pages and their count. A SQLite
// b-tree is balanced, so a level whose first page is a leaf is all leaves.
func (r *rawFile) tableLeaves(root int64, workers int) ([]uint64, int64, error) {
	level := []int64{root}
	buf := alignedBuf(r.pageSize)
	for depth := 0; ; depth++ {
		if depth > 20 {
			return nil, 0, fmt.Errorf("table b-tree deeper than 20")
		}
		if err := r.page(level[0], buf); err != nil {
			return nil, 0, err
		}
		if buf[0] == 0x0d {
			leaves := make([]uint64, r.pages/64+1)
			for _, pg := range level {
				leaves[pg/64] |= 1 << (pg % 64)
			}
			return leaves, int64(len(level)), nil
		}

		// Read the interior pages of this level in parallel.
		next := make([][]int64, workers)
		var g errgroup.Group
		for w := range workers {
			g.Go(func() error {
				b := alignedBuf(r.pageSize)
				for i := w; i < len(level); i += workers {
					pg := level[i]
					if err := r.page(pg, b); err != nil {
						return err
					}
					if b[0] != 0x05 {
						return fmt.Errorf("page %d at depth %d is type %#x, want a table interior page", pg, depth, b[0])
					}
					n := int(binary.BigEndian.Uint16(b[3:]))
					for c := range n {
						at := 12 + 2*c
						if at+2 > r.usable {
							return fmt.Errorf("interior page %d: cell %d out of range", pg, c)
						}
						off := int(binary.BigEndian.Uint16(b[at:]))
						if off+4 > r.usable {
							return fmt.Errorf("interior page %d: cell %d at %d out of range", pg, c, off)
						}
						next[w] = append(next[w], int64(binary.BigEndian.Uint32(b[off:])))
					}
					next[w] = append(next[w], int64(binary.BigEndian.Uint32(b[8:])))
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, 0, err
		}
		level = slices.Concat(next...)
		for _, pg := range level {
			if pg < 2 || pg > r.pages {
				return nil, 0, fmt.Errorf("child page %d out of range", pg)
			}
		}
		slices.Sort(level)
	}
}

// rawVisit is called with each row of a raw table scan: its rowid, its key
// (valid only during the call), its value length, and a reader of its whole
// value, which follows the overflow chain.
type rawVisit func(rowid int64, key []byte, vlen int64, value func() ([]byte, error)) error

// scanTable reads the leaf pages of the table whose leaf bitmap is leaves,
// front to back, with readers goroutines each on a contiguous share of the
// file. visit(w) makes reader w's visitor, called from its goroutine only.
// Every leaf page and cell must decode: the source is offline, so a page that
// does not is an error, not a torn read. A reader's flush, when not nil, runs
// after its last row. read, when set, counts the bytes of pages read; stop,
// when set, ends the readers early. The first error stops all readers.
func (r *rawFile) scanTable(leaves []uint64, readers int, read *atomic.Int64, stop *atomic.Bool, visit func(w int) (rawVisit, func() error)) error {
	const chunk = 4 << 20
	if r.direct && r.pageSize%4096 != 0 {
		return fmt.Errorf("%s: page size %d: the O_DIRECT reads of a share need 4096-byte pages or larger", r.path, r.pageSize)
	}
	per := chunk / r.pageSize
	share := (r.pages + int64(readers) - 1) / int64(readers)
	if stop == nil {
		stop = new(atomic.Bool)
	}
	var g errgroup.Group
	for w := range readers {
		fn, flush := visit(w)
		g.Go(func() error {
			err := r.scanShare(leaves, w, share, per, read, stop, fn)
			if err == nil && flush != nil {
				err = flush()
			}
			if err != nil {
				stop.Store(true)
			}
			return err
		})
	}
	return g.Wait()
}

// scanShare is one reader of scanTable: the pages of share w.
func (r *rawFile) scanShare(leaves []uint64, w int, share int64, per int, read *atomic.Int64, stop *atomic.Bool, fn rawVisit) error {
	buf := alignedBuf(per * r.pageSize)
	lo, hi := int64(w)*share+1, min(int64(w+1)*share, r.pages)
	for first := lo; first <= hi; first += int64(per) {
		if stop.Load() {
			return nil
		}
		n := min(int64(per), hi-first+1)
		if _, err := r.f.ReadAt(buf[:n*int64(r.pageSize)], (first-1)*int64(r.pageSize)); err != nil {
			return fmt.Errorf("read pages %d+%d: %w", first, n, err)
		}
		if read != nil {
			read.Add(n * int64(r.pageSize))
		}
		for i := range n {
			pg := first + i
			if leaves[pg/64]&(1<<(pg%64)) == 0 {
				continue
			}
			p := buf[i*int64(r.pageSize) : (i+1)*int64(r.pageSize)]
			if p[0] != 0x0d {
				return fmt.Errorf("leaf page %d is type %#x", pg, p[0])
			}
			var verr error
			bad := r.leafCells(pg, p, func(c rawCell) {
				if verr != nil {
					return
				}
				key, vlen, vlocal, ok := kvRecord(c)
				if !ok {
					verr = fmt.Errorf("page %d rowid %d: record did not decode", pg, c.rowid)
					return
				}
				// A NULL value stays NULL: nil, not zero bytes.
				if kvValueNull(c) {
					verr = fn(c.rowid, key, vlen, func() ([]byte, error) { return nil, nil })
					return
				}
				verr = fn(c.rowid, key, vlen, func() ([]byte, error) { return r.value(c, vlen, vlocal) })
			})
			if verr != nil {
				return verr
			}
			if bad != 0 {
				return fmt.Errorf("page %d: %d cells did not decode", pg, bad)
			}
		}
	}
	return nil
}

// rawRow is one kept row.
type rawRow struct{ key, value []byte }

// copyRawRows reads the source raw with o.readers goroutines, applies the
// drop rule in each, and sends batches of kept rows with their values to
// put, which runs in the caller's goroutine: reads and inserts overlap.
func copyRawRows(o opts, path string, root int64, p *dropPlan, st *copyStats, put func([]rawRow) error) error {
	r, err := openRaw(path)
	if err != nil {
		return err
	}
	defer r.Close()
	leaves, nleaves, err := r.tableLeaves(root, max(o.readers, 1))
	if err != nil {
		return fmt.Errorf("walk the table b-tree: %w", err)
	}
	if o.verbose {
		fmt.Printf("raw read: %d leaf pages of %d, %d readers\n", nleaves, r.pages, o.readers)
	}

	// The readers fill batches; the caller inserts them.
	const batchRows = 2000
	batches := make(chan []rawRow, 2*max(o.readers, 1))
	stats := make([]copyStats, max(o.readers, 1))
	var mu sync.Mutex
	var stop atomic.Bool
	done := make(chan error, 1)
	go func() {
		err := r.scanTable(leaves, max(o.readers, 1), nil, &stop, func(w int) (rawVisit, func() error) {
			var batch []rawRow
			s := &stats[w]
			flush := func() error {
				if len(batch) != 0 {
					batches <- batch
				}
				return nil
			}
			return func(_ int64, key []byte, vlen int64, value func() ([]byte, error)) error {
				if p.mutant != "" {
					mu.Lock()
					defer mu.Unlock()
				}
				if !p.decide(key, vlen, s) {
					return nil
				}
				v, err := value()
				if err != nil {
					return fmt.Errorf("read value of %q: %w", trunc(key), err)
				}
				batch = append(batch, rawRow{bytes.Clone(key), v})
				if len(batch) >= batchRows {
					batches <- batch
					batch = nil
				}
				return nil
			}, flush
		})
		done <- err
	}()

	// Insert until the readers finish; on an insert error, drain so the
	// readers can exit.
	var ierr error
	var readErr error
	for finished := false; !finished; {
		select {
		case b := <-batches:
			if ierr == nil {
				if ierr = put(b); ierr != nil {
					stop.Store(true)
				}
			}
		case readErr = <-done:
			finished = true
		}
	}
	for len(batches) > 0 {
		b := <-batches
		if ierr == nil {
			ierr = put(b)
		}
	}
	if readErr != nil {
		return readErr
	}
	if ierr != nil {
		return ierr
	}
	for i := range stats {
		st.merge(&stats[i])
	}
	return nil
}
