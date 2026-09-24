//go:build !js

package engine

import (
	"bytes"
	"runtime/trace"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// TestWorkloadRecordsLogicalOperations checks that a traced session records
// each caller operation once, in order, with the IDs that tie iterators and
// scoped reads to their transaction or scope, and that internal writeback,
// read-through, and fence calls add no records.
func TestWorkloadRecordsLogicalOperations(t *testing.T) {
	ctx := t.Context()
	e, err := Open(ctx, newDiskBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := NewBlockStore(ctx, e, 0)
	defer s.Close()

	// Run a key-value and block session under an execution trace.
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("start trace: %v", err)
	}
	tx, err := e.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("k1"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	read, err := e.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := read.Get(ctx, []byte("k1")); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Exists(ctx, []byte("k2")); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Size(ctx); err != nil {
		t.Fatal(err)
	}
	read.Discard()
	ref, _, err := s.PutBlock(ctx, []byte("block"), &block.PutOpts{Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	scope, release, err := s.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := scope.GetBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := s.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err)
	}
	trace.Stop()

	// Compare the extracted operations with the session.
	events, err := workload.Extract(&buf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	var ops []workload.Op
	for _, ev := range events {
		ops = append(ops, ev.Record.Op)
	}
	want := []workload.Op{
		workload.OpTxWrite, workload.OpSet, workload.OpCommit, workload.OpDiscard,
		workload.OpTxRead, workload.OpGet, workload.OpExists,
		workload.OpIterate, workload.OpIterEnd, workload.OpDiscard,
		workload.OpPut, workload.OpSync,
		workload.OpReadBegin, workload.OpGetBlock, workload.OpReadEnd,
		workload.OpExistsBatch, workload.OpBlockExists,
	}
	if !slices.Equal(ops, want) {
		t.Fatalf("ops = %v, want %v", ops, want)
	}

	// Check the fields that tie records together and carry results.
	rec := func(i int) workload.Record { return events[i].Record }
	if rec(5).Size != 5 || rec(6).Size != 0 || rec(8).Size != 1 {
		t.Fatalf("get, exists, iter-end sizes = %d %d %d, want 5 0 1", rec(5).Size, rec(6).Size, rec(8).Size)
	}
	if rec(7).Parent != rec(4).ID || rec(8).ID != rec(7).ID {
		t.Fatalf("iterator %+v does not belong to transaction %+v", rec(7), rec(4))
	}
	if rec(13).ID != rec(12).ID || rec(14).ID != rec(12).ID || rec(16).ID != rec(15).ID {
		t.Fatalf("scoped reads do not carry their scope IDs: %+v", events[12:])
	}
	if !bytes.Equal(rec(13).Key, rec(10).Key) || rec(13).Size != 5 {
		t.Fatalf("scoped read %+v does not match put %+v", rec(13), rec(10))
	}
}
