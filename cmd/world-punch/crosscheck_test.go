package main

import (
	"bytes"
	"encoding/binary"
	"os"
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
