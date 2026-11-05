package packcopy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// testBlock is one block of the test graph.
type testBlock struct {
	// ref is the block's ref.
	ref *block.BlockRef
	// data is the block's bytes.
	data []byte
	// refs are the block's children.
	refs []*block.BlockRef
}

// newTestBlock builds a block over its children.
func newTestBlock(t *testing.T, name string, children ...*testBlock) *testBlock {
	t.Helper()
	data := []byte(name)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := &testBlock{ref: ref, data: data}
	for _, child := range children {
		b.refs = append(b.refs, child.ref)
	}
	return b
}

// smallPacks gives a target two-block packs, so a small graph spans several.
type smallPacks struct {
	*block_store_s3.PackStore
}

// GetBatchLimits returns two-block packs.
func (s smallPacks) GetBatchLimits() block.BatchLimits {
	limits := s.PackStore.GetBatchLimits()
	limits.Blocks = 2
	return limits
}

// corruptSource serves wrong bytes for one block.
type corruptSource struct {
	Source
	// bad is the block served wrong.
	bad *block.BlockRef
}

// GetStoredBlock flips the bad block's bytes.
func (c corruptSource) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := c.Source.GetStoredBlock(ctx, ref)
	if stored != nil && ref.EqualsRef(c.bad) {
		return &block.StoredBlock{Data: []byte("wrong"), Refs: stored.Refs, RefsKnown: true}, nil
	}
	return stored, err
}

// holeList collects a copy's holes.
type holeList struct {
	// mtx guards reasons.
	mtx sync.Mutex
	// reasons holds each hole's reason by ref.
	reasons map[string]string
}

// add records a hole.
func (h *holeList) add(ref *block.BlockRef, reason string) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.reasons[ref.MarshalString()] = reason
	return nil
}

// provenRefs lists which of the blocks have a proof.
func provenRefs(t *testing.T, ctx context.Context, proofs kvtx.Store, blocks []*testBlock) []*testBlock {
	t.Helper()
	tx, err := proofs.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var out []*testBlock
	for _, b := range blocks {
		_, found, err := tx.Get(ctx, packfile.BlockKey(b.ref.GetHash()))
		if err != nil {
			t.Fatal(err)
		}
		if found {
			out = append(out, b)
		}
	}
	return out
}

// checkProofs fails when a proven block or anything under it is not durable
// with its true refs, as read by a store that shares nothing with the copy.
func checkProofs(t *testing.T, ctx context.Context, client *block_store_s3.Client, proofs kvtx.Store, blocks []*testBlock) {
	t.Helper()
	reader := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer reader.Close()
	byRef := make(map[string]*testBlock)
	for _, b := range blocks {
		byRef[b.ref.MarshalString()] = b
	}
	var check func(b *testBlock)
	check = func(b *testBlock) {
		stored, err := reader.GetStoredBlock(ctx, b.ref)
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil || !bytes.Equal(stored.Data, b.data) || len(stored.Refs) != len(b.refs) {
			t.Fatalf("proof runs ahead of block %q: %v", b.data, stored)
		}
		for _, ref := range b.refs {
			check(byRef[ref.MarshalString()])
		}
	}
	for _, b := range provenRefs(t, ctx, proofs, blocks) {
		check(b)
	}
}

