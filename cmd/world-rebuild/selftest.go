package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// selftest block categories by i % 10.
const (
	catMarked   = 0 // marked, not queued: keep
	catUnqueued = 1 // no eviction entry: keep
	catNoPack   = 2 // queued, no h row: keep
	catDeadPack = 3 // queued, h row names a removed pack: keep
	catHot      = 4 // queued, packed, in the read set: keep
	// 5..9: queued, packed, cold: drop
)

// runSelftest builds a test store in a temporary directory under -out, then
// checks count, copy, verify, the mutants and the source guards against it.
func runSelftest(o opts) error {
	// Make the scratch directory, removed after.
	dir, err := os.MkdirTemp(o.out, "world-rebuild-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Build the test store in the production layout.
	n := o.n
	o.full = true
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	want, err := buildTestStore(o.world, o.packs, n, o.storeID, o.rowid, false)
	if err != nil {
		return fmt.Errorf("build test store: %w", err)
	}
	fi, _ := os.Stat(o.world)
	fmt.Printf("test store: %d blocks, %s file, %d blocks the rule drops\n", n, gb(fi.Size()), want)

	// The raw count finds exactly the blocks the rule drops, and their
	// eviction entries, and none of the deleted rows on freelist pages.
	co := o
	co.mbps, co.rateOut, co.rateMB = 0, filepath.Join(dir, "rate.db"), 50
	res, err := runCount(co)
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}

	// Tally what the count must have found.
	hot := int64(0)
	for i := range n {
		if i%10 == catHot {
			hot++
		}
	}
	var blocks int64
	for _, t := range res.blocks {
		blocks += t.keys
	}

	// Compare the count with the store's known contents.
	for _, c := range []struct {
		name      string
		got, want int64
	}{
		{"block keys", blocks, int64(n)},
		{"drop keys", res.blocks[vDrop].keys, want},
		{"hot keys", res.blocks[vHot].keys, hot},
		{"dropped eviction entries", res.dropEvict.keys, want},
		{"cells that did not decode", res.badCells, 0},
	} {
		if c.got != c.want {
			return fmt.Errorf("count: %s %d, want %d", c.name, c.got, c.want)
		}
	}
	fmt.Println("count: ok")

	// count -rate-only samples and writes the scratch copy without error.
	ro := co
	ro.rateOnly = true
	if err := runRateOnly(ro); err != nil {
		return fmt.Errorf("count -rate-only: %w", err)
	}

	// The faithful copy verifies and drops exactly the cold packed blocks.
	o.out = filepath.Join(dir, "new.db")
	start := time.Now()
	if err := runCopy(o); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	el := time.Since(start)
	nf, _ := os.Stat(o.out)
	fmt.Printf("copy rate: %.0f MB/s of source file, %s\n", float64(fi.Size())/1e6/el.Seconds(), el.Round(time.Millisecond))

	// It verifies and holds exactly the kept blocks.
	if err := runVerify(o); err != nil {
		return fmt.Errorf("faithful copy failed verify: %w", err)
	}
	got, err := countBlocks(o.out)
	if err != nil {
		return err
	}
	if got != int64(n)-want {
		return fmt.Errorf("copy holds %d blocks, want %d", got, int64(n)-want)
	}
	fmt.Printf("faithful copy: ok, %d blocks kept, new file %s\n", got, gb(nf.Size()))

	// The copy read the other way holds the same rows and schema, and both
	// verify reads pass it.
	other := o
	other.read = map[string]string{"raw": "sqlite", "sqlite": "raw"}[o.read]
	if err := runVerify(other); err != nil {
		return fmt.Errorf("faithful copy failed verify -read %s: %w", other.read, err)
	}
	other.out = filepath.Join(dir, "new-"+other.read+".db")
	if err := runCopy(other); err != nil {
		return fmt.Errorf("copy -read %s: %w", other.read, err)
	}
	n1, s1, m1, err := copyDigest(o.out)
	if err != nil {
		return err
	}
	n2, s2, m2, err := copyDigest(other.out)
	if err != nil {
		return err
	}
	srcDB, err := openRO(o.world)
	if err != nil {
		return err
	}
	m0, err := masterRows(srcDB)
	srcDB.Close()
	if err != nil {
		return err
	}
	if n1 != n2 || s1 != s2 || m1 != m2 || m1 != m0 {
		return fmt.Errorf("copy -read %s: %d rows %s, -read %s: %d rows %s; schemas equal to the source %v %v", o.read, n1, s1, other.read, n2, s2, m1 == m0, m2 == m0)
	}
	fmt.Printf("copy -read %s and -read %s: %d rows, same row hash %s, source schema\n", o.read, other.read, n1, s1[:16])
	os.Remove(other.out)

	// Each mutant drops one key the rule keeps; verify must catch it.
	for _, m := range []string{"drop-marked", "drop-unqueued", "drop-hot", "drop-unpacked", "drop-meta", "stale-evict-bytes"} {
		o.mutant = m
		o.out = filepath.Join(dir, m+".db")
		if err := runCopy(o); err != nil {
			return fmt.Errorf("mutant %s copy: %w", m, err)
		}
		if err := runVerify(o); err == nil {
			return fmt.Errorf("mutant %s: verify passed, want a fault", m)
		}
		fmt.Printf("mutant %s: caught\n", m)
		os.Remove(o.out)
	}

	// A connection holding the source open makes copy refuse. Only Linux's
	// open file description probe sees this process's own locks.
	if runtime.GOOS == "linux" {
		if err := heldSourceRefused(o, filepath.Join(dir, "held.db")); err != nil {
			return err
		}
	}

	// Closing a read of a file with a non-empty WAL writes nothing to it.
	if err := roCloseLeavesWAL(dir); err != nil {
		return err
	}

	// All checks passed.
	fmt.Println("selftest: ok")
	return nil
}

