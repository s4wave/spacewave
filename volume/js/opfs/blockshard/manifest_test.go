package blockshard

import (
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	m := &Manifest{
		Generation: 42,
		Segments: []SegmentMeta{
			{
				Filename:   "seg-000001.sst",
				EntryCount: 100,
				Size:       4096,
				Level:      0,
				MinKey:     []byte("aaa"),
				MaxKey:     []byte("mmm"),
			},
			{
				Filename:   "seg-000002.sst",
				EntryCount: 200,
				Size:       8192,
				Level:      1,
				MinKey:     []byte("nnn"),
				MaxKey:     []byte("zzz"),
			},
		},
	}

	data := m.Encode()
	m2, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest: %v", err)
	}

	if m2.Generation != 42 {
		t.Errorf("generation: got %d, want 42", m2.Generation)
	}
	if len(m2.Segments) != 2 {
		t.Fatalf("segments: got %d, want 2", len(m2.Segments))
	}

	s := m2.Segments[0]
	if s.Filename != "seg-000001.sst" {
		t.Errorf("seg 0 filename: %q", s.Filename)
	}
	if s.EntryCount != 100 || s.Size != 4096 || s.Level != 0 {
		t.Errorf("seg 0 meta: count=%d size=%d level=%d", s.EntryCount, s.Size, s.Level)
	}
	if string(s.MinKey) != "aaa" || string(s.MaxKey) != "mmm" {
		t.Errorf("seg 0 keys: min=%q max=%q", s.MinKey, s.MaxKey)
	}

	s = m2.Segments[1]
	if s.Filename != "seg-000002.sst" || s.Level != 1 {
		t.Errorf("seg 1: filename=%q level=%d", s.Filename, s.Level)
	}
}

func TestManifestEmpty(t *testing.T) {
	m := &Manifest{Generation: 1}
	data := m.Encode()
	m2, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest: %v", err)
	}
	if m2.Generation != 1 || len(m2.Segments) != 0 {
		t.Errorf("got gen=%d segs=%d", m2.Generation, len(m2.Segments))
	}
}

func TestManifestCRCCorruption(t *testing.T) {
	m := &Manifest{
		Generation: 1,
		Segments:   []SegmentMeta{{Filename: "x.sst", EntryCount: 1, Size: 10}},
	}
	data := m.Encode()
	data[ManifestHeaderSize+2] ^= 0xFF // corrupt filename
	_, err := DecodeManifest(data)
	if err == nil {
		t.Fatal("expected CRC error")
	}
}

func TestPickManifest(t *testing.T) {
	a := &Manifest{Generation: 5, Segments: []SegmentMeta{{Filename: "a.sst"}}}
	b := &Manifest{Generation: 10, Segments: []SegmentMeta{{Filename: "b.sst"}}}

	result := PickManifest(a.Encode(), b.Encode())
	if result == nil {
		t.Fatal("PickManifest returned nil")
	}
	if result.Generation != 10 {
		t.Errorf("got gen %d, want 10", result.Generation)
	}

	// One corrupt: pick the valid one.
	corrupt := []byte("garbage")
	result = PickManifest(corrupt, b.Encode())
	if result == nil || result.Generation != 10 {
		t.Fatal("should pick b when a is corrupt")
	}

	result = PickManifest(a.Encode(), corrupt)
	if result == nil || result.Generation != 5 {
		t.Fatal("should pick a when b is corrupt")
	}

	// Both corrupt.
	result = PickManifest(corrupt, corrupt)
	if result != nil {
		t.Fatal("should return nil when both corrupt")
	}
}
