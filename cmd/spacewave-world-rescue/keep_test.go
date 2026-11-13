//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestOrderPacksByTime orders by wanted refs per size plus overhead: a big
// pack of many refs loses to a small pack of few when the overhead is small,
// and wins when the overhead dominates. Unknown sizes go last.
func TestOrderPacksByTime(t *testing.T) {
	mk := func() []*packWork {
		return []*packWork{
			{id: "big", wants: make([]want, 100), entry: &packfile.PackfileEntry{SizeBytes: 100 << 20}},
			{id: "small", wants: make([]want, 1), entry: &packfile.PackfileEntry{SizeBytes: 64 << 10}},
			{id: "nosize", wants: make([]want, 500), entry: &packfile.PackfileEntry{}},
		}
	}
	ids := func(ps []*packWork) string {
		var s []string
		for _, p := range ps {
			s = append(s, p.id)
		}
		return strings.Join(s, ",")
	}
	ps := mk()
	orderPacksBy(ps, orderTime, 0)
	if got := ids(ps); got != "small,big,nosize" {
		t.Fatalf("overhead 0: %s", got)
	}
	ps = mk()
	orderPacksBy(ps, orderTime, 10_304_340)
	if got := ids(ps); got != "big,small,nosize" {
		t.Fatalf("overhead 10.3 MB: %s", got)
	}
	ps = mk()
	orderPacksBy(ps, orderWanted, 0)
	if got := ids(ps); got != "nosize,big,small" {
		t.Fatalf("wanted: %s", got)
	}
}

// writeKeys writes the refs' hex tails, one a line.
func writeKeys(t *testing.T, path string, refs []*block.BlockRef) {
	var b strings.Builder
	for _, ref := range refs {
		k := packfile.BlockKey(ref.GetHash())
		b.WriteString(hex.EncodeToString(k[len(k)-16:]) + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRescueKeepSet keeps the keep set's blocks of a whole pack beside the
// wanted, verified, logs each once, and leaves the rest of the pack out;
// with a size cut it lists them dropped, with their offsets, instead.
func TestRescueKeepSet(t *testing.T) {
	for _, cut := range []uint64{0, 1} {
		src := newSource(t)
		refs := src.pack(20)
		wanted, extras, rest := refs[:5], refs[5:15], refs[15:]
		out := t.TempDir()
		setPath := filepath.Join(t.TempDir(), "keep.tails")
		// The keep set names a wanted block too: it is stored once, as wanted.
		writeKeys(t, setPath, append(append([]*block.BlockRef{}, extras...), wanted[0]))
		set, err := readKeepSet(setPath)
		if err != nil {
			t.Fatal(err)
		}
		var kept, dropped bytes.Buffer
		o := testOptions(src, writeList(t, out, wanted), out)
		o.Keep = keepOptions{Set: set, MaxSize: cut, Kept: &kept, Dropped: &dropped}
		res, err := run(context.Background(), o, &fakeServer{dir: src.dir})
		if err != nil {
			t.Fatal(err)
		}
		if res.Done != 5 {
			t.Fatalf("cut %d: done %d", cut, res.Done)
		}
		if n := checkOutput(t, out, wanted); n != 5 {
			t.Fatalf("cut %d: wanted stored %d", cut, n)
		}
		if n := checkOutput(t, out, rest); n != 0 {
			t.Fatalf("cut %d: %d blocks outside the keep set stored", cut, n)
		}
		keptLines := strings.Count(kept.String(), "\n")
		dropLines := strings.Split(strings.TrimSpace(dropped.String()), "\n")
		switch cut {
		case 0:
			if n := checkOutput(t, out, extras); n != 10 || keptLines != 10 || dropped.Len() != 0 {
				t.Fatalf("cut 0: extras stored %d, kept lines %d, dropped %q", n, keptLines, dropped.String())
			}
		default:
			if n := checkOutput(t, out, extras); n != 0 || keptLines != 0 || len(dropLines) != 10 {
				t.Fatalf("cut 1: extras stored %d, kept lines %d, dropped lines %d", n, keptLines, len(dropLines))
			}
			if f := strings.Split(dropLines[0], "\t"); len(f) != 4 {
				t.Fatalf("dropped line %q", dropLines[0])
			}
		}
	}
}

// TestKeeperOnceAndVerify gives a block once across packs and never keeps
// bytes that do not hash to their key.
func TestKeeperOnceAndVerify(t *testing.T) {
	data := []byte("round two extra")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(ref.GetHash())
	value, err := block.EncodeBlockObject(data, nil, true)
	if err != nil {
		t.Skipf("no encoder for a pack value here: %v", err)
	}
	index := []*kvfile.IndexEntry{{Key: key, Offset: 0, Size: uint64(len(value))}}
	k := newKeeper(keepOptions{Set: keepSet{tailOf(key)}})
	if xs := k.take(index, value, nil); len(xs) != 1 || xs[0].blk == nil {
		t.Fatalf("first pack: %+v", xs)
	}
	if xs := k.take(index, value, nil); len(xs) != 0 {
		t.Fatalf("second pack gave it again: %+v", xs)
	}
	bad := append([]byte(nil), value...)
	bad[len(bad)-1] ^= 0xff
	k = newKeeper(keepOptions{Set: keepSet{tailOf(key)}})
	if xs := k.take(index, bad, nil); len(xs) != 0 || k.Failed != 1 {
		t.Fatalf("bad bytes kept: %+v failed %d", xs, k.Failed)
	}
}

// TestRescueMaxPacks takes only the first packs in order and leaves the rest's
// refs unrecorded for a later run, which then takes them.
func TestRescueMaxPacks(t *testing.T) {
	src := newSource(t)
	a := src.pack(3)
	b := src.pack(2)
	refs := append(append([]*block.BlockRef{}, a...), b...)
	out := t.TempDir()
	list := writeList(t, out, refs)
	o := testOptions(src, list, out)
	o.MaxPacks = 1
	res, err := run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 3 || res.Deferred != 2 || res.PacksDone != 1 {
		t.Fatalf("first run %+v", res)
	}
	if n := checkOutput(t, out, b); n != 0 {
		t.Fatal("a deferred pack's block was stored")
	}
	o = testOptions(src, list, out)
	if res, err = run(context.Background(), o, &fakeServer{dir: src.dir}); err != nil {
		t.Fatal(err)
	}
	if res.Done != 5 || res.PacksDone != 1 || res.Deferred != 0 {
		t.Fatalf("second run %+v", res)
	}
	if n := checkOutput(t, out, refs); n != 5 {
		t.Fatalf("output holds %d of 5", n)
	}
}
