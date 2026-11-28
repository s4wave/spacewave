package refwalk

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestScanRefs checks that a scan of a block's bytes with no type finds every
// ref the block's type extracts, which is what lets a reader hold the
// children of a block it cannot type: a ref the scan missed would be a child
// nothing holds. It also checks that a child named twice is not reported as
// missed, that a ref in a field the type does not define is found as an
// extra, and that bytes cut short are refused whole.
func TestScanRefs(t *testing.T) {
	// Build a chunked blob whose first chunk repeats.
	newRef := func(seed string) *block.BlockRef {
		t.Helper()
		h, err := hash.Sum(hash.HashType_HashType_BLAKE3, []byte(seed))
		if err != nil {
			t.Fatal(err)
		}
		return block.NewBlockRef(h)
	}
	first := newRef("first")
	b := &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: 3, ChunkIndex: &blob.ChunkIndex{Chunks: []*blob.Chunk{
		{DataRef: first, Size: 1},
		{DataRef: newRef("second"), Size: 1, Start: 1},
		{DataRef: first, Size: 1, Start: 2},
	}}}
	data, err := b.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	typed, err := block.ExtractBlockRefs(b)
	if err != nil || len(typed) != 3 {
		t.Fatalf("typed refs: %d, err %v", len(typed), err)
	}

	// The scan finds both children, each once, and the compare misses none.
	hits, ok := ScanHits(data)
	if !ok || len(hits) != 2 {
		t.Fatalf("scan: ok %v, %d refs, want 2", ok, len(hits))
	}
	if missed, extra := scanCompare(typed, data); len(missed) != 0 || len(extra) != 0 {
		t.Fatalf("compare: %d missed, %d extra, want none", len(missed), len(extra))
	}

	// A ref in field 15, which no blob message defines, is an extra, with
	// the path of the hash below it.
	stray, err := newRef("stray").MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	with := append(bytes.Clone(data), 15<<3|2, byte(len(stray)))
	with = append(with, stray...)
	missed, extra := scanCompare(typed, with)
	if len(missed) != 0 || len(extra) != 1 || extra[0].Path != "15.1" {
		t.Fatalf("stray ref: %d missed, extra %v", len(missed), extra)
	}

	// Bytes cut short are not a message, and the scan returns nothing.
	if hits, ok := ScanHits(data[:len(data)-1]); ok || len(hits) != 0 {
		t.Fatalf("cut bytes: ok %v, %d refs", ok, len(hits))
	}

	// A histogram counts a scanned block in the row of no type, as a leaf
	// when the scan found no ref, and moves it to its type's row when a typed
	// read confirms one.
	hist := NewHistogram()
	hist.Add(Visit{Node: Node{Ref: first, Untyped: "no type"}, Found: true, Data: data})
	if rows := hist.Rows(); len(rows) != 1 || rows[0].Type != UntypedRow || rows[0].Blocks != 1 || rows[0].Leaves != 1 {
		t.Fatalf("scanned block: rows %+v", rows)
	}
	hist.Add(Visit{Node: Node{Ref: first, Ctor: blob.NewBlobBlock}, Found: true, Data: data, Refs: typed})
	rows, total := hist.Rows(), hist.Total()
	if len(rows) != 1 || rows[0].Type != "*blob.Blob" || rows[0].Leaves != 0 || total.Blocks != 1 || total.Leaves != 0 {
		t.Fatalf("confirmed block: rows %+v, total %+v", rows, total)
	}
}

