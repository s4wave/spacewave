package unixfs_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

func TestDirentApplyBlockRefClonesRef(t *testing.T) {
	// Prepare a source block reference and an empty directory entry.
	sum, err := hash.Sum(block.DefaultHashType, []byte("child node"))
	if err != nil {
		t.Fatal(err.Error())
	}
	src := block.NewBlockRef(sum)
	dirent := &Dirent{}

	// Apply the block reference and verify the entry owns independent reference data.
	if err := dirent.ApplyBlockRef(2, src); err != nil {
		t.Fatal(err.Error())
	}
	if dirent.NodeRef == src {
		t.Fatal("dirent retained source block ref")
	}
	if dirent.NodeRef.GetHash() == src.GetHash() {
		t.Fatal("dirent retained source hash")
	}

	// Mutate the source reference and verify the entry remains unchanged and serializable.
	expected := dirent.NodeRef.Clone()
	src.Hash.Hash = src.Hash.Hash[:len(src.Hash.Hash)-1]
	if !dirent.NodeRef.EqualVT(expected) {
		t.Fatal("dirent node ref changed after source mutation")
	}
	if _, err := dirent.MarshalVT(); err != nil {
		t.Fatal(err.Error())
	}
}
