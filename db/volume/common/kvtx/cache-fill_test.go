package kvtx

import (
	"fmt"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func TestCacheFillsShareTransactions(t *testing.T) {
	// Hold the first fill's transaction so the rest queue behind it.
	v, raw := newTestVolume(t)
	leaf, err := block.BuildBlockRef([]byte("leaf"), nil)
	if err != nil {
		t.Fatal(err)
	}
	const n = 40
	datas := make([][]byte, n)
	for i := range datas {
		datas[i] = fmt.Appendf(nil, "filled block %d", i)
	}
	fill := func(i int) (*block.BlockRef, bool, error) {
		return v.PrepareOwnedBlock(t.Context(), "bucket", datas[i], &block.PutOpts{
			Refs:      []*block.BlockRef{leaf},
			RefsKnown: true,
			CacheFill: true,
		})
	}
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()
	gate := raw.arm(t)
	type result struct {
		ref     *block.BlockRef
		existed bool
		err     error
	}
	results := make([]chan result, n)
	for i := range results {
		results[i] = make(chan result, 1)
	}
	go func() {
		ref, existed, err := fill(0)
		results[0] <- result{ref, existed, err}
	}()
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("first fill did not start a transaction")
	}

	// Queue the rest while the first holds the writer.
	for i := 1; i < n; i++ {
		go func() {
			ref, existed, err := fill(i)
			results[i] <- result{ref, existed, err}
		}()
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		v.fills.mu.Lock()
		queued := len(v.fills.pending)
		v.fills.mu.Unlock()
		if queued == n-1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d fills queued behind the first", queued, n-1)
		}
		time.Sleep(time.Millisecond)
	}
	gate.unblock()

	// Verify every fill is stored with its refs and no owner.
	for i, ch := range results {
		var res result
		select {
		case res = <-ch:
		case <-time.After(3 * time.Second):
			t.Fatalf("fill %d did not finish", i)
		}
		if res.err != nil || res.existed {
			t.Fatalf("fill %d: existed=%v err=%v", i, res.existed, res.err)
		}
		got, found, err := v.GetBlock(t.Context(), res.ref)
		if err != nil || !found || string(got) != string(datas[i]) {
			t.Fatalf("fill %d stored found=%v err=%v", i, found, err)
		}
		children, listed, err := v.GetChildren(t.Context(), res.ref)
		if err != nil || !listed || len(children) != 1 || !children[0].EqualsRef(leaf) {
			t.Fatalf("fill %d children=%v listed=%v err=%v", i, children, listed, err)
		}
	}

	// Verify the queued fills shared one transaction after the first.
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 2 {
		t.Fatalf("%d fills used %d commits, want 2", n, count)
	}

	// A batch of fills, such as a pack's neighbors, lands under the fill
	// roots in one commit, held as the queued fills are.
	batch := make([]*block.PutBatchEntry, 3)
	for i := range batch {
		batch[i] = &block.PutBatchEntry{Data: fmt.Appendf(nil, "pack neighbor %d", i), Refs: []*block.BlockRef{leaf}, RefsKnown: true, CacheFill: true}
	}
	unheld := v.notHeld[block_gc.NotHeldPut].Load()
	existed, err := v.PrepareOwnedBlockBatch(t.Context(), "bucket", batch)
	if err != nil || len(existed) != len(batch) {
		t.Fatalf("fill batch: existed=%v err=%v", existed, err)
	}
	if got := v.notHeld[block_gc.NotHeldPut].Load() - unheld; got != 0 {
		t.Fatalf("fill batch stored %d blocks unheld", got)
	}
	for i, entry := range batch {
		if got, found, err := v.GetBlock(t.Context(), entry.Ref); err != nil || !found || string(got) != string(entry.Data) {
			t.Fatalf("batch fill %d stored found=%v err=%v", i, found, err)
		}
	}
}