// heldSourceRefused holds a connection to the test World open and checks
// that copy refuses to start.
func heldSourceRefused(o opts, out string) error {
	// Open the source as the backend would and read from it.
	hold, err := sql.Open("sqlite", "file:"+o.world)
	if err != nil {
		return err
	}
	defer hold.Close()
	var c int64
	if err := hold.QueryRow("SELECT count(*) FROM kv WHERE rowid < 10").Scan(&c); err != nil {
		return err
	}

	// copy must refuse before writing anything.
	o.out, o.mutant = out, ""
	err = runCopy(o)
	if err == nil || !strings.Contains(err.Error(), "lock") {
		return fmt.Errorf("copy with a live connection on the source: got %v, want a lock refusal", err)
	}
	if _, serr := os.Stat(out); serr == nil {
		return fmt.Errorf("copy refused but wrote %s", out)
	}
	fmt.Printf("held source: copy refused: %v\n", err)
	return nil
}

// roCloseLeavesWAL checks that openRO on a file whose WAL holds a commit
// reads that commit and, on close, neither checkpoints it into the main file
// nor removes the WAL.
func roCloseLeavesWAL(dir string) error {
	// Commit a row that stays only in the WAL, and copy the main file and
	// WAL aside while the writer still has them open.
	src, path := filepath.Join(dir, "walsrc.db"), filepath.Join(dir, "wal.db")
	w, err := sql.Open("sqlite", "file:"+src+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		return err
	}
	defer w.Close()
	w.SetMaxOpenConns(1)
	for _, q := range []string{"CREATE TABLE kv (key BLOB PRIMARY KEY, value BLOB)", "PRAGMA wal_checkpoint(TRUNCATE)", "INSERT INTO kv VALUES (x'01', x'02')"} {
		if _, err := w.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	for _, sfx := range []string{"", "-wal"} {
		b, err := os.ReadFile(src + sfx)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path+sfx, b, 0o644); err != nil {
			return err
		}
	}

	// Read the copy through openRO and close it.
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	db, err := openRO(path)
	if err != nil {
		return err
	}
	var n int64
	err = db.QueryRow("SELECT count(*) FROM kv").Scan(&n)
	db.Close()
	if err != nil {
		return err
	}

	// The row was read, the main file is byte for byte unchanged and the
	// WAL is still there.
	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	wi, err := os.Stat(path + "-wal")
	switch {
	case n != 1:
		return fmt.Errorf("openRO read %d rows, want the 1 in the WAL", n)
	case !bytes.Equal(before, after):
		return fmt.Errorf("closing openRO changed the main file: a checkpoint ran")
	case err != nil || wi.Size() == 0:
		return fmt.Errorf("closing openRO removed or emptied the WAL: %v", err)
	}
	fmt.Printf("non-empty WAL: openRO read it and its close left the main file and the %d byte WAL as they were\n", wi.Size())
	return nil
}

