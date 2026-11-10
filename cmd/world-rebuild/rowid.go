package main

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// pacer keeps the reads of a live file short and out of the checkpointer's
// way: each read is sized to take about target, a pause follows it, and while
// a watched WAL is past walMax the run waits for the checkpointer to cut it.
// It prints a progress line each minute.
type pacer struct {
	target time.Duration
	pause  time.Duration
	wals   []string
	walMax int64
	rows   int
	reads  int64
	// busy is the time spent inside reads; waited is the time the WAL
	// guard held the run.
	busy   time.Duration
	waited time.Duration
	// phase and status describe the run for the progress line.
	phase    string
	status   func() string
	lastProg time.Time
}

// live is the pacer of the current run, nil for an offline file.
var live *pacer

// size returns the rows of the next read, at most limit.
func (p *pacer) size(limit int) int {
	if p == nil {
		return limit
	}
	if p.rows == 0 {
		p.rows = 200
	}
	return min(p.rows, limit)
}

// setPhase names the current phase and its progress text.
func (p *pacer) setPhase(phase string, status func() string) {
	if p == nil {
		return
	}
	p.phase, p.status = phase, status
	fmt.Printf("%s phase %s\n", time.Now().UTC().Format("15:04:05"), phase)
}

// done adapts the read size to the time the read took, prints the progress
// line, then pauses and waits out a long WAL.
func (p *pacer) done(took time.Duration, limit int) {
	if p == nil {
		return
	}
	p.reads++
	p.busy += took
	switch {
	case took > p.target:
		p.rows = max(p.rows/2, 10)
	case took < p.target/2:
		p.rows = min(p.rows+p.rows/2+1, limit)
	}
	if took > 100*time.Millisecond {
		fmt.Printf("  %s slow read: %s for %d rows\n", time.Now().UTC().Format("15:04:05"), took.Round(time.Millisecond), p.rows)
	}
	if time.Since(p.lastProg) >= time.Minute {
		p.lastProg = time.Now()
		st := ""
		if p.status != nil {
			st = p.status()
		}
		fmt.Printf("  %s %s: %s; %d reads, %d rows a read, busy %s, WAL waits %s\n",
			p.lastProg.UTC().Format("15:04:05"), p.phase, st, p.reads, p.rows, p.busy.Round(time.Second), p.waited.Round(time.Second))
	}
	time.Sleep(p.pause)
	for _, wal := range p.wals {
		p.waitWAL(wal)
	}
}

// waitWAL waits while the WAL at path is past walMax.
func (p *pacer) waitWAL(path string) {
	logged := false
	for {
		info, err := os.Stat(path)
		if err != nil || info.Size() <= p.walMax {
			if logged {
				fmt.Printf("  %s %s back under the bound, resuming\n", time.Now().UTC().Format("15:04:05"), path)
			}
			return
		}
		if !logged {
			fmt.Printf("  %s %s at %d MiB, waiting for a checkpoint\n", time.Now().UTC().Format("15:04:05"), path, info.Size()>>20)
			logged = true
		}
		time.Sleep(time.Second)
		p.waited += time.Second
	}
}

// hasRowid reports whether a table's SQL makes a rowid table, whose rowid
// order is its physical order.
func hasRowid(stmt string) bool {
	return !strings.Contains(strings.ToUpper(stmt), "WITHOUT ROWID")
}

// scanKeys calls fn with each key in [lo, hi) in key order, reading only the
// key's index (a covering scan), in short reads of chunk rows.
func scanKeys(db *sql.DB, table string, lo, hi []byte, chunk int, pause time.Duration, fn func(key []byte) error) error {
	first := true
	last := lo
	if last == nil {
		last = []byte{}
	}
	for {
		cond := "key > ?"
		if first {
			cond = "key >= ?"
		}
		q := "SELECT key FROM " + table + " WHERE " + cond
		args := []any{last}
		if hi != nil {
			q += " AND key < ?"
			args = append(args, hi)
		}
		q += " ORDER BY key LIMIT ?"
		lim := live.size(chunk)
		args = append(args, lim)
		t0 := time.Now()
		rows, err := db.Query(q, args...)
		if err != nil {
			return err
		}
		var keys [][]byte
		for rows.Next() {
			var key []byte
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, key)
		}
		err = rows.Err()
		rows.Close()
		live.done(time.Since(t0), chunk)
		if err != nil {
			return err
		}
		for _, key := range keys {
			last = key
			if err := fn(key); err != nil {
				return err
			}
		}
		if len(keys) < lim {
			return nil
		}
		first = false
		if pause > 0 {
			time.Sleep(pause)
		}
	}
}

