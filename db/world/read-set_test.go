package world

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/hash"
)

// readStubWorldState answers the reads ReadSet tracks with empty results.
type readStubWorldState struct {
	WorldState
}

// GetObject reports that no object exists.
func (readStubWorldState) GetObject(context.Context, string) (ObjectState, bool, error) {
	return nil, false, nil
}

// IterateObjects returns no iterator.
func (readStubWorldState) IterateObjects(context.Context, string, bool) ObjectIterator {
	return nil
}

// LookupGraphQuads returns no quads.
func (readStubWorldState) LookupGraphQuads(context.Context, GraphQuad, uint32) ([]GraphQuad, error) {
	return nil, nil
}

// BuildStorageCursor returns no cursor.
func (readStubWorldState) BuildStorageCursor(context.Context) (*bucket_lookup.Cursor, error) {
	return nil, nil
}

// AccessWorldState calls cb without a cursor.
func (readStubWorldState) AccessWorldState(_ context.Context, _ *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return cb(nil)
}

// countingBodyBatcher counts native object body batch reads.
type countingBodyBatcher struct {
	// WorldState is unused; the batch method supplies the bodies.
	WorldState
	// calls counts native batch reads.
	calls int
}

// GetObject reports that no object exists, so a missed batch falls back here.
func (*countingBodyBatcher) GetObject(context.Context, string) (ObjectState, bool, error) {
	return nil, false, nil
}

// GetObjectBodiesBatch records one native batch and returns a body per key.
func (b *countingBodyBatcher) GetObjectBodiesBatch(_ context.Context, keys []string) ([]*ObjectBody, error) {
	b.calls++
	out := make([]*ObjectBody, len(keys))
	for i, key := range keys {
		out[i] = &ObjectBody{ObjectKey: key, Body: []byte(key), Exists: true}
	}
	return out, nil
}

// TestReadSetTouched checks which ChangeSets touch the reads of a view.
func TestReadSetTouched(t *testing.T) {
	// Read key a and iterate prefix dir/.
	ctx := t.Context()
	reads := NewReadSet()
	ws := reads.Track(readStubWorldState{})
	obj, _, err := ws.GetObject(ctx, "a")
	ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	ws.IterateObjects(ctx, "dir/", false)

	// Look up (s, p, *), (*, q, o2), a literal object and (*, bare, o3).
	if _, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys("s", "p", "", ""), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys("", "q", "o2", ""), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.LookupGraphQuads(ctx, NewGraphQuad("", "q", `"literal"`, ""), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys("", "bare", "o3", ""), 0); err != nil {
		t.Fatal(err)
	}

	// Compare each ChangeSet against the reads.
	cases := []struct {
		name    string
		changes *ChangeSet
		touched bool
	}{
		{"empty", &ChangeSet{}, false},
		{"other key", &ChangeSet{Keys: []string{"b"}}, false},
		{"other subject", &ChangeSet{Quads: []GraphQuad{NewGraphQuad("<s2>", `"p"`, "<o>", "")}}, false},
		{"read key", &ChangeSet{Keys: []string{"a"}}, true},
		{"create under prefix", &ChangeSet{Keys: []string{"dir/x"}}, true},
		{"matching quad", &ChangeSet{Quads: []GraphQuad{NewGraphQuad("<s>", `"p"`, "<o>", "")}}, true},
		{"rename into read key", &ChangeSet{Keys: []string{"c", "a"}}, true},
		{"pattern subject", &ChangeSet{Keys: []string{"s"}}, true},
		{"pattern object", &ChangeSet{Keys: []string{"o2"}}, true},
		{"literal object", &ChangeSet{Keys: []string{"literal"}}, false},
		{"bare predicate", &ChangeSet{Quads: []GraphQuad{NewGraphQuad("<s3>", `"bare"`, "<o3>", "")}}, true},
		{"unknown", NewUnknownChangeSet(), true},
	}
	for _, c := range cases {
		if got := reads.Touched(c.changes); got != c.touched {
			t.Errorf("%s: touched = %v, want %v", c.name, got, c.touched)
		}
	}
}

