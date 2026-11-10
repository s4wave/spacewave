package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// countResult is what a dry run found; selftest checks it.
type countResult struct {
	blocks    map[string]*tally
	meta      map[string]*tally
	dropEvict tally
	badCells  int64
	census    census
	// nullValues counts the World rows whose value is NULL.
	nullValues int64
}

// rawEvict is an eviction entry seen by the raw pass.
type rawEvict struct {
	rowid, bytes, size int64
}

// rawBlock is a block value seen by the raw pass.
type rawBlock struct {
	d            string
	rowid, bytes int64
}

// rowidBucketBits sizes the rowid buckets of the clustering report.
const rowidBucketBits = 20

// runCount is the dry run. It reads the World file and the pack index page by
// page without SQLite (rawFile), so it holds no lock on the live files and
// cannot hold back their checkpointer; the pass is paced to -mbps and waits
// while either WAL is past -wal-max-mib. Its numbers describe the main files
// as of their last checkpoint: rows still only in a WAL are not seen.
func runCount(o opts) (*countResult, error) {
	start := time.Now()
	live = &pacer{wals: []string{o.world + "-wal", o.packs + "-wal"}, walMax: o.walMax << 20}
	defer func() {
		fmt.Printf("pacer: waited %s for the WAL\n", live.waited.Round(time.Second))
		live = nil
	}()
	world, err := openRaw(o.world)
	if err != nil {
		return nil, err
	}
	defer world.Close()
	packs, err := openRaw(o.packs)
	if err != nil {
		return nil, err
	}
	defer packs.Close()
	var freePages int64
	for _, w := range world.free {
		freePages += int64(popcount(w))
	}
	worldSize := world.pages * int64(world.pageSize)
	fmt.Printf("world %s: %d pages of %d (%s), %d free (%s), direct reads %v\n",
		o.world, world.pages, world.pageSize, gb(worldSize), freePages, gb(freePages*int64(world.pageSize)), world.direct)

	res := &countResult{blocks: map[string]*tally{}, meta: map[string]*tally{}}

	// Report the World's schema, read through an immutable open, which
	// takes no lock. Schema copy does not carry refuses the count, as it
	// refuses the copy.
	schemaErr := fmt.Errorf("World schema unread; refusing")
	if sdb, err := openImmutable(o.world); err == nil {
		rows, err := schemaRows(sdb)
		fmt.Printf("World schema (sqlite_master, as of the last checkpoint): %v %v\n", rows, err)
		if table, _, terr := kvTable(sdb); terr != nil {
			schemaErr = fmt.Errorf("World kv table: %w; refusing", terr)
		} else {
			schemaErr = onlyKVTable(sdb, table)
		}
		sdb.Close()
	} else {
		schemaErr = fmt.Errorf("World schema unread: %w; refusing", err)
	}

	// -needed-out is written aside and renamed into place only when the
	// count does not refuse.
	neededTmp := ""
	if o.neededOut != "" {
		neededTmp = o.neededOut + ".tmp"
		defer os.Remove(neededTmp)
	}
	f := &facts{
		marked:           map[string]struct{}{},
		evict:            map[string][]evictEntry{},
		hashTypes:        map[string]hash.HashType{},
		hot:              map[[readKeyLen]byte]struct{}{},
		packed:           map[string]struct{}{},
		objStores:        map[string]string{},
		evictBytesStored: map[string]int64{},
		otherFacts:       map[string]int{},
	}
	evicts := map[string][]rawEvict{}
	var blocks []rawBlock
	type readRef struct {
		c      rawCell
		vlen   int64
		vlocal []byte
	}
	gens := map[string]map[string][]readRef{}
	keptBy := map[int64]int64{}
	totalBy := map[int64]int64{}
	var maxRowid, rows, readBytes int64
	var objBytes tally

	// Pass 1: every row of the World table, from its leaf pages.
	live.setPhase("world pages", func() string {
		return fmt.Sprintf("%.1f%% read, %d rows, %d blocks", 100*float64(readBytes)/float64(worldSize), rows, len(blocks))
	})
	wst, err := world.scanLeaves(o.mbps, 4<<20, &readBytes, func(c rawCell) {
		key, vlen, vlocal, ok := kvRecord(c)
		if ok && kvValueNull(c) {
			res.nullValues++
		}
		if !ok {
			res.badCells++
			return
		}
		rows++
		maxRowid = max(maxRowid, c.rowid)
		b := int64(len(key)) + vlen
		bucket := c.rowid >> rowidBucketBits
		totalBy[bucket] += b
		if bytes.HasPrefix(key, []byte(blockPrefix)) {
			d, ok := digestOfBlockKey(key)
			if !ok {
				res.addBlock(vUnparsed, b)
				keptBy[bucket] += b
				return
			}
			blocks = append(blocks, rawBlock{d: d, rowid: c.rowid, bytes: b})
			return
		}
		if k := objKeyOf(key); k.ok {
			objBytes.add(b)
			res.census.add(key, k, vlen, vlocal)
			if k.kind == kindBad || k.kind == kindForeign {
				if k.kind == kindBad {
					f.badKeys++
				}
				res.addMeta(family(key), b)
				keptBy[bucket] += b
				return
			}
			obj := k.obj
			if k.kind == kindCopy || k.kind == kindCopyDone {
				f.noteCopy(k)
			}
			if (k.kind == kindMarked || k.kind == kindEvict || k.kind == kindReads) && !ownStore(o, obj) {
				if k.kind != kindReads {
					f.otherFacts[obj]++
				}
				res.addMeta(family(key), b)
				keptBy[bucket] += b
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
				d := k.digest
				var size int64
				if int64(len(vlocal)) == vlen {
					size, _ = evictSize(vlocal)
				}
				f.hashTypes[d] = k.hashType
				f.evict[d] = append(f.evict[d], evictEntry{obj: obj})
				evicts[d] = append(evicts[d], rawEvict{rowid: c.rowid, bytes: b, size: size})
				f.evictKeys++
				// Tallied once the drop rule has decided its block.
				return
			case kindReads:
				if gens[obj] == nil {
					gens[obj] = map[string][]readRef{}
				}
				gens[obj][k.gen] = append(gens[obj][k.gen], readRef{c: rawCell{rowid: c.rowid, size: c.size, overflow: c.overflow}, vlen: vlen, vlocal: bytes.Clone(vlocal)})
			case kindBytes:
				n, _ := evictSize(vlocal)
				f.evictBytesStored[obj] = n
			}
		}
		res.addMeta(family(key), b)
		keptBy[bucket] += b
	})
	if err != nil {
		return nil, fmt.Errorf("world pages: %w", err)
	}
	res.badCells += wst.badCells
	fmt.Printf("world pass (measured): %s read in %s (%.0f MB/s, paced to %.0f), %d leaf pages, %d free pages skipped, %d rows, %d cells did not decode\n",
		gb(wst.read), wst.took.Round(time.Second), float64(wst.read)/1e6/wst.took.Seconds(), o.mbps, wst.leaves, wst.free, rows, res.badCells)

	// The two newest read generations of each object store, read through
	// their overflow chains.
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
			n := 0
			for _, ref := range g[s] {
				v, err := world.value(ref.c, ref.vlen, ref.vlocal)
				if err != nil {
					fmt.Printf("warning: read chunk at rowid %d: %v; its keys are not counted hot\n", ref.c.rowid, err)
					continue
				}
				for ; len(v) >= readKeyLen; v = v[readKeyLen:] {
					f.hot[[readKeyLen]byte(v[:readKeyLen])] = struct{}{}
					n++
				}
			}
			f.gens = append(f.gens, fmt.Sprintf("%s gen %s: %d chunks, %d keys", obj, at.Format(time.RFC3339), len(g[s]), n))
		}
	}

	// Pass 2: the pack index. Live packs from p/ rows, and the h rows of
	// the queued, unmarked blocks.
	pre := packIndexPrefix(o.storeID)
	pp := append(bytes.Clone(pre), "p/"...)
	hp := append(bytes.Clone(pre), 'h')
	cands := map[[indexKeyBytes]byte]string{}
	for d := range f.evict {
		if _, marked := f.marked[d]; !marked {
			cands[packRowKey(d, f.hashTypes[d])] = d
		}
	}
	livePacks := map[uint32]struct{}{}
	// With -needed-out, the pack ids and the sizes their e/ rows record.
	packIDs := map[uint32]string{}
	packSizes := map[string]int64{}
	ep := append(bytes.Clone(pre), "e/"...)
	var badEntries int64
	hnum := map[string]uint32{}
	var hRows, packRead int64
	live.setPhase("pack index pages", func() string { return fmt.Sprintf("%d h rows, %d matched", hRows, len(hnum)) })
	pst, err := packs.scanLeaves(o.mbps, 4<<20, &packRead, func(c rawCell) {
		key, vlen, v, ok := kvRecord(c)
		if !ok || int64(len(v)) != vlen {
			return
		}
		switch {
		case bytes.HasPrefix(key, pp):
			if len(v) >= 4 {
				livePacks[binary.BigEndian.Uint32(v)] = struct{}{}
				if o.neededOut != "" {
					packIDs[binary.BigEndian.Uint32(v)] = string(key[len(pp):])
				}
			}
		case o.neededOut != "" && bytes.HasPrefix(key, ep):
			e := &packfile.PackfileEntry{}
			if e.UnmarshalVT(v) != nil {
				badEntries++
				return
			}
			packSizes[string(key[len(ep):])] = int64(e.GetSizeBytes())
		case len(key) == len(hp)+indexKeyBytes && bytes.HasPrefix(key, hp):
			hRows++
			if d, ok := cands[[indexKeyBytes]byte(key[len(hp):])]; ok && len(v) >= 4 {
				hnum[d] = binary.BigEndian.Uint32(v)
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("pack index pages: %w", err)
	}
	f.packsLive = len(livePacks)
	for _, d := range cands {
		f.lookups++
		num, ok := hnum[d]
		if !ok {
			f.hMissing++
			continue
		}
		if _, isLive := livePacks[num]; !isLive {
			f.hDead++
			continue
		}
		f.packed[d] = struct{}{}
	}
	fmt.Printf("pack index pass (measured): %s in %s, %d h rows, %d cells did not decode\n", gb(pst.read), pst.took.Round(time.Second), hRows, pst.badCells)

	// Classify the blocks, then their eviction entries.
	dropped := map[string]struct{}{}
	for _, b := range blocks {
		v := f.verdict(b.d)
		res.addBlock(v, b.bytes)
		if v == vDrop {
			dropped[b.d] = struct{}{}
		} else {
			keptBy[b.rowid>>rowidBucketBits] += b.bytes
		}
	}
	if o.neededOut != "" {
		if err := writeCountNeeded(neededTmp, dropped, hnum, packIDs, packSizes, badEntries); err != nil {
			return nil, err
		}
	}
	var evictKept int64
	for d, es := range evicts {
		_, drop := dropped[d]
		for _, e := range es {
			if drop {
				res.dropEvict.add(e.bytes)
				continue
			}
			res.addMeta("objs/<id>/evictable/", e.bytes)
			keptBy[e.rowid>>rowidBucketBits] += e.bytes
			evictKept += e.size
		}
	}

	// Report.
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
	fmt.Printf("\nfacts (measured, main files as of their last checkpoint):\n")
	fmt.Printf("  object stores %d (%d keys, %s); marked %d; queued blocks %d in %d entries\n",
		len(f.objStores), objBytes.keys, gb(objBytes.bytes), len(f.marked), len(f.evict), f.evictKeys)
	fmt.Printf("  evictable-bytes as stored %s; after the drop the kept entries sum to %s\n", gb(stored), gb(evictKept))
	fmt.Printf("  read set: %d hot keys\n", len(f.hot))
	for _, g := range f.gens {
		fmt.Println("    read", g)
	}
	fmt.Printf("  pack index: %d live packs; %d queued unmarked blocks: %d packed, %d no h row, %d h row on a dead pack\n",
		f.packsLive, f.lookups, len(f.packed), f.hMissing, f.hDead)
	fmt.Printf("  drop candidates: %d packed, of which %d hot (kept); %d object store keys did not parse\n", len(f.packed), hotPacked, f.badKeys)
	fmt.Printf("marked blocks (marked/ rows): %d\n", len(f.marked))
	fmt.Printf("NULL values (rows of the World table whose value is NULL, kept NULL by copy): %d\n", res.nullValues)
	fmt.Printf("read set keys per generation:\n")
	for _, g := range f.gens {
		fmt.Println("  ", g)
	}
	res.census.print()

	fmt.Println("\nblock values (h/b/), measured:")
	var keepBytes, dropBytes, keepRows, hotBytes, hotKeys int64
	for _, v := range sortedKeys(res.blocks) {
		t := res.blocks[v]
		fmt.Printf("  %-44s %12d keys %12s\n", v, t.keys, gb(t.bytes))
		switch v {
		case vDrop:
			dropBytes += t.bytes
		case vHot:
			hotBytes, hotKeys = t.bytes, t.keys
			fallthrough
		default:
			keepBytes += t.bytes
			keepRows += t.keys
		}
	}
	fmt.Println("\nother keys by family, measured (kept, except the eviction entries of dropped blocks):")
	fams := sortedKeys(res.meta)
	sort.SliceStable(fams, func(i, j int) bool { return res.meta[fams[i]].bytes > res.meta[fams[j]].bytes })
	for i, k := range fams {
		t := res.meta[k]
		keepBytes += t.bytes
		keepRows += t.keys
		if i < 30 {
			fmt.Printf("  %-44s %12d keys %12s\n", k, t.keys, gb(t.bytes))
		}
	}
	if len(fams) > 30 {
		fmt.Printf("  ... %d more families\n", len(fams)-30)
	}
	dropBytes += res.dropEvict.bytes
	fmt.Printf("  eviction entries of dropped blocks: %d keys %s (dropped)\n", res.dropEvict.keys, gb(res.dropEvict.bytes))

	reportClusters(keptBy, totalBy)

	// Projection: the payload, each key once more in the key index, about
	// 24 bytes of cell and index overhead a row, pages 90% full.
	fresh := int64((float64(keepBytes) + float64(keepRows)*(48+24)) / 0.9)
	fmt.Printf("\nkept payload %s in %d keys (hot set %d keys %s); dropped %s\n", gb(keepBytes), keepRows, hotKeys, gb(hotBytes), gb(dropBytes))
	fmt.Printf("projected new file: about %s\n", gb(fresh))
	for _, d := range []string{"/dev/shm", "/"} {
		var sfs syscall.Statfs_t
		if err := syscall.Statfs(d, &sfs); err == nil {
			free := int64(sfs.Bavail) * int64(sfs.Bsize)
			fit := "fits"
			if free < fresh {
				fit = "does NOT fit"
			}
			fmt.Printf("free on %s: %s; the projected new file %s\n", d, gb(free), fit)
		}
	}

	if o.rateOut != "" {
		if err := measureCopy(o, f, maxRowid, rows, keepBytes); err != nil {
			fmt.Printf("copy probe: %v\n", err)
		}
	}
	fmt.Printf("\ncount done in %s\n", time.Since(start).Round(time.Second))
	if f.badKeys != 0 {
		// The report stands, but its verdicts do not: an unparsed
		// marker or read set key would leave its block droppable.
		return res, fmt.Errorf("%d object store keys did not parse as write-back store keys; the drop verdicts are not trustworthy", f.badKeys)
	}
	if err := otherFactsErr(f); err != nil {
		return res, err
	}
	if err := unfinishedCopiesErr(f); err != nil {
		return res, err
	}
	if schemaErr != nil {
		return res, schemaErr
	}
	if neededTmp != "" {
		if err := os.Rename(neededTmp, o.neededOut); err != nil {
			return res, err
		}
		fmt.Printf("needed packs: %s renamed to %s\n", neededTmp, o.neededOut)
	}
	return res, nil
}

func (r *countResult) addBlock(v string, b int64) {
	t := r.blocks[v]
	if t == nil {
		t = &tally{}
		r.blocks[v] = t
	}
	t.add(b)
}

func (r *countResult) addMeta(fam string, b int64) {
	t := r.meta[fam]
	if t == nil {
		t = &tally{}
		r.meta[fam] = t
	}
	t.add(b)
}

func popcount(w uint64) int {
	n := 0
	for ; w != 0; w &= w - 1 {
		n++
	}
	return n
}

// reportClusters prints how the kept bytes spread over rowid buckets: if
// few buckets hold most of them, a copy could skip the rest by rowid range.
func reportClusters(keptBy, totalBy map[int64]int64) {
	var ids []int64
	var kept int64
	for id := range totalBy {
		ids = append(ids, id)
		kept += keptBy[id]
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	byKept := make([]int64, 0, len(ids))
	sparse := 0
	for _, id := range ids {
		byKept = append(byKept, keptBy[id])
		if keptBy[id]*20 < totalBy[id] {
			sparse++
		}
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
	fmt.Printf("\nclustering by rowid bucket (%d rowids each): %d buckets; %d keep under 5%% of their payload\n", 1<<rowidBucketBits, len(ids), sparse)
	if kept > 0 {
		fmt.Printf("  buckets holding 50%% / 90%% / 99%% of the kept bytes: %d / %d / %d of %d\n", need(0.5), need(0.9), need(0.99), len(ids))
	}
	const parts = 10
	line := ""
	for p := range parts {
		var k, t int64
		for _, id := range ids[p*len(ids)/parts : (p+1)*len(ids)/parts] {
			k += keptBy[id]
			t += totalBy[id]
		}
		line += fmt.Sprintf(" %.0f%%", 100*float64(k)/float64(max(t, 1)))
	}
	fmt.Printf("  kept share of payload by tenth of the rowid span:%s\n", line)
}

// openImmutable opens a SQLite file as immutable: SQLite reads the main file
// only, takes no lock and touches no WAL or shared memory file, so a read of a
// live file cannot hold its checkpointer. It sees the last checkpoint.
func openImmutable(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?immutable=1")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// measureCopy runs the copy's pipeline on rowid ranges spread over the table,
// reading an immutable view of the source and writing a scratch file, until
// o.rateMB megabytes are kept. It times the key scan per row and the value
// reads and writes per kept byte, and projects the copy from them. The
// scratch file is removed.
func measureCopy(o opts, f *facts, maxRowid, rows, keepBytes int64) error {
	src, err := openImmutable(o.world)
	if err != nil {
		return err
	}
	defer src.Close()
	table, stmt, err := kvTable(src)
	if err != nil {
		return err
	}
	h, err := readHeader(src)
	if err != nil {
		return err
	}
	dst, err := openNew(o.rateOut, stmt, h.pageSize, false)
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		for _, sfx := range []string{"", "-journal", "-wal", "-shm"} {
			_ = os.Remove(o.rateOut + sfx)
		}
	}()
	ins, err := dst.Prepare("INSERT INTO " + table + " (key, value) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer ins.Close()
	get, err := src.Prepare("SELECT key, value FROM " + table + " WHERE rowid = ?")
	if err != nil {
		return err
	}
	defer get.Close()

	saved := live
	live = nil
	defer func() { live = saved }()
	var kept, scanned int64
	var scanT, readT, writeT time.Duration
	limit := o.rateMB * 1e6
	const ranges = 64
	step := max(maxRowid/ranges, 1)
	width := max(step/50, 1000)
	for i := int64(0); i < ranges && kept < limit; i++ {
		lo := i*step + step/3
		var ids []int64
		t0 := time.Now()
		err := scanRowids(src, table, lo, lo+width, 5000, 0, func(id int64, key []byte, _ int64) error {
			scanned++
			if bytes.HasPrefix(key, []byte(blockPrefix)) && f.blockVerdict(key) == vDrop {
				return nil
			}
			if d, ok := evictDigest(key); ok && f.verdict(d) == vDrop {
				return nil
			}
			ids = append(ids, id)
			return nil
		})
		scanT += time.Since(t0)
		if err != nil {
			return fmt.Errorf("scan range %d: %w", lo, err)
		}
		tx, err := dst.Begin()
		if err != nil {
			return err
		}
		txins := tx.Stmt(ins)
		for _, id := range ids {
			r0 := time.Now()
			var k, v []byte
			if err := get.QueryRow(id).Scan(&k, &v); err != nil {
				continue
			}
			w0 := time.Now()
			readT += w0.Sub(r0)
			if _, err := txins.Exec(k, v); err != nil {
				_ = tx.Rollback()
				return err
			}
			writeT += time.Since(w0)
			kept += int64(len(k) + len(v))
		}
		w0 := time.Now()
		if err := tx.Commit(); err != nil {
			return err
		}
		writeT += time.Since(w0)
	}
	if scanned == 0 || kept == 0 {
		return fmt.Errorf("nothing kept in the probe ranges")
	}
	perRow := scanT / time.Duration(scanned)
	perMB := float64(readT+writeT) / (float64(kept) / 1e6)
	fmt.Printf("\ncopy probe (measured, live disk, immutable view, scratch %s): %d rows scanned in %s (%s a row); %s kept, value reads %s, writes %s: %.0f MB/s kept\n",
		o.rateOut, scanned, scanT.Round(time.Millisecond), perRow, gb(kept), readT.Round(time.Millisecond), writeT.Round(time.Millisecond), 1e9/perMB)
	scanAll := time.Duration(rows) * perRow
	valueAll := time.Duration(perMB * float64(keepBytes) / 1e6)
	fmt.Printf("projected copy at the probe's rates: scan %s + kept values %s = %s (verify reads as much again)\n",
		scanAll.Round(time.Minute), valueAll.Round(time.Minute), (scanAll + valueAll).Round(time.Minute))
	return nil
}

// runRateOnly measures the copy rate without count's passes. It reads 64
// slices spread over the World file raw, as count does (no SQLite, so no lock
// and no read transaction), keeps every row of their leaf pages with its whole
// value, and writes them to -rate-out as copy does: keeping every row makes the
// rate pessimistic. Reads, overflow pages included, are paced to -mbps, and
// the pacer waits while a WAL is past -wal-max-mib.
func runRateOnly(o opts) error {
	// Open the source raw and the scratch copy, removed after.
	if o.rateOut == "" {
		return fmt.Errorf("-rate-only needs -rate-out")
	}
	world, err := openRaw(o.world)
	if err != nil {
		return err
	}
	defer world.Close()
	dst, err := openNew(o.rateOut, "CREATE TABLE kv ( key BLOB PRIMARY KEY, value BLOB)", int64(world.pageSize), false)
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		for _, sfx := range []string{"", "-journal", "-wal", "-shm"} {
			_ = os.Remove(o.rateOut + sfx)
		}
	}()

	// Guard both WALs, as count does.
	live = &pacer{wals: []string{o.world + "-wal", o.packs + "-wal"}, walMax: o.walMax << 20}
	defer func() { live = nil }()
	live.setPhase("rate sample", nil)

	// Spread the sample over 64 slices, each a share of -rate-mb.
	const slices, step = 64, 4 << 20
	size, align, quota := world.pages*int64(world.pageSize), int64(max(world.pageSize, 4096)), o.rateMB*1e6/slices
	var sampled, read, rows, bad int64
	var slept time.Duration
	var werr error
	start := time.Now()

	// Copy each slice in one transaction until its share is sampled.
	for i := range int64(slices) {
		tx, err := dst.Begin()
		if err != nil {
			return err
		}
		ins, err := tx.Prepare("INSERT OR IGNORE INTO kv (key, value) VALUES (?, ?)")
		if err != nil {
			return err
		}
		end := (i + 1) * size / slices / align * align
		for off := i * size / slices / align * align; off < end && sampled < (i+1)*quota; off += step {
			// Copy the rows of one chunk, then sleep to the pace.
			st, err := world.scanRange(off, min(off+step, end), 0, step, nil, func(c rawCell) {
				// Read the whole value and write the row.
				key, vlen, vlocal, ok := kvRecord(c)
				v, err := world.value(c, vlen, vlocal)
				if !ok || err != nil {
					bad++
					return
				}

				// Count the overflow pages read toward the pace.
				read += (c.size - int64(len(c.local)) + int64(world.usable) - 5) / int64(world.usable-4) * int64(world.pageSize)
				if _, err := ins.Exec(key, v); err != nil && werr == nil {
					werr = err
				}
				rows++
				sampled += int64(len(key)) + vlen
			})
			if err != nil {
				_ = tx.Rollback()
				return err
			}

			// Hold the reads to -mbps.
			read += st.read
			if want := time.Duration(float64(read) / (o.mbps * 1e6) * float64(time.Second)); o.mbps > 0 && time.Since(start) < want {
				slept += want - time.Since(start)
				time.Sleep(want - time.Since(start))
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	// Report one line: the measured rate, then with pacing taken out.
	el := time.Since(start)
	if werr != nil || sampled == 0 {
		return fmt.Errorf("rate-only: %d rows written: %v", rows, werr)
	}
	rate := float64(sampled) / 1e6 / el.Seconds()
	free := float64(sampled) / 1e6 / max(el-slept-live.waited, time.Millisecond).Seconds()
	fmt.Printf("rate-only (measured): sampled %s in %d rows (64 slices, every row kept, %d cells skipped) in %s: %.0f MB/s to %s, %.0f MB/s without %s of pacing to %.0f MB/s and WAL waits; 195 GB at that rate: %.0f min (%.0f min unpaced)\n",
		gb(sampled), rows, bad, el.Round(time.Millisecond), rate, o.rateOut, free, (slept + live.waited).Round(time.Millisecond), o.mbps, 195e3/rate/60, 195e3/free/60)
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// runRateSplit measures the two halves of the copy rate apart. It reads 64
// raw slices of the World file as rate-only does, holding the rows in memory,
// so the read rate is the scan alone. Then, with the World file no longer
// read, it inserts the rows into -rate-out as copy does (copy's pragmas, 20000
// rows a transaction, in physical order), and again sorted by key into
// -rate-out.sorted, each file removed after.
func runRateSplit(o opts) error {
	// Open the source raw.
	if o.rateOut == "" {
		return fmt.Errorf("-rate-split needs -rate-out")
	}
	world, err := openRaw(o.world)
	if err != nil {
		return err
	}
	defer world.Close()
	live = &pacer{wals: []string{o.world + "-wal", o.packs + "-wal"}, walMax: o.walMax << 20}
	defer func() { live = nil }()
	live.setPhase("rate split", nil)

	// Read the slices into memory, paced to -mbps.
	type row struct{ key, value []byte }
	const slices, step = 64, 4 << 20
	size, align, quota := world.pages*int64(world.pageSize), int64(max(world.pageSize, 4096)), o.rateMB*1e6/slices
	var rows []row
	var sampled, read, bad int64
	var slept time.Duration
	start := time.Now()
	for i := range int64(slices) {
		end := (i + 1) * size / slices / align * align
		for off := i * size / slices / align * align; off < end && sampled < (i+1)*quota; off += step {
			st, err := world.scanRange(off, min(off+step, end), 0, step, nil, func(c rawCell) {
				// Keep the row with its whole value.
				key, vlen, vlocal, ok := kvRecord(c)
				v, err := world.value(c, vlen, vlocal)
				if !ok || err != nil {
					bad++
					return
				}
				read += (c.size - int64(len(c.local)) + int64(world.usable) - 5) / int64(world.usable-4) * int64(world.pageSize)
				rows = append(rows, row{bytes.Clone(key), v})
				sampled += int64(len(key)) + vlen
			})
			if err != nil {
				return err
			}
			read += st.read
			if want := time.Duration(float64(read) / (o.mbps * 1e6) * float64(time.Second)); o.mbps > 0 && time.Since(start) < want {
				slept += want - time.Since(start)
				time.Sleep(want - time.Since(start))
			}
		}
	}
	scan := time.Since(start)
	unpaced := max(scan-slept-live.waited, time.Millisecond)
	fmt.Printf("rate-split read (measured): %s of rows (%d rows, %d cells skipped) from %s of pages in %s: %.0f MB/s of pages, %.0f MB/s without %s of pacing to %.0f MB/s and WAL waits\n",
		gb(sampled), len(rows), bad, gb(read), scan.Round(time.Millisecond), float64(read)/1e6/scan.Seconds(), float64(read)/1e6/unpaced.Seconds(), (slept + live.waited).Round(time.Millisecond), o.mbps)
	world.Close()

	// Insert the rows into a new file as copy does, and report the rate.
	insert := func(path, order string) error {
		dst, err := openNew(path, "CREATE TABLE kv ( key BLOB PRIMARY KEY, value BLOB)", int64(world.pageSize), false)
		if err != nil {
			return err
		}
		defer func() {
			dst.Close()
			for _, sfx := range []string{"", "-journal", "-wal", "-shm"} {
				_ = os.Remove(path + sfx)
			}
		}()
		t0 := time.Now()
		for i := 0; i < len(rows); i += 20000 {
			tx, err := dst.Begin()
			if err != nil {
				return err
			}
			ins, err := tx.Prepare("INSERT OR IGNORE INTO kv (key, value) VALUES (?, ?)")
			if err != nil {
				return err
			}
			for _, r := range rows[i:min(i+20000, len(rows))] {
				if _, err := ins.Exec(r.key, r.value); err != nil {
					_ = tx.Rollback()
					return err
				}
			}
			ins.Close()
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		el := time.Since(t0)
		var fsz int64
		if fi, err := os.Stat(path); err == nil {
			fsz = fi.Size()
		}
		fmt.Printf("rate-split insert %s (measured): %d rows, %s in %s: %.0f rows/s, %.0f MB/s; file %s\n",
			order, len(rows), gb(sampled), el.Round(time.Millisecond), float64(len(rows))/el.Seconds(), float64(sampled)/1e6/el.Seconds(), gb(fsz))
		return nil
	}
	if err := insert(o.rateOut, "physical order"); err != nil {
		return err
	}
	sort.Slice(rows, func(a, b int) bool { return bytes.Compare(rows[a].key, rows[b].key) < 0 })
	return insert(o.rateOut+".sorted", "key order")
}

// writeCountNeeded writes the packs the dropped blocks' h rows name, with
// the sizes their e/ rows record, as packcheck -needed-out does: the list
// packcheck -needed checks a listing against before the downtime. A pack with
// no e/ row is written with size -1, which packcheck counts as a fault; the
// header records the dropped blocks with no h row or a pack number no p/ row
// holds, which packcheck -needed counts as faults too.
func writeCountNeeded(path string, dropped map[string]struct{}, hnum map[string]uint32, ids map[uint32]string, sizes map[string]int64, badEntries int64) error {
	byID := map[string]*neededPack{}
	var hdr neededHeader
	for d := range dropped {
		num, ok := hnum[d]
		if !ok {
			hdr.noRow++
			continue
		}
		id, ok := ids[num]
		if !ok {
			hdr.deadPack++
			continue
		}
		p := byID[id]
		if p == nil {
			size, ok := sizes[id]
			if !ok {
				size = -1
			}
			p = &neededPack{id: id, size: size}
			byID[id] = p
		}
		p.blocks++
	}
	packs := make([]neededPack, 0, len(byID))
	noEntry := 0
	for _, p := range byID {
		packs = append(packs, *p)
		if p.size < 0 {
			noEntry++
		}
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].id < packs[j].id })
	if err := writeNeeded(path, packs, hdr); err != nil {
		return err
	}
	fmt.Printf("needed packs (measured, as of the main files' last checkpoint): %d dropped blocks in %d packs, %d with no e/ row, %d e/ rows did not decode, %d dropped blocks with no h row, %d naming a pack no p/ row holds; written to %s\n",
		len(dropped), len(packs), noEntry, badEntries, hdr.noRow, hdr.deadPack, path)
	return nil
}