// scanPhysical calls fn with every row in physical order: rowid order for a
// rowid table, which reads the file front to back, key order otherwise. It
// passes the value length without loading the value. id names the row for
// readValue: the rowid, or the key for a table without one.
func scanPhysical(db *sql.DB, table string, rowid bool, chunk int, pause time.Duration, fn func(id any, key []byte, vlen int64) error) error {
	if !rowid {
		return scan(db, table, nil, nil, chunk, pause, false, func(key []byte, vlen int64, _ []byte) error {
			return fn(key, key, vlen)
		})
	}
	return scanRowids(db, table, math.MinInt64, math.MaxInt64, chunk, pause, func(id int64, key []byte, vlen int64) error {
		return fn(id, key, vlen)
	})
}

// scanRowids calls fn with every row of a rowid table with lo <= rowid < hi,
// in rowid order, in short reads of at most chunk rows. It passes the value
// length, which SQLite takes from the record header without reading the
// value's overflow pages.
func scanRowids(db *sql.DB, table string, lo, hi int64, chunk int, pause time.Duration, fn func(id int64, key []byte, vlen int64) error) error {
	first := true
	last := lo
	for {
		lim := live.size(chunk)
		cond := "rowid > ?"
		if first {
			cond = "rowid >= ?"
		}
		t0 := time.Now()
		rows, err := db.Query("SELECT rowid, key, length(value) FROM "+table+" WHERE "+cond+" AND rowid < ? ORDER BY rowid LIMIT ?", last, hi, lim)
		if err != nil {
			return err
		}
		type item struct {
			id   int64
			key  []byte
			vlen int64
		}
		var items []item
		for rows.Next() {
			var it item
			var vlen sql.NullInt64
			if err := rows.Scan(&it.id, &it.key, &vlen); err != nil {
				rows.Close()
				return err
			}
			it.vlen = vlen.Int64
			items = append(items, it)
		}
		err = rows.Err()
		rows.Close()
		live.done(time.Since(t0), chunk)
		if err != nil {
			return err
		}
		for _, it := range items {
			last = it.id
			if err := fn(it.id, it.key, it.vlen); err != nil {
				return err
			}
		}
		if len(items) < lim {
			return nil
		}
		first = false
		if pause > 0 {
			time.Sleep(pause)
		}
	}
}

// pointReads runs n point reads of one query in short read transactions:
// fn(i, stmt) runs read i on the statement bound to the transaction. The
// pacer sizes each transaction.
func pointReads(db *sql.DB, query string, n, chunk int, fn func(i int, st *sql.Stmt) error) error {
	st, err := db.Prepare(query)
	if err != nil {
		return err
	}
	defer st.Close()
	for i := 0; i < n; {
		lim := min(live.size(chunk), n-i)
		t0 := time.Now()
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		txst := tx.Stmt(st)
		for j := 0; j < lim; j++ {
			if err := fn(i+j, txst); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Rollback(); err != nil {
			return err
		}
		live.done(time.Since(t0), chunk)
		i += lim
	}
	return nil
}

// valueReader reads one value by the id scanPhysical passed.
func valueReader(db *sql.DB, table string, rowid bool) (func(id any) ([]byte, error), func(), error) {
	// The driver scans an empty blob as nil, as it does NULL: read the
	// type alongside, so an empty blob stays an empty blob.
	q := "SELECT value, typeof(value) FROM " + table + " WHERE key = ?"
	if rowid {
		q = "SELECT value, typeof(value) FROM " + table + " WHERE rowid = ?"
	}
	st, err := db.Prepare(q)
	if err != nil {
		return nil, nil, err
	}
	return func(id any) ([]byte, error) {
		var v []byte
		var ty string
		if err := st.QueryRow(id).Scan(&v, &ty); err != nil {
			return nil, err
		}
		if v == nil && ty != "null" {
			v = []byte{}
		}
		return v, nil
	}, func() { st.Close() }, nil
}
