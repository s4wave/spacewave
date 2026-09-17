package block

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestBufferedPublicationRetainsReadabilityAndRetries(t *testing.T) {
	ctx := t.Context()
	inner := newCountStore(0)
	s := NewBufferedStore(ctx, inner)
	data := []byte("first publication")
	ref, _, err := s.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := s.TakePending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entries) != 1 {
		t.Fatal(len(batch.Entries))
	}
	data[0] = '!'
	got, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != "first publication" {
		t.Fatalf("retained read: %q %v %v", got, found, err)
	}
	other, _, err := s.PutBlock(ctx, []byte("second publication"), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.TakePending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || !second.Entries[0].Ref.EqualsRef(other) {
		t.Fatal("borrow crossed publications")
	}
	rejected := errors.New("stale head")
	batch.Complete(rejected)
	batch.Complete(nil) // Exactly-once cleanup must not lose retry data.
	retry, err := s.TakePending(ctx)
	if err != nil || len(retry.Entries) != 1 || !retry.Entries[0].Ref.EqualsRef(ref) {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	if err := inner.PutBlockBatch(ctx, retry.Entries); err != nil {
		t.Fatal(err)
	}
	retry.Complete(nil)
	if err := inner.PutBlockBatch(ctx, second.Entries); err != nil {
		t.Fatal(err)
	}
	second.Complete(nil)
	if len(s.pending) != 0 || s.pendingBytes != 0 || s.inFlight != 0 {
		t.Fatal("leaked accounting")
	}
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBufferedPublicationCapacityAndSyncWaitForDurability(t *testing.T) {
	inner := newCountStore(0)
	s := NewBufferedStoreWithSettings(t.Context(), inner, &BufferedStoreSettings{MaxPendingBytes: 8, MaxPendingEntries: 1})
	_, _, err := s.PutBlock(t.Context(), []byte("12345678"), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.TakePending(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := s.PutBlock(ctx, []byte("next"), nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("capacity: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel2()
	if _, err := s.Sync(ctx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sync: %v", err)
	}
	if err := inner.PutBlockBatch(t.Context(), b.Entries); err != nil {
		t.Fatal(err)
	}
	b.Complete(nil)
	if _, _, err := s.PutBlock(t.Context(), []byte("next"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestBufferedPublicationOrdersReplacementAfterBorrow(t *testing.T) {
	inner := newCountStore(0)
	s := NewBufferedStore(t.Context(), inner)
	ref, _, err := s.PutBlock(t.Context(), []byte("kept"), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.TakePending(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.RmBlock(t.Context(), ref) }()
	select {
	case err := <-done:
		t.Fatalf("delete overtook borrowed put: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if err := inner.PutBlockBatch(t.Context(), b.Entries); err != nil {
		t.Fatal(err)
	}
	b.Complete(nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("delete stuck")
	}
	if _, err := s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if found, err := inner.GetBlockExists(t.Context(), ref); err != nil || found {
		t.Fatalf("deleted: %v %v", found, err)
	}
}

func TestBufferedOversizedEntryMakesProgress(t *testing.T) {
	s := NewBufferedStoreWithSettings(t.Context(), newCountStore(0), &BufferedStoreSettings{MaxPendingBytes: 8})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	data := bytes.Repeat([]byte("x"), 32)
	ref, _, err := s.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("read: %v %v", found, err)
	}
	if s.pendingBytes > 8 {
		t.Fatal("oversized entry exceeded bound")
	}
}

func TestBufferedReplacementsPreserveSingleQueueEntry(t *testing.T) {
	s := NewBufferedStore(t.Context(), newCountStore(0))
	ref, _, err := s.PutBlock(t.Context(), []byte("restore"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RmBlock(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutBlock(t.Context(), []byte("restore"), nil); err != nil {
		t.Fatal(err)
	}
	b, err := s.TakePending(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Complete(errors.New("test discarded"))
	if len(b.Entries) != 1 || b.Entries[0].Tombstone {
		t.Fatal("duplicate or stale queued operation")
	}
}
