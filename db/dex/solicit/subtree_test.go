package dex_solicit

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// newTestLinkedSessions connects a session of asker to a session of server
// over one pipe, starts both, and returns the asker's.
func newTestLinkedSessions(t *testing.T, ctx context.Context, asker, server *Controller) *peerSession {
	t.Helper()
	a, b := net.Pipe()
	ask := newPeerSession(asker, asker.le, &testDexMountedStream{stream: a, link: &testDexMountedLink{remote: "server"}, remote: peer.ID("server")}, nil)
	serve := newPeerSession(server, server.le, &testDexMountedStream{stream: b, link: &testDexMountedLink{remote: "asker"}, remote: peer.ID("asker")}, nil)
	t.Cleanup(func() {
		ask.close()
		serve.close()
	})
	ask.start(ctx)
	serve.start(ctx)
	return ask
}

// newSubtreeTestTree builds a tree of the given depth, each structure block
// with fanout children and every block about size bytes, in a store that keeps
// each block's refs, as a Volume's does. It returns the store, the root, each
// block's children, every block's ref and the total data bytes.
func newSubtreeTestTree(t *testing.T, depth, fanout, size int) (linkedServeStore, *block.BlockRef, map[string][]*block.BlockRef, []*block.BlockRef, int) {
	t.Helper()
	store := linkedServeStore{blocks: make(map[string]*block.StoredBlock)}
	children := make(map[string][]*block.BlockRef)
	var all []*block.BlockRef
	total := 0
	var build func(level, at int) *block.BlockRef
	build = func(level, at int) *block.BlockRef {
		var refs []*block.BlockRef
		if level < depth-1 {
			for i := range fanout {
				refs = append(refs, build(level+1, at*fanout+i))
			}
		}
		data := fmt.Appendf(bytes.Repeat([]byte{'.'}, size), "node %d.%d", level, at)
		ref, err := block.BuildBlockRef(data, &block.PutOpts{HashType: hash.HashType_HashType_SHA256})
		if err != nil {
			t.Fatal(err)
		}
		children[ref.MapKey()] = refs
		all = append(all, ref)
		total += len(data)
		store.blocks[ref.MarshalString()] = &block.StoredBlock{Data: data, Refs: refs, RefsSource: block.RefsAsserted}
		return ref
	}
	root := build(0, 0)
	return store, root, children, all, total
}

// syncSubtreeTestTree copies the tree under root from the peer behind sess
// into dst the way a walk through the bucket lookup does: breadth first, it
// asks for each block dst lacks, with a subtree of budget bytes unless budget
// is zero, and writes back what arrives. It returns the requests it sent.
func syncSubtreeTestTree(ctx context.Context, t *testing.T, sess *peerSession, dst block.StoreOps, root *block.BlockRef, children map[string][]*block.BlockRef, budget uint32) int {
	t.Helper()
	var sub *subtreeAsk
	if budget != 0 {
		sub = &subtreeAsk{budget: budget, have: dst.GetBlockExistsBatch}
	}
	requests := 0
	for level := []*block.BlockRef{root}; len(level) != 0; {
		held, err := dst.GetBlockExistsBatch(ctx, level)
		if err != nil {
			t.Fatal(err)
		}
		var next []*block.BlockRef
		for i, ref := range level {
			if !held[i] {
				requests++
				resp, blocks, err := sess.requestBlock(ctx, ref, 0, false, sub)
				if err != nil || resp == nil {
					t.Fatalf("request %s: found %v: %v", ref.MarshalString(), resp != nil, err)
				}
				blocks = append(blocks, &block.PutBatchEntry{Ref: ref, Data: resp.GetData(), Refs: resp.GetRefs(), RefsKnown: true})
				if _, err := dst.PutBlockBatch(ctx, blocks); err != nil {
					t.Fatal(err)
				}
			}
			next = append(next, children[ref.MapKey()]...)
		}
		level = next
	}
	return requests
}

