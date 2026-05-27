package blockshard

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

func buildTestReader(t *testing.T, entries []segment.Entry) *segment.Reader {
	t.Helper()
	w := segment.NewWriter()
	for _, e := range entries {
		if e.Tombstone {
			w.AddTombstone(e.Key)
		} else {
			w.Add(e.Key, e.Value)
		}
	}
	var buf bytes.Buffer
	written, err := w.Build(&buf)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data := buf.Bytes()
	rd, err := segment.NewReader(bytes.NewReader(data), written)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	return rd
}

func TestMergeSegments(t *testing.T) {
	// Older segment: keys a, b, c
	older := buildTestReader(t, []segment.Entry{
		{Key: []byte("a"), Value: []byte("old-a")},
		{Key: []byte("b"), Value: []byte("old-b")},
		{Key: []byte("c"), Value: []byte("old-c")},
	})

	// Newer segment: overwrites b, deletes c, adds d
	newer := buildTestReader(t, []segment.Entry{
		{Key: []byte("b"), Value: []byte("new-b")},
		{Key: []byte("c"), Tombstone: true},
		{Key: []byte("d"), Value: []byte("new-d")},
	})

	// Readers ordered oldest-first.
	merged, err := MergeSegments([]*segment.Reader{older, newer})
	if err != nil {
		t.Fatalf("MergeSegments: %v", err)
	}

	// Expected: a=old-a, b=new-b (overwrite), c=tombstone, d=new-d
	if len(merged) != 4 {
		t.Fatalf("merged count: got %d, want 4", len(merged))
	}

	check := func(idx int, key, val string, tomb bool) {
		t.Helper()
		e := merged[idx]
		if string(e.Key) != key {
			t.Errorf("entry %d key: got %q, want %q", idx, e.Key, key)
		}
		if e.Tombstone != tomb {
			t.Errorf("entry %d tombstone: got %v, want %v", idx, e.Tombstone, tomb)
		}
		if !tomb && string(e.Value) != val {
			t.Errorf("entry %d value: got %q, want %q", idx, e.Value, val)
		}
	}

	check(0, "a", "old-a", false)
	check(1, "b", "new-b", false)
	check(2, "c", "", true)
	check(3, "d", "new-d", false)
}

