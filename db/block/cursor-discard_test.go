package block

import "testing"

// TestDiscardDetachedTreeKeepsSharedDescendants verifies that discarding a
// detached tree releases its private descendants and keeps a descendant that
// another parent still references.
func TestDiscardDetachedTreeKeepsSharedDescendants(t *testing.T) {
	tx, root := NewTransaction(NopStoreOps{}, nil, nil, nil)
	root.SetBlock(&transactionWorkerBlock{}, true)
	detached := root.FollowRef(1, nil)
	detached.SetBlock(&transactionWorkerBlock{}, true)
	private := detached.FollowRef(1, nil)
	private.SetBlock(&transactionWorkerBlock{}, true)
	shared := detached.FollowRef(2, nil)
	shared.SetBlock(&transactionWorkerBlock{}, true)
	root.SetRef(2, shared)
	root.ClearRef(1)

	detached.DiscardDetachedTree()
	for name, cursor := range map[string]*Cursor{"detached": detached, "private": private} {
		if tx.blockGraph.Node(cursor.pos.ID()) == cursor.pos {
			t.Fatalf("%s position survived the discard", name)
		}
	}
	if tx.blockGraph.Node(shared.pos.ID()) != shared.pos {
		t.Fatal("shared position was discarded")
	}
}
