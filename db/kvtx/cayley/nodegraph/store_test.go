package kvtx_cayley_nodegraph

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/graphtest"
	"github.com/aperturerobotics/cayley/quad"
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
	return NewQuadStore(tx, nil), graph.Options{}, func() { tx.Discard() }
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

// TestChangedQuadsReusedID checks that the diff of two trees names the quads of
// an out key equal in both trees whose object ID was freed and then allocated
// to another value.
func TestChangedQuadsReusedID(t *testing.T) {
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	apply := func(action graph.Procedure, o string) {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		d := graph.Delta{Quad: quad.MakeIRI("a", "p", o, ""), Action: action}
		if err := NewQuadStore(tx, nil).ApplyDeltas(ctx, []graph.Delta{d}, graph.IgnoreOpts{}); err != nil {
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

	// x takes the last ID; deleting its quad frees it, and y takes it again.
	apply(graph.Add, "b")
	apply(graph.Add, "x")
	before := snapshot()
	apply(graph.Delete, "x")
	apply(graph.Add, "y")
	after := snapshot()

	// The tree diff lists no out key, only the reused meta and value keys.
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
	for _, k := range keys {
		if _, tag, ok := parseNodeKey(k); ok && tag == tagOut {
			t.Fatalf("diff lists out key %x; the test needs the key reused", k)
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