func TestMergeSegmentsDuplicateKeys(t *testing.T) {
	// Three segments, each with key "x" at different values.
	s1 := buildTestReader(t, []segment.Entry{{Key: []byte("x"), Value: []byte("v1")}})
	s2 := buildTestReader(t, []segment.Entry{{Key: []byte("x"), Value: []byte("v2")}})
	s3 := buildTestReader(t, []segment.Entry{{Key: []byte("x"), Value: []byte("v3")}})

	merged, err := MergeSegments([]*segment.Reader{s1, s2, s3})
	if err != nil {
		t.Fatalf("MergeSegments: %v", err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged count: got %d, want 1", len(merged))
	}
	// s3 is newest (highest index), should win.
	if string(merged[0].Value) != "v3" {
		t.Errorf("got %q, want v3", merged[0].Value)
	}
}

func TestBuildCompactionPlanIncludesOverlappingLowerLevel(t *testing.T) {
	m := &Manifest{
		Generation: 12,
		Segments: []SegmentMeta{
			{Filename: "seg-000001.sst", Level: 1, MinKey: []byte("a"), MaxKey: []byte("c")},
			{Filename: "seg-000002.sst", Level: 1, MinKey: []byte("x"), MaxKey: []byte("z")},
			{Filename: "seg-000003.sst", Level: 0, MinKey: []byte("b"), MaxKey: []byte("d")},
			{Filename: "seg-000004.sst", Level: 0, MinKey: []byte("d"), MaxKey: []byte("e")},
		},
	}

	plan := buildCompactionPlan(3, m, 2)
	if plan == nil {
		t.Fatal("expected compaction plan")
	}
	if plan.ShardID != 3 {
		t.Fatalf("shard id: got %d want 3", plan.ShardID)
	}
	if plan.Generation != 12 {
		t.Fatalf("generation: got %d want 12", plan.Generation)
	}
	if plan.OutputLevel != 1 {
		t.Fatalf("output level: got %d want 1", plan.OutputLevel)
	}
	got := segmentNames(plan.InputSegs)
	want := []string{"seg-000001.sst", "seg-000003.sst", "seg-000004.sst"}
	if !sameStrings(got, want) {
		t.Fatalf("inputs: got %v want %v", got, want)
	}
}

func TestBuildCompactionPlanUsesDeeperLevelThresholds(t *testing.T) {
	m := &Manifest{
		Generation: 20,
		Segments: []SegmentMeta{
			{Filename: "seg-000001.sst", Level: 2, MinKey: []byte("a"), MaxKey: []byte("z")},
			{Filename: "seg-000002.sst", Level: 1, MinKey: []byte("a"), MaxKey: []byte("b")},
			{Filename: "seg-000003.sst", Level: 1, MinKey: []byte("c"), MaxKey: []byte("d")},
			{Filename: "seg-000004.sst", Level: 1, MinKey: []byte("e"), MaxKey: []byte("f")},
			{Filename: "seg-000005.sst", Level: 1, MinKey: []byte("g"), MaxKey: []byte("h")},
		},
	}

	plan := buildCompactionPlan(0, m, 2)
	if plan == nil {
		t.Fatal("expected level-1 compaction plan")
	}
	if plan.OutputLevel != 2 {
		t.Fatalf("output level: got %d want 2", plan.OutputLevel)
	}
	got := segmentNames(plan.InputSegs)
	want := []string{
		"seg-000001.sst",
		"seg-000002.sst",
		"seg-000003.sst",
		"seg-000004.sst",
		"seg-000005.sst",
	}
	if !sameStrings(got, want) {
		t.Fatalf("inputs: got %v want %v", got, want)
	}
}

func TestBuildCompactionPlanWithLimitBoundsInputBatch(t *testing.T) {
	m := &Manifest{Generation: 1}
	for i := range 5 {
		m.Segments = append(m.Segments, SegmentMeta{
			Filename: "seg-" + string(rune('a'+i)) + ".sst",
			Level:    0,
			Size:     120,
			MinKey:   []byte{byte('a' + i)},
			MaxKey:   []byte{byte('a' + i)},
		})
	}

	plan := buildCompactionPlanWithLimit(0, m, 3, 100)
	if plan == nil {
		t.Fatal("expected bounded compaction plan")
	}
	if len(plan.InputSegs) != 3 {
		t.Fatalf("input segments: got %d want 3", len(plan.InputSegs))
	}
}

func TestBuildCompactionPlanWithLimitSkipsFutileMaxLevel(t *testing.T) {
	m := &Manifest{Generation: 1}
	for i := range 8 {
		m.Segments = append(m.Segments, SegmentMeta{
			Filename: "seg-" + string(rune('a'+i)) + ".sst",
			Level:    maxCompactionLevel,
			Size:     100,
			MinKey:   []byte{byte('a' + i)},
			MaxKey:   []byte{byte('a' + i)},
		})
	}

	if plan := buildCompactionPlanWithLimit(0, m, 2, 100); plan != nil {
		t.Fatalf("expected futile max-level compaction to be skipped: %+v", plan)
	}
}

func TestBuildCompactedManifestRetiresInputs(t *testing.T) {
	current := &Manifest{
		Generation: 4,
		Segments: []SegmentMeta{
			{Filename: "seg-000001.sst", Level: 1, MinKey: []byte("e"), MaxKey: []byte("f")},
			{Filename: "seg-000002.sst", Level: 0, MinKey: []byte("a"), MaxKey: []byte("b")},
			{Filename: "seg-000003.sst", Level: 0, MinKey: []byte("c"), MaxKey: []byte("d")},
		},
		PendingDelete: []RetiredSegmentMeta{
			{
				SegmentMeta:          SegmentMeta{Filename: "seg-000000.sst", Level: 0},
				RetireGeneration:     3,
				DeleteAfterUnixMilli: 1000,
			},
		},
	}
	inputs := map[string]bool{
		"seg-000002.sst": true,
		"seg-000003.sst": true,
	}
	output := SegmentMeta{
		Filename: "seg-000004.sst",
		Level:    1,
		MinKey:   []byte("a"),
		MaxKey:   []byte("d"),
	}

	next, err := buildCompactedManifest(current, inputs, []SegmentMeta{output}, 5, 2000, 250)
	if err != nil {
		t.Fatalf("buildCompactedManifest: %v", err)
	}

	if next.Generation != 5 {
		t.Fatalf("generation: got %d want 5", next.Generation)
	}
	if len(next.Segments) != 2 {
		t.Fatalf("segments: got %d want 2", len(next.Segments))
	}
	if next.Segments[0].Filename != "seg-000001.sst" {
		t.Fatalf("kept segment: got %q", next.Segments[0].Filename)
	}
	if next.Segments[1].Filename != "seg-000004.sst" {
		t.Fatalf("output segment: got %q", next.Segments[1].Filename)
	}
	if len(next.PendingDelete) != 3 {
		t.Fatalf("pending delete: got %d want 3", len(next.PendingDelete))
	}
	if next.PendingDelete[1].Filename != "seg-000002.sst" {
		t.Fatalf("retired[1] filename: got %q", next.PendingDelete[1].Filename)
	}
	if next.PendingDelete[1].RetireGeneration != 5 {
		t.Fatalf("retired[1] generation: got %d want 5", next.PendingDelete[1].RetireGeneration)
	}
	if next.PendingDelete[1].DeleteAfterUnixMilli != 2250 {
		t.Fatalf("retired[1] delete-after: got %d want 2250", next.PendingDelete[1].DeleteAfterUnixMilli)
	}
	if next.PendingDelete[2].Filename != "seg-000003.sst" {
		t.Fatalf("retired[2] filename: got %q", next.PendingDelete[2].Filename)
	}
	if len(current.PendingDelete) != 1 {
		t.Fatalf("current manifest mutated: pending=%d want 1", len(current.PendingDelete))
	}
	if len(current.Segments) != 3 {
		t.Fatalf("current manifest mutated: segments=%d want 3", len(current.Segments))
	}
}

func TestBuildCompactedManifestInsertsOutputAtNewestInput(t *testing.T) {
	current := &Manifest{
		Generation: 7,
		Segments: []SegmentMeta{
			{Filename: "older.sst", Level: 1},
			{Filename: "input-a.sst", Level: 1},
			{Filename: "input-b.sst", Level: 1},
			{Filename: "newer.sst", Level: 0},
		},
	}
	output := SegmentMeta{Filename: "output.sst", Level: 2}
	next, err := buildCompactedManifest(
		current,
		map[string]bool{"input-a.sst": true, "input-b.sst": true},
		[]SegmentMeta{output},
		8,
		1000,
		250,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := segmentNames(next.Segments)
	want := []string{"older.sst", "output.sst", "newer.sst"}
	if !sameStrings(got, want) {
		t.Fatalf("segments: got %v want %v", got, want)
	}
}

func TestPruneCompactedTombstones(t *testing.T) {
	merged := []segment.Entry{
		{Key: []byte("a"), Tombstone: true},
		{Key: []byte("b"), Tombstone: true},
		{Key: []byte("c"), Value: []byte("live")},
	}
	current := &Manifest{
		Segments: []SegmentMeta{
			{Filename: "input.sst", MinKey: []byte("a"), MaxKey: []byte("c")},
			{Filename: "older-overlap.sst", MinKey: []byte("b"), MaxKey: []byte("b")},
		},
	}
	pruned := pruneCompactedTombstones(merged, current, map[string]bool{"input.sst": true})
	if len(pruned) != 2 {
		t.Fatalf("pruned count: got %d want 2", len(pruned))
	}
	if string(pruned[0].Key) != "b" || !pruned[0].Tombstone {
		t.Fatalf("first retained entry: %+v", pruned[0])
	}
	if string(pruned[1].Key) != "c" || pruned[1].Tombstone {
		t.Fatalf("second retained entry: %+v", pruned[1])
	}
}

func TestBuildCompactedManifestCanRetireWithoutOutput(t *testing.T) {
	current := &Manifest{
		Generation: 3,
		Segments: []SegmentMeta{
			{Filename: "input.sst", Level: 0},
			{Filename: "newer.sst", Level: 0},
		},
	}
	next, err := buildCompactedManifest(
		current,
		map[string]bool{"input.sst": true},
		nil,
		4,
		1000,
		250,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := segmentNames(next.Segments)
	want := []string{"newer.sst"}
	if !sameStrings(got, want) {
		t.Fatalf("segments: got %v want %v", got, want)
	}
	if len(next.PendingDelete) != 1 || next.PendingDelete[0].Filename != "input.sst" {
		t.Fatalf("pending delete: %+v", next.PendingDelete)
	}
}

func TestBuildCompactedManifestInsertsMultipleOutputsAtNewestInput(t *testing.T) {
	current := &Manifest{
		Generation: 9,
		Segments: []SegmentMeta{
			{Filename: "older.sst", Level: 1},
			{Filename: "input-a.sst", Level: 1},
			{Filename: "input-b.sst", Level: 1},
			{Filename: "newer.sst", Level: 0},
		},
	}
	outputs := []SegmentMeta{
		{Filename: "output-a.sst", Level: 2},
		{Filename: "output-b.sst", Level: 2},
	}
	next, err := buildCompactedManifest(
		current,
		map[string]bool{"input-a.sst": true, "input-b.sst": true},
		outputs,
		10,
		1000,
		250,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := segmentNames(next.Segments)
	want := []string{"older.sst", "output-a.sst", "output-b.sst", "newer.sst"}
	if !sameStrings(got, want) {
		t.Fatalf("segments: got %v want %v", got, want)
	}
}

func TestSelectReclaimablePendingRequiresGenerationAndTime(t *testing.T) {
	current := &Manifest{
		Generation: 10,
		PendingDelete: []RetiredSegmentMeta{
			{
				SegmentMeta:          SegmentMeta{Filename: "seg-safe.sst"},
				RetireGeneration:     8,
				DeleteAfterUnixMilli: 500,
			},
			{
				SegmentMeta:          SegmentMeta{Filename: "seg-too-new.sst"},
				RetireGeneration:     9,
				DeleteAfterUnixMilli: 500,
			},
			{
				SegmentMeta:          SegmentMeta{Filename: "seg-too-early.sst"},
				RetireGeneration:     8,
				DeleteAfterUnixMilli: 1500,
			},
		},
	}

	keep, reclaim := selectReclaimablePending(current, 1000)
	if len(reclaim) != 1 {
		t.Fatalf("reclaim count: got %d want 1", len(reclaim))
	}
	if reclaim[0].Filename != "seg-safe.sst" {
		t.Fatalf("reclaim filename: got %q", reclaim[0].Filename)
	}
	if len(keep) != 2 {
		t.Fatalf("keep count: got %d want 2", len(keep))
	}
}

func segmentNames(segs []SegmentMeta) []string {
	out := make([]string, len(segs))
	for i := range segs {
		out[i] = segs[i].Filename
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildReclaimManifestAdvancesGeneration(t *testing.T) {
	current := &Manifest{
		Generation: 10,
		Segments:   []SegmentMeta{{Filename: "live.sst"}},
		PendingDelete: []RetiredSegmentMeta{
			{
				SegmentMeta:          SegmentMeta{Filename: "old.sst"},
				RetireGeneration:     8,
				DeleteAfterUnixMilli: 500,
			},
		},
	}
	next := buildReclaimManifest(current, nil)
	if next.Generation != 11 {
		t.Fatalf("generation: got %d want 11", next.Generation)
	}
	if len(next.Segments) != 1 || next.Segments[0].Filename != "live.sst" {
		t.Fatalf("segments changed unexpectedly: %+v", next.Segments)
	}
	if len(next.PendingDelete) != 0 {
		t.Fatalf("pending delete: got %d want 0", len(next.PendingDelete))
	}
	if len(current.PendingDelete) != 1 {
		t.Fatalf("current manifest mutated: pending=%d want 1", len(current.PendingDelete))
	}
}
