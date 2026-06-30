package unixfs_block

import (
	"context"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestMultiDirentSameTxLookup guards the same-transaction dirent lookup that the
// upload path (MknodWithContent -> WriteBlob -> LookupFSTreePath) relies on.
//
// Each insert sorts the directory via sort.Sort, which calls DirentSlice.Swap.
// On the bcs path Swap reorders the in-memory slice only as a side effect of
// SetAsSubBlock -> ApplySubBlock. Under GoScript that Swap is async, so sort.Sort
// must await it; a dropped promise leaves the slice unsorted and the
// same-transaction binary search misses a present entry, surfacing as
// unixfs ErrNotExist. Inserting names in descending order forces multiple swaps
// per insert, which is the case that regressed.
func TestMultiDirentSameTxLookup(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	ftree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	const n = 8
	created := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		name := fmt.Sprintf("file%02d", i)
		if _, err = ftree.Mknod(name, NodeType_NodeType_FILE, nil, 0, nil); err != nil {
			t.Fatalf("mknod %q: %s", name, err.Error())
		}
		created = append(created, name)

		for _, want := range created {
			node, dirent, lerr := ftree.LookupFollowDirent(want)
			if lerr != nil {
				t.Fatalf("after inserting %q, lookup %q: %s", name, want, lerr.Error())
			}
			if node == nil {
				t.Fatalf("after inserting %q, lookup %q returned nil node: entry not found in same transaction", name, want)
			}
			if got := dirent.GetName(); got != want {
				t.Fatalf("after inserting %q, lookup %q returned dirent %q: directory ordering corrupted", name, want, got)
			}
		}
	}
}