// TestReadSetStorageReads checks that only a read of the root World state
// touches every change. Blocks named by a ref never change.
func TestReadSetStorageReads(t *testing.T) {
	// Prepare an unrelated change and an empty cursor callback.
	ctx := t.Context()
	changes := &ChangeSet{Keys: []string{"other"}}
	noop := func(*bucket_lookup.Cursor) error { return nil }

	// Build a cursor and follow a ref.
	reads := NewReadSet()
	ws := reads.Track(readStubWorldState{})
	if _, err := ws.BuildStorageCursor(ctx); err != nil {
		t.Fatal(err)
	}
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("block"))
	if err != nil {
		t.Fatal(err)
	}
	ref := &bucket.ObjectRef{RootRef: &block.BlockRef{Hash: h}}
	if err := ws.AccessWorldState(ctx, ref, noop); err != nil {
		t.Fatal(err)
	}
	if reads.Touched(changes) {
		t.Error("a read through a ref touched an unrelated change")
	}

	// Read the root World state.
	if err := ws.AccessWorldState(ctx, nil, noop); err != nil {
		t.Fatal(err)
	}
	if !reads.Touched(changes) {
		t.Error("a read of the root World state missed a change")
	}
}

// TestMatchGraphQuad matches a lookup filter against changed quads.
func TestMatchGraphQuad(t *testing.T) {
	filter := NewGraphQuadWithKeys("", "pred", "o", "")
	if !MatchGraphQuad(filter, NewGraphQuad("<s>", `"pred"`, "<o>", "")) {
		t.Error("bare predicate filter missed the stored quad")
	}
	if MatchGraphQuad(filter, NewGraphQuad("<s>", `"pred"`, "<o2>", "")) {
		t.Error("filter matched another object")
	}
}

// TestReadSetKeepsObjectBodyBatch checks that a tracked read uses one native
// batch and records those keys.
func TestReadSetKeepsObjectBodyBatch(t *testing.T) {
	// Track a counting batcher.
	ctx := t.Context()
	batcher := &countingBodyBatcher{}
	reads := NewReadSet()
	tracked := reads.Track(batcher)

	// Read several keys through the batch helper.
	keys := []string{"read/a", "read/b", "read/c"}
	bodies, err := GetObjectBodiesBatch(ctx, tracked, keys)
	if err != nil {
		t.Fatal(err)
	}
	if batcher.calls != 1 || len(bodies) != len(keys) {
		t.Fatalf("calls = %d, bodies = %d, want 1 call and %d bodies", batcher.calls, len(bodies), len(keys))
	}
	for i, key := range keys {
		body := bodies[i]
		if body.ObjectKey != key || !body.Exists || string(body.Body) != key {
			t.Fatalf("body %d = %+v, want key %s from the batcher", i, body, key)
		}
	}

	// A change to a read key touches the set; an unread key does not.
	if !reads.Touched(&ChangeSet{Keys: []string{"read/b"}}) {
		t.Error("a change to a read key did not touch the ReadSet")
	}
	if reads.Touched(&ChangeSet{Keys: []string{"unread"}}) {
		t.Error("a change to an unread key touched the ReadSet")
	}
}

// TestReadSetTouchedMemo checks that a cached answer for a ChangeSet does not
// outlive a later recorded read.
func TestReadSetTouchedMemo(t *testing.T) {
	// Test the same ChangeSet twice against an empty set; the second answer
	// comes from the memo.
	set := NewReadSet()
	changes := &ChangeSet{Keys: []string{"a"}}
	for range 2 {
		if set.Touched(changes) {
			t.Fatal("empty set touched")
		}
	}

	// A read recorded afterward must be visible to the same ChangeSet.
	set.addKeys("a")
	if !set.Touched(changes) {
		t.Fatal("cached answer survived a recorded read")
	}
}

// TestReadSetJoin checks that a set holds the reads of the finished sets it
// joined, directly or through another joined set, and that joining itself or a
// set twice changes nothing.
func TestReadSetJoin(t *testing.T) {
	// Join a set that joined another.
	inner, middle, outer := NewReadSet(), NewReadSet(), NewReadSet()
	inner.addKeys("inner")
	middle.addKeys("middle")
	middle.join(inner)
	outer.join(middle)
	outer.join(middle)
	outer.join(outer)
	if len(outer.joined) != 1 {
		t.Fatalf("outer joined %d sets, want 1", len(outer.joined))
	}

	// A change to a read at any depth touches the outer set.
	for _, key := range []string{"inner", "middle"} {
		if !outer.Touched(&ChangeSet{Keys: []string{key}}) {
			t.Errorf("change to %q did not touch the outer set", key)
		}
	}
	if outer.Touched(&ChangeSet{Keys: []string{"other"}}) {
		t.Error("change to an unread key touched the outer set")
	}

	// The outer set's names cover every joined read, without copying them.
	names := &ChangeInterest{}
	outer.nameJoinedReads(names)
	if got := slices.Sorted(slices.Values(names.keys)); !slices.Equal(got, []string{"inner", "middle"}) {
		t.Fatalf("joined names %q, want [inner middle]", got)
	}
	if len(outer.keys) != 0 {
		t.Fatalf("outer set copied %d keys", len(outer.keys))
	}
}
