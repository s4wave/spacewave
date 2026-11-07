package block_store_nodepush

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// node answers each Send from flush, or waits for its context when flush is
// nil, and records when a Send was still running.
type node struct {
	flush   chan error
	sends   atomic.Int32
	running atomic.Int32
}

func (n *node) Send(ctx context.Context, _ []*block.PutBatchEntry) error {
	n.sends.Add(1)
	n.running.Add(1)
	defer n.running.Add(-1)
	if n.flush == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case err := <-n.flush:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestPublisherRootChanges checks disabled, successful, failed and silent node
// pushes, including a local commit that outlasts the configured wait.
func TestPublisherRootChanges(t *testing.T) {
	// Build one root change and its node sender.
	ref, err := block.BuildBlockRef([]byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Data: []byte("x"), Ref: ref}}
	roots := []block.NamedRoot{{Name: "main", Ref: ref}}
	n := &node{flush: make(chan error, 1)}
	var on atomic.Bool
	var results atomic.Int32
	p := NewPublisher(n, 20*time.Millisecond, &on, func(error, time.Duration) { results.Add(1) })
	commit := func(_ context.Context, got []*block.PutBatchEntry, names []block.NamedRoot) error {
		if len(got) != 1 || got[0] != entries[0] || len(names) != 1 || !names[0].Ref.EqualsRef(ref) {
			t.Error("root change altered")
		}
		return nil
	}

	// Disabled pushes commit locally without contacting the node.
	if err := p.SetRoots(t.Context(), entries, roots, commit); err != nil || n.sends.Load() != 0 {
		t.Fatalf("disabled push: %v, sends %d", err, n.sends.Load())
	}
	on.Store(true)

	// A local commit waits for the node to flush, then returns its own result.
	localDone := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.SetRoots(t.Context(), entries, roots, func(ctx context.Context, e []*block.PutBatchEntry, r []block.NamedRoot) error {
			defer close(localDone)
			return commit(ctx, e, r)
		})
	}()
	<-localDone
	select {
	case <-done:
		t.Fatal("root change returned before node flush")
	default:
	}
	n.flush <- nil
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// A node failure preserves a local failure and never replaces its result.
	failed := errors.New("local failed")
	n.flush <- errors.New("node away")
	if err := p.SetRoots(t.Context(), entries, roots, func(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("local result: %v", err)
	}

	// A silent node times out and releases every borrow before returning.
	n.flush = nil
	if err := p.SetRoots(t.Context(), entries, roots, commit); err != nil || n.running.Load() != 0 {
		t.Fatalf("silent node: %v, sends running %d", err, n.running.Load())
	}

	// A slow local commit lets the node use all of its time beyond the wait.
	n.flush = make(chan error, 1)
	if err := p.SetRoots(t.Context(), entries, roots, func(ctx context.Context, e []*block.PutBatchEntry, r []block.NamedRoot) error {
		time.Sleep(60 * time.Millisecond)
		n.flush <- nil
		return commit(ctx, e, r)
	}); err != nil {
		t.Fatal(err)
	}
	p.Close()
	if results.Load() != 4 {
		t.Fatalf("results %d", results.Load())
	}
}
