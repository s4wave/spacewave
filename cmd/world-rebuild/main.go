// Command world-rebuild copies a World SQLite kvtx store into a new file that
// keeps everything except the block values proven to be in the backend.
//
// A block value is dropped only when all of these hold, each checked locally:
//   - its write-back object store holds an eviction queue entry for it;
//   - no object store holds an upload marker for it;
//   - the pack index has an h row for it naming a pack its p/ table holds;
//   - no saved read generation names it (the hot set).
//
// Everything else is copied byte for byte, and the eviction queue entries of
// the dropped blocks are left out, with evictable-bytes recomputed from the
// entries kept.
//
// Subcommands:
//
//	count    dry run on a live file: reads its pages and the pack index's
//	         directly, without SQLite and so without a lock, paced to
//	         -mbps; reports by family and probes the copy rate
//	copy     writes the new file; the source must have no writer
//	verify   checks a copy against its source and the drop rule
//	packcheck checks the packs of the blocks a copy dropped against a bucket
//	         listing, before the old World is removed
//	synth    builds a test store shaped like production, of -n blocks, for timing
//	selftest builds a local test store, counts, copies, verifies, and runs
//	         mutants
package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	blockPrefix    = "h/b/"
	objsPrefix     = "h/objs/"
	markedSeg      = "marked/"
	evictSeg       = "evictable/"
	readsSeg       = "reads/"
	evictBytesSeg  = "evictable-bytes"
	readKeyLen     = 16
	indexKeyBytes  = 16
	defaultStoreID = "hyperline-world"
)

// opts are the shared flags.
type opts struct {
	world   string
	packs   string
	storeID string
	// wbID is the write-back object store id the drop rule takes facts
	// from; empty is prodWritebackID(storeID).
	wbID    string
	out     string
	chunk   int
	pause   time.Duration
	mutant  string
	verbose bool
	txn     time.Duration
	walMax  int64
	n       int
	full    bool
	rowid   bool
	// mbps paces count's raw page reads, in megabytes a second.
	mbps float64
	// maxKeptGB aborts a copy whose kept bytes pass it; 0 is no cap.
	maxKeptGB float64
	// rateOut and rateMB make count copy kept rows of some slices into a
	// scratch file to measure the copy rate.
	rateOut string
	rateMB  int64
	// rateOnly makes count skip its passes and only sample the copy rate.
	rateOnly bool
	// rateSplit makes count time the raw read and the insert apart.
	rateSplit bool
	// read picks how copy and verify read the source: raw pages, or SQLite.
	read string
	// readers is the raw read goroutines.
	readers int
	// txnRows is the rows a copy transaction holds.
	txnRows int
	// sortBatch sorts each batch of kept rows by key before inserting it.
	sortBatch bool
	// deferIndex builds the key index after the rows are in.
	deferIndex bool
	// rawFacts makes loadFacts read the object store keys raw from the
	// key index; copy and verify set it with -read raw.
	rawFacts bool
	// dryRead makes copy read and decide every row but insert none.
	dryRead bool
	// list is packcheck's bucket listing: lines of size and key.
	list string
	// listFormat is the listing's field order: sp (size, key) or ps.
	listFormat string
	// listPrefix, when set, is the directory the listing's packs/ and
	// entries/ must sit directly under, e.g. hyperline-world/.
	listPrefix string
	// noEntries lets packcheck pass a listing with no entries/ objects.
	noEntries bool
	// needed is a needed-packs file packcheck reads instead of the files.
	needed string
	// neededOut is where packcheck writes the needed packs.
	neededOut string
	// copyStatsOut, when set, receives the copy's stats; for tests.
	copyStatsOut *copyStats
}

