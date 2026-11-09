package git_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// TestGitInitOpPayloadRefs checks a transaction applying an import names the
// imported repository as its payload, so a Space holds the repository blocks
// while the operation is above the checkpoint, and that a new repository
// names none.
func TestGitInitOpPayloadRefs(t *testing.T) {
	ctx := context.Background()
	lookupOp := func(_ context.Context, typeID string) (world.Operation, error) {
		if typeID == GitInitOpTypeID {
			return &GitInitOp{}, nil
		}
		return nil, nil
	}
	root, err := block.BuildBlockRef([]byte("imported repository root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		repo *bucket.ObjectRef
		want []*block.BlockRef
	}{
		{name: "import", repo: &bucket.ObjectRef{RootRef: root}, want: []*block.BlockRef{root}},
		{name: "new", repo: nil, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := world_block_tx.NewTxApplyWorldOp(NewGitInitOp("repo", tc.repo, true, nil, nil), "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := tx.PayloadRefs(ctx, lookupOp)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("payloads = %v, want %v", got, tc.want)
			}
			for i := range got {
				if !got[i].EqualsRef(tc.want[i]) {
					t.Fatalf("payload %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
