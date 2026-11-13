//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/sirupsen/logrus"
)

// packIndexes returns the index keys of each pack under root/prefix, by id.
func packIndexes(t *testing.T, root, prefix string) map[string][][]byte {
	t.Helper()
	out := make(map[string][][]byte)
	err := dirObjects{}.ListObjects(context.Background(), root, prefix, func(key string, _ int64) error {
		data, err := os.ReadFile(objectPath(root, key))
		if err != nil {
			return err
		}
		index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			return err
		}
		id := key[strings.LastIndex(key, "/")+1:]
		for _, ie := range index {
			out[id] = append(out[id], append([]byte(nil), ie.GetKey()...))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// packOf returns the id of the source pack holding ref, failing unless
// exactly one does.
func packOf(t *testing.T, src *source, ref *block.BlockRef) string {
	t.Helper()
	key := packfile.BlockKey(ref.GetHash())
	var ids []string
	for id, keys := range packIndexes(t, filepath.Join(src.dir, testRoot), testPrefix+"packs/") {
		for _, k := range keys {
			if bytes.Equal(k, key) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) != 1 {
		t.Fatalf("block in %d source packs", len(ids))
	}
	return ids[0]
}

// outputCount returns how many times the output packs hold ref.
func outputCount(t *testing.T, out string, ref *block.BlockRef) int {
	t.Helper()
	key := packfile.BlockKey(ref.GetHash())
	var n int
	for _, keys := range packIndexes(t, out, "packs/") {
		for _, k := range keys {
			if bytes.Equal(k, key) {
				n++
			}
		}
	}
	return n
}

// dropHashRows closes the source and deletes the refs' h rows, as for a
// pack the indexer never indexed.
func (s *source) dropHashRows(refs []*block.BlockRef) {
	s.close()
	ctx := context.Background()
	tx, err := s.index.NewTransaction(ctx, true)
	if err != nil {
		s.t.Fatal(err)
	}
	defer tx.Discard()
	for _, ref := range refs {
		if err := tx.Delete(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(ref.GetHash()))); err != nil {
			s.t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatal(err)
	}
}

// keepOf returns keep options over the refs, logging to the buffers.
func keepOf(t *testing.T, refs []*block.BlockRef, kept, dropped *bytes.Buffer) keepOptions {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keep.tails")
	writeKeys(t, path, refs)
	set, err := readKeepSet(path)
	if err != nil {
		t.Fatal(err)
	}
	return keepOptions{Set: set, Kept: kept, Dropped: dropped}
}

// TestRescueWholePacksNoHashRow reads a listed pack none of whose blocks has
// an h row, keeping only its keep set blocks, beside a pack the refs place.
// The flag refuses ranges and needs a keep set.
func TestRescueWholePacksNoHashRow(t *testing.T) {
	src := newSource(t)
	a, b := src.pack(3), src.pack(5)
	idB := packOf(t, src, b[0])
	src.dropHashRows(b)
	out := t.TempDir()
	var kept, dropped bytes.Buffer
	o := testOptions(src, writeList(t, out, a), out)
	o.Keep = keepOf(t, b[:3], &kept, &dropped)
	o.WholePacks = []string{idB}
	res, err := run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 3 || res.PacksDone != 2 || res.Absent != 0 {
		t.Fatalf("result %+v", res)
	}
	if n := checkOutput(t, out, b[:3]); n != 3 {
		t.Fatalf("keep set blocks stored %d of 3", n)
	}
	if n := checkOutput(t, out, b[3:]); n != 0 {
		t.Fatalf("%d blocks outside the keep set stored", n)
	}
	if n := strings.Count(kept.String(), "\n"); n != 3 {
		t.Fatalf("kept lines %d", n)
	}

	// A second run finds the listed pack finished and fetches nothing.
	srv := &fakeServer{dir: src.dir}
	o = testOptions(src, writeList(t, out, a), out)
	o.Keep = keepOf(t, b[:3], &kept, &dropped)
	o.WholePacks = []string{idB}
	if res, err = run(context.Background(), o, srv); err != nil {
		t.Fatal(err)
	}
	if calls, _ := srv.count(); calls != 0 || res.PacksTotal != 0 {
		t.Fatalf("second run: calls %d, packs %d", calls, res.PacksTotal)
	}

	// The flag refuses ranges and needs a keep set.
	o = testOptions(src, writeList(t, out, a), out)
	o.WholePacks = []string{idB}
	if _, err := run(context.Background(), o, &fakeServer{dir: src.dir}); err == nil {
		t.Fatal("ran without a keep set")
	}
	o.Keep = keepOf(t, b[:3], &kept, &dropped)
	o.Plan.Ranges = true
	if _, err := run(context.Background(), o, &fakeServer{dir: src.dir}); err == nil {
		t.Fatal("ran with ranges")
	}
}

// TestRescueWholePacksSharedNum reads a listed pack on a pack number another
// pack shares, which no wanted ref's bloom filter test gives a ref, for its
// keep set blocks.
func TestRescueWholePacksSharedNum(t *testing.T) {
	ctx := context.Background()
	var src *source
	var a, b []*block.BlockRef
	for try := 0; ; try++ {
		var clean bool
		src, a, b, _, clean = sharedNumSource(ctx, t)
		if clean {
			break
		}
		if try == 20 {
			t.Fatal("every source had a bloom false positive")
		}
	}
	idB := packOf(t, src, b[0])
	out := t.TempDir()
	var kept, dropped bytes.Buffer
	o := testOptions(src, writeList(t, out, a), out)
	o.Keep = keepOf(t, b, &kept, &dropped)
	o.WholePacks = []string{idB}
	res, err := run(ctx, o, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 4 || res.PacksDone != 2 {
		t.Fatalf("result %+v", res)
	}
	if n := checkOutput(t, out, b); n != 4 {
		t.Fatalf("keep set blocks stored %d of 4", n)
	}
	lines := packLines(t, out)
	if len(lines) != 2 || lines[1][1] != idB || lines[1][3] != "0" {
		t.Fatalf("pack lines %v", lines)
	}
}

// TestRescueKeepOnceInFlush stores a key once when, in one flush, it is
// wanted in a placed pack and a keep set block of a listed pack that holds
// it too: an unplaced pack with no e row, sized by SIZE.
func TestRescueKeepOnceInFlush(t *testing.T) {
	src := newSource(t)
	a := src.pack(3)
	src.close()

	// Write a second pack of a[0] and two new blocks through a store with
	// its own index, so the source index has no row of it.
	other := kvtx_prefixer.NewPrefixer(hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()), block_store_s3.PackIndexPrefix(testStoreID))
	store := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), dirObjects{}, filepath.Join(src.dir, testRoot), testPrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true, SoleWriter: true, Index: other,
	})
	batch := []*block.PutBatchEntry{{Ref: a[0], Data: src.data[string(packfile.BlockKey(a[0].GetHash()))], RefsKnown: true}}
	var fresh []*block.BlockRef
	for range 2 {
		data := make([]byte, 300)
		_, _ = rand.Read(data)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		fresh = append(fresh, ref)
	}
	if _, err := store.PutBlockBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	store.Close()
	idA := packOf(t, src, a[1])
	var idB string
	for id := range packIndexes(t, filepath.Join(src.dir, testRoot), testPrefix+"packs/") {
		if id != idA {
			idB = id
		}
	}

	// Run with every block in one flush: a[0] is stored once.
	out := t.TempDir()
	var kept, dropped bytes.Buffer
	o := testOptions(src, writeList(t, out, a), out)
	o.Keep = keepOf(t, append([]*block.BlockRef{a[0]}, fresh...), &kept, &dropped)
	o.WholePacks = []string{idB}
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 3 || res.PacksDone != 2 || srv.sizes != 1 {
		t.Fatalf("result %+v, sizes %d", res, srv.sizes)
	}
	if n := outputCount(t, out, a[0]); n != 1 {
		t.Fatalf("a[0] stored %d times", n)
	}
	if n := checkOutput(t, out, fresh); n != 2 {
		t.Fatalf("fresh keep blocks stored %d of 2", n)
	}
	if n := strings.Count(kept.String(), "\n"); n != 2 {
		t.Fatalf("kept lines %d: %q", n, kept.String())
	}
}

// TestRescueWholePacksOrder fetches the listed packs no ref places after
// every placed pack, in list order, leaves a listed pack the refs place in
// its own place, and counts the tail last under --max-packs.
func TestRescueWholePacksOrder(t *testing.T) {
	src := newSource(t)
	x, y, l1, l2 := src.pack(3), src.pack(2), src.pack(2), src.pack(2)
	idX, idY, id1, id2 := packOf(t, src, x[0]), packOf(t, src, y[0]), packOf(t, src, l1[0]), packOf(t, src, l2[0])
	refs := append(append([]*block.BlockRef{}, x...), y[0])
	keepRefs := []*block.BlockRef{l1[0], l2[0]}
	order := func(out string) string {
		var ids []string
		for _, f := range packLines(t, out) {
			ids = append(ids, map[string]string{idX: "x", idY: "y", id1: "l1", id2: "l2"}[f[1]])
		}
		return strings.Join(ids, ",")
	}
	for _, c := range []struct {
		maxPacks int
		want     string
	}{{0, "x,y,l2,l1"}, {3, "x,y,l2"}} {
		out := t.TempDir()
		var kept, dropped bytes.Buffer
		o := testOptions(src, writeList(t, out, refs), out)
		o.Keep = keepOf(t, keepRefs, &kept, &dropped)
		o.WholePacks = []string{id2, idY, id1}
		o.MaxPacks = c.maxPacks
		if _, err := run(context.Background(), o, &fakeServer{dir: src.dir}); err != nil {
			t.Fatal(err)
		}
		if got := order(out); got != c.want {
			t.Fatalf("max-packs %d: order %s, want %s", c.maxPacks, got, c.want)
		}
	}
}

// TestReadWholePackList reads ids in file order without repeats, skipping blank
// and comment lines, and refuses a line that is not a pack id.
func TestReadWholePackList(t *testing.T) {
	a, b := identity.NewPackID(), identity.NewPackID()
	path := filepath.Join(t.TempDir(), "packs")
	if err := os.WriteFile(path, []byte("# unplaced\n"+b+"\n\n"+a+"\tshared\n"+b+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := readWholePackList(path)
	if err != nil || strings.Join(ids, ",") != b+","+a {
		t.Fatalf("ids %v: %v", ids, err)
	}
	if err := os.WriteFile(path, []byte(a+"\nnot-a-pack\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readWholePackList(path); err == nil {
		t.Fatal("read a bad line")
	}
}

// TestRescueWholePacksStopOnDrop drops the connection on the read of a
// listed pack the refs do not place: the run stops at once with the drop
// line, the listed pack is not finished, and a restart reads it alone and
// keeps its keep set blocks once.
func TestRescueWholePacksStopOnDrop(t *testing.T) {
	// Drop the second request, the listed pack's read after the placed one.
	src := newSource(t)
	a, b := src.pack(3), src.pack(5)
	idB := packOf(t, src, b[0])
	src.dropHashRows(b)
	out := t.TempDir()
	list := writeList(t, out, a)
	opts := func(log *bytes.Buffer) *options {
		o := dropOptions(src, list, out, log)
		o.Keep = keepOf(t, b[:3], &bytes.Buffer{}, &bytes.Buffer{})
		o.WholePacks = []string{idB}
		return o
	}
	first := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, nil
	}}
	var log bytes.Buffer
	res1, err := run(context.Background(), opts(&log), first)
	var hard *hardStopError
	if !errors.As(err, &hard) || res1.Done != 3 || res1.PacksDone != 1 {
		t.Fatalf("first run: %v, %+v", err, res1)
	}
	if calls, _ := first.count(); calls != 2 {
		t.Fatalf("first run: %d calls, want 2", calls)
	}
	if n := strings.Count(log.String(), "stop: connection dropped: "); n != 1 {
		t.Fatalf("%d drop lines in %q, want 1", n, log.String())
	}
	if n := checkOutput(t, out, b); n != 0 {
		t.Fatalf("%d listed pack blocks stored before the drop", n)
	}

	// The restart reads the listed pack alone and keeps its blocks once.
	second := &fakeServer{dir: src.dir}
	res2, err := run(context.Background(), opts(&bytes.Buffer{}), second)
	if calls, _ := second.count(); err != nil || calls != 1 || res2.PacksDone != 1 {
		t.Fatalf("restart: %v, calls %d, %+v", err, calls, res2)
	}
	for _, ref := range b[:3] {
		if n := outputCount(t, out, ref); n != 1 {
			t.Fatalf("keep set block stored %d times, want 1", n)
		}
	}

}

// TestRescueWholePacksNotInPacks fails at startup when --packs bounds the
// plan and a listed whole pack is outside it, rather than skip it in
// silence: the command fails before it opens the index or reads the netrc,
// and run fails before any request. The message names no pack id.
func TestRescueWholePacksNotInPacks(t *testing.T) {
	src := newSource(t)
	a, b := src.pack(3), src.pack(5)
	idA, idB := packOf(t, src, a[0]), packOf(t, src, b[0])
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	keepPath := filepath.Join(dir, "keep.tails")
	writeKeys(t, keepPath, b[:3])
	const want = "whole pack not in --packs"

	// The command refuses before the index and the netrc, both missing.
	cmd := &args{
		refs: filepath.Join(dir, "no-refs"), index: filepath.Join(dir, "no-index"),
		netrc: filepath.Join(dir, "no-netrc"), out: filepath.Join(dir, "out"),
		layout: layoutPackStore, order: orderWanted, sessions: 1, maxBytes: 1 << 20, maxPackMiB: 1,
		packList: write("packs", idA+"\n"), wholePacks: write("whole", idB+"\n"), keepSet: keepPath,
	}
	err := cmd.run()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("command: %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), idB) {
		t.Fatalf("message %q names the pack", err)
	}

	// A list that holds every whole pack passes the check.
	cmd.packList = write("packs2", idA+"\n"+idB+"\n")
	if err := cmd.run(); err == nil || strings.Contains(err.Error(), want) {
		t.Fatalf("command with both packs: %v, want a later error", err)
	}

	// run refuses before any request.
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, a), out)
	o.Keep = keepOf(t, b[:3], &bytes.Buffer{}, &bytes.Buffer{})
	o.WholePacks = []string{idB}
	o.Packs = map[string]bool{idA: true}
	srv := &fakeServer{dir: src.dir}
	if _, err := run(context.Background(), o, srv); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("run: %v, want %q", err, want)
	}
	if calls, _ := srv.count(); calls != 0 {
		t.Fatalf("run: %d calls, want 0", calls)
	}
}
