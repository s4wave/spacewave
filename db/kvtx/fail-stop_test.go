package kvtx

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestFailStopSharedAdmit runs a commit through flusher b's Admit while
// flusher a, sharing b's file, fails its flush. The failure must wait for
// the commit, which shows its write before any flush, and b must admit
// nothing after it: a commit landing after the failure would sit behind
// frames the drive lacks, and b's readers would take it for stored.
func TestFailStopSharedAdmit(t *testing.T) {
	if FailStopOff {
		t.Skip("fail-stop is off")
	}
	errFault := errors.New("injected flush failure")
	stop := NewFailStop()
	a := NewOrderedFlusher(func() error { return errFault }, time.Hour, stop)
	b := NewOrderedFlusher(func() error { return nil }, time.Hour, stop)

	// Hold a commit inside b's Admit.
	entered, release := make(chan struct{}), make(chan struct{})
	admitted := make(chan error, 1)
	go func() {
		admitted <- b.Admit(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	// Wait on b for an ordered commit its deadline flush, an hour away,
	// would make durable.
	b.Ordered()
	waited := make(chan error, 1)
	go func() { waited <- b.WaitDurable(context.Background()) }()

	// Fail a's flush. Its failure must wait for b's commit.
	a.Ordered()
	failed := make(chan error, 1)
	go func() { failed <- a.Sync(context.Background()) }()
	select {
	case err := <-failed:
		close(release)
		t.Fatalf("a's failed flush returned %v beside b's admitted commit", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-admitted; err != nil {
		t.Fatalf("b's commit admitted before the failure returned %v", err)
	}
	if err := <-failed; !errors.Is(err, errFault) || !errors.Is(err, ErrFlushFailed) {
		t.Fatalf("a's failed flush returned %v", err)
	}

	// b's waiter wakes with the failure, and b admits, flushes and waits for
	// nothing after it.
	select {
	case err := <-waited:
		if !errors.Is(err, ErrFlushFailed) {
			t.Fatalf("b's durable wait across the failure returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("b's durable wait did not wake at a's failure")
	}
	ran := false
	if err := b.Admit(func() error { ran = true; return nil }); !errors.Is(err, ErrFlushFailed) || ran {
		t.Fatalf("b's commit after the failure ran %v and returned %v", ran, err)
	}
	if err := b.Sync(context.Background()); !errors.Is(err, ErrFlushFailed) {
		t.Fatalf("b's flush after the failure returned %v", err)
	}
	if err := b.WaitDurable(context.Background()); !errors.Is(err, ErrFlushFailed) {
		t.Fatalf("b's durable wait after the failure returned %v", err)
	}
	_ = a.Close()
	_ = b.Close()
}
