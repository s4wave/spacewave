package kvtx_block_iavl

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

var (
	benchNodeCodecBytes []byte
	benchNodeCodecNode  *Node
)

func BenchmarkNodeCodecMarshalVT(b *testing.B) {
	for _, tc := range []struct {
		name string
		node *Node
	}{
		{name: "leaf_value_ref", node: benchCodecLeafValueRefNode()},
		{name: "leaf_value_blob", node: benchCodecLeafValueBlobNode()},
		{name: "internal_child_refs", node: benchCodecInternalNode()},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.node.SizeVT()))

			for b.Loop() {
				data, err := tc.node.MarshalVT()
				if err != nil {
					b.Fatal(err)
				}
				benchNodeCodecBytes = data
			}
		})
	}
}

func BenchmarkNodeCodecUnmarshalVT(b *testing.B) {
	for _, tc := range []struct {
		name string
		node *Node
	}{
		{name: "leaf_value_ref", node: benchCodecLeafValueRefNode()},
		{name: "leaf_value_blob", node: benchCodecLeafValueBlobNode()},
		{name: "internal_child_refs", node: benchCodecInternalNode()},
	} {
		data, err := tc.node.MarshalVT()
		if err != nil {
			b.Fatal(err)
		}

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for b.Loop() {
				node := &Node{}
				if err := node.UnmarshalVT(data); err != nil {
					b.Fatal(err)
				}
				benchNodeCodecNode = node
			}
		})
	}
}

func benchCodecLeafValueRefNode() *Node {
	return &Node{
		Size:     1,
		Key:      []byte("account:0000000000000042"),
		ValueRef: benchCodecBlockRef(1),
	}
}

func benchCodecLeafValueBlobNode() *Node {
	data := []byte("inline-iavl-value-payload-00000042")

	return &Node{
		Size: 1,
		Key:  []byte("account:0000000000000042"),
		ValueBlob: &blob.Blob{
			TotalSize: uint64(len(data)),
			RawData:   data,
		},
	}
}

func benchCodecInternalNode() *Node {
	return &Node{
		Height:        8,
		Size:          256,
		Key:           []byte("account:0000000000000128"),
		LeftChildRef:  benchCodecBlockRef(2),
		RightChildRef: benchCodecBlockRef(3),
	}
}

func benchCodecBlockRef(seed byte) *block.BlockRef {
	data := make([]byte, hash.HashType_HashType_BLAKE3.GetHashLen())
	for i := range data {
		data[i] = seed + byte(i)
	}

	return &block.BlockRef{
		Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, data),
	}
}