func countBlocks(path string) (int64, error) {
	db, err := openRO(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	t, _, err := kvTable(db)
	if err != nil {
		return 0, err
	}
	var c int64
	err = db.QueryRow("SELECT count(*) FROM "+t+" WHERE key >= ? AND key < ?", []byte(blockPrefix), prefixEnd([]byte(blockPrefix))).Scan(&c)
	return c, err
}

// buildTestStore writes a World store and pack index in the production
// layout, returning how many blocks the drop rule drops.
//
// synth makes a store shaped like production for timing: block values of
// 600-3000 bytes, 15% of them 4100-12000 bytes and so on overflow pages, no
// h/c rows, and three quarters of the blocks dropped.
// testPackSizes are the sizes the test store's e/ rows give packA and packB.
var testPackSizes = [2]uint64{1000, 2000}

func buildTestStore(world, packs string, n int, storeID string, rowid, synth bool) (int64, error) {
	stmt := "CREATE TABLE IF NOT EXISTS kv ( key BLOB PRIMARY KEY, value BLOB)"
	if !rowid {
		stmt += " WITHOUT ROWID"
	}
	w, err := sql.Open("sqlite", "file:"+world+"?_pragma=journal_mode(WAL)&_pragma=synchronous(OFF)")
	if err != nil {
		return 0, err
	}
	defer w.Close()
	w.SetMaxOpenConns(1)
	if _, err := w.Exec(stmt); err != nil {
		return 0, err
	}
	p, err := sql.Open("sqlite", "file:"+packs+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return 0, err
	}
	defer p.Close()
	p.SetMaxOpenConns(1)
	if _, err := p.Exec(stmt); err != nil {
		return 0, err
	}

	wtx, _ := w.Begin()
	ptx, _ := p.Begin()
	wput := func(k string, v []byte) error {
		_, err := wtx.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(k), v)
		return err
	}
	sum := sha256.Sum256([]byte(storeID))
	pre := string(append([]byte("pi"), sum[:8]...))
	pput := func(k string, v []byte) error {
		_, err := ptx.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(pre+k), v)
		return err
	}
	num := func(x uint32) []byte { return binary.BigEndian.AppendUint32(nil, x) }
	if err := pput("p/packA", append(num(1), 1)); err != nil {
		return 0, err
	}
	if err := pput("p/packB", append(num(2), 1)); err != nil {
		return 0, err
	}
	// The e/ rows hold the entries the packs' objects are listed by, with
	// their sizes: packA testPackSizes[0] bytes, packB testPackSizes[1].
	for i, id := range []string{"packA", "packB"} {
		e, _ := (&packfile.PackfileEntry{Id: id, SizeBytes: testPackSizes[i]}).MarshalVT()
		if err := pput("e/"+id, e); err != nil {
			return 0, err
		}
	}

	const obj = testObjPrefix
	var hot []byte
	var drop int64
	var evictTotal int64
	stamp := func(x uint64) string { return hex.EncodeToString(binary.BigEndian.AppendUint64(nil, x)) }
	for i := range n {
		digest := sha256.Sum256([]byte("block-" + strconv.Itoa(i)))
		h := hash.NewHash(hash.HashType_HashType_SHA256, digest[:])
		ref := &block.BlockRef{Hash: h}
		rk, _ := ref.MarshalKey()
		size := 500 + (i*7919)%15500
		if synth {
			u := uint32(i) * 2654435761
			size = 600 + int(u/100%2400)
			if u%100 >= 85 {
				size = 4100 + int(u/100%7900)
			}
		}
		val := make([]byte, size)
		_, _ = rand.Read(val)
		if err := wput(blockPrefix+string(rk), val); err != nil {
			return 0, err
		}
		if !synth {
			if err := wput("h/c/"+string(rk), []byte("children")); err != nil {
				return 0, err
			}
		}
		b58 := h.MarshalString()
		vsize := strconv.Itoa(len(val))
		cat := i % 10
		if synth && i%20 >= 10 {
			cat = 5
		}
		queued := cat != catMarked && cat != catUnqueued
		if cat == catMarked {
			if err := wput(obj+"marked/"+b58, nil); err != nil {
				return 0, err
			}
			if err := wput(obj+"queue/"+stamp(uint64(i))+"/"+b58, []byte(vsize)); err != nil {
				return 0, err
			}
		}
		if queued {
			if err := wput(obj+"evictable/"+stamp(uint64(i%3))+"/"+b58, []byte(vsize)); err != nil {
				return 0, err
			}
			evictTotal += int64(len(val))
		}
		hr := "h" + string(h.MarshalDigest()[len(h.MarshalDigest())-indexKeyBytes:])
		switch {
		case cat == catDeadPack:
			if err := pput(hr, num(7)); err != nil {
				return 0, err
			}
		case queued && cat != catNoPack:
			if err := pput(hr, num(uint32(1+i%2))); err != nil {
				return 0, err
			}
		}
		if cat == catHot {
			hot = append(hot, digest[:readKeyLen]...)
		}
		if cat >= 5 {
			drop++
		}
	}
	// An eviction entry for a block the store does not hold stays.
	ghost := hash.NewHash(hash.HashType_HashType_SHA256, make([]byte, 32))
	if err := wput(obj+"evictable/"+stamp(0)+"/"+ghost.MarshalString(), []byte("10")); err != nil {
		return 0, err
	}
	gen := uint64(time.Now().Add(-30 * time.Minute).UnixNano())
	old := uint64(time.Now().Add(-5 * time.Hour).UnixNano())
	for _, kv := range []struct {
		k string
		v []byte
	}{
		{obj + "reads/" + stamp(gen) + "/" + stamp(gen+1), hot[:len(hot)/2]},
		{obj + "reads/" + stamp(gen) + "/" + stamp(gen+2), hot[len(hot)/2:]},
		{obj + "reads/" + stamp(old) + "/" + stamp(old+1), []byte("0123456789abcdef")},
		{obj + "evictable-bytes", []byte("999")},
		{obj + "epoch", []byte("3")},
		{obj + "queue-count", []byte("12")},
		{"h/r/root", []byte("root")},
		{"zz-format", []byte("1")},
	} {
		if err := wput(kv.k, kv.v); err != nil {
			return 0, err
		}
	}
	_ = evictTotal
	if err := wtx.Commit(); err != nil {
		return 0, err
	}
	if err := ptx.Commit(); err != nil {
		return 0, err
	}
	// Block rows written then deleted leave whole leaf pages on the
	// freelist with their stale cells: a raw count must skip them.
	jtx, _ := w.Begin()
	for i := range 3000 {
		if _, err := jtx.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(blockPrefix+"junk-"+strconv.Itoa(i)), make([]byte, 1500)); err != nil {
			return 0, err
		}
	}
	if err := jtx.Commit(); err != nil {
		return 0, err
	}
	if _, err := w.Exec("DELETE FROM kv WHERE key >= ? AND key < ?", []byte(blockPrefix+"junk-"), prefixEnd([]byte(blockPrefix+"junk-"))); err != nil {
		return 0, err
	}
	// The raw count reads the main files only.
	for _, db := range []*sql.DB{w, p} {
		if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return 0, err
		}
	}
	var free int64
	if err := w.QueryRow("PRAGMA freelist_count").Scan(&free); err != nil || free < 100 {
		return 0, fmt.Errorf("deleted junk rows left %d free pages (%v), want at least 100", free, err)
	}
	return drop, nil
}

