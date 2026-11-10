package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// packCheckResult is what packcheck found.
type packCheckResult struct {
	// dropped is the blocks the old World holds and the new one lacks.
	dropped int64
	// noRow and deadPack are dropped blocks with no h row, or one naming a
	// pack number no p/ row holds.
	noRow, deadPack int64
	// needed is the packs the dropped blocks' h rows name; noEntry of them
	// have no e/ row to give their size.
	needed, noEntry int64
	// missing and mismatch are needed packs the listing lacks, or lists at
	// another size; entriesMissing are needed packs whose entries/ object the
	// listing lacks, checked when it lists any entries/ object.
	missing, mismatch, entriesMissing int64
}

func (r packCheckResult) faults() int64 {
	return r.noRow + r.deadPack + r.noEntry + r.missing + r.mismatch + r.entriesMissing
}

// neededPack is one pack the dropped blocks need.
type neededPack struct {
	id     string
	size   int64
	blocks int64
}

// runPackCheck is the gate on removing the old World: every block the copy
// dropped must be in a pack object the bucket holds at the size the pack
// index records.
//
// The dropped set is the blocks whose key the old World (-world) holds and
// the new one (-out) lacks: what the copy left out, plus any block the
// backend evicted from the new file since, which is as much in need of its
// pack. Each block's h row in the pack index (-packs) names a pack number, the
// p/ rows name its id and the e/ row its size. The packs are checked against
// -list, lines of size and key as `rclone lsf -R --format sp` writes them:
// the key's last segment is the pack id, under packs/ (or with no directory)
// for a pack object and under entries/ for its entry. Nothing is downloaded.
//
// The old World is read as copy reads it, so it must be offline. The new
// World and the pack index are read through SQLite read-only in short reads,
// without the live lock refusal: the backend may have them open.
//
// Before the downtime, count -needed-out writes the packs the rule would
// drop from the live World, from count's own pass, for packcheck -needed.
//
// With -needed, the files are not read: the packs come from a needed-packs
// file an earlier run wrote with -needed-out, so a rerun inside the downtime
// is one listing and a compare.
func runPackCheck(o opts) (packCheckResult, error) {
	var res packCheckResult
	start := time.Now()
	if o.list == "" {
		return res, fmt.Errorf("packcheck needs -list, a listing of the bucket's packs")
	}
	var packs []neededPack
	var err error
	if o.needed != "" {
		packs, err = readNeeded(o.needed)
	} else {
		packs, err = neededPacks(o, &res)
	}
	if err != nil {
		return res, err
	}
	res.needed = int64(len(packs))
	if o.neededOut != "" {
		if err := writeNeeded(o.neededOut, packs); err != nil {
			return res, err
		}
	}

	// Compare with the listing.
	objs, entries, ignored, err := readListing(o.list, o.listFormat)
	if err != nil {
		return res, err
	}
	fmt.Printf("packcheck: listing %s: %d pack objects, %d entry objects, %d other keys ignored\n", o.list, len(objs), len(entries), ignored)
	for _, p := range packs {
		if p.size < 0 {
			continue
		}
		sizes, ok := objs[p.id]
		switch {
		case !ok:
			res.missing++
			fmt.Printf("  MISSING pack %s (%d bytes, %d dropped blocks)\n", p.id, p.size, p.blocks)
		case !containsSize(sizes, p.size):
			res.mismatch++
			fmt.Printf("  SIZE pack %s: listed %v bytes, the index records %d (%d dropped blocks)\n", p.id, sizes, p.size, p.blocks)
		}
		if len(entries) != 0 && !entries[p.id] {
			res.entriesMissing++
			fmt.Printf("  MISSING entry of pack %s\n", p.id)
		}
	}
	if len(entries) == 0 {
		fmt.Println("packcheck: the listing has no entries/ keys: entry objects not checked")
	}
	fmt.Printf("packcheck: %d packs checked: %d missing, %d size mismatches, %d entries missing; %d packs with no e/ row; in %s\n",
		res.needed, res.missing, res.mismatch, res.entriesMissing, res.noEntry, time.Since(start).Round(time.Millisecond))
	if n := res.faults(); n != 0 {
		return res, fmt.Errorf("packcheck failed: %d faults: do not remove the old World", n)
	}
	fmt.Println("packcheck: ok")
	return res, nil
}