// TestPayloadNodeBlob checks that a payload root guessed as a blob is
// confirmed when it is one: the walk keeps the decode and returns the refs of
// a chunked blob's chunks, so ref-repair verifies its list. A walk that
// refused real blobs would leave every payload blob unverified. It then
// checks the other side of the guess: bytes that decode as a valid blob and
// also hold a field no blob type defines, at any level, are another type's
// block, whose refs the blob decode skipped, so the walk reports them unsure
// with no refs and a rebuild writes no short list from them.
func TestPayloadNodeBlob(t *testing.T) {
	// Write a blob of several chunks with the blob package's builder.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	data := make([]byte, 2<<20)
	if _, err := prng.BuildSeededReader([]byte("payload blob")).Read(data); err != nil {
		t.Fatal(err)
	}
	opts := &blob.BuildBlobOpts{RawHighWaterMark: 1}
	if _, err := blob.BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), cursor, opts); err != nil {
		t.Fatal(err)
	}
	ref, cursor, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := blob.UnmarshalBlob(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	chunks := b.GetChunkIndex().GetChunks()
	if len(chunks) < 2 {
		t.Fatalf("built %d chunks, want several", len(chunks))
	}

	// Walk the root as an operation payload no World typed.
	w := NewWalker(logrus.NewEntry(logrus.New()), store, nil)
	defer w.Close()
	root, ok, err := w.PayloadNode(ctx, &bucket.ObjectRef{}, ref)
	if err != nil || !ok {
		t.Fatalf("payload node: ok %v, err %v", ok, err)
	}
	var visits []Visit
	err = w.Walk(ctx, root, func(_ context.Context, batch []Visit) error {
		visits = append(visits, batch...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The root is certain and references every chunk.
	if len(visits) == 0 || visits[0].Node.Ref.MarshalString() != ref.MarshalString() {
		t.Fatal("walk did not visit the payload root first")
	}
	top := visits[0]
	if !top.Found || top.Unsure || top.DecodeErr != nil {
		t.Fatalf("root: found %v, unsure %v, decode err %v", top.Found, top.Unsure, top.DecodeErr)
	}
	refs := make(map[string]bool, len(top.Refs))
	for _, r := range top.Refs {
		refs[r.MarshalString()] = true
	}
	for i, c := range chunks {
		if !refs[c.GetDataRef().MarshalString()] {
			t.Fatalf("root refs lack chunk %d", i)
		}
	}

	// Build the same blob holding field 15, which no blob message defines,
	// in the blob and in each message below it that the blob package
	// defines.
	type message interface {
		MarshalVT() ([]byte, error)
		UnmarshalVT(data []byte) error
	}
	encode := func(msg message, suffix ...byte) []byte {
		t.Helper()
		data, err := msg.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return append(data, suffix...)
	}
	unknown := []byte{15 << 3, 1}
	reread := func(msg, out message) {
		t.Helper()
		if err := out.UnmarshalVT(encode(msg, unknown...)); err != nil {
			t.Fatal(err)
		}
	}
	index, chunk, args := &blob.ChunkIndex{}, &blob.Chunk{}, &blob.ChunkerArgs{}
	rabin, jc := &blob.RabinArgs{}, &blob.JcArgs{}
	reread(b.GetChunkIndex(), index)
	reread(chunks[0], chunk)
	reread(b.GetChunkIndex().GetChunkerArgs(), args)
	reread(rabin.CloneVT(), rabin)
	reread(jc.CloneVT(), jc)
	edit := func(fn func(index *blob.ChunkIndex)) []byte {
		t.Helper()
		other := b.CloneVT()
		fn(other.ChunkIndex)
		return encode(other)
	}
	foreign := map[string][]byte{
		"blob":         encode(b, unknown...),
		"chunk index":  encode(&blob.Blob{BlobType: b.GetBlobType(), TotalSize: b.GetTotalSize(), ChunkIndex: index}),
		"chunk":        edit(func(index *blob.ChunkIndex) { index.Chunks[0] = chunk }),
		"chunker args": edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = args }),
		"rabin args":   edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = &blob.ChunkerArgs{RabinArgs: rabin} }),
		"jc args":      edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = &blob.ChunkerArgs{JcArgs: jc} }),
	}

	// Each still references every chunk as a blob, and the walk trusts
	// none.
	for name, data := range foreign {
		guessed := &blob.Blob{}
		if err := guessed.UnmarshalVT(data); err != nil || guessed.Validate() != nil || len(guessed.GetChunkIndex().GetChunks()) != len(chunks) {
			t.Fatalf("field in the %s: not a valid blob of %d chunks: %v", name, len(chunks), err)
		}
		foreignRef, _, err := store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		node, ok, err := w.PayloadNode(ctx, &bucket.ObjectRef{}, foreignRef)
		if err != nil || !ok {
			t.Fatalf("field in the %s: payload node: ok %v, err %v", name, ok, err)
		}
		var got []Visit
		err = w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			got = append(got, batch...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !got[0].Found || !got[0].Unsure || len(got[0].Refs) != 0 {
			t.Fatalf("field in the %s: visits %+v, want one unsure visit with no refs", name, got)
		}
	}
}

