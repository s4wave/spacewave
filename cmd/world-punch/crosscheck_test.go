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

// TestCrosscheckClean: every page of a clean file is reached once or free,
// and nothing conflicts.
func TestCrosscheckClean(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	res := crosscheckFile(t, path)
	if len(res.conflicts) != 0 || res.unexplained() != 0 || len(res.bad) != 0 {
		t.Fatalf("clean file: conflicts %v bad %v", res.conflicts, res.bad)
	}
	h, w := fileHeader(t, path)
	if res.unaccounted != 0 || res.reached+res.free+res.lockPage != uint64(h.dbPages) {
		t.Fatalf("reached %d + free %d + lock %d vs %d pages, unaccounted %d", res.reached, res.free, res.lockPage, h.dbPages, res.unaccounted)
	}
	if res.free != uint64(len(w.trunks)+len(w.leaves)) || res.kinds[kindOverflow] == 0 || res.kinds[kindIndex] == 0 {
		t.Fatalf("free %d, kinds %v", res.free, res.kinds)
	}
	var out bytes.Buffer
	res.print(&out)
	if !strings.Contains(out.String(), "unexplained conflicts 0") {
		t.Fatalf("report:\n%s", out.String())
	}
}

// listAsFree rewrites the head trunk's first leaf entry to page pg, so pg is
// both reached and on the freelist.
func listAsFree(t *testing.T, path string, pg uint32) {
	t.Helper()
	h, _ := fileHeader(t, path)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], pg)
	if _, err := f.WriteAt(b[:], int64(h.trunk-1)*h.pageSize+8); err != nil {
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

// TestCrosscheckExplained: a conflict whose page has left the freelist by
// the recheck is explained, not unexplained.
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
	t.Helper()
	h, w := fileHeader(t, path)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := w.perTrunk[0]
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n-1))
	if _, err := f.WriteAt(b[:], int64(h.trunk-1)*h.pageSize+4); err != nil {
		t.Fatal(err)
	}
	// A freed page read as zeros, so the late walk finds nothing under it.
	if _, err := f.WriteAt(make([]byte, h.pageSize), int64(w.leaves[n-1]-1)*h.pageSize); err != nil {
		t.Fatal(err)
	}
	return w.leaves[n-1]
}

