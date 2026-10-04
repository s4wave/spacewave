package block_store_s3

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestPackStoreScanBlocks visits every block of every packfile, including the
// packfiles another store wrote, with their refs.
func TestPackStoreScanBlocks(t *testing.T) {
	// Open a writing store on the fake bucket.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	writer := newTestPackStore(t, client)

	// Write the child and then its root as two packfiles.
	child, _, err := writer.PutBlock(ctx, []byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	rootRef, err := block.BuildBlockRef([]byte("root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = writer.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: rootRef, Data: []byte("root"), Refs: []*block.BlockRef{child}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Scan the bucket from a fresh store.
	var data []string
	var rootRefs []*block.BlockRef
	err = newTestPackStore(t, client).ScanBlocks(ctx, func(ref *block.BlockRef, stored *block.StoredBlock) error {
		data = append(data, string(stored.Data))
		if ref.EqualsRef(rootRef) {
			rootRefs = stored.Refs
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify both blocks and the root's edge were visited.
	slices.Sort(data)
	if !slices.Equal(data, []string{"child", "root"}) {
		t.Fatalf("scanned %v; want child and root", data)
	}
	if len(rootRefs) != 1 || !rootRefs[0].EqualsRef(child) {
		t.Fatalf("root refs = %v; want the child", rootRefs)
	}
}
