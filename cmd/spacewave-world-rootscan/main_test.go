//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

func mkRef(t *testing.T, s string) *block.BlockRef {
	ref, err := block.BuildBlockRef([]byte(s), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func marshal(t *testing.T, m interface{ MarshalVT() ([]byte, error) }) []byte {
	b, err := m.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestClassify types each shape: a chunked blob gives its chunk refs, a zero
// chunk none; a sql Root its database refs; a leaf holding a ref gives it
// blind; a near-blob whose chunks do not tile its size is a leaf.
func TestClassify(t *testing.T) {
	a, b, c := mkRef(t, "a"), mkRef(t, "b"), mkRef(t, "c")
	chunked := &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: 30, ChunkIndex: &blob.ChunkIndex{Chunks: []*blob.Chunk{
		{DataRef: a, Size: 10, Start: 0}, {Size: 10, Start: 10}, {DataRef: b, Size: 10, Start: 20},
	}}}
	bad := chunked.CloneVT()
	bad.TotalSize = 31
	raw := &blob.Blob{TotalSize: 3, RawData: []byte("xyz")}
	root := appendLD(nil, 1, appendLD(appendLD(nil, 1, []byte("hl")), 2, marshal(t, c)))
	root2 := append(append([]byte{}, root...), 0x10, 0x01)
	leaf := append([]byte{0x0a, 0x03, 'k', 'e', 'y', 0x12, 0x26, 0x0a, 0x24}, marshal(t, a.GetHash())...)
	cases := []struct {
		name  string
		body  []byte
		shape string
		refs  int
	}{
		{"chunked", marshal(t, chunked), shapeBlobChunk, 2},
		{"bad-tiling", marshal(t, bad), shapeOther, 2},
		{"raw", marshal(t, raw), shapeBlobChunk, 0},
		{"sql", root, shapeSQLNode, 1},
		{"sql-extra-field", root2, shapeOther, 1},
		{"leaf", leaf, shapeOther, 1},
		{"text", []byte("plain leaf bytes"), shapeOther, 0},
		{"empty", nil, shapeOther, 0},
	}
	for _, tc := range cases {
		st := &stats{Errors: map[string]uint64{}}
		shape, refs := classify(tc.body, nil, st)
		if shape != tc.shape || len(refs) != tc.refs {
			t.Fatalf("%s: shape %s refs %d, want %s %d", tc.name, shape, len(refs), tc.shape, tc.refs)
		}
		if tc.name == "leaf" && !refs[0].EqualsRef(a) {
			t.Fatal("leaf: blind ref is not the embedded one")
		}
	}
}

// TestFixtures scans the round-1 pack copies under ROOTSCAN_FIXTURES with the
// head file ROOTSCAN_HEAD, and checks the output agrees with the counts: every
// body classified once, every refs.tsv line well formed, shape counts summing
// to the lines. Skipped when the variables are unset.
func TestFixtures(t *testing.T) {
	dir, head := os.Getenv("ROOTSCAN_FIXTURES"), os.Getenv("ROOTSCAN_HEAD")
	if dir == "" || head == "" {
		t.Skip("ROOTSCAN_FIXTURES and ROOTSCAN_HEAD unset")
	}
	xfrm, err := headTransform(head)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	st, err := run([]string{dir}, xfrm, out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Bodies == 0 || st.BlobRoots+st.SQLRoots+st.Leaves+st.Undecodable != st.Bodies {
		t.Fatalf("stats %+v", st)
	}
	f, err := os.Open(filepath.Join(out, "refs.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	by := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Split(sc.Text(), "\t")
		if len(fs) != 3 || len(fs[0]) != 72 || len(fs[2]) != 72 {
			t.Fatalf("bad line of %d fields", len(fs))
		}
		by[fs[1]]++
	}
	if by[shapeBlobChunk]+by[shapeSQLNode]+by[shapeOther] != st.BlobChunks+st.SQLNodes+st.OtherRefs-st.SelfRefs || by[shapeSQLNode] > st.SQLNodes || by[shapeBlobChunk] > st.BlobChunks {
		t.Fatalf("lines %v, stats %+v", by, st)
	}
	t.Logf("fixtures: %s", st.json())
}
