package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// makeWorld builds a WAL database with rows, overflow chains and a freelist
// of several trunks, checkpoints it to an empty WAL and closes it.
func makeWorld(t *testing.T, pageSize int, rows int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world.db")
	db := openDB(t, path)
	defer db.Close()
	for _, s := range []string{
		"PRAGMA page_size=" + strconv.Itoa(pageSize),
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE t(k INTEGER PRIMARY KEY, v BLOB)",
		"CREATE INDEX t_v ON t(substr(v,1,16))",
	} {
		mustExec(t, db, s)
	}
	rng := rand.New(rand.NewSource(1))
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		v := make([]byte, 200+rng.Intn(6000))
		rng.Read(v)
		if _, err := tx.Exec("INSERT INTO t VALUES(?,?)", i, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "DELETE FROM t WHERE (k*7919)%10 < 6")
	mustExec(t, db, "PRAGMA wal_checkpoint(TRUNCATE)")
	return path
}

func mustExec(t *testing.T, db *sql.DB, s string) {
	t.Helper()
	if _, err := db.Exec(s); err != nil {
		t.Fatalf("%s: %v", s, err)
	}
}

// rowHash hashes every row through SQLite, in key order.
func rowHash(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query("SELECT k, v FROM t ORDER BY k")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	d := sha256.New()
	n := 0
	for rows.Next() {
		var k int64
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		binary.Write(d, binary.BigEndian, k)
		binary.Write(d, binary.BigEndian, int64(len(v)))
		d.Write(v)
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(n) + ":" + hex.EncodeToString(d.Sum(nil))
}

func integrity(t *testing.T, db *sql.DB) {
	t.Helper()
	var s string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&s); err != nil || s != "ok" {
		t.Fatalf("integrity_check: %q %v", s, err)
	}
}

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func dryRun(t *testing.T, path string, reserve int64) (string, string) {
	t.Helper()
	var out bytes.Buffer
	if err := runLocked(&out, opts{path: path, reserve: reserve}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	m := regexp.MustCompile(`range sha256 ([0-9a-f]{64})`).FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("no sha in %s", out.String())
	}
	return m[1], out.String()
}

func needLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("locks and punch need Linux")
	}
}

func fileHeader(t *testing.T, path string) (header, *walk) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	h, err := readHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	w, err := walkFreelist(f, h, false)
	if err != nil {
		t.Fatal(err)
	}
	return h, w
}

