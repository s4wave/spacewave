package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
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
}

// openNew creates the new World file: incremental auto_vacuum set and made
// to hold by a VACUUM before any table exists, as spacewave's
// setFreshAutoVacuum does, then the source's table, journal off for the build.
func openNew(path, stmt string, pageSize int64) (*sql.DB, error) {
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

// keepBlock applies the drop rule (and a mutant) to one block key.
func (p *dropPlan) keepBlock(key []byte) bool {
	d, ok := digestOfBlockKey(key)
	if !ok {
		return true
	}
	v := p.f.verdict(d)
	if p.hit == 0 {
		switch {
		case p.mutant == "drop-marked" && v == vMarked,
			p.mutant == "drop-unqueued" && v == vUnqueued,
			p.mutant == "drop-hot" && v == vHot,
			p.mutant == "drop-unpacked" && v == vUnpacked:
			p.hit++
			return false
		}
	}
	return v != vDrop
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
	f, err := loadFacts(o, src, table, packs)
	if err != nil {
		return err
	}
	present, err := blockDigests(o, src, table)
	if err != nil {
		return err
	}
	p := planDrops(f, present)
	p.mutant = o.mutant
	fmt.Printf("facts and block keys read in %s: %d blocks, %d eviction entries to drop\n", time.Since(start).Round(time.Second), len(present), len(p.skipEvict))
	h, err := readHeader(src)
	if err != nil {
		return err
	}

	dst, err := openNew(o.out, stmt, h.pageSize)
	if err != nil {
		return err
	}
	defer dst.Close()
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
		if pending >= 20000 {
			return commit()
		}
		return nil
	}
	read, closeRead, err := valueReader(src, table, hasRowid(stmt))
	if err != nil {
		return err
	}
	defer closeRead()

	last := time.Now()
	progress := func() {
		if time.Since(last) < 30*time.Second {
			return
		}
		last = time.Now()
		el := time.Since(start).Seconds()
		fmt.Printf("  %s kept %d keys %s, dropped %d keys %s, %.0f MB/s written\n", time.Now().UTC().Format("15:04:05"), st.keptKeys, gb(st.keptBytes), st.droppedKeys, gb(st.droppedBytes), float64(st.keptBytes)/1e6/el)
	}

	// One pass in physical order: decide each row from its key, then read
	// the values of the kept rows only.
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
			if _, rest, ok := objKey(k.key); ok && rest == evictBytesSeg {
				bytesKeys = append(bytesKeys, k.key)
			}
			sums.add(k.key, v)
			if err := put(k.key, v); err != nil {
				return err
			}
		}
		batch = batch[:0]
		return nil
	}
	err = scanPhysical(src, table, hasRowid(stmt), o.chunk, 0, func(id any, key []byte, vlen int64) error {
		progress()
		switch {
		case bytes.HasPrefix(key, []byte(blockPrefix)):
			if !p.keepBlock(key) {
				st.droppedKeys++
				st.droppedBytes += int64(len(key)) + vlen
				return nil
			}
		default:
			if _, skip := p.skipEvict[string(key)]; skip {
				st.droppedKeys++
				st.droppedBytes += int64(len(key)) + vlen
				st.droppedEvict++
				return nil
			}
			if p.mutant == "drop-meta" && p.hit == 0 && !strings.Contains(string(key), "/"+evictSeg) {
				p.hit++
				return nil
			}
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
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := commit(); err != nil {
		return err
	}

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
	fmt.Printf("copy done in %s: kept %d keys %s, dropped %d keys %s (%d eviction entries); new file %s, auto_vacuum %d, %.0f MB/s kept\n",
		el.Round(time.Second), st.keptKeys, gb(st.keptBytes), st.droppedKeys, gb(st.droppedBytes), st.droppedEvict,
		gb(h.pageCount*h.pageSize), h.autoVacuum, float64(st.keptBytes)/1e6/el.Seconds())
	return nil
}

// runVerify checks the new file against the source: every source key is in
// the copy with the same value, except a block the drop rule drops, computed
// again here, and its eviction entries; evictable-bytes holds the sum of the
// entries kept; the copy holds no other key.
func runVerify(o opts) error {
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
