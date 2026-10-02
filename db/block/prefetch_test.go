package block

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// prefetchTestStore records the order and concurrency of block reads.
type prefetchTestStore struct {
	NopStoreOps
	// fail is the ref whose read fails.
	fail *BlockRef
	// started receives a value as each read starts, if set.
	started chan struct{}
	// release unblocks reads when closed.
	release chan struct{}
	// mtx guards the fields below.
	mtx sync.Mutex
	// order holds the refs in the order reads started.
	order []*BlockRef
	// inFlight and peak count concurrent reads.
	inFlight, peak int
}

// GetBlock records the read and waits for release.
func (s *prefetchTestStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	// Record the read and the peak concurrency.
	s.mtx.Lock()
	s.order = append(s.order, ref)
	s.inFlight++
	s.peak = max(s.peak, s.inFlight)
	s.mtx.Unlock()
	defer func() {
		s.mtx.Lock()
		s.inFlight--
		s.mtx.Unlock()
	}()
	if s.started != nil {
		s.started <- struct{}{}
	}

	// Hold the read until released, then fail the chosen ref.
	<-s.release
	if ref == s.fail {
		return nil, false, errors.New("read failed")
	}
	return []byte("data"), true, nil
}

// prefetchTestRefs builds n distinct refs.
func prefetchTestRefs(t *testing.T, n int) []*BlockRef {
	refs := make([]*BlockRef, n)
	for i := range refs {
		ref, err := BuildBlockRef([]byte(strconv.Itoa(i)), &PutOpts{HashType: hash.HashType_HashType_SHA256})
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	return refs
}

// TestPrefetchReadsInOrder checks that one worker reads every ref in order.
func TestPrefetchReadsInOrder(t *testing.T) {
	// Prefetch with one read in flight.
	refs := prefetchTestRefs(t, 20)
	store := &prefetchTestStore{release: make(chan struct{})}
	close(store.release)
	if err := Prefetch(context.Background(), store, refs, 1); err != nil {
		t.Fatal(err)
	}

	// The reads follow the list.
	if len(store.order) != len(refs) {
		t.Fatalf("read %d of %d refs", len(store.order), len(refs))
	}
	for i, ref := range refs {
		if store.order[i] != ref {
			t.Fatalf("read %d is not ref %d", i, i)
		}
	}
}

// TestPrefetchBoundsConcurrency checks that reads run concurrently up to the
// bound, and that a failed read does not stop the others.
func TestPrefetchBoundsConcurrency(t *testing.T) {
	// Prefetch with four reads in flight, one of which fails.
	refs := prefetchTestRefs(t, 40)
	store := &prefetchTestStore{
		fail:    refs[3],
		started: make(chan struct{}, len(refs)),
		release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		done <- Prefetch(context.Background(), store, refs, 4)
	}()

	// Release the reads once four have started.
	for range 4 {
		<-store.started
	}
	close(store.release)

	// Every ref was read, never more than four at once, and the failure is
	// reported.
	if err := <-done; err == nil {
		t.Fatal("expected the failed read's error")
	}
	if len(store.order) != len(refs) {
		t.Fatalf("read %d of %d refs", len(store.order), len(refs))
	}
	if store.peak != 4 {
		t.Fatalf("expected 4 concurrent reads, got %d", store.peak)
	}
}

// TestPrefetchStopsWithContext checks that prefetch stops when ctx ends.
func TestPrefetchStopsWithContext(t *testing.T) {
	// Cancel before any read can finish.
	refs := prefetchTestRefs(t, 10)
	store := &prefetchTestStore{release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	close(store.release)

	// No read starts and the context error returns.
	if err := Prefetch(ctx, store, refs, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(store.order) != 0 {
		t.Fatalf("read %d refs after cancel", len(store.order))
	}
}
