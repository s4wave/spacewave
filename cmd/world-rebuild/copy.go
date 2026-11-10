package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// copyStats counts a copy.
type copyStats struct {
	keptKeys, keptBytes       int64
	droppedKeys, droppedBytes int64
	droppedEvict              int64
	mutantHits                int
	// verdicts tallies the block keys, kept and dropped, by verdict.
	verdicts map[string]*tally
}

// openNew creates the new World file: incremental auto_vacuum set and made
// to hold by a VACUUM before any table exists, as spacewave's
// setFreshAutoVacuum does, then the source's table, journal off for the build.
//
// deferIndex creates the table without its key index (stripPK), for
// buildKeyIndex to add after the rows are in.
func openNew(path, stmt string, pageSize int64, deferIndex bool) (*sql.DB, error) {
	if deferIndex {
		var err error
		if stmt, err = stripPK(stmt); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%s exists", path)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	exec := func(qs ...string) error {
		for _, q := range qs {
			if _, err := db.Exec(q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}
	var tables int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&tables); err != nil || tables != 0 {
		db.Close()
		return nil, fmt.Errorf("new file holds %d tables: %v", tables, err)
	}
	if err := exec(fmt.Sprintf("PRAGMA page_size = %d", pageSize), "PRAGMA auto_vacuum = INCREMENTAL", "VACUUM"); err != nil {
		db.Close()
		return nil, err
	}
	var av int64
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&av); err != nil || av != 2 {
		db.Close()
		return nil, fmt.Errorf("auto_vacuum is %d, want 2 (incremental): %v", av, err)
	}
	if err := exec("PRAGMA journal_mode = OFF", "PRAGMA synchronous = OFF", "PRAGMA cache_size = -4194304", stmt); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// stripPK removes the one PRIMARY KEY clause of a CREATE TABLE.
func stripPK(stmt string) (string, error) {
	up := strings.ToUpper(stmt)
	i := strings.Index(up, " PRIMARY KEY")
	if i < 0 || strings.Count(up, "PRIMARY KEY") != 1 {
		return "", fmt.Errorf("table SQL %q: want one PRIMARY KEY clause", stmt)
	}
	return stmt[:i] + stmt[i+len(" PRIMARY KEY"):], nil
}

// buildKeyIndex adds the key index to a table openNew made with deferIndex,
// one sort over the rows, then rewrites the schema rows to the source's: the
// table row to stmt and the index row to the PRIMARY KEY's autoindex, which
// is a unique index on key alone with no SQL. It closes db and returns the
// file reopened, so the schema is read again, and checks SQLite sees the
// index as the table's PRIMARY KEY.
func buildKeyIndex(db *sql.DB, path, table, stmt string) (*sql.DB, error) {
	const tmp = "world_rebuild_key"
	auto := "sqlite_autoindex_" + table + "_1"
	var ver int64
	steps := []func() error{
		func() error { _, err := db.Exec("CREATE UNIQUE INDEX " + tmp + " ON " + table + " (key)"); return err },
		func() error { return db.QueryRow("PRAGMA schema_version").Scan(&ver) },
		func() error { _, err := db.Exec("PRAGMA writable_schema = ON"); return err },
		func() error {
			_, err := db.Exec("UPDATE sqlite_master SET name = ?, sql = NULL WHERE type = 'index' AND name = ? AND tbl_name = ?", auto, tmp, table)
			return err
		},
		func() error {
			_, err := db.Exec("UPDATE sqlite_master SET sql = ? WHERE type = 'table' AND name = ?", stmt, table)
			return err
		},
		func() error { _, err := db.Exec(fmt.Sprintf("PRAGMA schema_version = %d", ver+1)); return err },
		func() error { _, err := db.Exec("PRAGMA writable_schema = OFF"); return err },
	}
	for _, f := range steps {
		if err := f(); err != nil {
			db.Close()
			return nil, fmt.Errorf("build the key index: %w", err)
		}
	}
	if err := db.Close(); err != nil {
		return nil, err
	}

	// Reopen and check the index is the PRIMARY KEY's.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode = OFF", "PRAGMA synchronous = OFF", "PRAGMA cache_size = -4194304"} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	var name, origin string
	var unique int
	if err := db.QueryRow("SELECT name, \"unique\", origin FROM pragma_index_list(?)", table).Scan(&name, &unique, &origin); err != nil {
		db.Close()
		return nil, fmt.Errorf("index list after the schema rewrite: %w", err)
	}
	if name != auto || unique != 1 || origin != "pk" {
		db.Close()
		return nil, fmt.Errorf("after the schema rewrite the index is %s unique=%d origin=%s, want %s 1 pk", name, unique, origin, auto)
	}
	return db, nil
}

// dropPlan holds what a copy leaves out, from the facts.
type dropPlan struct {
	f *facts
	// skipEvict holds the eviction entry keys of the dropped blocks.
	skipEvict map[string]struct{}
	// mutant forces one wrong drop of the named kind, for the verify check.
	mutant string
	hit    int
}

// planDrops decides the dropped blocks' queue entries. present reports
// whether the block value exists, so the entry of a block the store does not
// hold is kept.
func planDrops(f *facts, present map[string]bool) *dropPlan {
	p := &dropPlan{f: f, skipEvict: map[string]struct{}{}}
	for d, es := range f.evict {
		if !present[d] || f.verdict(d) != vDrop {
			continue
		}
		for _, e := range es {
			p.skipEvict[string(e.key)] = struct{}{}
		}
	}
	return p
}

// evictSums sums the sizes of the eviction entries kept, by object store, as
// rows pass; evictable-bytes is rewritten to the sums after the copy.
type evictSums map[string]int64

// add counts a kept row if it is an eviction entry.
func (s evictSums) add(key, value []byte) {
	obj, rest, ok := objKey(key)
	if !ok || !strings.HasPrefix(rest, evictSeg) {
		return
	}
	n, _ := evictSize(value)
	s[obj] += n
}

// keepBlock applies the drop rule (and a mutant) to one block key, and
// returns the verdict with it.
func (p *dropPlan) keepBlock(key []byte) (bool, string) {
	d, ok := digestOfBlockKey(key)
	if !ok {
		return true, vUnparsed
	}
	v := p.f.verdict(d)
	if p.hit == 0 {
		switch {
		case p.mutant == "drop-marked" && v == vMarked,
			p.mutant == "drop-unqueued" && v == vUnqueued,
			p.mutant == "drop-hot" && v == vHot,
			p.mutant == "drop-unpacked" && v == vUnpacked:
			p.hit++
			return false, v
		}
	}
	return v != vDrop, v
}

// addVerdict tallies a block row of b bytes under its verdict.
func (s *copyStats) addVerdict(v string, b int64) {
	if s.verdicts == nil {
		s.verdicts = map[string]*tally{}
	}
	t := s.verdicts[v]
	if t == nil {
		t = &tally{}
		s.verdicts[v] = t
	}
	t.add(b)
}

// merge adds a reader's drop and verdict stats to s.
func (s *copyStats) merge(r *copyStats) {
	s.droppedKeys += r.droppedKeys
	s.droppedBytes += r.droppedBytes
	s.droppedEvict += r.droppedEvict
	for v, t := range r.verdicts {
		if s.verdicts == nil {
			s.verdicts = map[string]*tally{}
		}
		if s.verdicts[v] == nil {
			s.verdicts[v] = &tally{}
		}
		s.verdicts[v].keys += t.keys
		s.verdicts[v].bytes += t.bytes
	}
}

func (p *dropPlan) decide(key []byte, vlen int64, st *copyStats) bool {
	switch {
	case bytes.HasPrefix(key, []byte(blockPrefix)):
		keep, v := p.keepBlock(key)
		st.addVerdict(v, int64(len(key))+vlen)
		if !keep {
			st.droppedKeys++
			st.droppedBytes += int64(len(key)) + vlen
			return false
		}
	default:
		if _, skip := p.skipEvict[string(key)]; skip {
			st.droppedKeys++
			st.droppedBytes += int64(len(key)) + vlen
			st.droppedEvict++
			return false
		}
		if p.mutant == "drop-meta" && p.hit == 0 && !strings.Contains(string(key), "/"+evictSeg) {
			p.hit++
			return false
		}
	}
	return true
}

// readBlockDigests is blockDigests, read raw from the key index with -read
// raw.
func readBlockDigests(o opts, db *sql.DB, path, table, stmt string) (map[string]bool, error) {
	if o.read == "raw" {
		return blockDigestsRaw(o, db, path, table, stmt)
	}
	return blockDigests(o, db, table)
}

// blockDigests scans the block range keys only and returns the digests held.
func blockDigests(o opts, db *sql.DB, table string) (map[string]bool, error) {
	present := map[string]bool{}
	err := scanKeys(db, table, []byte(blockPrefix), prefixEnd([]byte(blockPrefix)), o.chunk, 0, func(key []byte) error {
		if d, ok := digestOfBlockKey(key); ok {
			present[d] = true
		}
		return nil
	})
	return present, err
}

func runCopy(o opts) error {
	start := time.Now()

	// Stamp the sources before reading them, for verify and checkswap.
	if err := stampSources(o); err != nil {
		return fmt.Errorf("stamp sources: %w", err)
	}
	src, err := openRO(o.world)
	if err != nil {
		return err
	}
	defer src.Close()
	packs, err := openRO(o.packs)
	if err != nil {
		return err
	}
	defer packs.Close()
	table, stmt, err := kvTable(src)
	if err != nil {
		return err
	}
	o.rawFacts = o.read == "raw"
	f, err := loadFacts(o, src, table, packs)
	if err != nil {
		return err
	}
	tf := time.Now()
	present, err := readBlockDigests(o, src, o.world, table, stmt)
	if err != nil {
		return err
	}
	p := planDrops(f, present)
	p.mutant = o.mutant
	fmt.Printf("facts and block keys read in %s (facts %s, block keys %s): %d blocks, %d eviction entries to drop\n", time.Since(start).Round(time.Second), f.took.Round(time.Second), time.Since(tf).Round(time.Second), len(present), len(p.skipEvict))
	h, err := readHeader(src)
	if err != nil {
		return err
	}

	var root int64
	if o.read == "raw" {
		if root, err = rawSource(src, o.world, table, stmt); err != nil {
			return err
		}
	} else if o.read != "sqlite" {
		return fmt.Errorf("-read %q: want raw or sqlite", o.read)
	}
	if o.deferIndex && !hasRowid(stmt) {
		return fmt.Errorf("-defer-index needs a rowid table")
	}
	dst, err := openNew(o.out, stmt, h.pageSize, o.deferIndex)
	if err != nil {
		return err
	}
	defer func() { dst.Close() }()
	txnRows := o.txnRows
	if txnRows <= 0 {
		txnRows = 20000
	}
	maxKept := int64(o.maxKeptGB * 1e9)
	sums := evictSums{}
	var bytesKeys [][]byte

	var st copyStats
	var tx *sql.Tx
	var ins *sql.Stmt
	pending := 0
	commit := func() error {
		if tx == nil {
			return nil
		}
		ins.Close()
		err := tx.Commit()
		tx, ins, pending = nil, nil, 0
		return err
	}
	put := func(key, value []byte) error {
		if tx == nil {
			var err error
			if tx, err = dst.Begin(); err != nil {
				return err
			}
			if ins, err = tx.Prepare("INSERT INTO " + table + " (key, value) VALUES (?, ?)"); err != nil {
				return err
			}
		}
		if _, err := ins.Exec(key, value); err != nil {
			return err
		}
		st.keptKeys++
		st.keptBytes += int64(len(key) + len(value))
		if maxKept > 0 && st.keptBytes > maxKept {
			return fmt.Errorf("kept bytes passed the cap of %s; the drop rule keeps more than planned", gb(maxKept))
		}
		pending++
		if pending >= txnRows {
			return commit()
		}
		return nil
	}
	last := time.Now()
	progress := func() {
		if time.Since(last) < 30*time.Second {
			return
		}
		last = time.Now()
		el := time.Since(start).Seconds()
		fmt.Printf("  %s kept %d keys %s, dropped %d keys %s, %.0f MB/s written\n", time.Now().UTC().Format("15:04:05"), st.keptKeys, gb(st.keptBytes), st.droppedKeys, gb(st.droppedBytes), float64(st.keptBytes)/1e6/el)
	}

	// keep inserts one kept row.
	keep := func(key, v []byte) error {
		if _, rest, ok := objKey(key); ok && rest == evictBytesSeg {
			bytesKeys = append(bytesKeys, key)
		}
		sums.add(key, v)
		return put(key, v)
	}
	if o.read == "raw" {
		err = copyRawRows(o, o.world, root, p, &st, func(rows []rawRow) error {
			progress()
			if o.dryRead {
				for _, r := range rows {
					st.keptKeys++
					st.keptBytes += int64(len(r.key) + len(r.value))
				}
				return nil
			}
			if o.sortBatch {
				slices.SortFunc(rows, func(a, b rawRow) int { return bytes.Compare(a.key, b.key) })
			}
			for _, r := range rows {
				if err := keep(r.key, r.value); err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		err = copySQLiteRows(o, src, table, stmt, p, &st, progress, keep)
	}
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := commit(); err != nil {
		return err
	}
	if o.deferIndex {
		t0 := time.Now()
		if dst, err = buildKeyIndex(dst, o.out, table, stmt); err != nil {
			return err
		}
		fmt.Printf("key index built in %s\n", time.Since(t0).Round(time.Second))
	}
	fmt.Printf("rows written in %s\n", time.Since(start).Round(time.Second))

	// evictable-bytes holds the sum of the entries kept.
	for _, k := range bytesKeys {
		obj, _, _ := objKey(k)
		if o.mutant == "stale-evict-bytes" {
			p.hit++
			continue
		}
		if _, err := dst.Exec("UPDATE "+table+" SET value = ? WHERE key = ?", []byte(strconv.FormatInt(sums[obj], 10)), k); err != nil {
			return err
		}
		fmt.Printf("  %s evictable-bytes: %s\n", obj, gb(sums[obj]))
	}
	if o.mutant != "" && p.hit == 0 {
		return errors.New("mutant found nothing to drop")
	}

	// Leave the file in WAL mode, as the store opens it.
	if _, err := dst.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return err
	}
	h, err = readHeader(dst)
	if err != nil {
		return err
	}
	el := time.Since(start)
	fmt.Println("copy block values (h/b/) by verdict, measured, as count prints them:")
	for _, v := range sortedKeys(st.verdicts) {
		t := st.verdicts[v]
		fmt.Printf("  %-44s %12d keys %12s\n", v, t.keys, gb(t.bytes))
	}
	if o.copyStatsOut != nil {
		*o.copyStatsOut = st
	}
	fmt.Printf("copy done in %s (read %s): kept %d keys %s, dropped %d keys %s (%d eviction entries); new file %s, auto_vacuum %d, %.0f MB/s kept\n",
		el.Round(time.Second), o.read, st.keptKeys, gb(st.keptBytes), st.droppedKeys, gb(st.droppedBytes), st.droppedEvict,
		gb(h.pageCount*h.pageSize), h.autoVacuum, float64(st.keptBytes)/1e6/el.Seconds())
	return nil
}

// copySQLiteRows is the SQLite read of copy: one pass in physical order,
// deciding each row from its key, then reading the values of the kept rows
// only, by point reads.
func copySQLiteRows(o opts, src *sql.DB, table, stmt string, p *dropPlan, st *copyStats, progress func(), keep func(key, v []byte) error) error {
	read, closeRead, err := valueReader(src, table, hasRowid(stmt))
	if err != nil {
		return err
	}
	defer closeRead()
	type kept struct {
		id  any
		key []byte
	}
	var batch []kept
	flush := func() error {
		for _, k := range batch {
			v, err := read(k.id)
			if err != nil {
				return fmt.Errorf("read value: %w", err)
			}
			if err := keep(k.key, v); err != nil {
				return err
			}
		}
		batch = batch[:0]
		return nil
	}
	err = scanPhysical(src, table, hasRowid(stmt), o.chunk, 0, func(id any, key []byte, vlen int64) error {
		progress()
		if !p.decide(key, vlen, st) {
			return nil
		}
		batch = append(batch, kept{id: id, key: bytes.Clone(key)})
		if len(batch) >= o.chunk {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	}
	return err
}

// runVerify checks the new file against the source: every source key is in
// the copy with the same value, except a block the drop rule drops, computed
// again here, and its eviction entries; evictable-bytes holds the sum of the
// entries kept; the copy holds no other key.
func runVerify(o opts) error {
	// The sources must be as the copy found them, before and after.
	if err := checkStamps(o); err != nil {
		return err
	}
	if err := runVerifyBody(o); err != nil {
		return err
	}
	return checkStamps(o)
}

// runVerifyBody compares the new World with its sources.
func runVerifyBody(o opts) error {
	if o.read == "raw" {
		return runVerifyRaw(o)
	}
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
	table, _, err := kvTable(src)
	if err != nil {
		return err
	}
	dtable, _, err := kvTable(dst)
	if err != nil {
		return err
	}
	f, err := loadFacts(o, src, table, packs)
	if err != nil {
		return err
	}
	present, err := blockDigests(o, src, table)
	if err != nil {
		return err
	}
	p := planDrops(f, present)
	var av int64
	if err := dst.QueryRow("PRAGMA auto_vacuum").Scan(&av); err != nil {
		return err
	}

	var faults []string
	fault := func(s string, a ...any) {
		if len(faults) < 20 {
			faults = append(faults, fmt.Sprintf(s, a...))
		}
		if len(faults) == 20 {
			faults = append(faults, "...")
		}
	}
	if av != 2 {
		fault("auto_vacuum is %d, want 2", av)
	}
	srcStmt := ""
	if _, st, err := kvTable(src); err == nil {
		srcStmt = st
	}
	read, closeRead, err := valueReader(src, table, hasRowid(srcStmt))
	if err != nil {
		return err
	}
	defer closeRead()
	dget, err := dst.Prepare("SELECT length(value), value FROM " + dtable + " WHERE key = ?")
	if err != nil {
		return err
	}
	defer dget.Close()

	// Every source row: in the copy with an equal value (blocks by length,
	// or by bytes with -full), or left out by the rule.
	var checked, dropped int64
	sums := evictSums{}
	gotBytes := map[string][]byte{}
	err = scanPhysical(src, table, hasRowid(srcStmt), o.chunk, 0, func(id any, key []byte, vlen int64) error {
		var dlen sql.NullInt64
		var dval []byte
		err := dget.QueryRow(key).Scan(&dlen, &dval)
		if errors.Is(err, sql.ErrNoRows) {
			ok := false
			if bytes.HasPrefix(key, []byte(blockPrefix)) {
				if dg, parsed := digestOfBlockKey(key); parsed && f.verdict(dg) == vDrop {
					ok = true
				}
			} else if _, skip := p.skipEvict[string(key)]; skip {
				ok = true
			}
			if !ok {
				fault("missing from copy: %q (%s)", trunc(key), reason(f, key))
			}
			dropped++
			return nil
		}
		if err != nil {
			return err
		}
		checked++
		if _, skip := p.skipEvict[string(key)]; skip {
			fault("eviction entry of a dropped block kept: %q", trunc(key))
		}
		isBlock := bytes.HasPrefix(key, []byte(blockPrefix))
		if isBlock && !o.full {
			if dlen.Int64 != vlen {
				fault("block length differs: %q", trunc(key))
			}
			return nil
		}
		want, err := read(id)
		if err != nil {
			return err
		}
		if obj, rest, ok := objKey(key); ok && rest == evictBytesSeg {
			gotBytes[obj] = dval
			return nil
		}
		if !bytes.Equal(want, dval) {
			fault("value differs: %q", trunc(key))
		}
		sums.add(key, dval)
		return nil
	})
	if err != nil {
		return err
	}
	// evictable-bytes in the copy is the sum of the entries it kept.
	for obj, v := range gotBytes {
		if want := strconv.FormatInt(sums[obj], 10); string(v) != want {
			fault("%s evictable-bytes is %q, the kept entries sum to %s", obj, v, want)
		}
	}
	// The copy holds no key the source lacks: keys are unique, so its row
	// count must equal the matches.
	var dcount int64
	if err := dst.QueryRow("SELECT count(*) FROM " + dtable).Scan(&dcount); err != nil {
		return err
	}
	if dcount != checked {
		fault("copy holds %d rows, %d match the source", dcount, checked)
	}
	fmt.Printf("verify: %d keys equal, %d left out\n", checked, dropped)
	if len(faults) != 0 {
		for _, s := range faults {
			fmt.Println("  FAULT", s)
		}
		return fmt.Errorf("verify failed: %d faults", len(faults))
	}
	fmt.Println("verify: ok")
	return nil
}

// reason names why a missing key had to be kept.
func reason(f *facts, key []byte) string {
	if bytes.HasPrefix(key, []byte(blockPrefix)) {
		if d, ok := digestOfBlockKey(key); ok {
			return f.verdict(d)
		}
		return vUnparsed
	}
	return "non-block key"
}
