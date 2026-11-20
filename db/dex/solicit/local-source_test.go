package dex_solicit

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
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

// TestLinkedAnswersSkipOnlyLinkedBlocks tries to make a copy skip a block that
// no linked file holds. A copy skips a block only on a linked answer, so it
// tries each way to get one: a block the responder holds in its Volume as well
// as in a linked file, a linked answer to a data request, and a reader that
// answers every request linked. Only the block the admitted Device holds only
// in a linked file may go unwritten.
func TestLinkedAnswersSkipOnlyLinkedBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// The owner's Device holds one leaf in its Volume and one only in a
	// linked file, under a root that names both.
	volumeLeaf := testDexBlockRef(t, "volume-leaf")
	linkedLeaf := testDexBlockRef(t, "linked-leaf")
	root := testDexBlockRef(t, "root")
	serve := linkedServeStore{blocks: map[string]*block.StoredBlock{
		root.MarshalString():       {Data: []byte("root"), Refs: []*block.BlockRef{volumeLeaf, linkedLeaf}, RefsKnown: true},
		volumeLeaf.MarshalString(): {Data: []byte("volume-leaf"), RefsKnown: true},
		linkedLeaf.MarshalString(): {Data: []byte("linked-leaf"), RefsKnown: true, Linked: true},
	}}
	ownerCtrl := newTestDexSolicitController()
	ownerCtrl.SetLocalSource(func() block.StoreOps { return serve })
	owner, _, ownerCleanup := newTestPeerSessionPair(ownerCtrl, peer.ID("requester"))
	defer ownerCleanup()

	// The responder answers linked only when asked, and only for the block
	// its linked file alone holds.
	for _, tc := range []struct {
		ref      *block.BlockRef
		linkedOk bool
		linked   bool
	}{
		{volumeLeaf, true, false},
		{linkedLeaf, false, false},
		{linkedLeaf, true, true},
	} {
		resp := owner.buildResponse(ctx, &DexMessage{RequestId: 1, Ref: tc.ref, LinkedOk: tc.linkedOk})
		if !resp.GetFound() || resp.GetLinked() != tc.linked || (len(resp.GetData()) == 0) != tc.linked {
			t.Fatalf("%s linked_ok=%v: found=%v linked=%v data=%q", tc.ref.MarshalString(), tc.linkedOk, resp.GetFound(), resp.GetLinked(), resp.GetData())
		}
	}

	// copy reads the graph through the DEX from the given peers, honouring
	// linked answers from the owner's Device only, as account replication
	// honours them from writers only, and returns what the destination holds.
	copyFrom := func(peers map[peer.ID]func(*DexMessage) *DexMessage) (block.StoreOps, error) {
		c := newTestDexSolicitController()
		for id, answer := range peers {
			sess, remote, cleanup := newTestPeerSessionPair(c, id)
			t.Cleanup(cleanup)
			sess.start(ctx)
			c.sessions[id.String()] = []*peerSession{sess}
			serveTestDexPeer(remote, answer)
		}
		dst := block_mock.NewMockStore(hash.HashType_HashType_SHA256)
		readCtx := WithLinkedAnswers(ctx, func(id peer.ID) bool { return id == "owner" })
		return dst, block.CopyGraph(readCtx, NewStore(c), dst, root, &block.GraphCopyOptions{CountLinked: true})
	}
	ownerAnswer := func(req *DexMessage) *DexMessage { return owner.buildResponse(ctx, req) }
	liar := func(req *DexMessage) *DexMessage {
		if req.GetLinkedOk() {
			t.Errorf("reader was asked for a linked answer to %s", req.GetRef().MarshalString())
		}
		return &DexMessage{RequestId: req.GetRequestId(), IsResponse: true, Found: true, Linked: true, RefsKnown: true}
	}
	holds := func(dst block.StoreOps, ref *block.BlockRef) bool {
		_, found, err := dst.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	// With the owner's Device and a lying reader connected, the copy writes
	// every block but the owner's linked leaf.
	dst, err := copyFrom(map[peer.ID]func(*DexMessage) *DexMessage{"owner": ownerAnswer, "reader": liar})
	if err != nil {
		t.Fatal(err)
	}
	if !holds(dst, root) || !holds(dst, volumeLeaf) || holds(dst, linkedLeaf) {
		t.Fatalf("root=%v volume leaf=%v linked leaf=%v, want only the linked leaf skipped", holds(dst, root), holds(dst, volumeLeaf), holds(dst, linkedLeaf))
	}

	// With only the reader, its linked answers fail its requests: the copy
	// stops at the root, unwritten, and skips nothing.
	dst, err = copyFrom(map[peer.ID]func(*DexMessage) *DexMessage{"reader": liar})
	if !errors.Is(err, ErrUnaskedLinked) || holds(dst, root) {
		t.Fatalf("copy from the reader alone: err=%v root written=%v, want ErrUnaskedLinked and nothing written", err, holds(dst, root))
	}
}

// linkedServeStore serves its blocks as a Device's serve reads do, marking
// those only a linked file holds.
type linkedServeStore struct {
	block.StoreOps
	// blocks holds the stored blocks by ref string.
	blocks map[string]*block.StoredBlock
}

// GetStoredBlock returns a copy of the stored block, or nil.
func (s linkedServeStore) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored := s.blocks[ref.MarshalString()]
	if stored == nil {
		return nil, nil
	}
	out := *stored
	return &out, nil
}

// serveTestDexPeer answers each request on remote with answer, skipping
// cancels, until remote closes.
func serveTestDexPeer(remote *stream_packet.Session, answer func(*DexMessage) *DexMessage) {
	go func() {
		for {
			var req DexMessage
			if err := remote.RecvMsg(&req); err != nil {
				return
			}
			if req.GetCancel() {
				continue
			}
			if err := remote.SendMsg(answer(&req)); err != nil {
				return
			}
		}
	}()
}