// TestSubtreeSyncScales copies a tree of fanout 10 and about 1 KiB blocks,
// of 111 and then 1111 blocks, between two sessions with a 64 KiB subtree
// budget. The requests stay a few per budget's worth of data at either size,
// where asking one ref per request costs one per block: each budget's cut
// leaves the unsent siblings along one path, each asked for on its own.
func TestSubtreeSyncScales(t *testing.T) {
	ctx := t.Context()
	const budget = 64 << 10
	var perBudget [2]float64
	for i, depth := range []int{3, 4} {
		src, root, children, all, total := newSubtreeTestTree(t, depth, 10, 1<<10)
		server := newTestDexSolicitController()
		server.SetLocalSource(func() block.StoreOps { return src })
		sess := newTestLinkedSessions(t, ctx, newTestDexSolicitController(), server)

		perRef := syncSubtreeTestTree(ctx, t, sess, block_mock.NewMockStore(hash.HashType_HashType_SHA256), root, children, 0)
		dst := block_mock.NewMockStore(hash.HashType_HashType_SHA256)
		got := syncSubtreeTestTree(ctx, t, sess, dst, root, children, budget)
		held, err := dst.GetBlockExistsBatch(ctx, all)
		if err != nil {
			t.Fatal(err)
		}
		for j, ref := range all {
			if !held[j] {
				t.Fatalf("depth %d: block %s not copied", depth, ref.MarshalString())
			}
		}
		budgets := float64(total) / budget
		perBudget[i] = float64(got) / budgets
		t.Logf("depth %d: %d blocks, %.1f budgets of data: %d requests with subtrees, %d one ref each", depth, len(children), budgets, got, perRef)
		if perRef != len(children) {
			t.Fatalf("depth %d: one ref each took %d requests for %d blocks", depth, perRef, len(children))
		}
	}
	if perBudget[0] > 6 || perBudget[1] > 6 {
		t.Fatalf("requests per budget %.1f at n and %.1f at 10n, want at most 6", perBudget[0], perBudget[1])
	}
	if perBudget[1] > 1.5*perBudget[0] {
		t.Fatalf("requests per budget grew from %.1f at n to %.1f at 10n", perBudget[0], perBudget[1])
	}
}

// TestSubtreeKeepsOnlyNamedVerifiedBlocks tries to make an asker keep a block
// the stream did not earn: a peer answers a subtree request with a block no
// kept block names, then a named block, then a named block whose data fails
// its hash, then a block the corrupt one would have named. Only the named,
// verified block is kept, and the stream ends at the corrupt one.
func TestSubtreeKeepsOnlyNamedVerifiedBlocks(t *testing.T) {
	ctx := t.Context()
	unnamed, named, corrupt := testDexBlockRef(t, "unnamed"), testDexBlockRef(t, "named"), testDexBlockRef(t, "corrupt")
	root := testDexBlockRef(t, "root")
	sess, remote, cleanup := newTestPeerSessionPair(newTestDexSolicitController(), peer.ID("liar"))
	defer cleanup()
	sess.start(ctx)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			return
		}
		id := req.GetRequestId()
		for _, msg := range []*DexMessage{
			{Found: true, Data: []byte("root"), Refs: []*block.BlockRef{named, corrupt}, RefsKnown: true, More: true},
			{Ref: unnamed, Found: true, Data: []byte("unnamed"), RefsKnown: true, More: true},
			{Ref: named, Found: true, Data: []byte("named"), RefsKnown: true, More: true},
			{Ref: corrupt, Found: true, Data: []byte("not corrupt"), Refs: []*block.BlockRef{unnamed}, RefsKnown: true, More: true},
			{Ref: unnamed, Found: true, Data: []byte("unnamed"), RefsKnown: true, More: true},
			{},
		} {
			msg.RequestId, msg.IsResponse = id, true
			if err := remote.SendMsg(msg); err != nil {
				return
			}
		}
	}()

	resp, blocks, err := sess.requestBlock(ctx, root, 0, false, &subtreeAsk{budget: 1 << 16})
	if err != nil || resp == nil {
		t.Fatalf("found %v: %v", resp != nil, err)
	}
	if len(blocks) != 1 || !blocks[0].Ref.EqualsRef(named) {
		var got []string
		for _, b := range blocks {
			got = append(got, string(b.Data))
		}
		t.Fatalf("kept %q, want only the named block", got)
	}
}