// TestPayloadNodeRootType checks that a payload root no walked World typed,
// such as the key-value tree an operation above the checkpoint set, is
// guessed as a blob and left unsure, which compares none of its children, and
// that a walker told every root is such a tree reads it with certainty and
// reports its children.
func TestPayloadNodeRootType(t *testing.T) {
	// Write a key-value tree large enough to have children.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	ref := writeTree(t, store, 500)

	// Walk it as a payload, guessed and then declared.
	root := func(w *Walker) Visit {
		t.Helper()
		defer w.Close()
		node, ok, err := w.PayloadNode(ctx, &bucket.ObjectRef{}, ref)
		if err != nil || !ok {
			t.Fatalf("payload node: ok %v, err %v", ok, err)
		}
		var top Visit
		err = w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			if top.Node.Ref == nil {
				top = batch[0]
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return top
	}
	le := logrus.NewEntry(logrus.New())
	if guessed := root(NewWalker(le, store, nil)); !guessed.Unsure || len(guessed.Refs) != 0 {
		t.Fatalf("guessed root: unsure %v, %d refs, want unsure with none", guessed.Unsure, len(guessed.Refs))
	}
	w := NewWalker(le, store, nil)
	w.TypeRootsAs(blocktype.New[kvtx_block.KeyValueStore](kvtx_block.KeyValueStoreTypeID))
	if typed := root(w); typed.Unsure || typed.DecodeErr != nil || len(typed.Refs) == 0 {
		t.Fatalf("declared root: unsure %v, decode err %v, %d refs, want certain with refs", typed.Unsure, typed.DecodeErr, len(typed.Refs))
	}
}

// TestWalkResume checks the contract a stored Visited relies on: a walk that
// stops leaves finished only blocks whose whole subtree its visitor received,
// and a walk resumed from the same Visited reads every block the stopped one
// did not finish. A walk that recorded a block when it took it, before reading
// what is below it, would leave a resumed walk skipping that subtree.
func TestWalkResume(t *testing.T) {
	// Write a tree of several levels, and read its graph with one walk.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	ref := writeTree(t, store, 3000)
	kvs := blocktype.New[kvtx_block.KeyValueStore](kvtx_block.KeyValueStoreTypeID)
	walk := func(v Visited, stopAfter int) (map[string]Visit, error) {
		t.Helper()
		w := NewWalker(logrus.NewEntry(logrus.New()), store, nil)
		defer w.Close()
		w.TypeRootsAs(kvs)
		w.VisitIn(v)
		node, _, err := w.PayloadNode(ctx, &bucket.ObjectRef{}, ref)
		if err != nil {
			t.Fatal(err)
		}
		read := make(map[string]Visit)
		var batches int
		err = w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			if batches == stopAfter {
				return errStop
			}
			batches++
			for _, rd := range batch {
				read[rd.Node.Ref.MarshalString()] = rd
			}
			return nil
		})
		return read, err
	}
	graph, err := walk(NewMemVisited(), -1)
	if err != nil || len(graph) < 8*batchSize {
		t.Fatalf("whole walk: %d blocks, err %v", len(graph), err)
	}

	// Stop a walk at points through it, then resume it.
	batches := len(graph) / batchSize
	for _, stopAfter := range []int{1, batches / 3, batches / 2, batches - 1} {
		v := NewMemVisited()
		first, err := walk(v, stopAfter)
		if err != errStop {
			t.Fatalf("stop after %d batches: %v", stopAfter, err)
		}

		// Every finished block's subtree was read.
		read := make(map[string]bool)
		var below func(key string) bool
		below = func(key string) bool {
			if got, ok := read[key]; ok {
				return got
			}
			_, got := first[key]
			for _, child := range graph[key].Refs {
				if _, ok := graph[child.MarshalString()]; ok && got {
					got = below(child.MarshalString())
				}
			}
			read[key] = got
			return got
		}
		var finished int
		for key, rd := range graph {
			if typed, _ := v.Seen(rd.Node.Ref); typed {
				finished++
				if !below(key) {
					t.Fatalf("stop after %d batches: finished %s with %d children, not all read", stopAfter, key, len(rd.Refs))
				}
			}
		}

		// The resumed walk reads every block the first did not finish.
		second, err := walk(v, -1)
		if err != nil {
			t.Fatal(err)
		}
		for key := range graph {
			if _, again := second[key]; !again && !below(key) {
				t.Fatalf("stop after %d batches: resumed walk skipped %s, whose subtree the first did not read", stopAfter, key)
			}
		}
		if len(second) > len(graph)-finished {
			t.Fatalf("stop after %d batches: resumed walk read %d of %d blocks, %d finished", stopAfter, len(second), len(graph), finished)
		}
	}
}

