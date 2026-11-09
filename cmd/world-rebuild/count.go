package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"syscall"
	"time"
)

// countResult is what a dry run found; selftest checks it.
type countResult struct {
	// blockKeys counts the block keys by verdict (exact, with -blocks).
	blockKeys map[string]int64
	// blocks and meta are the sampled tallies, not scaled.
	blocks    map[string]*tally
	meta      map[string]*tally
	dropEvict tally
	scale     float64
}

// sliceStat is one sampled rowid slice.
type sliceStat struct {
	lo, hi      int64
	rows, bytes int64
	keptBlock   int64
	dropped     int64
}

func runCount(o opts) (*countResult, error) {
	start := time.Now()
	live = &pacer{target: o.txn, pause: o.pause, wals: []string{o.world + "-wal", o.packs + "-wal"}, walMax: o.walMax << 20}
	defer func() {
		fmt.Printf("pacer: %d reads, last size %d rows, busy %s, waited %s for the WAL\n", live.reads, live.rows, live.busy.Round(time.Second), live.waited.Round(time.Second))
		live = nil
	}()
	world, err := openRO(o.world)
	if err != nil {
		return nil, err
	}
	defer world.Close()
	packs, err := openRO(o.packs)
	if err != nil {
		return nil, err
	}
	defer packs.Close()
	table, stmt, err := kvTable(world)
	if err != nil {
		return nil, err
	}
	h, err := readHeader(world)
	if err != nil {
		return nil, err
	}
	fmt.Printf("world %s\n  table %s: %s\n  page_size %d page_count %d (%s) freelist %d (%s) auto_vacuum %d journal %s\n",
		o.world, table, stmt, h.pageSize, h.pageCount, gb(h.pageSize*h.pageCount), h.freelist, gb(h.pageSize*h.freelist), h.autoVacuum, h.journal)

	f, err := loadFacts(o, world, table, packs)
	if err != nil {
		return nil, err
	}
	res := &countResult{blockKeys: map[string]int64{}, blocks: map[string]*tally{}, meta: map[string]*tally{}}
	var stored int64
	for _, n := range f.evictBytesStored {
		stored += n
	}
	hotPacked := 0
	for d := range f.packed {
		if f.verdict(d) == vHot {
			hotPacked++
		}
	}
	fmt.Printf("\nfacts in %s (measured, exact):\n", f.took.Round(time.Second))
	fmt.Printf("  object stores %d; marked %d; queued blocks %d in %d entries; evictable-bytes as stored %s\n",
		len(f.objStores), len(f.marked), len(f.evict), f.evictKeys, gb(stored))
	fmt.Printf("  read set: %d hot keys\n", len(f.hot))
	for _, g := range f.gens {
		fmt.Println("    read", g)
	}
	fmt.Printf("  pack index: %d live packs; %d queued unmarked blocks looked up: %d packed, %d no h row, %d h row on a dead pack\n",
		f.packsLive, f.lookups, len(f.packed), f.hMissing, f.hDead)
	fmt.Printf("  drop candidates: %d packed, of which %d hot (kept)\n", len(f.packed), hotPacked)

	// Exact block counts from a key-only scan of the block range.
	if o.blocks {
		var n int64
		live.setPhase("block keys", func() string { return fmt.Sprintf("%d block keys", n) })
		err := scanKeys(world, table, []byte(blockPrefix), prefixEnd([]byte(blockPrefix)), o.chunk, o.pause, func(key []byte) error {
			n++
			res.blockKeys[f.blockVerdict(key)]++
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan block keys: %w", err)
		}
		fmt.Printf("\nblock keys (measured, exact, %d):\n", n)
		for _, v := range sortedKeys(res.blockKeys) {
			fmt.Printf("  %-44s %12d\n", v, res.blockKeys[v])
		}
	}

	if !hasRowid(stmt) {
		return nil, fmt.Errorf("count samples rowid slices; %s has no rowid", table)
	}
	slices, scale, err := sampleRowids(o, world, table, f, res)
	if err != nil {
		return nil, err
	}
	res.scale = scale
	sampleBusy := live.busy
	var sampleRows, sampleBytes int64
	for _, s := range slices {
		sampleRows += s.rows
		sampleBytes += s.bytes
	}
	if sampleRows == 0 {
		return nil, fmt.Errorf("sample read no rows")
	}
	_ = sampleBusy

	// Report, scaled to the whole table.
	est := func(b int64) int64 { return int64(float64(b) * scale) }
	fmt.Printf("\nsample: %d slices, %.2f%% of the rowid span, %d rows, %s payload; scale x%.1f (projected below)\n",
		len(slices), 100/scale, sampleRows, gb(sampleBytes), scale)
	fmt.Println("\nblock values (h/b/), projected keys and bytes:")
	var keepBytes, dropBytes, keepRows, hotBytes int64
	for _, v := range sortedKeys(res.blocks) {
		t := res.blocks[v]
		fmt.Printf("  %-44s %12d keys %12s\n", v, est(t.keys), gb(est(t.bytes)))
		switch v {
		case vDrop:
			dropBytes += t.bytes
		case vHot:
			hotBytes += t.bytes
			fallthrough
		default:
			keepBytes += t.bytes
			keepRows += t.keys
		}
	}
	fmt.Println("\nother keys by family, projected (kept, except the eviction entries of dropped blocks):")
	fams := sortedKeys(res.meta)
	sort.SliceStable(fams, func(i, j int) bool { return res.meta[fams[i]].bytes > res.meta[fams[j]].bytes })
	for i, k := range fams {
		t := res.meta[k]
		keepBytes += t.bytes
		keepRows += t.keys
		if i < 30 {
			fmt.Printf("  %-44s %12d keys %12s\n", k, est(t.keys), gb(est(t.bytes)))
		}
	}
	if len(fams) > 30 {
		fmt.Printf("  ... %d more families\n", len(fams)-30)
	}
	dropBytes += res.dropEvict.bytes
	fmt.Printf("  eviction entries of dropped blocks: %d keys %s (dropped)\n", est(res.dropEvict.keys), gb(est(res.dropEvict.bytes)))

	// Clustering: how the kept block bytes spread over the slices.
	reportClusters(slices)

	// Projection.
	keepB, dropB, keepN := est(keepBytes), est(dropBytes), est(keepRows)
	avgKey := 48.0
	used := (h.pageCount - h.freelist) * h.pageSize
	ratio := float64(used) / float64(max(est(sampleBytes), 1))
	// A fresh file: the table's cells and the key index, each key once
	// more plus about 24 bytes of cell and page overhead, pages 90% full.
	fresh := int64((float64(keepB) + float64(keepN)*(avgKey+24)) / 0.9)
	fmt.Printf("\nprojected: kept payload %s in %d keys (hot set %s), dropped %s\n", gb(keepB), keepN, gb(est(hotBytes)), gb(dropB))
	fmt.Printf("file %s = used %s + freelist %s; used pages per payload byte %.3f\n", gb(h.pageCount*h.pageSize), gb(used), gb(h.freelist*h.pageSize), ratio)
	fmt.Printf("projected new file: about %s fresh (payload, index, 90%% full pages); %s at the old file's ratio\n", gb(fresh), gb(int64(float64(keepB)*ratio)))

	// Copy rate on real rows, into a scratch file.
	scanRate := float64(sampleRows) / max(sampleBusy.Seconds(), 0.001)
	fmt.Printf("\nscan rate (measured, live, busy time only): %.0f rows/s, %s busy for %d rows\n", scanRate, sampleBusy.Round(time.Second), sampleRows)
	totalRows := est(sampleRows)
	scanTime := time.Duration(float64(totalRows) / scanRate * float64(time.Second))
	fmt.Printf("projected full rowid scan: %s for %d rows\n", scanTime.Round(time.Minute), totalRows)
	if o.rateOut != "" {
		mbps, err := measureCopy(o, world, table, stmt, h.pageSize, f, slices)
		if err != nil {
			return nil, fmt.Errorf("copy rate: %w", err)
		}
		if mbps <= 0 {
			fmt.Println("copy rate: no kept rows outside the sample to measure")
		} else {
			valueTime := time.Duration(float64(keepB) / 1e6 / mbps * float64(time.Second))
			fmt.Printf("projected copy: scan %s + kept values %s at %.0f MB/s = %s\n", scanTime.Round(time.Minute), valueTime.Round(time.Minute), mbps, (scanTime + valueTime).Round(time.Minute))
		}
	}

	for _, d := range []string{"/dev/shm", "/"} {
		var st syscall.Statfs_t
		if err := syscall.Statfs(d, &st); err == nil {
			free := int64(st.Bavail) * int64(st.Bsize)
			fit := "fits"
			if free < fresh {
				fit = "does NOT fit"
			}
			fmt.Printf("free on %s: %s; the projected new file %s\n", d, gb(free), fit)
		}
	}
	fmt.Printf("\ncount done in %s\n", time.Since(start).Round(time.Second))
	return res, nil
}

// sampleRowids reads o.sample of the rowid span, spread over o.slices
// slices, each read from its start. It tallies the rows into res and returns
// the slices and the scale from the sample to the whole span.
func sampleRowids(o opts, db *sql.DB, table string, f *facts, res *countResult) ([]sliceStat, float64, error) {
	var lo, hi sql.NullInt64
	if err := db.QueryRow("SELECT min(rowid), max(rowid) FROM "+table).Scan(&lo, &hi); err != nil {
		return nil, 0, err
	}
	if !lo.Valid {
		return nil, 1, nil
	}
	span := hi.Int64 - lo.Int64 + 1
	n := int64(max(o.slices, 1))
	n = min(n, span)
	width := span / n
	take := max(int64(float64(width)*o.sample), 1)
	if o.sample >= 1 {
		take = width
	}
	slices := make([]sliceStat, n)
	var sampled int64
	for i := range slices {
		s := &slices[i]
		s.lo = lo.Int64 + int64(i)*width
		s.hi = s.lo + take
		if o.sample >= 1 && i == len(slices)-1 {
			s.hi = hi.Int64 + 1
		}
		sampled += s.hi - s.lo
	}
	var rows int64
	cur := 0
	live.setPhase("rowid sample", func() string {
		return fmt.Sprintf("slice %d of %d, %d rows", cur, len(slices), rows)
	})
	for i := range slices {
		cur = i
		s := &slices[i]
		err := scanRowids(db, table, s.lo, s.hi, o.chunk, o.pause, func(_ int64, key []byte, vlen int64) error {
			rows++
			b := int64(len(key)) + vlen
			s.rows++
			s.bytes += b
			if bytes.HasPrefix(key, []byte(blockPrefix)) {
				v := f.blockVerdict(key)
				t := res.blocks[v]
				if t == nil {
					t = &tally{}
					res.blocks[v] = t
				}
				t.add(b)
				if v == vDrop {
					s.dropped += b
				} else {
					s.keptBlock += b
				}
				return nil
			}
			if d, ok := evictDigest(key); ok && f.verdict(d) == vDrop {
				res.dropEvict.add(b)
				s.dropped += b
				return nil
			}
			fam := family(key)
			t := res.meta[fam]
			if t == nil {
				t = &tally{}
				res.meta[fam] = t
			}
			t.add(b)
			return nil
		})
		if err != nil {
			return nil, 0, fmt.Errorf("sample slice %d: %w", i, err)
		}
	}
	return slices, float64(span) / float64(sampled), nil
}

// reportClusters prints how the kept block bytes spread over the slices: if a
// few slices hold most of them, a copy could skip the rest by rowid range.
func reportClusters(slices []sliceStat) {
	var kept, total int64
	empty := 0
	byKept := make([]int64, 0, len(slices))
	for _, s := range slices {
		kept += s.keptBlock
		total += s.bytes
		if s.keptBlock == 0 {
			empty++
		}
		byKept = append(byKept, s.keptBlock)
	}
	sort.Slice(byKept, func(i, j int) bool { return byKept[i] > byKept[j] })
	need := func(share float64) int {
		var sum int64
		for i, b := range byKept {
			sum += b
			if float64(sum) >= share*float64(kept) {
				return i + 1
			}
		}
		return len(byKept)
	}
	fmt.Printf("\nclustering: kept block bytes %s of %s sampled; %d of %d slices hold no kept block value\n", gb(kept), gb(total), empty, len(slices))
	if kept > 0 {
		fmt.Printf("  slices holding 50%% / 90%% / 99%% of the kept block bytes: %d / %d / %d of %d\n", need(0.5), need(0.9), need(0.99), len(slices))
	}
	// Kept share by tenth of the rowid span.
	const parts = 10
	var line string
	for p := range parts {
		var k, t int64
		for i := p * len(slices) / parts; i < (p+1)*len(slices)/parts; i++ {
			k += slices[i].bytes - slices[i].dropped
			t += slices[i].bytes
		}
		line += fmt.Sprintf(" %.0f%%", 100*float64(k)/float64(max(t, 1)))
	}
	fmt.Printf("  kept share of payload by tenth of the rowid span:%s\n", line)
}

// measureCopy copies the kept rows just past the sampled part of spread
// slices into a scratch file, as the copy does (key scan, then values by
// rowid), until o.rateMB megabytes are kept, and returns the kept MB/s over
// the time spent reading the kept values and writing them; the key scan is
// timed by the sample. The scratch file is removed.
func measureCopy(o opts, src *sql.DB, table, stmt string, pageSize int64, f *facts, slices []sliceStat) (float64, error) {
	dst, err := openNew(o.rateOut, stmt, pageSize)
	if err != nil {
		return 0, err
	}
	defer func() {
		dst.Close()
		for _, sfx := range []string{"", "-journal", "-wal", "-shm"} {
			_ = os.Remove(o.rateOut + sfx)
		}
	}()
	ins, err := dst.Prepare("INSERT INTO " + table + " (key, value) VALUES (?, ?)")
	if err != nil {
		return 0, err
	}
	defer ins.Close()
	var kept, rows int64
	var writeTime, readTime time.Duration
	limit := o.rateMB * 1e6
	live.setPhase("copy rate", func() string { return fmt.Sprintf("%s kept", gb(kept)) })
	// Visit slices in a spread order: every 7th, then the next offset.
	order := make([]int, 0, len(slices))
	for off := range 7 {
		for i := off; i < len(slices); i += 7 {
			order = append(order, i)
		}
	}
	for _, i := range order {
		if kept >= limit {
			break
		}
		s := slices[i]
		end := s.hi + (s.hi - s.lo)
		if i+1 < len(slices) {
			end = min(end, slices[i+1].lo)
		}
		var ids []int64
		err := scanRowids(src, table, s.hi, end, o.chunk, o.pause, func(id int64, key []byte, _ int64) error {
			rows++
			if bytes.HasPrefix(key, []byte(blockPrefix)) && f.blockVerdict(key) == vDrop {
				return nil
			}
			if d, ok := evictDigest(key); ok && f.verdict(d) == vDrop {
				return nil
			}
			ids = append(ids, id)
			return nil
		})
		if err != nil {
			return 0, err
		}
		type kv struct{ k, v []byte }
		var batch []kv
		busy0 := live.busy
		err = pointReads(src, "SELECT key, value FROM "+table+" WHERE rowid = ?", len(ids), o.chunk, func(j int, st *sql.Stmt) error {
			var k, v []byte
			if err := st.QueryRow(ids[j]).Scan(&k, &v); err != nil {
				if err == sql.ErrNoRows {
					return nil
				}
				return err
			}
			batch = append(batch, kv{k, v})
			return nil
		})
		if err != nil {
			return 0, err
		}
		readTime += live.busy - busy0
		t0 := time.Now()
		tx, err := dst.Begin()
		if err != nil {
			return 0, err
		}
		txins := tx.Stmt(ins)
		for _, e := range batch {
			if _, err := txins.Exec(e.k, e.v); err != nil {
				_ = tx.Rollback()
				return 0, err
			}
			kept += int64(len(e.k) + len(e.v))
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		writeTime += time.Since(t0)
	}
	el := readTime + writeTime
	mbps := float64(kept) / 1e6 / max(el.Seconds(), 0.001)
	fmt.Printf("value copy rate (measured, live source, scratch %s): %s kept from %d rows in %s (value reads %s, writes %s): %.0f MB/s\n",
		o.rateOut, gb(kept), rows, el.Round(time.Millisecond), readTime.Round(time.Millisecond), writeTime.Round(time.Millisecond), mbps)
	return mbps, nil
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
