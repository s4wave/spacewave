package block

import (
	"context"
	"errors"
	"testing"
)

// peerWaitStore misses every block like a peer exchange with no session: a
// read waits for a peer to connect unless its context forbids waiting.
type peerWaitStore struct {
	NopStoreOps
}

func (peerWaitStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	_, err := peerWaitStore{}.GetStoredBlock(ctx, ref)
	return nil, false, err
}

func (peerWaitStore) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	if wait := GetPeerWait(ctx); wait != nil {
		wait.Skip(ref)
		return nil, ErrUnavailable
	}
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

// TestBufferedStoreWithoutPeerWait checks that a store set WithoutPeerWait
// reports a block no peer can serve as unavailable instead of waiting.
func TestBufferedStoreWithoutPeerWait(t *testing.T) {
	store := NewBufferedStoreWithSettings(t.Context(), peerWaitStore{}, &BufferedStoreSettings{WithoutPeerWait: true})
	ref := &BlockRef{}
	if _, _, err := store.GetBlock(t.Context(), ref); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("GetBlock: got %v, want ErrUnavailable", err)
	}
	if _, err := store.GetStoredBlock(t.Context(), ref); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("GetStoredBlock: got %v, want ErrUnavailable", err)
	}
}
