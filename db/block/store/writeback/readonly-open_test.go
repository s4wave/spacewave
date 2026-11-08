package block_store_writeback

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// refuseWrites is a key-value store that fails every write transaction.
type refuseWrites struct {
	kvtx.Store
	refused int
}

func (s *refuseWrites) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		s.refused++
		return nil, errors.New("write refused")
	}
	return s.Store.NewTransaction(ctx, write)
}

// TestReadOnlyOpenWritesNothing opens a queue that holds a write in read-only
// mode over markers that refuse writes: the open succeeds, opens no write
// transaction, and reads the kept count.
func TestReadOnlyOpenWritesNothing(t *testing.T) {
	// Queue one write through a normal open.
	ctx := t.Context()
	env := newTestEnv(t)
	s, err := NewStore(ctx, ownedStore{env.vol}, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutBlock(ctx, []byte("served block"), leafOpts); err != nil {
		t.Fatal(err)
	}

	// Open it again read-only over markers that refuse writes.
	SetReadOnlyOpen(true)
	defer SetReadOnlyOpen(false)
	markers := &refuseWrites{Store: env.markers(t)}
	ro, err := NewStore(ctx, ownedStore{env.vol}, env.vol, markers, testMarkersID, true)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	if markers.refused != 0 {
		t.Fatalf("read-only open tried %d write transactions", markers.refused)
	}
	status, _ := ro.GetStatus()
	if status.Pending != 1 || status.Counting {
		t.Fatalf("read-only open pending=%d counting=%v, want 1 and false", status.Pending, status.Counting)
	}
}
