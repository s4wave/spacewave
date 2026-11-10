package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// scanFile runs scanCheck on path with small reader and stream counts.
func scanFile(t *testing.T, path string) ccResult {
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
	res, err := scanCheck(f, h, ccOpts{readers: 4, streams: 3, parsers: 2})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// makeBigIndex builds a world table with an index on large tags, so index
// interior and index leaf cells overflow, and frees some pages.
func makeBigIndex(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.db")
	db := openDB(t, path)
	defer db.Close()
	for _, s := range []string{
		"PRAGMA page_size=4096",
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE world ( key BLOB PRIMARY KEY, value BLOB, tag BLOB)",
		"CREATE INDEX big ON world(tag) WHERE tag IS NOT NULL",
	} {
		mustExec(t, db, s)
	}

	// Insert mostly small rows, some large values and some large tags.
	rng := rand.New(rand.NewSource(2))
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6000; i++ {
		k, v := make([]byte, 32), make([]byte, 100+rng.Intn(300))
		if i%25 == 0 {
			v = make([]byte, 6000+rng.Intn(8000))
		}
		var tag []byte
		if i%10 == 3 {
			tag = make([]byte, 1200+rng.Intn(2000))
			rng.Read(tag)
		}
		rng.Read(k)
		rng.Read(v)
		if _, err := tx.Exec("INSERT INTO world VALUES(?,?,?)", k, v, tag); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Free pages and fold the WAL into the file.
	mustExec(t, db, "DELETE FROM world WHERE rowid%7=2")
	mustExec(t, db, "PRAGMA wal_checkpoint(TRUNCATE)")
	return path
}

// TestScanParity: on clean files the scan gives the walk's verdict and counts.
func TestScanParity(t *testing.T) {
	for name, mk := range map[string]func(*testing.T) string{
		"world":    func(t *testing.T) string { return makeWorld(t, 4096, 3000) },
		"bigindex": makeBigIndex,
	} {
		t.Run(name, func(t *testing.T) {
			// Run both checks on the same file.
			path := mk(t)
			walk, scan := crosscheckFile(t, path), scanFile(t, path)
			if walk.failed() || scan.failed() {
				t.Fatalf("walk %v scan %v", walk.failReasons(), scan.failReasons())
			}

			// Compare every count.
			if walk.reached != scan.reached || walk.free != scan.free || walk.lockPage != scan.lockPage ||
				walk.kinds != scan.kinds || walk.leafCells != scan.leafCells || len(walk.byType) != len(scan.byType) {
				t.Fatalf("walk reached %d free %d kinds %v cells %d types %v; scan reached %d free %d kinds %v cells %d types %v",
					walk.reached, walk.free, walk.kinds, walk.leafCells, walk.byType, scan.reached, scan.free, scan.kinds, scan.leafCells, scan.byType)
			}
			for k, n := range walk.byType {
				if scan.byType[k] != n {
					t.Fatalf("type 0x%02x: walk %d scan %d", k, n, scan.byType[k])
				}
			}
		})
	}
}

// longChain finds the first page of an overflow chain of at least two pages,
// from the cells of in-use table leaves.
func longChain(t *testing.T, path string) uint32 {
	t.Helper()
	h, w := fileHeader(t, path)
	free := slices.Concat(w.Trunks, w.Leaves)
	for pg := uint32(2); pg <= h.DBPages; pg++ {
		if slices.Contains(free, pg) {
			continue
		}
		r := parsePage(h, ccItem{pg: pg, kind: kindTree}, readPages(t, path, h, []uint32{pg})[0])
		for _, c := range r.children {
			if r.bad == "" && r.ptype == 0x0d && c.kind == kindOverflow && c.left >= 2 {
				return c.pg
			}
		}
	}
	t.Fatal("no chain of two pages")
	return 0
}

// patch writes b at off in path.
func patch(t *testing.T, path string, off int64, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(b, off); err != nil {
		t.Fatal(err)
	}
}

// TestScanFaults: the scan fails with the right reason on planted faults.
func TestScanFaults(t *testing.T) {
	// A freelist leaf entry that is also an overflow chain page.
	t.Run("free", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		target := crosscheckFile(t, path).firstOfKind[kindOverflow]
		listAsFree(t, path, target)
		res := scanFile(t, path)
		if len(res.conflicts) != 1 || res.conflicts[0].page != target || res.conflicts[0].dup || !strings.Contains(strings.Join(res.failReasons(), ","), "unexplained conflicts 1") {
			t.Fatalf("conflicts %+v reasons %v", res.conflicts, res.failReasons())
		}
	})

	// A chain cut short: its first page's next pointer is 0.
	t.Run("cut", func(t *testing.T) {
		path := makeBigIndex(t)
		h, _ := fileHeader(t, path)
		head := longChain(t, path)
		patch(t, path, int64(head-1)*h.PageSize, make([]byte, 4))
		res := scanFile(t, path)
		if len(res.bad) != 1 || res.bad[0].page != head || !strings.Contains(res.bad[0].err, "ends") || !slices.Contains(res.failReasons(), "bad-unexplained 1") {
			t.Fatalf("bad %+v reasons %v", res.bad, res.failReasons())
		}
	})

	// Two tree parents: the root's right child repeats its first child.
	t.Run("dup", func(t *testing.T) {
		path := makeWorld(t, 4096, 3000)
		db := openDB(t, path)
		var root uint32
		if err := db.QueryRow("SELECT rootpage FROM sqlite_master WHERE name='t'").Scan(&root); err != nil {
			t.Fatal(err)
		}
		db.Close()
		h, _ := fileHeader(t, path)
		rp := readPages(t, path, h, []uint32{root})[0]
		first := binary.BigEndian.Uint32(rp[binary.BigEndian.Uint16(rp[12:]):])
		patch(t, path, int64(root-1)*h.PageSize+8, binary.BigEndian.AppendUint32(nil, first))
		res := scanFile(t, path)
		var dup bool
		for _, c := range res.conflicts {
			dup = dup || (c.dup && c.page == first)
		}
		if !dup || !res.failed() || res.sanityMatch() {
			t.Fatalf("conflicts %+v reasons %v", res.conflicts, res.failReasons())
		}
	})
}

// TestScanProbe: a probe projecting past its limit stops phase 2 as an
// interrupt, with the projection in the partial report.
func TestScanProbe(t *testing.T) {
	// Pace phase 2 so the probe fires mid-pass.
	path := makeWorld(t, 4096, 3000)
	f, _ := os.Open(path)
	defer f.Close()
	st, _ := f.Stat()
	h, _ := readHeader(f, st.Size())
	res, err := scanCheck(f, h, ccOpts{readers: 2, streams: 2, parsers: 1, mbps: 2, probeAt: 50 * time.Millisecond, probeLimit: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	// The run is interrupted and says why.
	var out bytes.Buffer
	res.printPartial(&out)
	if !res.interrupted || !res.probe.stopped || !strings.Contains(out.String(), "over, stopping") {
		t.Fatalf("interrupted %v probe %+v:\n%s", res.interrupted, res.probe, out.String())
	}
}

// shortReader returns half of each read that cut selects, with io.EOF,
// the way a file cut short under a reader looks.
type shortReader struct {
	f   *os.File
	cut func(n int, off int64) bool
}

// ReadAt reads from f, shortened when cut selects the read.
func (s shortReader) ReadAt(b []byte, off int64) (int, error) {
	if s.cut(len(b), off) {
		n, _ := s.f.ReadAt(b[:len(b)/2], off)
		return n, io.EOF
	}
	return s.f.ReadAt(b, off)
}

// TestScanShortRead checks that a short read is a hard error with no
// verdict: a file truncated mid-file, a short phase 1 read, a short
// phase 2 read, and a short read in the v3 walk.
func TestScanShortRead(t *testing.T) {
	// Truncate a copy mid-file: -scan refuses it before phase 1.
	path := makeBigIndex(t)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	cut := filepath.Join(t.TempDir(), "cut.db")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cut, data[:st.Size()/2+123], 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := runCrosscheck(&out, opts{path: cut, cross: true, scan: true, readers: 4, streams: 3, parsers: 2}, nil)
	if err == nil || code != 0 || strings.Contains(out.String(), "result:") {
		t.Fatalf("truncated file: code %d err %v, output:\n%s", code, err, out.String())
	}

	// Short reads under each reader: phase 1 (reads of at most 32 pages
	// past page 1), phase 2 (1 MiB reads) and the v3 walk.
	if st.Size() <= scanChunk {
		t.Fatalf("fixture is %d bytes, want more than one scan chunk", st.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h, err := readHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		scan bool
		cut  func(n int, off int64) bool
	}{
		{"phase 1", true, func(n int, off int64) bool { return n <= 32*4096 && off > 0 }},
		{"phase 2", true, func(n int, off int64) bool { return n > 32*4096 }},
		{"walk", false, func(n int, off int64) bool { return off > 0 }},
	} {
		o := ccOpts{readers: 4, streams: 3, parsers: 2, bulk: shortReader{f, tc.cut}}
		check := crosscheck
		if tc.scan {
			check = scanCheck
		}
		if _, err := check(f, h, o); err == nil || !strings.Contains(err.Error(), "short read") {
			t.Errorf("%s: got err %v, want a short read error", tc.name, err)
		}
	}
}

// TestScanLockPage: a b-tree, leaf or overflow page on the lock-byte page
// fails both modes. The lock-byte offset is moved onto a page the fixture
// uses, since a real lock-byte page needs a file over 1 GiB.
func TestScanLockPage(t *testing.T) {
	path := makeBigIndex(t)
	ref := crosscheckFile(t, path)
	for _, kind := range []pageKind{kindOverflow, kindTable, kindIndex} {
		target := ref.firstOfKind[kind]
		if target < 2 {
			t.Fatalf("no page of kind %v", kind)
		}
		h, _ := fileHeader(t, path)
		lockByteOffset = int64(target-1)*h.PageSize + 100
		var out bytes.Buffer
		code, err := runCrosscheck(&out, opts{path: path, cross: true, scan: true, readers: 4, streams: 3, parsers: 2}, nil)
		cc := crosscheckFile(t, path)
		lockByteOffset = pendingByte
		if err != nil || code != 3 || !strings.Contains(out.String(), "lock-byte page") {
			t.Fatalf("scan, lock-byte page %d (kind %v): code %d err %v, output:\n%s", target, kind, code, err, out.String())
		}
		if !cc.failed() || !slices.ContainsFunc(cc.bad, func(b badPage) bool { return strings.Contains(b.err, "lock-byte page") }) {
			t.Fatalf("crosscheck, lock-byte page %d (kind %v): bad %+v reasons %v", target, kind, cc.bad, cc.failReasons())
		}
	}
}

// TestScanHeaderChecks: the scan fails on a header freelist count unlike
// the walk and on a header changed during the scan, and refuses a hot
// journal and an invalid page size.
func TestScanHeaderChecks(t *testing.T) {
	// A header freelist count one more than the walk finds.
	path := makeBigIndex(t)
	h, _ := fileHeader(t, path)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], h.FreeCount+1)
	patch(t, path, 36, b[:])
	res0 := scanFile(t, path)
	if r := strings.Join(res0.failReasons(), ","); !strings.Contains(r, "header freelist count") {
		t.Fatalf("freelist count: reasons %s", r)
	}
	binary.BigEndian.PutUint32(b[:], h.FreeCount)
	patch(t, path, 36, b[:])

	// The change counter bumped after the start header was read.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := scanCheck(f, h, ccOpts{readers: 4, streams: 3, parsers: 2, freelistOverride: func(bitset, bool) {
		binary.BigEndian.PutUint32(b[:], h.Change+1)
		patch(t, path, 24, b[:])
	}})
	if err != nil || !strings.Contains(strings.Join(res.failReasons(), ","), "file changed during scan") {
		t.Fatalf("change counter: err %v reasons %v", err, res.failReasons())
	}

	// A hot journal, and a page size that is not a power of two.
	if err := os.WriteFile(path+"-journal", []byte{1}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCrosscheck(io.Discard, opts{path: path, cross: true, scan: true, readers: 4, streams: 3, parsers: 2}, nil); err == nil || !strings.Contains(err.Error(), "-journal is 1 bytes") {
		t.Fatalf("hot journal: err %v", err)
	}
	os.Remove(path + "-journal")
	patch(t, path, 16, []byte{0x0b, 0xb8})
	if _, err := runCrosscheck(io.Discard, opts{path: path, cross: true, scan: true, readers: 4, streams: 3, parsers: 2}, nil); err == nil || !strings.Contains(err.Error(), "power of two") {
		t.Fatalf("page size 3000: err %v", err)
	}
}