// TestWalkResumeSharedChild checks that a block named by two blocks taken in
// one batch finishes neither until its own subtree does. In R -> {A, B},
// A -> B, B -> C, one batch takes A and B, and A then queues B while B is in
// progress. A walk that counted B done for A at once would finish A before C
// was read, and a resumed walk of A would skip B and C.
func TestWalkResumeSharedChild(t *testing.T) {
	// Write the graph as tree nodes, so each block's children are its refs.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	put := func(name string, left, right *block.BlockRef) *block.BlockRef {
		t.Helper()
		n := &kvtx_block_iavl.Node{Key: []byte(name), LeftChildRef: left, RightChildRef: right}
		if left != nil || right != nil {
			n.Height = 1
		}
		data, err := n.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		ref, _, err := store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	c := put("C", nil, nil)
	b := put("B", c, nil)
	a := put("A", b, nil)
	r := put("R", a, b)
	walk := func(v Visited, root *block.BlockRef, stop bool) (map[string]bool, error) {
		t.Helper()
		w := NewWalker(logrus.NewEntry(logrus.New()), store, nil)
		defer w.Close()
		w.VisitIn(v)
		read := make(map[string]bool)
		node := Node{Ref: root, Ctor: func() block.Block { return &kvtx_block_iavl.Node{} }}
		err := w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			for _, rd := range batch {
				if stop && rd.Node.Ref.EqualsRef(c) {
					return errStop
				}
			}
			for _, rd := range batch {
				read[rd.Node.Ref.MarshalString()] = true
			}
			return nil
		})
		return read, err
	}

	// Stop before C: no block above C may be finished.
	v := NewMemVisited()
	if _, err := walk(v, r, true); err != errStop {
		t.Fatalf("stop before C: %v", err)
	}
	for name, ref := range map[string]*block.BlockRef{"R": r, "A": a, "B": b, "C": c} {
		if typed, _ := v.Seen(ref); typed {
			t.Fatalf("%s finished with C unread", name)
		}
	}

	// The resumed walk of A reads A, B and C, and finishes the whole graph.
	second, err := walk(v, a, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, ref := range map[string]*block.BlockRef{"A": a, "B": b, "C": c} {
		if !second[ref.MarshalString()] {
			t.Fatalf("resumed walk of A skipped %s", name)
		}
	}
	if _, err := walk(v, r, false); err != nil {
		t.Fatal(err)
	}
	for name, ref := range map[string]*block.BlockRef{"R": r, "A": a, "B": b, "C": c} {
		if typed, _ := v.Seen(ref); !typed {
			t.Fatalf("%s not finished after a whole walk", name)
		}
	}
}

// TestWalkInexactDecode checks that the walk trusts a typed decode only when
// the type explains every byte. Bytes that hold a ref in a field the type does
// not declare, at the top or in a nested message, or that repeat a field the
// decode overwrites, decode without error, so a walk that trusted them would
// miss a child. Each is undecodable and the walk does not descend.
func TestWalkInexactDecode(t *testing.T) {
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	put := func(data []byte) *block.BlockRef {
		t.Helper()
		ref, _, err := store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	encode := func(n *kvtx_block_iavl.Node) []byte {
		t.Helper()
		data, err := n.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	child := put(encode(&kvtx_block_iavl.Node{Key: []byte("C")}))
	childRef, err := child.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Field 15 holds the child's ref as bytes, in the node or in its left
	// child ref, and a second encoding overwrites the node's key.
	unknown := append([]byte{15<<3 | 2, byte(len(childRef))}, childRef...)
	nested := &block.BlockRef{}
	if err := nested.UnmarshalVT(append(slices.Clone(childRef), unknown...)); err != nil {
		t.Fatal(err)
	}
	parent := &kvtx_block_iavl.Node{Key: []byte("P"), LeftChildRef: child, Height: 1}
	blocks := map[string][]byte{
		"exact":          encode(parent),
		"unknown field":  append(encode(parent), unknown...),
		"nested unknown": encode(&kvtx_block_iavl.Node{Key: []byte("P"), LeftChildRef: nested, Height: 1}),
		"repeated field": append(encode(parent), encode(&kvtx_block_iavl.Node{Key: []byte("Q")})...),
	}
	for name, data := range blocks {
		w := NewWalker(logrus.NewEntry(logrus.New()), store, nil)
		var got []Visit
		node := Node{Ref: put(data), Ctor: func() block.Block { return &kvtx_block_iavl.Node{} }}
		err := w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			got = append(got, batch...)
			return nil
		})
		w.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if name == "exact" {
			if len(got) != 2 || got[0].DecodeErr != nil || len(got[0].Refs) != 1 {
				t.Fatalf("exact: visits %+v, want the node and its child", got)
			}
			continue
		}
		if len(got) != 1 || !got[0].Found || !errors.Is(got[0].DecodeErr, ErrInexactDecode) || len(got[0].Refs) != 0 {
			t.Fatalf("%s: visits %+v, want one inexact visit with no refs", name, got)
		}
	}
}

// errStop stops a walk in TestWalkResume.
var errStop = errors.New("stop")

// writeTree writes a key-value tree of rows rows to store and returns its
// root.
func writeTree(t *testing.T, store block.StoreOps, rows int) *block.BlockRef {
	t.Helper()
	ctx := t.Context()
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	kvs := kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault)
	cursor.SetBlock(kvs, true)
	kv, err := kvs.BuildKvTransaction(ctx, cursor, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		key := []byte("row-" + strconv.Itoa(i))
		if err := kv.Set(ctx, key, bytes.Repeat(key, 8)); err != nil {
			t.Fatal(err)
		}
	}
	if err := kv.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	kv.Discard()
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
