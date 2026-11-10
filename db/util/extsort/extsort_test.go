package extsort

import (
	"bytes"
	"fmt"
	"iter"
	"math/rand/v2"
	"os"
	"slices"
	"testing"

	"github.com/pkg/errors"
)

// kv is one test record.
type kv struct{ k, v string }

// testRecords returns n records with repeated keys, in a fixed random order.
func testRecords(n int) []kv {
	r := rand.New(rand.NewPCG(1, 2))
	recs := make([]kv, n)
	for i := range recs {
		recs[i] = kv{fmt.Sprintf("k%04d", r.IntN(n/4)), fmt.Sprintf("v%d", i)}
	}
	return recs
}

// sortAll sorts recs through a Sorter with budget and the sorted sources, and
// returns the stream and the bytes spilled.
func sortAll(t *testing.T, budget int, recs []kv, sorted ...iter.Seq2[[]byte, []byte]) ([]kv, int64) {
	t.Helper()
	s := New(t.TempDir(), budget)
	defer s.Close()
	for _, r := range recs {
		if err := s.Add([]byte(r.k), []byte(r.v)); err != nil {
			t.Fatal(err)
		}
	}
	var out []kv
	err := s.Merge(t.Context(), sorted, func(k, v []byte) error {
		out = append(out, kv{string(k), string(v)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, s.Spilled()
}

// seq yields recs as a sorted source.
func seq(recs []kv) iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for _, r := range recs {
			if !yield([]byte(r.k), []byte(r.v)) {
				return
			}
		}
	}
}

// TestSorterMatchesStableSort checks that the merged stream is the stable sort
// of the input at every budget, from all in memory to one record per run, so
// run boundaries never change the output.
func TestSorterMatchesStableSort(t *testing.T) {
	recs := testRecords(4000)
	want := slices.Clone(recs)
	slices.SortStableFunc(want, func(a, b kv) int { return bytes.Compare([]byte(a.k), []byte(b.k)) })
	for _, budget := range []int{1 << 30, 64 << 10, 4 << 10, 1} {
		got, spilled := sortAll(t, budget, recs)
		if !slices.Equal(got, want) {
			t.Fatalf("budget %d: stream differs from the stable sort", budget)
		}
		if budget == 1<<30 && spilled != 0 {
			t.Fatalf("budget %d spilled %d bytes", budget, spilled)
		}
		if budget < 1<<30 && spilled == 0 {
			t.Fatalf("budget %d spilled nothing", budget)
		}
	}
}

// TestSorterMergesSortedSource checks that a sorted source merges unspilled,
// after added records with the same key, and that one out of order fails.
func TestSorterMergesSortedSource(t *testing.T) {
	added := []kv{{"b", "added"}, {"d", "added"}, {"a", "added"}}
	source := []kv{{"a", "src"}, {"c", "src"}, {"d", "src"}}
	got, _ := sortAll(t, 1, added, seq(source))
	want := []kv{{"a", "added"}, {"a", "src"}, {"b", "added"}, {"c", "src"}, {"d", "added"}, {"d", "src"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	s := New(t.TempDir(), 1)
	defer s.Close()
	err := s.Merge(t.Context(), []iter.Seq2[[]byte, []byte]{seq([]kv{{"b", ""}, {"a", ""}})}, func(k, v []byte) error { return nil })
	if !errors.Is(err, ErrUnsorted) {
		t.Fatalf("merge of an unsorted source returned %v", err)
	}
}

// TestSorterCloseRemovesRuns checks that Close removes every run, also after a
// merge that fn stopped.
func TestSorterCloseRemovesRuns(t *testing.T) {
	parent := t.TempDir()
	s := New(parent, 1)
	for _, r := range testRecords(64) {
		if err := s.Add([]byte(r.k), []byte(r.v)); err != nil {
			t.Fatal(err)
		}
	}
	stop := errors.New("stop")
	if err := s.Merge(t.Context(), nil, func(k, v []byte) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("merge returned %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("parent holds %d entries after Close: %v", len(entries), err)
	}
}
