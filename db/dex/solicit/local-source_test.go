package dex_solicit

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLocalSourceServesBlockBelowVolumes proves a peer's request reads the
// controller's local source, such as a placed store whose block sits only on
// its backend, and answers with the block instead of a miss.
func TestLocalSourceServesBlockBelowVolumes(t *testing.T) {
	ctx := t.Context()
	store := block_mock.NewMockStore(hash.HashType_HashType_SHA256)
	want := []byte("block-on-the-backend")
	ref, _, err := store.PutBlock(ctx, want, &block.PutOpts{HashType: hash.HashType_HashType_SHA256})
	if err != nil {
		t.Fatal(err)
	}

	c := newTestDexSolicitController()
	c.SetLocalSource(func() block.StoreOps { return store })
	sess, _, cleanup := newTestPeerSessionPair(c, peer.ID("reader"))
	defer cleanup()

	resp := sess.buildResponse(ctx, &DexMessage{RequestId: 1, Ref: ref})
	if resp.GetError() != "" {
		t.Fatalf("error = %q", resp.GetError())
	}
	if !resp.GetFound() || string(resp.GetData()) != string(want) {
		t.Fatalf("found = %v data = %q, want %q", resp.GetFound(), resp.GetData(), want)
	}
}

// TestLocalSourceErrorIsNotAMiss proves a local source that fails, such as a
// backend that is away, answers the peer with an error, which the peer asks
// again after, and not with a miss, which the peer takes as final.
func TestLocalSourceErrorIsNotAMiss(t *testing.T) {
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetLocalSource(func() block.StoreOps { return failingStore{} })
	sess, _, cleanup := newTestPeerSessionPair(c, peer.ID("reader"))
	defer cleanup()

	resp := sess.buildResponse(ctx, &DexMessage{RequestId: 1, Ref: testDexBlockRef(t, "away")})
	if resp.GetFound() || resp.GetError() == "" {
		t.Fatalf("found = %v error = %q, want an error", resp.GetFound(), resp.GetError())
	}
}

// failingStore fails every read with block.ErrUnavailable.
type failingStore struct {
	block.StoreOps
}

func (failingStore) GetStoredBlock(context.Context, *block.BlockRef) (*block.StoredBlock, error) {
	return nil, errors.Join(block.ErrUnavailable, errors.New("backend away"))
}
