package provider_local

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// rateGraphLeaves and rateGraphFanout size the measured graph at about 20,600
// blocks: 20,000 leaves of 1 KiB under interior nodes of 32 children.
const (
	rateGraphLeaves = 20000
	rateGraphFanout = 32
)

// TestAccountReplicaCopyRate measures a replica's graph copy of a large graph
// from its paired peer over DEX against a copy that reads the peer's volume in
// process. It runs only when SPACEWAVE_COPY_RATE is set.
func TestAccountReplicaCopyRate(t *testing.T) {
	if os.Getenv("SPACEWAVE_COPY_RATE") == "" {
		t.Skip("set SPACEWAVE_COPY_RATE to measure the replica copy rate")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()

	network := inproc.NewNetwork()
	var accounts []*ProviderAccount
	var sessions []*Session
	for range 2 {
		_, _, account, sess, release := setupProviderAndSessionInternal(ctx, t)
		defer release()
		account.StopSessionTransport()
		account.t.p.localNetwork = transport.WithInprocNetwork(network)
		if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, account)
		sessions = append(sessions, sess)
	}
	a := accounts[0]
	spaceRef, err := a.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	seedAccountReplicaPayload(ctx, t, a, spaceRef)
	b, _ := enrollMeshReplica(ctx, t, a, sessions[0], accounts[1], sessions[1])
	waitReplicaCopy(ctx, t, b, spaceRef)

	source, releaseSource, err := a.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSource()
	replica, releaseReplica, err := b.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReplica()

	// Each run copies a fresh graph into the replica's volume the way
	// RetainWorld does: batched writes, then a durability fence.
	measure := func(name string, src block.StoreOps, reads int) float64 {
		root, blocks := seedRateGraph(ctx, t, source.GetBlockStore(), fmt.Sprintf("%s-%d", name, reads))
		dst := block.NewBufferedStoreWithSettings(ctx, replica.GetBlockStore(), &block.BufferedStoreSettings{
			MaxPendingEntries: 1024,
			MaxPendingBytes:   4 << 20,
			DrainBatchEntries: 1024,
		})
		start := time.Now()
		if err := block.CopyGraph(ctx, src, dst, root, &block.GraphCopyOptions{Reads: reads}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := dst.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		rate := float64(blocks) / elapsed.Seconds()
		t.Logf("%s reads=%d blocks=%d time=%v rate=%.0f blocks/s", name, reads, blocks, elapsed.Round(time.Millisecond), rate)
		return rate
	}

	local := measure("local", source.GetBlockStore(), 16)
	for _, reads := range []int{1, 8, 32} {
		rate := measure("dex", replica.GetBlockStore(), reads)
		t.Logf("dex reads=%d runs at %.2f of the local rate", reads, rate/local)
	}
}

// seedRateGraph writes a tree of unique blocks with their edges and returns
// its root and block count.
func seedRateGraph(ctx context.Context, t *testing.T, store block.StoreOps, salt string) (*block.BlockRef, int) {
	t.Helper()
	put := func(data []byte, refs []*block.BlockRef) *block.BlockRef {
		ref, _, err := store.PutBlock(ctx, data, &block.PutOpts{Refs: refs})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	payload := make([]byte, 1024)
	level := make([]*block.BlockRef, 0, rateGraphLeaves)
	for i := range rateGraphLeaves {
		copy(payload, fmt.Sprintf("%s leaf %d", salt, i))
		level = append(level, put(payload, []*block.BlockRef{}))
	}
	blocks := len(level)
	for len(level) > 1 {
		var parents []*block.BlockRef
		for i := 0; i < len(level); i += rateGraphFanout {
			children := level[i:min(i+rateGraphFanout, len(level))]
			parents = append(parents, put(fmt.Appendf(nil, "%s node %d/%d", salt, len(level), i), children))
		}
		blocks += len(parents)
		level = parents
	}
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	return level[0], blocks
}