func readPages(t *testing.T, path string, h header, pages []uint32) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]byte
	for _, p := range pages {
		b := make([]byte, h.pageSize)
		if _, err := f.ReadAt(b, int64(p-1)*h.pageSize); err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func equalPages(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestPunchProof punches a synthetic World and checks rows, integrity,
// trunks, released blocks and reuse afterwards.
func TestPunchProof(t *testing.T) {
	needLinux(t)
	path := makeWorld(t, 4096, 12000)
	db := openDB(t, path)
	before := rowHash(t, db)
	db.Close()

	h, w := fileHeader(t, path)
	if len(w.trunks) < 4 {
		t.Fatalf("fixture has %d trunks, want several", len(w.trunks))
	}
	trunksBefore := readPages(t, path, h, w.trunks)
	keptBefore := readPages(t, path, h, w.leaves[:w.perTrunk[0]])
	reserve := int64(w.perTrunk[0]) * h.pageSize // keeps the head trunk's leaves
	sha, out := dryRun(t, path, reserve)
	t.Log(out)
	stBefore, _ := os.Stat(path)

	var pout bytes.Buffer
	if err := runLocked(&pout, opts{path: path, reserve: reserve, rangeSHA: sha}); err != nil {
		t.Fatalf("punch: %v", err)
	}
	t.Log(pout.String())
	stAfter, _ := os.Stat(path)
	if stAfter.Size() != stBefore.Size() {
		t.Fatalf("size changed %d -> %d", stBefore.Size(), stAfter.Size())
	}
	p := makePlan(w, h, reserve)
	if p.keptTrunks != 1 || p.punchPages == 0 {
		t.Fatalf("plan kept %d trunks, punches %d pages", p.keptTrunks, p.punchPages)
	}
	if got, want := allocated(stBefore)-allocated(stAfter), int64(p.punchPages)*h.pageSize; got < want*9/10 {
		t.Fatalf("released %d bytes, want about %d", got, want)
	}
	if !equalPages(trunksBefore, readPages(t, path, h, w.trunks)) {
		t.Fatal("a trunk page changed")
	}
	if !equalPages(keptBefore, readPages(t, path, h, w.leaves[:w.perTrunk[0]])) {
		t.Fatal("a kept-back leaf changed")
	}
	zero := make([]byte, h.pageSize)
	for _, r := range p.ranges {
		for pg := r.start; pg < r.start+r.count; pg++ {
			if !bytes.Equal(readPages(t, path, h, []uint32{pg})[0], zero) {
				t.Fatalf("punched page %d is not zero", pg)
			}
		}
	}

	db = openDB(t, path)
	defer db.Close()
	integrity(t, db)
	if after := rowHash(t, db); after != before {
		t.Fatalf("row hash %s after punch, %s before", after, before)
	}
	// Reuse the punched pages, free some, and rebuild the file.
	var free0, free1 int
	db.QueryRow("PRAGMA freelist_count").Scan(&free0)
	rng := rand.New(rand.NewSource(2))
	tx, _ := db.Begin()
	for i := 0; i < 4000; i++ {
		v := make([]byte, 200+rng.Intn(6000))
		rng.Read(v)
		if _, err := tx.Exec("INSERT INTO t VALUES(?,?)", 100000+i, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.QueryRow("PRAGMA freelist_count").Scan(&free1)
	if free1 >= free0 {
		t.Fatalf("freelist %d -> %d: inserts did not reuse free pages", free0, free1)
	}
	integrity(t, db)
	mustExec(t, db, "PRAGMA wal_checkpoint(TRUNCATE)")
	integrity(t, db)
	mustExec(t, db, "DELETE FROM t WHERE k%3 = 0")
	mustExec(t, db, "VACUUM")
	integrity(t, db)
}

func TestRefusals(t *testing.T) {
	needLinux(t)
	refuse := func(t *testing.T, o opts, want string) {
		t.Helper()
		st, _ := os.Stat(o.path)
		err := runLocked(&bytes.Buffer{}, o)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("got %v, want refusal containing %q", err, want)
		}
		st2, _ := os.Stat(o.path)
		if allocated(st) != allocated(st2) {
			t.Fatal("a refused run changed the file")
		}
	}
	t.Run("wal not empty", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		os.WriteFile(path+"-wal", []byte("x"), 0o644)
		refuse(t, opts{path: path}, "-wal is 1 bytes")
	})
	t.Run("idle open connection", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		db := openDB(t, path)
		defer db.Close()
		var n int
		db.QueryRow("SELECT count(*) FROM t").Scan(&n) // the pooled conn keeps the WAL index open
		refuse(t, opts{path: path}, "in use")
	})
	t.Run("open read transaction", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		db := openDB(t, path)
		defer db.Close()
		tx, _ := db.Begin()
		defer tx.Rollback()
		var n int
		tx.QueryRow("SELECT count(*) FROM t").Scan(&n)
		refuse(t, opts{path: path}, "in use")
	})
	t.Run("rollback journal mode reader", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		db := openDB(t, path)
		defer db.Close()
		mustExec(t, db, "PRAGMA journal_mode=DELETE")
		os.Remove(path + "-shm")
		tx, _ := db.Begin()
		defer tx.Rollback()
		var n int
		tx.QueryRow("SELECT count(*) FROM t").Scan(&n)
		refuse(t, opts{path: path}, "in use")
	})
	t.Run("page size", func(t *testing.T) {
		refuse(t, opts{path: makeWorld(t, 8192, 2000)}, "page_size is 8192")
	})
	t.Run("header count mismatch", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		f, _ := os.OpenFile(path, os.O_RDWR, 0)
		var b [4]byte
		f.ReadAt(b[:], 36)
		binary.BigEndian.PutUint32(b[:], binary.BigEndian.Uint32(b[:])+1)
		f.WriteAt(b[:], 36)
		f.Close()
		refuse(t, opts{path: path}, "header freelist count is")
	})
	t.Run("stale range sha", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		refuse(t, opts{path: path, rangeSHA: strings.Repeat("0", 64)}, "freelist changed since the dry run")
	})
	t.Run("freelist changed after dry run", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		sha, _ := dryRun(t, path, 0)
		db := openDB(t, path)
		mustExec(t, db, "INSERT INTO t VALUES(999999, zeroblob(20000))")
		mustExec(t, db, "PRAGMA wal_checkpoint(TRUNCATE)")
		db.Close()
		refuse(t, opts{path: path, rangeSHA: sha}, "freelist changed since the dry run")
	})
}