func main() {
	// The subcommand comes first; its flags follow.
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var o opts

	// Files and the pack index store.
	fs.StringVar(&o.world, "world", "", "World SQLite file")
	fs.StringVar(&o.packs, "packs", "", "pack index SQLite file")
	fs.StringVar(&o.storeID, "store-id", defaultStoreID, "pack index block store id")
	fs.StringVar(&o.wbID, "writeback-id", "", "write-back object store id the drop rule reads (default p/local/hyperline-backend/blk/<store-id>/writeback)")
	fs.StringVar(&o.out, "out", "", "new World file (copy, verify)")

	// SQLite reads and the pacing of reads of a live file.
	fs.IntVar(&o.chunk, "chunk", 20000, "rows per short read")
	fs.DurationVar(&o.pause, "pause", 0, "pause between short reads (count)")
	fs.DurationVar(&o.txn, "txn", 40*time.Millisecond, "count: target time of one read of the live file")
	fs.Int64Var(&o.walMax, "wal-max-mib", 2560, "count: wait while a live WAL is larger")
	fs.Float64Var(&o.mbps, "mbps", 60, "count: megabytes a second the raw page reads take (0: no limit)")

	// Copy, verify and selftest behaviour.
	fs.StringVar(&o.mutant, "mutant", "", "copy only: drop-marked | drop-unqueued | drop-hot | drop-unpacked | drop-meta | stale-evict-bytes")
	fs.BoolVar(&o.verbose, "v", false, "verbose")
	fs.IntVar(&o.n, "n", 100000, "selftest blocks")
	fs.BoolVar(&o.full, "full", false, "verify: compare block values byte for byte, not only their lengths")
	fs.BoolVar(&o.rowid, "rowid", true, "selftest: rowid table, as production, instead of WITHOUT ROWID")
	fs.Float64Var(&o.maxKeptGB, "max-kept-gb", 0, "copy: abort once the kept bytes pass this (0: no cap)")
	fs.StringVar(&o.read, "read", "raw", "copy, verify: read the source raw (pages, pipelined, needs an empty WAL) or sqlite")
	fs.IntVar(&o.readers, "readers", 8, "copy, verify: raw read goroutines")
	fs.IntVar(&o.txnRows, "txn-rows", 20000, "copy: rows a transaction holds")
	fs.BoolVar(&o.sortBatch, "sort-batch", false, "copy: sort each batch of kept rows by key before inserting it")
	fs.BoolVar(&o.dryRead, "dry-read", false, "copy: read raw and decide every row, insert none (times the read alone)")
	fs.BoolVar(&o.deferIndex, "defer-index", false, "copy: build the key index after the rows are in, then give it the source's autoindex schema row")

	// The copy rate measurement of count.
	fs.StringVar(&o.rateOut, "rate-out", "", "count: scratch file for the copy rate measurement (removed after)")
	fs.Int64Var(&o.rateMB, "rate-mb", 1000, "count: kept megabytes the copy rate measurement copies")
	fs.BoolVar(&o.rateOnly, "rate-only", false, "count: skip the passes; copy ~rate-mb of rows from 64 slices to -rate-out, keeping all, and report the MB/s")
	fs.BoolVar(&o.rateSplit, "rate-split", false, "count: read ~rate-mb of rows from 64 slices into memory, then insert them into -rate-out as copy does, physical then key order, and report each rate")

	// The pack existence check.
	fs.StringVar(&o.list, "list", "", "packcheck: bucket listing, size and key lines (rclone lsf -R --format sp)")
	fs.StringVar(&o.listFormat, "list-format", "sp", "packcheck: the listing's field order, sp (size;key) or ps (key;size)")
	fs.StringVar(&o.listPrefix, "list-prefix", "", "packcheck: take only keys <prefix>packs/<id> and <prefix>entries/<id> of the listing, e.g. hyperline-world/")
	fs.BoolVar(&o.noEntries, "no-entries", false, "packcheck: pass a listing with no entries/ objects (entry objects then go unchecked)")
	fs.StringVar(&o.needed, "needed", "", "packcheck: check the packs of this needed-packs file, reading no SQLite file")
	fs.StringVar(&o.neededOut, "needed-out", "", "packcheck, count: write the packs the dropped blocks need (id size blocks) here, for packcheck -needed")
	_ = fs.Parse(os.Args[2:])

	// Run the subcommand.
	var err error
	switch {
	case cmd == "count" && o.rateSplit:
		err = runRateSplit(o)
	case cmd == "count" && o.rateOnly:
		err = runRateOnly(o)
	case cmd == "count":
		_, err = runCount(o)
	case cmd == "copy":
		err = runCopy(o)
	case cmd == "verify":
		err = runVerify(o)
	case cmd == "checkswap":
		err = runCheckSwap(o)
	case cmd == "selftest":
		err = runSelftest(o)
	case cmd == "synth":
		var drop int64
		if drop, err = buildTestStore(o.world, o.packs, o.n, o.storeID, o.rowid, true); err == nil {
			fmt.Printf("synth: %d blocks, %d the rule drops\n", o.n, drop)
		}
	case cmd == "packcheck":
		_, err = runPackCheck(o)
	case cmd == "probe":
		err = runProbe(o)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: world-rebuild count|copy|verify|checkswap|packcheck|selftest|probe -world F -packs F [-out F]")
	os.Exit(2)
}

// openRO opens a SQLite file read-only (mode=ro), one connection. It refuses
// a file another process holds a lock on, such as a running backend. The
// read-only open reads a WAL that still holds commits, and closing it cannot
// checkpoint: the checkpoint on close needs an exclusive lock and a write
// through the main file's descriptor, which a read-only open does not have.
// It may leave an empty -wal or -shm beside the file.
func openRO(path string) (*sql.DB, error) {
	// The file must exist and have no live connection.
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if err := refuseLiveLock(path); err != nil {
		return nil, err
	}

	// Open one connection.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// kvTable finds the key-value table of a kvtx SQLite file and its SQL.
func kvTable(db *sql.DB) (string, string, error) {
	rows, err := db.Query("SELECT name, sql FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	var found []string
	var name, stmt string
	for rows.Next() {
		var n, s sql.NullString
		if err := rows.Scan(&n, &s); err != nil {
			return "", "", err
		}
		if strings.Contains(strings.ToLower(s.String), "key blob primary key") {
			found = append(found, n.String)
			name, stmt = n.String, s.String
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	if len(found) != 1 {
		return "", "", fmt.Errorf("expected one kv table, found %v", found)
	}
	return name, stmt, nil
}

// header holds the header pragmas of a file.
type header struct {
	pageSize, pageCount, freelist, autoVacuum int64
	journal                                   string
}

func readHeader(db *sql.DB) (header, error) {
	var h header
	for _, q := range []struct {
		p string
		d *int64
	}{{"page_size", &h.pageSize}, {"page_count", &h.pageCount}, {"freelist_count", &h.freelist}, {"auto_vacuum", &h.autoVacuum}} {
		if err := db.QueryRow("PRAGMA " + q.p).Scan(q.d); err != nil {
			return h, fmt.Errorf("pragma %s: %w", q.p, err)
		}
	}
	err := db.QueryRow("PRAGMA journal_mode").Scan(&h.journal)
	return h, err
}

// scan calls fn with each key in [lo, hi) in key order and its value length,
// in short reads of chunk rows, each its own read transaction. With values
// set it also passes the value. hi nil scans to the end.
func scan(db *sql.DB, table string, lo, hi []byte, chunk int, pause time.Duration, values bool, fn func(key []byte, vlen int64, value []byte) error) error {
	cols := "key, length(value)"
	if values {
		cols += ", value"
	}
	first := true
	last := lo
	if last == nil {
		// A NULL bound matches no key; the empty blob sorts before all.
		last = []byte{}
	}
	for {
		cond := "key > ?"
		if first {
			cond = "key >= ?"
		}
		q := "SELECT " + cols + " FROM " + table + " WHERE " + cond
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
		type item struct {
			key   []byte
			vlen  int64
			value []byte
		}
		var items []item
		for rows.Next() {
			var key []byte
			var vlen sql.NullInt64
			var value []byte
			if values {
				err = rows.Scan(&key, &vlen, &value)
			} else {
				err = rows.Scan(&key, &vlen)
			}
			if err != nil {
				rows.Close()
				return err
			}
			items = append(items, item{key, vlen.Int64, value})
		}
		err = rows.Err()
		rows.Close()
		live.done(time.Since(t0), chunk)
		if err != nil {
			return err
		}
		// The read is closed: fn may query the same pool.
		n := len(items)
		for _, it := range items {
			last = it.key
			if err := fn(it.key, it.vlen, it.value); err != nil {
				return err
			}
		}
		if n < lim {
			return nil
		}
		first = false
		if pause > 0 {
			time.Sleep(pause)
		}
	}
}

// prefixEnd returns the first key after every key with prefix p.
func prefixEnd(p []byte) []byte {
	end := bytes.Clone(p)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

// tally counts keys and bytes.
type tally struct{ keys, bytes int64 }

func (t *tally) add(b int64) { t.keys++; t.bytes += b }

func gb(b int64) string { return fmt.Sprintf("%.2f GB", float64(b)/1e9) }

// family names the family of a non-block key for the report.
func family(key []byte) string {
	if obj, rest, ok := objKey(key); ok {
		_ = obj
		seg := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			seg = rest[:i+1]
		}
		return "objs/<id>/" + seg
	}
	s := string(key)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		if j := strings.IndexByte(s[i+1:], '/'); j >= 0 {
			return s[:i+1+j+1]
		}
		return s[:i+1]
	}
	if len(s) > 12 {
		s = s[:12]
	}
	return strconv.Quote(s)
}

func trunc(b []byte) string {
	if len(b) > 24 {
		b = b[:24]
	}
	return string(b)
}