// neededPacks reads the old and new World and the pack index, and returns the
// packs the dropped blocks need, with their recorded sizes; a pack with no e/
// row has size -1.
func neededPacks(o opts, res *packCheckResult) ([]neededPack, error) {
	if o.world == "" || o.out == "" || o.packs == "" {
		return nil, fmt.Errorf("packcheck needs -world (old), -out (new) and -packs, or -needed")
	}

	// The blocks the new World holds, by h row key.
	dst, err := openLiveRO(o.out)
	if err != nil {
		return nil, err
	}
	defer dst.Close()
	dtable, _, err := kvTable(dst)
	if err != nil {
		return nil, err
	}
	kept := map[[indexKeyBytes]byte]struct{}{}
	bp := []byte(blockPrefix)
	err = scanKeys(dst, dtable, bp, prefixEnd(bp), o.chunk, 0, func(key []byte) error {
		if k, ok := blockRowKey(key); ok {
			kept[k] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: block keys: %w", o.out, err)
	}

	// The blocks the old World holds and the new one lacks.
	src, err := openRO(o.world)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	table, _, err := kvTable(src)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var oldBlocks int64
	dropped := map[[indexKeyBytes]byte]uint32{}
	add := func(key []byte) error {
		k, ok := blockRowKey(key)
		if !ok {
			return nil
		}
		mu.Lock()
		oldBlocks++
		if _, ok := kept[k]; !ok {
			dropped[k] = 0
		}
		mu.Unlock()
		return nil
	}
	if o.read == "raw" {
		err = rawIndexKeys(o, src, o.world, table, bp, add)
	} else {
		err = scanKeys(src, table, bp, prefixEnd(bp), o.chunk, 0, add)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: block keys: %w", o.world, err)
	}
	res.dropped = int64(len(dropped))
	fmt.Printf("packcheck: old World %d blocks, new %d, dropped %d\n", oldBlocks, len(kept), len(dropped))
	kept = nil

	// The pack index: the p/ and e/ rows, then the h rows of the dropped
	// blocks.
	pdb, err := openLiveRO(o.packs)
	if err != nil {
		return nil, err
	}
	defer pdb.Close()
	ptable, _, err := kvTable(pdb)
	if err != nil {
		return nil, err
	}
	pre := packIndexPrefix(o.storeID)
	ids := map[uint32]string{}
	pp := append(bytes.Clone(pre), "p/"...)
	err = scan(pdb, ptable, pp, prefixEnd(pp), o.chunk, 0, true, func(key []byte, _ int64, v []byte) error {
		if len(v) < 4 {
			return fmt.Errorf("p/ row %q of %d bytes", trunc(key), len(v))
		}
		ids[binary.BigEndian.Uint32(v)] = string(key[len(pp):])
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: p/ rows: %w", o.packs, err)
	}
	sizes := map[string]int64{}
	ep := append(bytes.Clone(pre), "e/"...)
	err = scan(pdb, ptable, ep, prefixEnd(ep), o.chunk, 0, true, func(key []byte, _ int64, v []byte) error {
		e := &packfile.PackfileEntry{}
		if err := e.UnmarshalVT(v); err != nil {
			return fmt.Errorf("e/ row %q: %w", trunc(key), err)
		}
		sizes[string(key[len(ep):])] = int64(e.GetSizeBytes())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: e/ rows: %w", o.packs, err)
	}
	hp := append(bytes.Clone(pre), 'h')
	found := map[[indexKeyBytes]byte]bool{}
	err = scan(pdb, ptable, hp, prefixEnd(hp), o.chunk, 0, true, func(key []byte, _ int64, v []byte) error {
		if len(key) != len(hp)+indexKeyBytes || len(v) < 4 {
			return nil
		}
		k := [indexKeyBytes]byte(key[len(hp):])
		if _, ok := dropped[k]; ok {
			dropped[k] = binary.BigEndian.Uint32(v)
			found[k] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: h rows: %w", o.packs, err)
	}

	// Group the dropped blocks by pack.
	byID := map[string]*neededPack{}
	for k, num := range dropped {
		if !found[k] {
			res.noRow++
			continue
		}
		id, ok := ids[num]
		if !ok {
			res.deadPack++
			continue
		}
		p := byID[id]
		if p == nil {
			size, ok := sizes[id]
			if !ok {
				size = -1
				res.noEntry++
				fmt.Printf("  NO e/ row for pack %s: its size is unknown\n", id)
			}
			p = &neededPack{id: id, size: size}
			byID[id] = p
		}
		p.blocks++
	}
	fmt.Printf("packcheck: pack index: %d packs, %d e/ rows; dropped blocks: %d in %d packs, %d with no h row, %d on a pack number no p/ row holds\n",
		len(ids), len(sizes), res.dropped-res.noRow-res.deadPack, len(byID), res.noRow, res.deadPack)
	packs := make([]neededPack, 0, len(byID))
	for _, p := range byID {
		packs = append(packs, *p)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].id < packs[j].id })
	return packs, nil
}

// blockRowKey returns the h row key of a block key: the end of its hash.
func blockRowKey(key []byte) ([indexKeyBytes]byte, bool) {
	var k [indexKeyBytes]byte
	ref := &block.BlockRef{}
	if !bytes.HasPrefix(key, []byte(blockPrefix)) || ref.UnmarshalVT(key[len(blockPrefix):]) != nil || ref.GetHash() == nil {
		return k, false
	}
	md := ref.GetHash().MarshalDigest()
	if len(md) < indexKeyBytes {
		return k, false
	}
	copy(k[:], md[len(md)-indexKeyBytes:])
	return k, true
}

// openLiveRO opens a SQLite file read-only, one connection, without refusing
// a live one: a read-only reader beside a running backend reads committed
// rows, WAL included, and writes nothing to the file.
func openLiveRO(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// writeNeeded writes the needed packs, one "id size blocks" line each.
func writeNeeded(path string, packs []neededPack) error {
	var b bytes.Buffer
	for _, p := range packs {
		fmt.Fprintf(&b, "%s %d %d\n", p.id, p.size, p.blocks)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// readNeeded reads a needed-packs file writeNeeded wrote.
func readNeeded(path string) ([]neededPack, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var packs []neededPack
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		fs := strings.Fields(sc.Text())
		if len(fs) == 0 {
			continue
		}
		if len(fs) != 3 {
			return nil, fmt.Errorf("%s:%d: want id size blocks", path, n)
		}
		size, err1 := strconv.ParseInt(fs[1], 10, 64)
		blocks, err2 := strconv.ParseInt(fs[2], 10, 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s:%d: bad size or block count", path, n)
		}
		packs = append(packs, neededPack{fs[0], size, blocks})
	}
	return packs, sc.Err()
}

// readListing reads a listing of size and key lines, separated by ';' as
// rclone lsf writes them or by white space, in the order format names (sp:
// size first, ps: key first). It returns the sizes of each pack id, the ids
// with an entry object, and the count of other keys.
func readListing(path, format string) (map[string][]int64, map[string]bool, int, error) {
	if format == "" {
		format = "sp"
	}
	if format != "sp" && format != "ps" {
		return nil, nil, 0, fmt.Errorf("-list-format %q: want sp or ps", format)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()
	objs := map[string][]int64{}
	entries := map[string]bool{}
	ignored := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var a, b string
		var ok bool
		if format == "sp" {
			a, b, ok = strings.Cut(line, ";")
		} else {
			if i := strings.LastIndexByte(line, ';'); i >= 0 {
				a, b, ok = line[:i], line[i+1:], true
			}
		}
		if !ok {
			fs := strings.Fields(line)
			if len(fs) == 2 {
				a, b, ok = fs[0], fs[1], true
			}
		}
		if !ok {
			return nil, nil, 0, fmt.Errorf("%s:%d: want a size and a key", path, n)
		}
		sizeStr, key := a, b
		if format == "ps" {
			sizeStr, key = b, a
		}
		if strings.HasSuffix(key, "/") {
			// A directory line.
			ignored++
			continue
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 10, 64)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("%s:%d: size %q", path, n, sizeStr)
		}
		key = strings.TrimSpace(key)
		dir, id := "", key
		if i := strings.LastIndexByte(key, '/'); i >= 0 {
			dir, id = key[:i], key[i+1:]
			if j := strings.LastIndexByte(dir, '/'); j >= 0 {
				dir = dir[j+1:]
			}
		}
		switch {
		case dir == "" && !strings.Contains(key, "/"), dir == "packs":
			objs[id] = append(objs[id], size)
		case dir == "entries":
			entries[id] = true
		default:
			ignored++
		}
	}
	return objs, entries, ignored, sc.Err()
}

func containsSize(sizes []int64, s int64) bool {
	for _, x := range sizes {
		if x == s {
			return true
		}
	}
	return false
}