// writeFrames writes blocks as a frame file, ending in a cut frame.
func writeFrames(t *testing.T, path string, blocks ...*testBlock) {
	t.Helper()
	var out []byte
	for _, b := range blocks {
		key := packfile.BlockKey(b.ref.GetHash())
		obj, err := block.EncodeBlockObject(b.data, b.refs, true)
		if err != nil {
			t.Fatal(err)
		}
		out = binary.AppendUvarint(out, uint64(len(key)))
		out = append(out, key...)
		out = binary.AppendUvarint(out, 1)
		out = binary.AppendUvarint(out, uint64(len(obj)))
		out = append(out, obj...)
	}
	out = append(out, 40, 1, 2)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCopyRunThrough walks one store through the life of a copy: a corrupted
// read, a missing block, a block listed with unknown refs, a failed pack among
// concurrent uploads, a resume, a spent budget and a pull that fills the holes.
func TestCopyRunThrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Build the graph. shared has two parents, and the root reaches it twice.
	leaf := newTestBlock(t, "leaf")
	missing := newTestBlock(t, "missing")
	unknown := newTestBlock(t, "unknown", leaf)
	bad := newTestBlock(t, "bad")
	shared := newTestBlock(t, "shared", missing)
	left := newTestBlock(t, "left", shared, bad)
	right := newTestBlock(t, "right", shared, unknown)
	other := newTestBlock(t, "other")
	filler := newTestBlock(t, "filler", other)
	root := newTestBlock(t, "root", left, right, filler)
	blocks := []*testBlock{root, left, right, filler, other, shared, unknown, leaf, bad, missing}

	// Fill the source without the missing block. The unknown block's pack
	// entry claims it is a leaf, as the bad old records do.
	bucket, client := s3test.New(t)
	source := block_store_s3.NewPackStore(le, client, s3test.BucketName, "src/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer source.Close()
	var batch []*block.PutBatchEntry
	for _, b := range blocks {
		switch b {
		case missing:
		case unknown:
			batch = append(batch, &block.PutBatchEntry{Ref: b.ref, Data: b.data, RefsKnown: true})
		default:
			batch = append(batch, &block.PutBatchEntry{Ref: b.ref, Data: b.data, Refs: b.refs, RefsKnown: true})
		}
	}
	if _, err := source.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	list := hex.EncodeToString(packfile.BlockKey(unknown.ref.GetHash()))
	listed, err := NewUnknownRefs(source, strings.NewReader("hyperline-world "+list+" null\n"))
	if err != nil {
		t.Fatal(err)
	}
	sources := []Source{corruptSource{Source: listed, bad: bad.ref}}

	// Every run opens the target afresh, as a restart does.
	proofs := store_kvtx_inmem.NewStore()
	index := store_kvtx_inmem.NewStore()
	graph := NewGraphStream(client, s3test.BucketName, "v2/graph/")
	pass := func(srcs []Source) (*Result, *holeList, error) {
		target := block_store_s3.NewPackStore(le, client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true, Index: index})
		defer target.Close()
		if err := target.BuildIndex(ctx, 2, nil); err != nil {
			t.Fatal(err)
		}
		holes := &holeList{reasons: make(map[string]string)}
		result, err := Copy(ctx, &Options{
			Roots:   []*block.BlockRef{root.ref},
			Target:  smallPacks{target},
			Sources: srcs,
			Proofs:  proofs,
			Graph:   graph,
			Holes:   holes.add,
			Uploads: 3,
		})
		return result, holes, err
	}

	// Fail every pack after the first, past the client's retries. Packs may
	// land around the failure; no proof may lead them.
	var mtx sync.Mutex
	packs := 0
	bucket.FailPut(func(key string) bool {
		mtx.Lock()
		defer mtx.Unlock()
		if !strings.HasPrefix(key, "v2/packs/") {
			return false
		}
		packs++
		return packs > 1
	})
	if _, _, err := pass(sources); err == nil {
		t.Fatal("a failed pack must fail the run")
	}
	checkProofs(t, ctx, client, proofs, blocks)
	bucket.FailPut(nil)

	// Resume. The holes keep every block above them unproven.
	result, holes, err := pass(sources)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("a copy with holes is not complete")
	}
	want := map[string]string{
		missing.ref.MarshalString(): ReasonMissing,
		unknown.ref.MarshalString(): ReasonRefsUnknown,
		bad.ref.MarshalString():     ReasonCorrupt,
	}
	if len(holes.reasons) != len(want) {
		t.Fatalf("holes: %v", holes.reasons)
	}
	for ref, reason := range want {
		if holes.reasons[ref] != reason {
			t.Fatalf("hole %s: %q, want %q", ref, holes.reasons[ref], reason)
		}
	}
	proven := provenRefs(t, ctx, proofs, blocks)
	for _, b := range []*testBlock{root, left, right, shared, missing, unknown, bad} {
		if slices.Contains(proven, b) {
			t.Fatalf("block %q is proven over a hole", b.data)
		}
	}
	if !slices.Contains(proven, filler) || !slices.Contains(proven, other) {
		t.Fatalf("blocks with no hole stay unproven: %d proven", len(proven))
	}
	checkProofs(t, ctx, client, proofs, blocks)

	// A pull answers the holes, with the block a pack calls a leaf carrying
	// its true refs. The leaf below it is walked only now.
	path := filepath.Join(t.TempDir(), "blocks.frames")
	writeFrames(t, path, missing, unknown, bad)
	frames, err := OpenFrameStore([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer frames.Close()
	result, holes, err = pass(append(sources, frames))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(holes.reasons) != 0 {
		t.Fatalf("copy not complete: %+v, holes %v", result, holes.reasons)
	}
	checkProofs(t, ctx, client, proofs, blocks)
	if got := provenRefs(t, ctx, proofs, blocks); len(got) != len(blocks) {
		t.Fatalf("%d of %d blocks proven", len(got), len(blocks))
	}

	// The graph stream names every block with its true refs.
	seen := make(map[string]int)
	err = graph.Scan(ctx, func(r *GraphRecord) error {
		seen[r.GetRef().MarshalString()] = len(r.GetRefs())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if n, ok := seen[b.ref.MarshalString()]; !ok || n != len(b.refs) {
			t.Fatalf("graph stream on %q: %d refs, found %v", b.data, n, ok)
		}
	}

	// A finished copy reads nothing and writes nothing.
	puts := bucket.Puts()
	result, _, err = pass(nil)
	if err != nil || !result.Complete || result.Held != 0 {
		t.Fatalf("repeat pass: %+v, %v", result, err)
	}
	if bucket.Puts() != puts {
		t.Fatal("a finished copy wrote")
	}

	// A spent budget stops a walk as an error, not as a hole.
	meter := NewMeter(client, 1, 1)
	metered := block_store_s3.NewPackStore(le, meter, s3test.BucketName, "src/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer metered.Close()
	_, err = metered.GetStoredBlock(ctx, root.ref)
	if err == nil {
		t.Fatal("a spent budget must stop the read")
	}
	if _, reason, stop := readFrom(ctx, metered, root.ref); stop == nil {
		t.Fatalf("spent budget became a hole: %q", reason)
	}
}