// TestCrosscheckUnaccounted: a page neither reached nor free is
// unexplained, unless the final freelist has it (freed during the walk).
func TestCrosscheckUnaccounted(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	lost := dropLastLeaf(t, path)
	res := crosscheckFile(t, path)
	if res.unaccounted != 1 || res.unaccFreed != 0 || res.unaccUnexplained() != 1 || !res.failed() || res.leftoverLeaf != 1 {
		t.Fatalf("unaccounted %d freed %d failed %v leftover leaf %d", res.unaccounted, res.unaccFreed, res.failed(), res.leftoverLeaf)
	}

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
	if res.unaccounted != 1 || res.unaccFreed != 1 || res.unaccUnexplained() != 0 || res.leftoverLeaf != 0 {
		t.Fatalf("unaccounted %d freed %d", res.unaccounted, res.unaccFreed)
	}
	// Freed during the walk explains the page, but the sanity sum still
	// misses it, and a sanity mismatch fails the run.
	if why := res.failReasons(); len(why) != 1 || why[0] != "sanity match false" {
		t.Fatalf("fail reasons %v, want only the sanity mismatch", why)
	}
	var out bytes.Buffer
	res.print(&out)
	for _, want := range []string{
		"unaccounted-total 1, unaccounted-freed-during-walk 1, unaccounted-unexplained 0",
		"bad-total 0, bad-freed-during-walk 0, bad-unexplained 0",
		"header pages start " + strconv.Itoa(int(h.dbPages)) + " end " + strconv.Itoa(int(h.dbPages)),
		"table interior", "table leaf", "index interior", "index leaf", "overflow", "trunks", "leaves",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// TestCrosscheckLateWalk cuts an interior page out of table t's root and
// lists one of its leaves on the freelist. The main walk cannot see the
// conflict; the late walk from the unaccounted interior page must.
func TestCrosscheckLateWalk(t *testing.T) {
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
	n := int(binary.BigEndian.Uint16(rp[3:]))
	last := int(binary.BigEndian.Uint16(rp[12+2*(n-1):]))
	cut := binary.BigEndian.Uint32(rp[last:])
	ip := readPages(t, path, h, []uint32{cut})[0]
	first := int(binary.BigEndian.Uint16(ip[12:]))
	leaf := binary.BigEndian.Uint32(ip[first:])
	if ip[0] != 0x05 || readPages(t, path, h, []uint32{leaf})[0][0] != 0x0d {
		t.Fatalf("page %d type 0x%02x, leaf %d", cut, ip[0], leaf)
	}
	old := w.leaves[0]
	f, _ := os.OpenFile(path, os.O_RDWR, 0)
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(n-1))
	f.WriteAt(b[:], int64(root-1)*h.pageSize+3)
	f.WriteAt(make([]byte, h.pageSize), int64(old-1)*h.pageSize) // the leaf listAsFree drops reads as zeros
	f.Close()
	listAsFree(t, path, leaf)

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
	var out bytes.Buffer
	res.print(&out)
	for _, want := range []string{"leftover interior 1", "leftover leaf+overflow 1", "freed-during-walk"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// TestCrosscheckFailRule: each strict field fails the run on its own.
func TestCrosscheckFailRule(t *testing.T) {
	pass := ccResult{dbPages: 10, reached: 8, free: 2}
	if pass.failed() {
		t.Fatalf("clean result fails: %v", pass.failReasons())
	}
	for name, edit := range map[string]func(r *ccResult){
		"sanity match false":          func(r *ccResult) { r.reached = 7 },
		"unaccounted-unexplained 1":   func(r *ccResult) { r.unaccounted = 1 },
		"bad-unexplained 1":           func(r *ccResult) { r.bad = []badPage{{page: 3}} },
		"unexplained conflicts 1":     func(r *ccResult) { r.conflicts = []conflict{{page: 3}} },
		"freelist walk stopped early": func(r *ccResult) { r.freeTorn = os.ErrClosed },
		"interrupted":                 func(r *ccResult) { r.interrupted = true },
	} {
		r := pass
		edit(&r)
		var out bytes.Buffer
		r.print(&out)
		if why := r.failReasons(); len(why) != 1 || why[0] != name || !strings.Contains(out.String(), "result: FAIL ("+name+")") {
			t.Fatalf("%s: reasons %v, report:\n%s", name, why, out.String())
		}
	}
	r := pass
	r.bad = []badPage{{page: 3, freed: true}}
	r.unaccounted, r.unaccFreed = 1, 1
	r.reached--
	if why := r.failReasons(); len(why) != 1 || why[0] != "sanity match false" {
		t.Fatalf("freed pages: reasons %v", why)
	}
}

// TestCrosscheckInterrupt: a closed interrupt stops the walk and the
// partial report gives totals and INCOMPLETE; a full run counts reads.
func TestCrosscheckInterrupt(t *testing.T) {
	path := makeWorld(t, 4096, 3000)
	full := crosscheckFile(t, path)
	if full.interrupted || full.reads == 0 || full.readPages < full.reads || full.inUse != full.dbPages-uint32(full.free) {
		t.Fatalf("full run: interrupted %v reads %d pages %d in use %d", full.interrupted, full.reads, full.readPages, full.inUse)
	}
	var out bytes.Buffer
	full.print(&out)
	if !strings.Contains(out.String(), "pages per read") || !strings.Contains(out.String(), "result: PASS") {
		t.Fatalf("report:\n%s", out.String())
	}

	f, _ := os.Open(path)
	defer f.Close()
	st, _ := f.Stat()
	h, _ := readHeader(f, st.Size())
	stop := make(chan struct{})
	close(stop)
	res, err := crosscheck(f, h, ccOpts{readers: 2, retries: 1, interrupt: stop})
	if err != nil {
		t.Fatal(err)
	}
	if !res.interrupted || res.readPages >= full.readPages || !res.failed() {
		t.Fatalf("interrupted %v, read %d of %d pages", res.interrupted, res.readPages, full.readPages)
	}
	out.Reset()
	res.printPartial(&out)
	for _, want := range []string{"INTERRUPTED at round", "in use", "partial totals: conflicts 0 (reached twice 0), bad 0", "result: INCOMPLETE"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("partial report lacks %q:\n%s", want, out.String())
		}
	}
}