// TestNeverTrunk builds a walk whose trunks sit between leaves and checks
// no range covers a trunk or a kept-back leaf.
func TestNeverTrunk(t *testing.T) {
	h := header{pageSize: 4096, usable: 4096, dbPages: 100}
	w := &walk{
		trunks:   []uint32{10, 20, 30},
		leaves:   []uint32{11, 12, 9, 19, 21, 22, 29, 31, 32},
		perTrunk: []int{3, 3, 3},
	}
	p := makePlan(w, h, 1) // keeps trunk 10's leaves 11, 12, 9
	if p.keptTrunks != 1 || p.keptPages != 3 {
		t.Fatalf("kept %d trunks %d pages", p.keptTrunks, p.keptPages)
	}
	want := []pageRange{{19, 1}, {21, 2}, {29, 1}, {31, 2}}
	if len(p.ranges) != len(want) {
		t.Fatalf("ranges %v, want %v", p.ranges, want)
	}
	for i := range want {
		if p.ranges[i] != want[i] {
			t.Fatalf("ranges %v, want %v", p.ranges, want)
		}
	}
	if p0 := makePlan(w, h, 0); p0.punchPages != 9 || p0.sha == p.sha {
		t.Fatalf("reserve 0: %d pages, sha %s", p0.punchPages, p0.sha)
	}
}

// TestWalkFaults checks the strict walk refuses a broken chain and the
// tolerant walk keeps what it read before the fault.
func TestWalkFaults(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	h, w := fileHeader(t, path)
	if len(w.trunks) < 2 {
		t.Fatalf("fixture has %d trunks", len(w.trunks))
	}
	f, _ := os.OpenFile(path, os.O_RDWR, 0)
	defer f.Close()
	// Point the second trunk's first leaf at the head trunk: listed twice.
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], w.trunks[0])
	f.WriteAt(b[:], int64(w.trunks[1]-1)*h.pageSize+8)
	if _, err := walkFreelist(f, h, false); err == nil || !strings.Contains(err.Error(), "listed twice") {
		t.Fatalf("strict walk: %v", err)
	}
	tw, err := walkFreelist(f, h, true)
	if err != nil || tw.torn == nil || len(tw.trunks) != 1 {
		t.Fatalf("tolerant walk: %v torn=%v trunks=%d", err, tw.torn, len(tw.trunks))
	}
}

// TestLive reads a database another connection holds open with a non-empty
// WAL, changes nothing, and cannot punch through its descriptor.
func TestLive(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	db := openDB(t, path)
	defer db.Close()
	mustExec(t, db, "DELETE FROM t WHERE k < 500")
	st, _ := os.Stat(path)
	var out bytes.Buffer
	if err := runLive(&out, opts{path: path, reserve: 1 << 20}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "LIVE read-only estimate") || !strings.Contains(out.String(), "range sha256") {
		t.Fatalf("live output:\n%s", out.String())
	}
	t.Log(out.String())
	st2, _ := os.Stat(path)
	if st.Size() != st2.Size() || allocated(st) != allocated(st2) {
		t.Fatal("live run changed the file")
	}
	if runtime.GOOS == "linux" {
		f, _ := os.Open(path)
		defer f.Close()
		h, _ := readHeader(f, st.Size())
		if err := punchRanges(f, h, []pageRange{{2, 1}}); err == nil {
			t.Fatal("punch on a read-only descriptor succeeded")
		}
	}
}
