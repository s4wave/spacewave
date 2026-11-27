package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"testing"
)

func crosscheckFile(t *testing.T, path string) ccResult {
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
	res, err := crosscheck(f, h, ccOpts{readers: 4, retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestCrosscheckClean checks that every page of a clean file is reached
// once or free, and nothing conflicts.
func TestCrosscheckClean(t *testing.T) {
	// A clean fixture has no conflicting or unreadable pages.
	path := makeWorld(t, 4096, 3000)
	res := crosscheckFile(t, path)
	if len(res.conflicts) != 0 || res.unexplained() != 0 || len(res.bad) != 0 {
		t.Fatalf("clean file: conflicts %v bad %v", res.conflicts, res.bad)
	}

	// The freelist walk and the crosscheck must account for the same pages.
	h, w := fileHeader(t, path)
	if res.unaccounted != 0 || res.reached+res.free+res.lockPage != uint64(h.dbPages) {
		t.Fatalf("reached %d + free %d + lock %d vs %d pages, unaccounted %d", res.reached, res.free, res.lockPage, h.dbPages, res.unaccounted)
	}
	if res.free != uint64(h.freeCount) || w.pageCount() != int64(h.freeCount) || res.kinds[kindOverflow] == 0 || res.kinds[kindIndex] == 0 {
		t.Fatalf("free %d, kinds %v", res.free, res.kinds)
	}

	// The report exposes the conflict count.
	var out bytes.Buffer
	res.print(&out)
	if !strings.Contains(out.String(), "unexplained conflicts 0") {
		t.Fatalf("report:\n%s", out.String())
	}
}

// listAsFree rewrites the head trunk's first leaf entry to page pg, so pg is
// both reached and on the freelist.
func listAsFree(t *testing.T, path string, pg uint32) {
	// Open the file at the head trunk reported by the header.
	t.Helper()
	h, _ := fileHeader(t, path)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Replace the first leaf pointer with the selected page.
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], pg)
	if _, err := f.WriteAt(b[:], h.pageBytes(h.trunk-1)+8); err != nil {
		t.Fatal(err)
	}
}

func TestCrosscheckCatches(t *testing.T) {
	for _, kind := range []pageKind{kindTable, kindOverflow} {
		t.Run(kind.String(), func(t *testing.T) {
			path := makeWorld(t, 4096, 3000)
			target := crosscheckFile(t, path).firstOfKind[kind]
			if target == 0 {
				t.Fatalf("no %s page in fixture", kind)
			}
			listAsFree(t, path, target)
			res := crosscheckFile(t, path)
			if res.unexplained() != 1 || len(res.conflicts) != 1 || res.conflicts[0].page != target || res.conflicts[0].kind != kind {
				t.Fatalf("conflicts %+v, want page %d of kind %s unexplained", res.conflicts, target, kind)
			}
			if res.unaccounted != 1 {
				t.Fatalf("unaccounted %d, want 1 (the leaf the entry used to list)", res.unaccounted)
			}
			var out bytes.Buffer
			res.print(&out)
			if !strings.Contains(out.String(), "unexplained conflicts 1") {
				t.Fatalf("report:\n%s", out.String())
			}
		})
	}
}

// TestCrosscheckExplained checks that a conflict whose page has left the
// freelist by the recheck is explained, not unexplained.
func TestCrosscheckExplained(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	target := crosscheckFile(t, path).firstOfKind[kindTable]
	f, _ := os.Open(path)
	defer f.Close()
	st, _ := f.Stat()
	h, _ := readHeader(f, st.Size())
	res, err := crosscheck(f, h, ccOpts{readers: 2, retries: 1, freelistOverride: func(free bitset, recheck bool) {
		if !recheck {
			free.set(target)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.conflicts) != 1 || res.unexplained() != 0 || res.conflicts[0].class == "" {
		t.Fatalf("conflicts %+v", res.conflicts)
	}
}

// dropLastLeaf shortens the head trunk's leaf count by one, so its last
// leaf is neither reached nor on the freelist. It returns that page.
func dropLastLeaf(t *testing.T, path string) uint32 {
	// Read the original freelist before opening the fixture for mutation.
	t.Helper()
	h, w := fileHeader(t, path)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Reduce the leaf count so the last pointer is ignored.
	n := w.perTrunk[0]
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n-1)
	if _, err := f.WriteAt(b[:], h.pageBytes(h.trunk-1)+4); err != nil {
		t.Fatal(err)
	}

	// A freed page read as zeros, so the late walk finds nothing under it.
	if _, err := f.WriteAt(make([]byte, h.pageSize), h.pageBytes(w.leaves[n-1]-1)); err != nil {
		t.Fatal(err)
	}
	return w.leaves[n-1]
}

// TestCrosscheckUnaccounted checks that a page neither reached nor free is
// unexplained, unless the final freelist has the page (freed during the walk).
func TestCrosscheckUnaccounted(t *testing.T) {
	// The shortened freelist leaves a page without an owner.
	path := makeWorld(t, 4096, 3000)
	lost := dropLastLeaf(t, path)
	res := crosscheckFile(t, path)
	if res.unaccounted != 1 || res.unaccFreed != 0 || res.unaccUnexplained() != 1 || res.failed() || res.leftoverLeaf != 1 {
		t.Fatalf("unaccounted %d freed %d failed %v leftover leaf %d", res.unaccounted, res.unaccFreed, res.failed(), res.leftoverLeaf)
	}

	// A later snapshot can explain the missing page as freed by a writer.
	f, _ := os.Open(path)
	defer f.Close()
	st, _ := f.Stat()
	h, _ := readHeader(f, st.Size())
	res, err := crosscheck(f, h, ccOpts{readers: 2, retries: 1, freelistOverride: func(free bitset, recheck bool) {
		if recheck {
			free.set(lost)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.unaccounted != 1 || res.unaccFreed != 1 || res.unaccUnexplained() != 0 || res.failed() || res.leftoverLeaf != 0 {
		t.Fatalf("unaccounted %d freed %d failed %v", res.unaccounted, res.unaccFreed, res.failed())
	}

	// The report distinguishes the unaccounted page from unresolved problems.
	var out bytes.Buffer
	res.print(&out)
	pageCount := strconv.FormatUint(uint64(h.dbPages), 10)
	for _, want := range []string{
		"unaccounted-total 1, unaccounted-freed-during-walk 1, unaccounted-unexplained 0",
		"bad-total 0, bad-freed-during-walk 0, bad-unexplained 0",
		"header pages start " + pageCount + " end " + pageCount,
		"table interior", "table leaf", "index interior", "index leaf", "overflow", "trunks", "leaves",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// TestCrosscheckLateWalk cuts an interior page out of table t's root and
// lists one of its leaves on the freelist. The main walk cannot see the
// conflict, but the late walk from the unaccounted interior page must.
func TestCrosscheckLateWalk(t *testing.T) {
	// Locate the table root and the child whose subtree will be detached.
	path := makeWorld(t, 4096, 3000)
	db := openDB(t, path)
	var root uint32
	if err := db.QueryRow("SELECT rootpage FROM sqlite_master WHERE name='t'").Scan(&root); err != nil {
		t.Fatal(err)
	}
	db.Close()
	h, w := fileHeader(t, path)
	rp := readPages(t, path, h, []uint32{root})[0]
	if rp[0] != 0x05 {
		t.Fatalf("root %d is type 0x%02x, want a table interior page", root, rp[0])
	}
	n := binary.BigEndian.Uint16(rp[3:])
	last := binary.BigEndian.Uint16(rp[12+2*(uint32(n)-1):])
	cut := binary.BigEndian.Uint32(rp[last:])
	ip := readPages(t, path, h, []uint32{cut})[0]
	first := binary.BigEndian.Uint16(ip[12:])
	leaf := binary.BigEndian.Uint32(ip[first:])
	if ip[0] != 0x05 || readPages(t, path, h, []uint32{leaf})[0][0] != 0x0d {
		t.Fatalf("page %d type 0x%02x, leaf %d", cut, ip[0], leaf)
	}

	// Detach the child and put one of its descendants on the freelist.
	old := w.leaves[0]
	f, _ := os.OpenFile(path, os.O_RDWR, 0)
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], n-1)
	f.WriteAt(b[:], h.pageBytes(root-1)+3)
	f.WriteAt(make([]byte, h.pageSize), h.pageBytes(old-1)) // the leaf listAsFree drops reads as zeros
	f.Close()
	listAsFree(t, path, leaf)

	// The late walk must find the conflicting reference under the detached child.
	res := crosscheckFile(t, path)
	var found *conflict
	for i := range res.conflicts {
		if res.conflicts[i].page == leaf {
			found = &res.conflicts[i]
		}
	}
	if found == nil || found.class != "" || found.parent != cut || !found.late || !res.failed() {
		t.Fatalf("conflicts %+v, want leaf %d under %d unexplained from the late walk", res.conflicts, leaf, cut)
	}
	if res.leftoverInterior != 1 || res.leftoverLeaf != 1 {
		t.Fatalf("leftover interior %d leaf+overflow %d, want 1 and 1 (%v, %d roots)", res.leftoverInterior, res.leftoverLeaf, res.leftoverByType, res.lateRoots)
	}

	// The report retains the leftover counts and whether the pages were freed.
	var out bytes.Buffer
	res.print(&out)
	for _, want := range []string{"leftover interior 1", "leftover leaf+overflow 1", "freed-during-walk"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// TestCrosscheckSchemaRoots checks that a schema record is a bad page and
// fails the run if its rootpage cannot be read from the page because long
// names push the rootpage onto an overflow page, or if the record gives a
// table root 0, rather than leaving that tree's pages unreached. Views,
// triggers and virtual tables own no tree and pass.
func TestCrosscheckSchemaRoots(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  []string
		bad  bool
	}{
		{"rootless objects", []string{
			"CREATE VIEW tv AS SELECT k FROM t",
			"CREATE TRIGGER tt AFTER DELETE ON t BEGIN SELECT 1; END",
			"CREATE VIRTUAL TABLE tf USING fts5(a)",
		}, false},
		{"rootpage on an overflow page", []string{
			"CREATE TABLE t_" + strings.Repeat("x", 3000) + "(k INTEGER PRIMARY KEY)",
		}, true},
		{"table with rootpage 0", []string{
			"CREATE TABLE t2(k INTEGER PRIMARY KEY)",
			"PRAGMA writable_schema=ON",
			"UPDATE sqlite_schema SET rootpage=0 WHERE name='t2'",
			"PRAGMA writable_schema=OFF",
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := makeWorld(t, 4096, 50)
			db := openDB(t, path)
			for _, q := range append(tc.sql, "PRAGMA wal_checkpoint(TRUNCATE)") {
				mustExec(t, db, q)
			}
			db.Close()
			res := crosscheckFile(t, path)
			if !tc.bad {
				if len(res.bad) != 0 || res.failed() {
					t.Fatalf("bad %+v, failed %v; want a clean pass", res.bad, res.failed())
				}
				return
			}
			if len(res.bad) != 1 || res.bad[0].page != 1 || res.bad[0].freed || !strings.Contains(res.bad[0].err, "schema record") {
				t.Fatalf("bad %+v, want page 1 unexplained with a bad schema record", res.bad)
			}
			if !res.failed() {
				t.Fatal("crosscheck passed with an unwalked schema root")
			}
		})
	}
}

// putVarint appends v as an SQLite varint: big-endian groups of seven bits,
// and all eight bits in a ninth byte.
func putVarint(b []byte, v uint64) []byte {
	// Nine-byte varints end with a full byte.
	if v>>56 != 0 {
		for i := 7; i >= 0; i-- {
			b = append(b, byte(v>>(8+7*i)&0x7f)|0x80)
		}
		return append(b, byte(v&0xff))
	}

	// Short varints end with a seven-bit group whose high bit is clear.
	n := 1
	for v>>(7*n) != 0 {
		n++
	}
	for i := n - 1; i > 0; i-- {
		b = append(b, byte(v>>(7*i)&0x7f)|0x80)
	}
	return append(b, byte(v&0x7f))
}

// TestCrosscheckCorruptLengths feeds lengths no SQLite file holds, which
// would turn negative or truncate as ints, and expects a refusal, not a
// panic or a root.
func TestCrosscheckCorruptLengths(t *testing.T) {
	// Reject invalid page sizes before using them to size buffers or offsets.
	for _, tc := range []struct {
		encoded uint16
		want    uint32
	}{
		{0, 0}, {1, 65536}, {511, 0}, {512, 512}, {513, 0}, {65535, 0},
	} {
		var b [100]byte
		copy(b[:], "SQLite format 3\x00")
		binary.BigEndian.PutUint16(b[16:], tc.encoded)
		binary.BigEndian.PutUint32(b[28:], 1)
		h, err := readHeader(bytes.NewReader(b[:]), int64(tc.want))
		if h.pageSize != tc.want || (err == nil) != (tc.want != 0) {
			t.Errorf("encoded page size %d: got (%d, %v), want size %d", tc.encoded, h.pageSize, err, tc.want)
		}
	}

	// record builds a schema record whose type column is st, then an empty
	// name, a table name and a one-byte rootpage of 0, and sql of serial
	// type sql.
	record := func(st, sql uint64) []byte {
		var hdr []byte
		for _, v := range []uint64{st, 13, 13, 1, sql} {
			hdr = putVarint(hdr, v)
		}
		hsize, err := checkedUint32(int64(len(hdr)) + 1)
		if err != nil {
			t.Fatal(err)
		}
		rec := putVarint(nil, uint64(hsize))
		return append(append(rec, hdr...), 0, 't', 'a', 'b', 'l', 'e')
	}
	for _, tc := range []struct {
		name    string
		st, sql uint64
	}{
		{"type column past 2^63", 1<<63 + 13, 13},
		{"type column longer than the record", 1<<31 + 13, 13},
		{"sql column past 2^63", 13, 1<<63 + 13},
	} {
		if root, ok := schemaRoot(record(tc.st, tc.sql)); ok {
			t.Errorf("%s: root %d accepted", tc.name, root)
		}
	}

	// SQLite stores rootpage as a signed integer regardless of its width.
	for _, tc := range []struct {
		name   string
		serial byte
		value  []byte
		want   uint32
		ok     bool
	}{
		{"negative root", 4, []byte{0xff, 0xff, 0xff, 0xff}, 0, false},
		{"root above uint32", 5, []byte{0, 1, 0, 0, 0, 0}, 0, false},
		{"largest uint32 root", 5, []byte{0, 0, 0xff, 0xff, 0xff, 0xff}, 0xffffffff, true},
	} {
		rec := append([]byte{6, 13, 13, 13, tc.serial, 13}, tc.value...)
		if root, ok := schemaRoot(rec); root != tc.want || ok != tc.ok {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", tc.name, root, ok, tc.want, tc.ok)
		}
	}

	// A table leaf page with one cell whose payload size SQLite never writes.
	for _, size := range []uint64{1 << 31, 1 << 63} {
		p := make([]byte, 4096)
		p[0] = 0x0d
		binary.BigEndian.PutUint16(p[3:], 1)
		binary.BigEndian.PutUint16(p[8:], 4000)
		copy(p[4000:], putVarint(putVarint(nil, size), 1))
		r := parsePage(header{pageSize: 4096, usable: 4096}, ccItem{pg: 2, kind: kindTree}, p)
		if !strings.Contains(r.bad, "payload size") {
			t.Errorf("payload size %d: bad %q, want payload size", size, r.bad)
		}
	}
}
