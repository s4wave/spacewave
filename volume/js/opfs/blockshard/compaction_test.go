package blockshard

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
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

func TestBuildCompactedManifestRetiresInputs(t *testing.T) {
	current := &Manifest{
		Generation: 4,
		Segments: []SegmentMeta{
			{Filename: "seg-000001.sst", Level: 0, MinKey: []byte("a"), MaxKey: []byte("b")},
			{Filename: "seg-000002.sst", Level: 0, MinKey: []byte("c"), MaxKey: []byte("d")},
			{Filename: "seg-000003.sst", Level: 1, MinKey: []byte("e"), MaxKey: []byte("f")},
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
		"seg-000001.sst": true,
		"seg-000002.sst": true,
	}
	output := SegmentMeta{
		Filename: "seg-000004.sst",
		Level:    1,
		MinKey:   []byte("a"),
		MaxKey:   []byte("d"),
	}

	next, err := buildCompactedManifest(current, inputs, output, 5, 2000, 250)
	if err != nil {
		t.Fatalf("buildCompactedManifest: %v", err)
	}

	if next.Generation != 5 {
		t.Fatalf("generation: got %d want 5", next.Generation)
	}
	if len(next.Segments) != 2 {
		t.Fatalf("segments: got %d want 2", len(next.Segments))
	}
	if next.Segments[0].Filename != "seg-000003.sst" {
		t.Fatalf("kept segment: got %q", next.Segments[0].Filename)
	}
	if next.Segments[1].Filename != "seg-000004.sst" {
		t.Fatalf("output segment: got %q", next.Segments[1].Filename)
	}
	if len(next.PendingDelete) != 3 {
		t.Fatalf("pending delete: got %d want 3", len(next.PendingDelete))
	}
	if next.PendingDelete[1].Filename != "seg-000001.sst" {
		t.Fatalf("retired[1] filename: got %q", next.PendingDelete[1].Filename)
	}
	if next.PendingDelete[1].RetireGeneration != 5 {
		t.Fatalf("retired[1] generation: got %d want 5", next.PendingDelete[1].RetireGeneration)
	}
	if next.PendingDelete[1].DeleteAfterUnixMilli != 2250 {
		t.Fatalf("retired[1] delete-after: got %d want 2250", next.PendingDelete[1].DeleteAfterUnixMilli)
	}
	if next.PendingDelete[2].Filename != "seg-000002.sst" {
		t.Fatalf("retired[2] filename: got %q", next.PendingDelete[2].Filename)
	}
	if len(current.PendingDelete) != 1 {
		t.Fatalf("current manifest mutated: pending=%d want 1", len(current.PendingDelete))
	}
	if len(current.Segments) != 3 {
		t.Fatalf("current manifest mutated: segments=%d want 3", len(current.Segments))
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
