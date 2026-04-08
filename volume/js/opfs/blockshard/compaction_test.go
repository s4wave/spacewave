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
