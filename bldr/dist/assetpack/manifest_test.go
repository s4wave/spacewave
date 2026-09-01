package assetpack

import (
	"bytes"
	"io"
	"testing"
)

func TestReaderAtCrossesParts(t *testing.T) {
	parts := []Part{{URL: "a", Size: 3}, {URL: "b", Size: 4}}
	r, err := NewReaderAt(parts, []io.ReaderAt{bytes.NewReader([]byte("abc")), bytes.NewReader([]byte("defg"))})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 5)
	if _, err := r.ReadAt(got, 1); err != nil {
		t.Fatal(err)
	}
	if string(got) != "bcdef" {
		t.Fatalf("ReadAt = %q, want bcdef", got)
	}
	if r.Size() != 7 {
		t.Fatalf("Size = %d, want 7", r.Size())
	}
}

func TestPartsRoundTrip(t *testing.T) {
	want := []Part{{URL: "../hash/assets.kvfile-000", Size: 42}}
	data, err := MarshalParts(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalParts(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("parts = %#v, want %#v", got, want)
	}
}
