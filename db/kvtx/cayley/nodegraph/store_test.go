package kvtx_cayley_nodegraph

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/graphtest"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// newTestStore opens a QuadStore over one write transaction of an in-memory
// store.
func newTestStore(t testing.TB) (graph.QuadStore, graph.Options, func()) {
	ctx := context.Background()
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return NewQuadStore(tx), graph.Options{}, func() { tx.Discard() }
}

// TestQuadStore_Conformance runs the Cayley quad store conformance suite.
//
// The path tests load data/testdata.nq relative to the working directory, so
// the test runs from the graphtest package's source directory.
func TestQuadStore_Conformance(t *testing.T) {
	file, _ := runtime.FuncForPC(reflect.ValueOf(graphtest.TestAll).Pointer()).FileLine(0)
	if !filepath.IsAbs(file) {
		t.Skip("graphtest source path unavailable")
	}
	t.Chdir(filepath.Dir(file))
	graphtest.TestAll(t, newTestStore, &graphtest.Config{})
}

// TestNodeIDsNeverReused checks that a node created after the node with the
// highest ID is deleted, in a later transaction, gets a new ID, and that the
// diff of the two trees names both quads through their out keys.
func TestNodeIDsNeverReused(t *testing.T) {
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	apply := func(action graph.Procedure, o string) {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		d := graph.Delta{Quad: quad.MakeIRI("a", "p", o, ""), Action: action}
		if err := NewQuadStore(tx).ApplyDeltas(ctx, []graph.Delta{d}, graph.IgnoreOpts{}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() map[string]string {
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		m := make(map[string]string)
		err = tx.ScanPrefix(ctx, nil, func(k, v []byte) error {
			m[string(k)] = string(v)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	open := func(m map[string]string) kvtx.Tx {
		s := store_kvtx_inmem.NewStore()
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range m {
			if err := tx.Set(ctx, []byte(k), []byte(v)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		tx, err = s.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		return tx
	}
	nodeID := func(m map[string]string, iri string) uint64 {
		id, ok := decodeID([]byte(m[string(valueKey(quad.IRI(iri).String()))]))
		if !ok {
			t.Fatalf("no node for %s", iri)
		}
		return id
	}

	// x takes the highest ID; deleting its quad deletes it, and y comes next.
	apply(graph.Add, "b")
	apply(graph.Add, "x")
	before := snapshot()
	apply(graph.Delete, "x")
	apply(graph.Add, "y")
	after := snapshot()
	if x, y := nodeID(before, "x"), nodeID(after, "y"); y <= x {
		t.Fatalf("y has ID %d, not past deleted x's %d", y, x)
	}

	// The tree diff names each quad by its own out key.
	var keys [][]byte
	for k, v := range before {
		if w, ok := after[k]; !ok || w != v {
			keys = append(keys, []byte(k))
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			keys = append(keys, []byte(k))
		}
	}
	quads, complete, err := ChangedQuads(ctx, open(before), open(after), keys, 100)
	if err != nil || !complete {
		t.Fatalf("changed quads: complete %v, %v", complete, err)
	}
	got := make(map[string]bool)
	for _, q := range quads {
		got[q.String()] = true
	}
	for _, o := range []string{"x", "y"} {
		if q := quad.MakeIRI("a", "p", o, ""); !got[q.String()] {
			t.Errorf("changed quads %v lack %v", quads, q)
		}
	}
	if len(quads) != 2 {
		t.Errorf("changed quads %v, want 2", quads)
	}
}

// testTree is a node graph tree in an in-memory store, written one committed
// transaction per call.
type testTree struct {
	t     *testing.T
	ctx   context.Context
	store kvtx.Store
}

// newTestTree returns an empty tree.
func newTestTree(t *testing.T) *testTree {
	return &testTree{t: t, ctx: context.Background(), store: store_kvtx_inmem.NewStore()}
}

// apply applies deltas in one transaction and commits it.
func (tr *testTree) apply(deltas ...graph.Delta) {
	tr.t.Helper()
	if err := tr.tryApply(nil, deltas...); err != nil {
		tr.t.Fatal(err)
	}
}

// tryApply applies deltas in one transaction, through wrap when it is set,
// and commits it, or discards it when they fail.
func (tr *testTree) tryApply(wrap func(kvtx.Tx) kvtx.Tx, deltas ...graph.Delta) error {
	tx, err := tr.store.NewTransaction(tr.ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	qtx := tx
	if wrap != nil {
		qtx = wrap(tx)
	}
	if err := NewQuadStore(qtx).ApplyDeltas(tr.ctx, deltas, graph.IgnoreOpts{}); err != nil {
		return err
	}
	return tx.Commit(tr.ctx)
}

// keys returns every committed key and value.
func (tr *testTree) keys() map[string]string {
	tr.t.Helper()
	tx, err := tr.store.NewTransaction(tr.ctx, false)
	if err != nil {
		tr.t.Fatal(err)
	}
	defer tx.Discard()
	m := make(map[string]string)
	err = tx.ScanPrefix(tr.ctx, nil, func(k, v []byte) error {
		m[string(k)] = string(v)
		return nil
	})
	if err != nil {
		tr.t.Fatal(err)
	}
	return m
}

// read calls cb with a QuadStore over a read transaction of the tree.
func (tr *testTree) read(cb func(qs *QuadStore)) {
	tr.t.Helper()
	tx, err := tr.store.NewTransaction(tr.ctx, false)
	if err != nil {
		tr.t.Fatal(err)
	}
	defer tx.Discard()
	cb(NewQuadStore(tx))
}

// quads returns the committed quads matching filter.
func (tr *testTree) quads(filter quad.Quad) []quad.Quad {
	tr.t.Helper()
	var out []quad.Quad
	tr.read(func(qs *QuadStore) {
		res, err := qs.CollectFilteredQuadsBatch(tr.ctx, []quad.Quad{filter}, 0)
		if err != nil {
			tr.t.Fatal(err)
		}
		out = res[0]
	})
	return out
}

// id returns the committed node ID of an IRI, or 0 when it has no node.
func (tr *testTree) id(iri string) uint64 {
	tr.t.Helper()
	var id uint64
	tr.read(func(qs *QuadStore) {
		var err error
		if id, err = qs.lookupID(tr.ctx, quad.IRI(iri).String()); err != nil {
			tr.t.Fatal(err)
		}
	})
	return id
}

// add and del are the deltas adding and deleting the IRI quad s p o.
func add(s, p, o string) graph.Delta {
	return graph.Delta{Quad: quad.MakeIRI(s, p, o, ""), Action: graph.Add}
}

func del(s, p, o string) graph.Delta {
	return graph.Delta{Quad: quad.MakeIRI(s, p, o, ""), Action: graph.Delete}
}

// TestDeleteKeepsNodeWithEdges checks that deleting a quad keeps each
// endpoint that still has an edge, out or in, with its ID and its other quads,
// and deletes only the endpoint left with none.
func TestDeleteKeepsNodeWithEdges(t *testing.T) {
	// a has two out edges; b is the object of one and the subject of another.
	tr := newTestTree(t)
	tr.apply(add("a", "p", "b"), add("a", "p", "c"), add("b", "p", "d"))
	a, b := tr.id("a"), tr.id("b")

	// Deleting a p b keeps a (out edge to c) and b (out edge to d).
	tr.apply(del("a", "p", "b"))
	if tr.id("a") != a || tr.id("b") != b {
		t.Fatalf("a %d->%d, b %d->%d: a node with edges was deleted or moved", a, tr.id("a"), b, tr.id("b"))
	}
	if got := tr.quads(quad.Quad{Subject: quad.IRI("a")}); len(got) != 1 || got[0] != quad.MakeIRI("a", "p", "c", "") {
		t.Fatalf("a's quads = %v", got)
	}
	if got := tr.quads(quad.Quad{Object: quad.IRI("d")}); len(got) != 1 || got[0] != quad.MakeIRI("b", "p", "d", "") {
		t.Fatalf("d's in quads = %v", got)
	}
	if got := tr.quads(quad.Quad{Object: quad.IRI("b")}); len(got) != 0 {
		t.Fatalf("deleted quad still read: %v", got)
	}

	// Deleting a p c leaves a and c with no edge, so both go, meta and ID map.
	tr.apply(del("a", "p", "c"))
	keys := tr.keys()
	for _, iri := range []string{"a", "c"} {
		if _, ok := keys[string(valueKey(quad.IRI(iri).String()))]; ok {
			t.Errorf("%s left with no edge kept its ID map entry", iri)
		}
	}
	if _, ok := keys[string(metaKey(a))]; ok {
		t.Errorf("a left with no edge kept its meta")
	}
	if tr.id("b") != b {
		t.Errorf("b, with an out edge to d, lost its node")
	}

	// Deleting b p d leaves b with only the in edge from x: b stays, d goes.
	tr.apply(add("x", "p", "b"))
	d := tr.id("d")
	tr.apply(del("b", "p", "d"))
	if tr.id("b") != b {
		t.Errorf("b, with an in edge from x, lost its node")
	}
	if got := tr.quads(quad.Quad{Object: quad.IRI("b")}); len(got) != 1 || got[0] != quad.MakeIRI("x", "p", "b", "") {
		t.Fatalf("b's in quads = %v", got)
	}
	if tr.id("d") != 0 || tr.keys()[string(metaKey(d))] != "" {
		t.Errorf("d left with no edge kept its node")
	}

	// A time outside UTC keys its node the same way when it goes.
	at := quad.Time(time.Date(2026, 11, 12, 4, 0, 0, 0, time.FixedZone("PST", -8*3600)))
	q := quad.Quad{Subject: quad.IRI("x"), Predicate: quad.IRI("at"), Object: at}
	tr.apply(graph.Delta{Quad: q, Action: graph.Add})
	tr.apply(graph.Delta{Quad: q, Action: graph.Delete})
	for k := range tr.keys() {
		if strings.HasPrefix(k, string(valuePrefix)) && strings.Contains(k, "2026") {
			t.Errorf("deleted time left its ID map entry %q", k)
		}
	}
}

// TestPinnedPredicateAlsoObject checks that a value used as a predicate keeps
// its node when it also appears as an object and its last edge goes, in both
// orders of first use, so its ID never changes under the quads that name it.
func TestPinnedPredicateAlsoObject(t *testing.T) {
	for _, objectFirst := range []bool{false, true} {
		tr := newTestTree(t)
		if objectFirst {
			tr.apply(add("c", "q", "p"))
			tr.apply(add("a", "p", "b"))
		} else {
			tr.apply(add("a", "p", "b"), add("c", "q", "p"))
		}
		p := tr.id("p")

		// p's only edge is the in edge from c; deleting it keeps the pinned node.
		tr.apply(del("c", "q", "p"))
		if tr.id("p") != p {
			t.Fatalf("objectFirst=%v: pinned p moved from %d to %d", objectFirst, p, tr.id("p"))
		}
		if got := tr.quads(quad.Quad{Predicate: quad.IRI("p")}); len(got) != 1 || got[0] != quad.MakeIRI("a", "p", "b", "") {
			t.Fatalf("objectFirst=%v: quads by p = %v", objectFirst, got)
		}

		// With no quad naming it, p stays pinned, and its object role returns
		// under the same ID.
		tr.apply(del("a", "p", "b"))
		tr.apply(add("e", "q", "p"))
		if tr.id("p") != p {
			t.Fatalf("objectFirst=%v: p's object role took ID %d, not %d", objectFirst, tr.id("p"), p)
		}
		if got := tr.quads(quad.Quad{Object: quad.IRI("p")}); len(got) != 1 || got[0] != quad.MakeIRI("e", "q", "p", "") {
			t.Fatalf("objectFirst=%v: p's in quads = %v", objectFirst, got)
		}
	}
}

// failingTx fails the first Set of a node key with tag, after letting the
// others through.
type failingTx struct {
	kvtx.Tx
	tag    byte
	failed bool
}

// errInjected is the failure failingTx injects.
var errInjected = errors.New("injected set failure")

// Set fails the first node key with the tag.
func (f *failingTx) Set(ctx context.Context, key, value []byte) error {
	if !f.failed && len(key) > nodeKeyLen && string(key[:len(nodePrefix)]) == string(nodePrefix) && key[nodeKeyLen] == f.tag {
		f.failed = true
		return errInjected
	}
	return f.Tx.Set(ctx, key, value)
}

// TestPairedEdgeKeysAtomic checks that a call failing partway leaves no half
// quad and no half node even when its transaction is committed anyway, as an
// owner that ignores the error would: the call undoes its writes, all but the
// next ID key, which keeps the abandoned ID from being handed out again. The
// call pins c and creates d before the failure, and a retry in the same
// transaction pins c again.
func TestPairedEdgeKeysAtomic(t *testing.T) {
	for _, tag := range []byte{tagMeta, tagIn} {
		for _, retry := range []bool{false, true} {
			// c is an unpinned object; the call makes it a predicate.
			tr := newTestTree(t)
			tr.apply(add("x", "q", "c"), add("a", "p", "b"))
			before := tr.keys()
			next := tr.nextID()

			// Fail one write, optionally retry in the same store, and commit.
			tx, err := tr.store.NewTransaction(tr.ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			ftx := &failingTx{Tx: tx, tag: tag}
			qs := NewQuadStore(ftx)
			err = qs.ApplyDeltas(tr.ctx, []graph.Delta{add("a", "c", "d")}, graph.IgnoreOpts{})
			if !errors.Is(err, errInjected) {
				tx.Discard()
				t.Fatalf("tag %d: apply = %v, want the injected failure", tag, err)
			}
			if retry {
				if err := qs.ApplyDeltas(tr.ctx, []graph.Delta{add("a", "c", "d")}, graph.IgnoreOpts{}); err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
			if err := tx.Commit(tr.ctx); err != nil {
				t.Fatal(err)
			}
			tr.checkConsistent()

			// Without the retry only the next ID key changed, and it grew.
			if !retry {
				after := tr.keys()
				delete(before, string(nextIDKey))
				delete(after, string(nextIDKey))
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("tag %d: failed call changed the tree:\nbefore %q\nafter  %q", tag, before, after)
				}
				if tr.nextID() <= next {
					t.Fatalf("tag %d: next ID %d did not pass the abandoned ID %d", tag, tr.nextID(), next)
				}
				continue
			}

			// With it, d took a fresh ID and c is pinned in storage.
			if d := tr.id("d"); d <= next {
				t.Fatalf("tag %d: d took ID %d, at or below the abandoned ID %d", tag, d, next)
			}
			tr.read(func(qs *QuadStore) {
				meta, err := qs.readMeta(tr.ctx, tr.id("c"))
				if err != nil || !meta.GetPinned() {
					t.Fatalf("tag %d: c's meta %v, err %v: predicate not pinned after the retry", tag, meta, err)
				}
			})
		}
	}
}

// nextID returns the committed next ID.
func (tr *testTree) nextID() uint64 {
	tr.t.Helper()
	id, ok := decodeID([]byte(tr.keys()[string(nextIDKey)]))
	if !ok {
		tr.t.Fatal("tree has no next ID")
	}
	return id
}

// checkConsistent checks that every committed out key has its in key and the
// reverse, every ID map entry names a node with meta, and every ID is below
// the next ID.
func (tr *testTree) checkConsistent() {
	tr.t.Helper()
	keys := tr.keys()
	next := tr.nextID()
	for k, v := range keys {
		if strings.HasPrefix(k, string(valuePrefix)) {
			id, ok := decodeID([]byte(v))
			if _, meta := keys[string(metaKey(id))]; !ok || !meta || id >= next {
				tr.t.Fatalf("ID map entry %q names %x, with no node below next ID %d", k, v, next)
			}
			continue
		}
		id, tag, ok := parseNodeKey([]byte(k))
		if !ok {
			continue
		}
		if id >= next {
			tr.t.Fatalf("key %x has ID %d, at or above next ID %d", k, id, next)
		}
		if tag == tagMeta {
			continue
		}
		q, ok := parseEdgeKey([]byte(k))
		if !ok {
			tr.t.Fatalf("bad edge key %x", k)
		}
		out, in := edgeKeys(q)
		if _, ok := keys[string(out)]; !ok {
			tr.t.Fatalf("in key %x has no out key", k)
		}
		if _, ok := keys[string(in)]; !ok {
			tr.t.Fatalf("out key %x has no in key", k)
		}
	}
}

// TestMissingNextIDRefused checks that a tree holding nodes but no next ID is
// refused rather than counted on from its highest live ID, which would give a
// deleted node's ID to a new one.
func TestMissingNextIDRefused(t *testing.T) {
	// x takes the highest ID and is deleted, so the highest live ID is below
	// an ID already used.
	tr := newTestTree(t)
	tr.apply(add("a", "p", "b"), add("a", "p", "x"))
	tr.apply(del("a", "p", "x"))

	// Drop the next ID key.
	tx, err := tr.store.NewTransaction(tr.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(tr.ctx, nextIDKey); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(tr.ctx); err != nil {
		t.Fatal(err)
	}
	before := tr.keys()

	// Allocating a node fails and writes nothing.
	if err := tr.tryApply(nil, add("a", "p", "y")); !errors.Is(err, ErrMissingNextID) {
		t.Fatalf("apply on a tree without a next ID = %v, want ErrMissingNextID", err)
	}
	if after := tr.keys(); !reflect.DeepEqual(before, after) {
		t.Fatalf("refused write changed the tree")
	}

	// An empty tree without the key still starts at ID 1.
	empty := newTestTree(t)
	empty.apply(add("a", "p", "b"))
	if id := empty.id("a"); id != 1 {
		t.Fatalf("first node of an empty tree has ID %d, want 1", id)
	}
}
