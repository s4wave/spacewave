package provider_local

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// hookSQLStore stands in for the SQL backend, calling hook before each
// insert.
type hookSQLStore struct {
	*batchForwardTestStore
	mu   sync.Mutex
	hook func(ctx context.Context, entries []*block.PutBatchEntry) error
}

// PutBlockBatch calls the hook, then inserts entries.
func (s *hookSQLStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Call the hook, failing the insert with its error.
	s.mu.Lock()
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		if err := hook(ctx, entries); err != nil {
			return nil, err
		}
	}

	// Insert the entries.
	return s.Store.PutBlockBatch(ctx, entries)
}

// setHook sets the hook called before each insert.
func (s *hookSQLStore) setHook(hook func(ctx context.Context, entries []*block.PutBatchEntry) error) {
	s.mu.Lock()
	s.hook = hook
	s.mu.Unlock()
}

// has reports whether the backend holds ref.
func (s *hookSQLStore) has(t *testing.T, ref *block.BlockRef) bool {
	found, err := s.GetBlockExists(context.Background(), ref)
	if err != nil {
		t.Error(err)
	}
	return found
}

// newSQLPublishTest runs a publisher of one-row inserts over a fresh backend
// and a real account volume until the test ends. The returned cancel closes
// it and waits.
func newSQLPublishTest(t *testing.T) (*sqlPublisher, *hookSQLStore, func()) {
	// Build the backend and the account volume.
	t.Helper()
	sql := &hookSQLStore{batchForwardTestStore: newBatchForwardTestStore()}
	vol, err := volume_kvtx.NewVolume(t.Context(), "test/sql-publish", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	// Run the publisher until the test ends.
	p := newSQLPublisher(logrus.NewEntry(logrus.New()), sql, vol)
	p.workers, p.chunkRows = 4, 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return p, sql, stop
}

// sqlPublicationFor builds publication i of blocks, whose head moves from
// i-1 to i and calls check as it does.
func sqlPublicationFor(t *testing.T, i int, after *block.PublicationReceipt, check func(), blocks ...string) (*block.AtomicPublication, []*block.BlockRef) {
	// Build an entry for each block.
	t.Helper()
	pub := &block.AtomicPublication{After: after}
	var refs []*block.BlockRef
	for _, data := range blocks {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		pub.Entries = append(pub.Entries, &block.PutBatchEntry{Ref: ref, Data: []byte(data)})
		refs = append(refs, ref)
	}

	// Move the head from i-1 to i.
	pub.Heads = []*block.AtomicHeadUpdate{{ObjectStoreID: "so", Key: []byte("head"), Replace: func(_ context.Context, current []byte, found bool) ([]byte, error) {
		if check != nil {
			check()
		}
		if want := strconv.Itoa(i - 1); (i > 1) != found || (found && string(current) != want) {
			return nil, errors.New("head " + string(current) + " is not " + want)
		}
		return []byte(strconv.Itoa(i)), nil
	}}}
	return pub, refs
}

// waitFor polls cond for a second.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSQLPublishOrder checks that publications whose inserts land out of
// order commit their heads in admission order, each only after the blocks of
// every publication up to it are in the backend, and that the overlay serves
// the admitted blocks until they land.
func TestSQLPublishOrder(t *testing.T) {
	// Run a publisher with reads through its overlay.
	p, sql, _ := newSQLPublishTest(t)
	reads := newSQLPublishReads(p, sql)

	// Hold the inserts of the first publication's block.
	release := make(chan struct{})
	sql.setHook(func(ctx context.Context, entries []*block.PutBatchEntry) error {
		if bytes.HasPrefix(entries[0].Data, []byte("slow")) {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Admit three publications, each after the one before.
	var mu sync.Mutex
	var order []int
	var all []*block.BlockRef
	var receipts []*block.PublicationReceipt
	var after *block.PublicationReceipt
	for i, data := range [][]string{{"slow-a", "fast-a"}, {"fast-b"}, {"fast-c", "fast-a"}} {
		n := i + 1
		var upTo []*block.BlockRef
		pub, refs := sqlPublicationFor(t, n, after, func() {
			for _, ref := range upTo {
				if !sql.has(t, ref) {
					t.Errorf("head %d committed before %s landed", n, ref.MarshalString())
				}
			}
			mu.Lock()
			order = append(order, n)
			mu.Unlock()
		}, data...)
		all = append(all, refs...)
		upTo = append(upTo, all...)
		receipt, err := p.submit(t.Context(), pub)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
		after = receipt
	}

	// The later publications drain and wait for the first.
	waitFor(t, "the later drains", func() bool { return p.getStats().Drained == 2 })
	for i, r := range receipts {
		select {
		case <-r.Done():
			t.Fatalf("receipt %d resolved before the first publication's rows landed", i+1)
		default:
		}
	}

	// The overlay serves the held block, including in a read scope.
	slow := all[0]
	if sql.has(t, slow) {
		t.Fatal("the held block landed")
	}
	scoped, rel, err := reads.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []block.StoreOps{reads, scoped} {
		data, found, err := store.GetBlock(t.Context(), slow)
		if err != nil || !found || string(data) != "slow-a" {
			t.Fatalf("overlay read = %q, %v, %v", data, found, err)
		}
		exists, err := store.GetBlockExistsBatch(t.Context(), []*block.BlockRef{all[1], slow})
		if err != nil || !exists[0] || !exists[1] {
			t.Fatalf("overlay exists = %v, %v", exists, err)
		}
	}
	rel()

	// Releasing it commits all three in order.
	close(release)
	for i, r := range receipts {
		if err := r.Wait(t.Context()); err != nil {
			t.Fatalf("receipt %d: %v", i+1, err)
		}
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("heads committed in order %v", order)
	}
	stats := p.getStats()
	if stats.Committed != 3 || stats.Publications != 0 || stats.Blocks != 0 || stats.Bytes != 0 {
		t.Fatalf("stats after the commits = %+v", stats)
	}
	if _, found, _ := reads.GetBlock(t.Context(), slow); !found {
		t.Fatal("the landed block is not read from the backend")
	}
}

// TestSQLPublishRetry checks that a failed insert retries with the receipt
// pending, and the receipt succeeds once it lands.
func TestSQLPublishRetry(t *testing.T) {
	// Fail the first two inserts.
	p, sql, _ := newSQLPublishTest(t)
	var mu sync.Mutex
	fails := 2
	sql.setHook(func(context.Context, []*block.PutBatchEntry) error {
		mu.Lock()
		defer mu.Unlock()
		if fails > 0 {
			fails--
			return errors.New("injected insert failure")
		}
		return nil
	})

	// Submit a publication; its receipt waits through the failed insert.
	pub, refs := sqlPublicationFor(t, 1, nil, nil, "retried")
	receipt, err := p.submit(t.Context(), pub)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a failed insert", func() bool { return p.getStats().StatementFailures >= 1 })
	select {
	case <-receipt.Done():
		t.Fatal("a failed insert resolved the receipt")
	default:
	}

	// The retry lands the block and succeeds.
	if err := receipt.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !sql.has(t, refs[0]) {
		t.Fatal("the retried block is not in the backend")
	}
	if stats := p.getStats(); stats.StatementFailures != 2 || stats.Failed != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

// TestSQLPublishClose checks that unmount fails the publications that did
// not commit, and refuses new ones.
func TestSQLPublishClose(t *testing.T) {
	// Hold every insert until close.
	p, sql, stop := newSQLPublishTest(t)
	sql.setHook(func(ctx context.Context, _ []*block.PutBatchEntry) error {
		<-ctx.Done()
		return ctx.Err()
	})

	// Submit two publications that cannot land.
	pub1, _ := sqlPublicationFor(t, 1, nil, nil, "stuck-1")
	r1, err := p.submit(t.Context(), pub1)
	if err != nil {
		t.Fatal(err)
	}
	pub2, _ := sqlPublicationFor(t, 2, r1, nil, "stuck-2")
	r2, err := p.submit(t.Context(), pub2)
	if err != nil {
		t.Fatal(err)
	}

	// Closing fails both.
	stop()
	for _, r := range []*block.PublicationReceipt{r1, r2} {
		if err := r.Wait(t.Context()); !errors.Is(err, block.ErrPublicationClosed) {
			t.Fatalf("receipt after close = %v", err)
		}
	}

	// A closed publisher refuses new ones.
	pub3, _ := sqlPublicationFor(t, 3, nil, nil, "late")
	if _, err := p.submit(t.Context(), pub3); err == nil {
		t.Fatal("a closed publisher admitted a publication")
	}
}

// TestSQLPublishDependency checks that a publication whose head fails to
// commit fails the one after it with a dependency error, and that a
// tombstone is left to the old path.
func TestSQLPublishDependency(t *testing.T) {
	// Run a publisher.
	p, _, _ := newSQLPublishTest(t)

	// The first head expects a head that is not there.
	pub1, _ := sqlPublicationFor(t, 2, nil, nil, "stale")
	r1, err := p.submit(t.Context(), pub1)
	if err != nil {
		t.Fatal(err)
	}
	pub2, _ := sqlPublicationFor(t, 3, r1, nil, "dependent")
	r2, err := p.submit(t.Context(), pub2)
	if err != nil {
		t.Fatal(err)
	}

	// The first fails and fails the second.
	if err := r1.Wait(t.Context()); err == nil {
		t.Fatal("the stale head committed")
	}
	if err := r2.Wait(t.Context()); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatalf("dependent receipt = %v", err)
	}
	if stats := p.getStats(); stats.Failed != 2 || stats.Publications != 0 {
		t.Fatalf("stats = %+v", stats)
	}

	// A tombstone is unsupported.
	pub3, refs := sqlPublicationFor(t, 1, nil, nil, "tombstone")
	pub3.Entries[0] = &block.PutBatchEntry{Ref: refs[0], Tombstone: true}
	if _, err := p.submit(t.Context(), pub3); !errors.Is(err, block.ErrAtomicPublicationUnsupported) {
		t.Fatalf("tombstone admission = %v", err)
	}
}

// countingReads counts the block reads that reach the backend.
type countingReads struct {
	*hookSQLStore
	reads atomic.Int32
}

func (s *countingReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.reads.Add(1)
	return s.hookSQLStore.GetBlock(ctx, ref)
}

func (s *countingReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.reads.Add(1)
	return s.hookSQLStore.GetStoredBlock(ctx, ref)
}

// TestSQLPublishLanded checks that the pages of a landed head read from the
// landed blocks without a row read, read one row each with the bound at zero,
// and that a page evicted or removed after landing reads its row, with the
// same bytes the landed block held.
func TestSQLPublishLanded(t *testing.T) {
	// publish lands a head of pages through a publisher bounded by bound and
	// returns its reads, the backend's read count and the pages.
	publish := func(bound int64, pages ...string) (*sqlPublishReads, *countingReads, []*block.BlockRef) {
		// Start the publisher with the bound and count its backend reads.
		t.Helper()
		p, sql, _ := newSQLPublishTest(t)
		p.landed = newSQLLanded(bound)
		lower := &countingReads{hookSQLStore: sql}

		// Publish the pages and wait for their rows and head.
		pub, refs := sqlPublicationFor(t, 1, nil, nil, pages...)
		receipt, err := p.submit(t.Context(), pub)
		if err != nil {
			t.Fatal(err)
		}
		if err := receipt.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		return newSQLPublishReads(p, lower), lower, refs
	}

	// diff reads every page as the change watch does and checks its bytes.
	diff := func(reads *sqlPublishReads, refs []*block.BlockRef, pages ...string) {
		t.Helper()
		for i, ref := range refs {
			stored, err := reads.GetStoredBlock(t.Context(), ref)
			if err != nil || stored == nil || string(stored.Data) != pages[i] {
				t.Fatalf("page %d = %v, %v; want %q", i, stored, err, pages[i])
			}
		}
	}

	// A landed head reads no row; with the bound at zero each page reads one.
	pages := []string{"root", "dir", "leaf-a", "leaf-b"}
	reads, lower, refs := publish(defaultSQLLandedBytes, pages...)
	diff(reads, refs, pages...)
	if n := lower.reads.Load(); n != 0 {
		t.Fatalf("landed head read %d rows, want 0", n)
	}
	if stats := reads.pub.getStats(); stats.LandedHits != 4 || stats.LandedMisses != 0 || stats.LandedBytes != 19 {
		t.Fatalf("stats = %+v", stats)
	}
	reads, lower, refs = publish(0, pages...)
	diff(reads, refs, pages...)
	if n := lower.reads.Load(); n != 4 {
		t.Fatalf("unbounded head read %d rows, want 4", n)
	}

	// A bound of one page keeps only the last to land: the evicted pages read
	// their rows with the same bytes.
	pages = []string{"page-1", "page-2", "page-3"}
	reads, lower, refs = publish(6, pages...)
	diff(reads, refs, pages...)
	if n := lower.reads.Load(); n != 2 {
		t.Fatalf("evicted pages read %d rows, want 2", n)
	}

	// A page removed after it landed misses the landed blocks and reads the
	// backend, which no longer holds it.
	pages = []string{"removed"}
	reads, lower, refs = publish(defaultSQLLandedBytes, pages...)
	block.InvalidateRemovedBlock(refs[0])
	if err := lower.RmBlock(t.Context(), refs[0]); err != nil {
		t.Fatal(err)
	}
	if stored, err := reads.GetStoredBlock(t.Context(), refs[0]); err != nil || stored != nil {
		t.Fatalf("removed page = %v, %v; want none", stored, err)
	}
	if n := lower.reads.Load(); n != 1 {
		t.Fatalf("removed page read %d rows, want 1", n)
	}
}
