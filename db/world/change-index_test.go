package world

import (
	"maps"
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
		if got := slices.Sorted(slices.Values(candidates(x, c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// Removing every watcher leaves no name behind.
	for _, name := range []string{"keyed", "linked", "prefixed", "filter"} {
		x.Remove(name)
	}
	expectEmptyIndex(t, x)
}

// TestChangeIndexAcceptsKeyed checks that a watcher a changed key names,
// directly or through a set it joined, is accepted without a test, and that
// one named only by a quad field or tested on every change is tested.
func TestChangeIndexAcceptsKeyed(t *testing.T) {
	// Read a key, a set joining a key read, and a pattern twice.
	keyed, part, joining, linked, sharedLinked := NewReadSet(), NewReadSet(), NewReadSet(), NewReadSet(), NewReadSet()
	keyed.addKeys("obj/a")
	part.addKeys("obj/b")
	joining.join(part)
	linked.addPatterns(NewGraphQuad("<obj/c>", "<p>", "", ""))
	sharedLinked.addPatterns(NewGraphQuad("<obj/c>", "<p>", "", ""))

	// Index them, the pattern directly and through a shared set, beside a
	// plain filter.
	x := NewChangeIndex[string]()
	x.Add("keyed", readSetInterest(keyed))
	x.Add("joining", &ChangeInterest{shared: []*ReadSet{joining}})
	x.Add("linked", readSetInterest(linked))
	x.Add("shared", &ChangeInterest{shared: []*ReadSet{sharedLinked}})
	x.Add("filter", NewChangeInterest(func(*ChangeSet) bool { return false }))

	// Changed keys accept their readers; a quad field or every change tests.
	cases := []struct {
		changes          *ChangeSet
		accepted, tested []string
	}{
		{&ChangeSet{Keys: []string{"obj/a", "obj/b"}}, []string{"joining", "keyed"}, []string{"filter"}},
		{&ChangeSet{Quads: []GraphQuad{NewGraphQuad("<obj/c>", "<q>", "<obj/d>", "")}}, nil, []string{"filter", "linked", "shared"}},
		{&ChangeSet{Keys: []string{"obj/c"}}, []string{"linked", "shared"}, []string{"filter"}},
	}
	for i, c := range cases {
		accepted, tested := x.Candidates(c.changes)
		slices.Sort(accepted)
		slices.Sort(tested)
		if !slices.Equal(accepted, c.accepted) || !slices.Equal(tested, c.tested) {
			t.Errorf("case %d: accepted %q tested %q, want %q and %q", i, accepted, tested, c.accepted, c.tested)
		}
	}
}

// TestReadSetsInterest checks that an interest in several finished sets is
// named by a change to any of them, accepted untested for a changed key, and
// accepts only changes that touch one.
func TestReadSetsInterest(t *testing.T) {
	// Index one watcher over a key read and a pattern read.
	keyed, linked := NewReadSet(), NewReadSet()
	keyed.addKeys("obj/a")
	linked.addPatterns(NewGraphQuad("<obj/b>", "<p>", "", ""))
	interest := NewReadSetsInterest(keyed, linked)
	x := NewChangeIndex[string]()
	x.Add("w", interest)

	// A changed key accepts it, a quad field tests it, and others skip it.
	cases := []struct {
		changes          *ChangeSet
		accepted, tested bool
		accepts          bool
	}{
		{&ChangeSet{Keys: []string{"obj/a"}}, true, false, true},
		{&ChangeSet{Quads: []GraphQuad{NewGraphQuad("<obj/b>", "<q>", "", "")}}, false, true, false},
		{&ChangeSet{Quads: []GraphQuad{NewGraphQuad("<obj/b>", "<p>", "<obj/c>", "")}}, false, true, true},
		{&ChangeSet{Keys: []string{"other"}}, false, false, false},
	}
	for i, c := range cases {
		accepted, tested := x.Candidates(c.changes)
		if (len(accepted) == 1) != c.accepted || (len(tested) == 1) != c.tested || interest.Accepts(c.changes) != c.accepts {
			t.Errorf("case %d: accepted %v tested %v accepts %v", i, accepted, tested, interest.Accepts(c.changes))
		}
	}
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
		if got := slices.Sorted(slices.Values(candidates(x, c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// The set stays indexed while a watcher holds it.
	x.Remove("a")
	x.Remove("b")
	if got := candidates(x, &ChangeSet{Keys: []string{"obj/inner"}}); !slices.Equal(got, []string{"c"}) {
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
		if got := slices.Sorted(slices.Values(candidates(x, c.changes))); !slices.Equal(got, c.want) {
			t.Errorf("case %d: candidates %q, want %q", i, got, c.want)
		}
	}

	// The part stays while a merged set or a watcher holds it.
	x.Remove("c")
	x.Remove("a")
	if got := candidates(x, &ChangeSet{Keys: []string{"part/1"}}); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("candidates after two left: %q, want [b]", got)
	}
	x.Remove("b")
	expectEmptyIndex(t, x)
}

// candidates returns every watcher x names for changes, accepted or tested.
func candidates[T comparable](x *ChangeIndex[T], changes *ChangeSet) []T {
	accepted, tested := x.Candidates(changes)
	return slices.Concat(accepted, tested)
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
	index := func(i int) {
		sets[i] = randomReadSet(rng, pick, maybe)
		shared[i] = nil
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
	for i := range sets {
		index(i)
	}

	// Each random ChangeSet names every set it touches, and accepts without a
	// test only sets it touches.
	check := func() {
		t.Helper()
		for range 2000 {
			changes := &ChangeSet{}
			for range rng.IntN(3) {
				changes.Keys = append(changes.Keys, pick())
			}
			for range rng.IntN(3) {
				changes.Quads = append(changes.Quads, NewGraphQuad(pick(), pick(), pick(), maybe()))
			}
			accepted, tested := x.Candidates(changes)
			for i, set := range sets {
				touched := set.Touched(changes) || slices.ContainsFunc(shared[i], func(s *ReadSet) bool { return s.Touched(changes) })
				isAccepted, isTested := slices.Contains(accepted, i), slices.Contains(tested, i)
				if touched && !isAccepted && !isTested {
					t.Fatalf("set %d touched by %+v is not a candidate", i, changes)
				}
				if isAccepted && (!touched || isTested) {
					t.Fatalf("set %d accepted for %+v: touched %v, also tested %v", i, changes, touched, isTested)
				}
			}
		}
	}
	check()

	// Replacing every watcher's interest, as a view's rerun does, moves its
	// entry by the diff and keeps the index sound.
	for i := range sets {
		index(i)
	}
	check()

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

// BenchmarkChangeIndexMergedDeliver measures handing one revision's changes
// to the views, as a change feed does, when each of 1000 views holds its own
// set joining the same 100 parts of 200 keys each and the revision changes 20
// keys of one part. test-all tests every candidate's filter; accept-keyed
// accepts the candidates a changed key names and tests the rest.
func BenchmarkChangeIndexMergedDeliver(b *testing.B) {
	// Build the parts the views share.
	parts := make([]*ReadSet, 100)
	for p := range parts {
		parts[p] = NewReadSet()
		for i := range 200 {
			parts[p].addKeys("part/" + strconv.Itoa(p) + "/" + strconv.Itoa(i))
		}
	}

	// Give each view its own set joining every part, waiting as a view does.
	x := NewChangeIndex[*ChangeInterest]()
	for v := range 1000 {
		own, merged := NewReadSet(), NewReadSet()
		own.addKeys("own/" + strconv.Itoa(v))
		for _, part := range parts {
			merged.join(part)
		}
		touches := func(cs *ChangeSet) bool { return own.Touched(cs) || merged.Touched(cs) }
		interest := own.waitInterest(touches, []*ReadSet{merged})
		x.Add(interest, interest)
	}

	// Each revision is a new ChangeSet, so no set's memo answers it.
	keys := make([]string, 20)
	for i := range keys {
		keys[i] = "part/50/" + strconv.Itoa(i*10)
	}
	deliver := func(b *testing.B, testAll bool) {
		b.ReportAllocs()
		for b.Loop() {
			changes := &ChangeSet{Keys: keys}
			accepted, tested := x.Candidates(changes)
			if testAll {
				tested = slices.Concat(accepted, tested)
				accepted = nil
			}
			n := len(accepted)
			for _, interest := range tested {
				if interest.Accepts(changes) {
					n++
				}
			}
			if n != 1000 {
				b.Fatalf("delivered to %d views, want 1000", n)
			}
		}
	}
	b.Run("test-all", func(b *testing.B) { deliver(b, true) })
	b.Run("accept-keyed", func(b *testing.B) { deliver(b, false) })
}

// TestNameEntriesPromoteOnRemove keeps every other entry of a name reachable
// when the entry held inline leaves, and drops the name with its last entry.
func TestNameEntriesPromoteOnRemove(t *testing.T) {
	index := make(map[string]nameEntries[int])
	for w := range 3 {
		addIndexed(index, "k", w)
	}
	addIndexed(index, "k", 1)
	found := func() map[int]struct{} {
		out := make(map[int]struct{})
		if entries, ok := index["k"]; ok {
			entries.collect(out)
		}
		return out
	}
	want := map[int]struct{}{0: {}, 1: {}, 2: {}}
	for _, w := range []int{0, 2, 1} {
		removeIndexed(index, "k", w)
		removeIndexed(index, "k", w)
		delete(want, w)
		if got := found(); !maps.Equal(got, want) {
			t.Fatalf("found %v after removing %d, want %v", got, w, want)
		}
	}
	if len(index) != 0 {
		t.Fatalf("index keeps %v after its last entry left", index)
	}
}
