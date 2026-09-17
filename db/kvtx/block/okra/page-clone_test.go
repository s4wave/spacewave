package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

func cloneTestEntries(n int) []*Entry {
	entries := make([]*Entry, n)
	for i := range entries {
		entries[i] = &Entry{
			Key: []byte(fmt.Sprintf("key-%04d", i)), Hash: bytes.Repeat([]byte{byte(i)}, 16), Size: 1,
			ChildRef:    &block.BlockRef{Hash: &hash.Hash{Hash: []byte("child")}},
			ValueRef:    &block.BlockRef{Hash: &hash.Hash{Hash: []byte("value")}},
			ValueIsBlob: true, ValueBlob: &blob.Blob{RawData: []byte("payload"), TotalSize: 7},
			unknownFields: []byte{0x78, 0x01},
		}
	}
	return entries
}

func TestPageLocalClonesMatchDeepClones(t *testing.T) {
	original := cloneTestEntries(32)
	original = append(original, nil, &Entry{Anchor: true, Hash: mustAnchorHash()}, &Entry{Key: []byte{}, Hash: nil})
	cloned := slices.Clone(original)
	clonePageEntries(cloned)
	for i, src := range original {
		want := src.CloneVT()
		got := cloned[i]
		if !want.EqualVT(got) {
			t.Fatalf("entry %d differs from CloneVT", i)
		}
		if src == nil {
			continue
		}
		wantBytes, err := want.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		gotBytes, err := got.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wantBytes, gotBytes) {
			t.Fatalf("entry %d changed wire bytes", i)
		}
		if cap(got.Key) != len(got.Key) || cap(got.Hash) != len(got.Hash) || cap(got.unknownFields) != len(got.unknownFields) {
			t.Fatal("an arena field can reslice into its neighbor")
		}
		if src == got || (src.ValueBlob != nil && src.ValueBlob == got.ValueBlob) || (src.ChildRef != nil && src.ChildRef == got.ChildRef) || (src.ValueRef != nil && src.ValueRef == got.ValueRef) {
			t.Fatal("clone shares mutable objects")
		}
	}
	// Updating references and inline subblocks is legal after page construction;
	// byte pooling must not change that ownership contract.
	for i, src := range original[:32] {
		before := cloned[i].CloneVT()
		clear(src.Key)
		clear(src.Hash)
		clear(src.unknownFields)
		clear(src.ValueBlob.RawData)
		clear(src.ChildRef.Hash.Hash)
		clear(src.ValueRef.Hash.Hash)
		if !before.EqualVT(cloned[i]) {
			t.Fatalf("entry %d aliases source", i)
		}
	}
	for i := range 31 {
		before := cloned[i+1].CloneVT()
		cloned[i].Key = append(cloned[i].Key, 'x')
		cloned[i].Hash = append(cloned[i].Hash, 'y')
		cloned[i].unknownFields = append(cloned[i].unknownFields, 'z')
		if !before.EqualVT(cloned[i+1]) {
			t.Fatalf("entry %d append overwrote sibling", i)
		}
	}
}

func BenchmarkPageEntryCloning(b *testing.B) {
	entries := cloneTestEntries(32)
	for _, packed := range []bool{false, true} {
		b.Run(fmt.Sprintf("page-local=%v", packed), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				output := slices.Clone(entries)
				if packed {
					clonePageEntries(output)
				} else {
					for i, e := range output {
						output[i] = e.CloneVT()
					}
				}
				if !output[0].EqualVT(entries[0]) {
					b.Fatal("clone differs")
				}
			}
		})
	}
}
