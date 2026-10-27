package world

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
)

// TestChangeIndexCandidates checks which indexed ReadSets a ChangeSet names.
func TestChangeIndexCandidates(t *testing.T) {
	// Index a key read, a pattern read, a prefix read and a plain filter.
	keyed, linked, prefixed := NewReadSet(), NewReadSet(), NewReadSet()
	keyed.addKeys("obj/a")
	linked.addPatterns(NewGraphQuad("<obj/b>", "", "", ""))
	prefixed.addPrefix("obj/")
	x := NewChangeIndex[string]()
	for name, set := range map[string]*ReadSet{"keyed": keyed, "linked": linked, "prefixed": prefixed} {
		x.Add(name, readSetInterest(set))
	}
	x.Add("filter", NewChangeInterest(func(*ChangeSet) bool { return false }))

	// Each change names its readers and the unindexed watchers.
	cases := []struct {
		changes *ChangeSet
		want    []string
	}{
		{&ChangeSet{Keys: []string{"obj/a"}}, []string{"filter", "keyed", "prefixed"}},
		{&ChangeSet{Keys: []string{"obj/b"}}, []string{"filter", "linked", "prefixed"}},
		{&ChangeSet{Quads: []GraphQuad{NewGraphQuad("<obj/b>", "<p>", "<obj/c>", "")}}, []string{"filter", "linked", "prefixed"}},
		{&ChangeSet{Keys: []string{"other"}}, []string{"filter", "prefixed"}},
		{NewUnknownChangeSet(), []string{"filter", "keyed", "linked", "prefixed"}},
	}
	for i, c := range cases {
		if got := slices.Sorted(slices.Values(x.Candidates(c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// Removing every watcher leaves no name behind.
	for _, name := range []string{"keyed", "linked", "prefixed", "filter"} {
		x.Remove(name)
	}
	expectEmptyIndex(t, x)
}

// TestChangeIndexSharedOnce checks that watchers sharing a read set name it
// once, that a change to it reaches each of them, and that the set's names
// leave the index with its last watcher.
func TestChangeIndexSharedOnce(t *testing.T) {
	// Share a set that joined another, among watchers with their own reads.
	inner, shared := NewReadSet(), NewReadSet()
	inner.addKeys("obj/inner")
	shared.addKeys("obj/shared")
	shared.join(inner)
	x := NewChangeIndex[string]()
	for _, name := range []string{"a", "b", "c"} {
		own := NewReadSet()
		own.addKeys("own/" + name)
		interest := readSetInterest(own)
		interest.shared = []*ReadSet{shared}
		x.Add(name, interest)
	}

	// The set and the one it joined are indexed once, for all three watchers.
	if len(x.held) != 2 || len(x.sets.keys) != 2 {
		t.Fatalf("index holds %d sets by %d keys, want 2 by 2", len(x.held), len(x.sets.keys))
	}
	cases := []struct {
		changes *ChangeSet
		want    []string
	}{
		{&ChangeSet{Keys: []string{"obj/shared"}}, []string{"a", "b", "c"}},
		{&ChangeSet{Keys: []string{"obj/inner"}}, []string{"a", "b", "c"}},
		{&ChangeSet{Keys: []string{"own/b"}}, []string{"b"}},
		{&ChangeSet{Keys: []string{"other"}}, nil},
	}
	for i, c := range cases {
		if got := slices.Sorted(slices.Values(x.Candidates(c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// The set stays indexed while a watcher holds it.
	x.Remove("a")
	x.Remove("b")
	if got := x.Candidates(&ChangeSet{Keys: []string{"obj/inner"}}); !slices.Equal(got, []string{"c"}) {
		t.Fatalf("candidates after two left: %q, want [c]", got)
	}
	x.Remove("c")
	expectEmptyIndex(t, x)
}

// TestChangeIndexJoinedOnce checks that watchers holding their own sets built
// from one large set index that set's reads once, that a change to it reaches
// each of them through their sets, and that it stays indexed while either a
// watcher or a set that joined it holds it.
func TestChangeIndexJoinedOnce(t *testing.T) {
	// Each watcher holds its own set joining the same part.
	part := NewReadSet()
	part.addKeys("part/1", "part/2")
	x := NewChangeIndex[string]()
	for _, name := range []string{"a", "b"} {
		merged := NewReadSet()
		merged.addKeys("merged/" + name)
		merged.join(part)
		x.Add(name, &ChangeInterest{shared: []*ReadSet{merged}})
	}

	// A third holds the part directly.
	x.Add("c", &ChangeInterest{shared: []*ReadSet{part}})

	// The part's reads are indexed once, beside each merged set's own.
	if len(x.held) != 3 || len(x.sets.keys) != 4 {
		t.Fatalf("index holds %d sets by %d keys, want 3 by 4", len(x.held), len(x.sets.keys))
	}
	cases := []struct {
		changes *ChangeSet
		want    []string
	}{
		{&ChangeSet{Keys: []string{"part/2"}}, []string{"a", "b", "c"}},
		{&ChangeSet{Keys: []string{"merged/a"}}, []string{"a"}},
		{&ChangeSet{Keys: []string{"other"}}, nil},
	}
	for i, c := range cases {
		if got := slices.Sorted(slices.Values(x.Candidates(c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// The part stays while a merged set or a watcher holds it.
	x.Remove("c")
	x.Remove("a")
	if got := x.Candidates(&ChangeSet{Keys: []string{"part/1"}}); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("candidates after two left: %q, want [b]", got)
	}
	x.Remove("b")
	expectEmptyIndex(t, x)
}

// expectEmptyIndex checks that x holds no watcher, set or name.
func expectEmptyIndex[T comparable](t *testing.T, x *ChangeIndex[T]) {
	t.Helper()
	for _, n := range []int{x.Len(), len(x.held), len(x.watchers.keys), len(x.watchers.terms), len(x.watchers.all), len(x.sets.keys), len(x.sets.terms), len(x.sets.all)} {
		if n != 0 {
			t.Fatalf("index not empty after removal: %d watchers, %d held sets, names %d %d %d, set names %d %d %d", x.Len(), len(x.held), len(x.watchers.keys), len(x.watchers.terms), len(x.watchers.all), len(x.sets.keys), len(x.sets.terms), len(x.sets.all))
		}
	}
}

// TestChangeIndexSound checks on random reads and changes that every ReadSet
// a ChangeSet touches, directly or through a shared or joined set, is among
// the index's candidates.
func TestChangeIndexSound(t *testing.T) {
	// Index random ReadSets over a small pool of keys and quad values.
	rng := rand.New(rand.NewPCG(1, 2))
	pick := func() string { return "<k" + strconv.Itoa(rng.IntN(12)) + ">" }
	maybe := func() string {
		if rng.IntN(2) == 0 {
			return ""
		}
		return pick()
	}
	pool := make([]*ReadSet, 12)
	for i := range pool {
		pool[i] = randomReadSet(rng, pick, maybe)
		if i != 0 && rng.IntN(3) == 0 {
			pool[i].join(pool[rng.IntN(i)])
		}
	}

	// Each watcher reads its own set and shares a few pool sets.
	x := NewChangeIndex[int]()
	sets := make([]*ReadSet, 200)
	shared := make([][]*ReadSet, len(sets))
	for i := range sets {
		sets[i] = randomReadSet(rng, pick, maybe)
		for range rng.IntN(3) {
			shared[i] = append(shared[i], pool[rng.IntN(len(pool))])
		}
		if rng.IntN(2) == 0 {
			merged := randomReadSet(rng, pick, maybe)
			for range rng.IntN(4) {
				merged.join(pool[rng.IntN(len(pool))])
			}
			shared[i] = append(shared[i], merged)
		}
		interest := readSetInterest(sets[i])
		interest.shared = append(interest.shared, shared[i]...)
		x.Add(i, interest)
	}

	// Each random ChangeSet names every set it touches.
	for range 2000 {
		changes := &ChangeSet{}
		for range rng.IntN(3) {
			changes.Keys = append(changes.Keys, pick())
		}
		for range rng.IntN(3) {
			changes.Quads = append(changes.Quads, NewGraphQuad(pick(), pick(), pick(), maybe()))
		}
		candidates := x.Candidates(changes)
		for i, set := range sets {
			touched := set.Touched(changes) || slices.ContainsFunc(shared[i], func(s *ReadSet) bool { return s.Touched(changes) })
			if touched && !slices.Contains(candidates, i) {
				t.Fatalf("set %d touched by %+v is not a candidate", i, changes)
			}
		}
	}

	// Removing every watcher leaves no name behind.
	for i := range sets {
		x.Remove(i)
	}
	expectEmptyIndex(t, x)
}

// randomReadSet records a few random reads of every kind but conservative ones.
func randomReadSet(rng *rand.Rand, pick, maybe func() string) *ReadSet {
	// Read some keys and look up some quad patterns.
	set := NewReadSet()
	for range rng.IntN(3) {
		set.addKeys(pick())
	}
	for range rng.IntN(3) {
		set.addPatterns(NewGraphQuad(maybe(), maybe(), maybe(), maybe()))
	}

	// Iterate a prefix or read the whole graph now and then.
	switch rng.IntN(20) {
	case 0:
		set.addPrefix("<k1")
	case 1:
		set.markGraph()
	}
	return set
}

// readSetInterest names the reads of set over a filter that tests them.
func readSetInterest(set *ReadSet) *ChangeInterest {
	interest := &ChangeInterest{filter: set.Touched}
	interest.shared = slices.Clone(set.nameReads(interest))
	return interest
}

// BenchmarkChangeIndexSharedWake measures one view's wake: it leaves the
// index and subscribes again, as a view does after each rerun, while 1000
// views share a set of 20000 keys and each reads 20 keys of its own.
func BenchmarkChangeIndexSharedWake(b *testing.B) {
	// Share one large set among the views.
	shared := NewReadSet()
	for i := range 20000 {
		shared.addKeys("shared/" + strconv.Itoa(i))
	}
	own := make([]*ReadSet, 1000)
	x := NewChangeIndex[int]()
	for v := range own {
		own[v] = NewReadSet()
		for i := range 20 {
			own[v].addKeys("own/" + strconv.Itoa(v) + "/" + strconv.Itoa(i))
		}
		x.Add(v, own[v].waitInterest(nil, []*ReadSet{shared}))
	}

	// Wake one view after another.
	b.ReportAllocs()
	v := 0
	for b.Loop() {
		x.Remove(v)
		x.Add(v, own[v].waitInterest(nil, []*ReadSet{shared}))
		v = (v + 1) % len(own)
	}
}

// BenchmarkChangeIndexMergedWake measures one view's wake when each of 1000
// views holds its own set joining the same 100 parts of 200 keys each, as a
// view built from a cache of shared parts does.
func BenchmarkChangeIndexMergedWake(b *testing.B) {
	// Build the parts the views share.
	parts := make([]*ReadSet, 100)
	for p := range parts {
		parts[p] = NewReadSet()
		for i := range 200 {
			parts[p].addKeys("part/" + strconv.Itoa(p) + "/" + strconv.Itoa(i))
		}
	}

	// Give each view its own set joining every part.
	own := make([]*ReadSet, 1000)
	merged := make([]*ReadSet, len(own))
	x := NewChangeIndex[int]()
	for v := range own {
		own[v] = NewReadSet()
		own[v].addKeys("own/" + strconv.Itoa(v))
		merged[v] = NewReadSet()
		for _, part := range parts {
			merged[v].join(part)
		}
		x.Add(v, own[v].waitInterest(nil, []*ReadSet{merged[v]}))
	}

	// Wake one view after another.
	b.ReportAllocs()
	v := 0
	for b.Loop() {
		x.Remove(v)
		x.Add(v, own[v].waitInterest(nil, []*ReadSet{merged[v]}))
		v = (v + 1) % len(own)
	}
}