// copyDigest hashes every row of a kv file in key order, and lists its
// schema rows without their root pages.
func copyDigest(path string) (rows int64, sum string, schema string, err error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, "", "", err
	}
	defer db.Close()
	r, err := db.Query("SELECT key, value FROM kv ORDER BY key")
	if err != nil {
		return 0, "", "", err
	}
	defer r.Close()
	h := sha256.New()
	for r.Next() {
		var k, v []byte
		if err := r.Scan(&k, &v); err != nil {
			return 0, "", "", err
		}
		fmt.Fprintf(h, "%d:%x %d:%x\n", len(k), k, len(v), v)
		rows++
	}
	if err := r.Err(); err != nil {
		return 0, "", "", err
	}
	schema, err = masterRows(db)
	return rows, fmt.Sprintf("%x", h.Sum(nil)), schema, err
}

// masterRows lists sqlite_master without the root pages.
func masterRows(db *sql.DB) (string, error) {
	r, err := db.Query("SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY rowid")
	if err != nil {
		return "", err
	}
	defer r.Close()
	s := ""
	for r.Next() {
		var ty, name, tbl string
		var q sql.NullString
		if err := r.Scan(&ty, &name, &tbl, &q); err != nil {
			return "", err
		}
		s += fmt.Sprintf("%s|%s|%s|%v|%q\n", ty, name, tbl, q.Valid, q.String)
	}
	return s, r.Err()
}

// testObjPrefix is the test store's write-back object store prefix, shaped
// like production's BlockStoreWritebackObjectStoreID: its id holds slashes.
const testObjPrefix = objsPrefix + "p/local/" + prodAccount + "/blk/" + defaultStoreID + "/writeback/"
