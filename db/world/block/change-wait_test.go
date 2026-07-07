package world_block_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

type waitChangeResult struct {
	seqno uint64
	err   error
}

func TestEngineWaitChangeObjectFilterIgnoresNonMatchingCommit(t *testing.T) {
	ctx := context.Background()
	eng, cleanup := setupChangeWaitEngine(ctx, t)
	defer cleanup()

	initialSeqno, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan waitChangeResult, 1)
	go func() {
		seqno, err := eng.WaitChange(waitCtx, initialSeqno, world.ChangeFilter{ObjectKeys: []string{"watched/object"}})
		resultCh <- waitChangeResult{seqno: seqno, err: err}
	}()

	commitObject(t, ctx, eng, "ignored/object")
	assertNoWaitChangeResult(t, resultCh, "non-matching object commit")

	commitObject(t, ctx, eng, "watched/object")
	result := recvWaitChangeResult(t, resultCh, "matching object commit")
	if result.err != nil {
		t.Fatal(result.err.Error())
	}
	if result.seqno <= initialSeqno {
		t.Fatalf("wait seqno = %d, want > %d", result.seqno, initialSeqno)
	}
}

func TestEngineWaitChangeGraphFilterIgnoresNonMatchingCommit(t *testing.T) {
	ctx := context.Background()
	eng, cleanup := setupChangeWaitEngine(ctx, t)
	defer cleanup()

	for _, key := range []string{"graph/a", "graph/b"} {
		commitObject(t, ctx, eng, key)
	}
	initialSeqno, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan waitChangeResult, 1)
	go func() {
		seqno, err := eng.WaitChange(waitCtx, initialSeqno, world.ChangeFilter{
			GraphQuads: []world.GraphQuad{world.NewGraphQuadWithKeys("graph/a", "rel/match", "graph/b", "")},
		})
		resultCh <- waitChangeResult{seqno: seqno, err: err}
	}()

	commitGraphQuad(t, ctx, eng, world.NewGraphQuadWithKeys("graph/a", "rel/other", "graph/b", ""))
	assertNoWaitChangeResult(t, resultCh, "non-matching graph commit")

	commitGraphQuad(t, ctx, eng, world.NewGraphQuadWithKeys("graph/a", "rel/match", "graph/b", ""))
	result := recvWaitChangeResult(t, resultCh, "matching graph commit")
	if result.err != nil {
		t.Fatal(result.err.Error())
	}
	if result.seqno <= initialSeqno {
		t.Fatalf("wait seqno = %d, want > %d", result.seqno, initialSeqno)
	}
}

func setupChangeWaitEngine(ctx context.Context, t *testing.T) (*world_block.Engine, func()) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	root, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	eng, err := world_block.NewEngine(ctx, le, root, world_mock.LookupMockOp, nil, false)
	if err != nil {
		root.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	return eng, func() {
		if err := eng.Close(); err != nil {
			t.Fatal(err.Error())
		}
		root.Release()
		tb.Release()
	}
}

func commitObject(t *testing.T, ctx context.Context, eng *world_block.Engine, key string) {
	t.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, key, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func commitGraphQuad(t *testing.T, ctx context.Context, eng *world_block.Engine, q world.GraphQuad) {
	t.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	if err := tx.SetGraphQuad(ctx, q); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func assertNoWaitChangeResult(t *testing.T, resultCh <-chan waitChangeResult, event string) {
	t.Helper()
	select {
	case result := <-resultCh:
		t.Fatalf("WaitChange returned after %s: seqno=%d err=%v", event, result.seqno, result.err)
	case <-time.After(100 * time.Millisecond):
	}
}

func recvWaitChangeResult(t *testing.T, resultCh <-chan waitChangeResult, event string) waitChangeResult {
	t.Helper()
	select {
	case result := <-resultCh:
		return result
	case <-time.After(5 * time.Second):
		t.Fatalf("WaitChange did not return after %s", event)
		return waitChangeResult{}
	}
}
