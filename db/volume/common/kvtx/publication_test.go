package kvtx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type publicationGate struct {
	started, release chan struct{}
	once             sync.Once
}

func (g *publicationGate) unblock() { g.once.Do(func() { close(g.release) }) }

type publicationTestStore struct {
	db_kvtx.Store
	mu                   sync.Mutex
	gate                 *publicationGate
	failSet, errorCommit error
	afterCommit          bool
	writersWaiting       bool
	commits              int
	// readDelay slows each read of a write transaction, as a cold page does.
	readDelay time.Duration
	// longestWrite is the longest a write transaction stayed open.
	longestWrite time.Duration
}

// holdWrite records the time a write transaction stayed open.
func (s *publicationTestStore) holdWrite(opened time.Time) {
	s.mu.Lock()
	s.longestWrite = max(s.longestWrite, time.Since(opened))
	s.mu.Unlock()
}

func (s *publicationTestStore) SupportsAtomicCommit() bool { return true }

// WritersWaiting reports the injected writer queue, so a sweep transaction
// yields after one block.
func (s *publicationTestStore) WritersWaiting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writersWaiting
}
func (s *publicationTestStore) arm(t *testing.T) *publicationGate {
	// Arm the publication store gate and register its release.
	g := &publicationGate{started: make(chan struct{}), release: make(chan struct{})}
	s.mu.Lock()
	s.gate = g
	s.mu.Unlock()
	t.Cleanup(g.unblock)
	return g
}

func (s *publicationTestStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	// Capture the publication gate and injected write faults.
	var g *publicationGate
	var setErr, commitErr error
	var after bool
	var delay time.Duration
	if write {
		s.mu.Lock()
		g = s.gate
		s.gate = nil
		setErr = s.failSet
		commitErr = s.errorCommit
		after = s.afterCommit
		delay = s.readDelay
		s.mu.Unlock()
	}

	// Hold the physical transaction until the test releases its gate.
	if g != nil {
		close(g.started)
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Open the underlying transaction and attach write fault injection.
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write {
		return tx, nil
	}
	return &publicationTestTx{Tx: tx, store: s, failSet: setErr, errorCommit: commitErr, afterCommit: after, delay: delay, opened: time.Now()}, nil
}

type publicationTestTx struct {
	db_kvtx.Tx
	store                *publicationTestStore
	failSet, errorCommit error
	afterCommit          bool
	// delay slows each read, and opened times the transaction's hold.
	delay  time.Duration
	opened time.Time
	ended  sync.Once
}

// end records the transaction's hold once, on commit or discard.
func (t *publicationTestTx) end() {
	t.ended.Do(func() { t.store.holdWrite(t.opened) })
}

func (t *publicationTestTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	time.Sleep(t.delay)
	return t.Tx.Get(ctx, key)
}

func (t *publicationTestTx) Exists(ctx context.Context, key []byte) (bool, error) {
	time.Sleep(t.delay)
	return t.Tx.Exists(ctx, key)
}

func (t *publicationTestTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	time.Sleep(t.delay)
	return t.Tx.ScanPrefixKeys(ctx, prefix, cb)
}

func (t *publicationTestTx) Discard() {
	t.end()
	t.Tx.Discard()
}

func (t *publicationTestTx) Set(ctx context.Context, key, value []byte) error {
	if err := t.Tx.Set(ctx, key, value); err != nil {
		return err
	}
	if t.failSet != nil && strings.HasSuffix(string(key), "/head") {
		return t.failSet
	}
	return nil
}

func (t *publicationTestTx) Commit(ctx context.Context) error {
	// Inject failures before committing the publication transaction.
	if t.errorCommit != nil && !t.afterCommit {
		return t.errorCommit
	}
	err := t.Tx.Commit(ctx)
	t.end()
	if err != nil {
		return err
	}

	// Count committed physical transactions before returning the injected result.
	t.store.mu.Lock()
	t.store.commits++
	t.store.mu.Unlock()
	return t.errorCommit
}

func newPublicationTestVolume(t *testing.T) (*Volume, *publicationTestStore) {
	// Create an atomic publication Volume with transaction fault injection.
	t.Helper()
	s := &publicationTestStore{Store: store_kvtx_inmem.NewStore()}
	v, err := NewVolume(t.Context(), "test/publication", store_kvkey.NewDefaultKVKey(), s, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	return v, s
}

func publicationFor(t *testing.T, id, base, next string) *block.AtomicPublication {
	// Build publication bytes and their content reference for the requested head.
	t.Helper()
	data := []byte("block for " + id + "/" + next)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.AtomicPublication{
		Entries: []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}, BucketID: "owner", TrackGC: true,
		Heads: []*block.AtomicHeadUpdate{{ObjectStoreID: id, Key: []byte("head"), Replace: func(_ context.Context, current []byte, found bool) ([]byte, error) {
			if (base != "") != found || !bytes.Equal(current, []byte(base)) {
				return nil, coord.ErrStaleGeneration
			}
			return []byte(next), nil
		}}},
	}
}

// leafOpts returns put options declaring a block with no children.
func leafOpts() *block.PutOpts {
	return &block.PutOpts{RefsKnown: true}
}

func submitPublication(t *testing.T, v *Volume, p *block.AtomicPublication) *block.PublicationReceipt {
	t.Helper()
	r, err := v.SubmitAtomic(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func awaitPublication(t *testing.T, r *block.PublicationReceipt) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return r.Wait(ctx)
}

func assertPublishedBlock(t *testing.T, v *Volume, p *block.AtomicPublication, want bool) {
	// Verify the publication block presence and stored bytes.
	t.Helper()
	ref := p.Entries[0].Ref
	got, found, err := v.GetBlock(t.Context(), ref)
	if err != nil || found != want {
		t.Fatalf("block found=%v want=%v err=%v", found, want, err)
	}
	if want && !bytes.Equal(got, p.Entries[0].Data) {
		t.Fatal("block bytes changed")
	}

	// Verify the publication bucket owns exactly the expected block.
	owners, err := v.GetRefGraph().GetIncomingRefs(t.Context(), block_gc.BlockIRI(ref))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(owners, block_gc.BucketIRI(p.BucketID)) != want {
		t.Fatalf("owners=%v want=%v", owners, want)
	}
}

func assertReadable(t *testing.T, v *Volume, p *block.AtomicPublication, want bool) {
	// Read the publication block through the Volume and one of its read scopes.
	t.Helper()
	ref := p.Entries[0].Ref
	scope, release, err := v.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, store := range []block.StoreOps{v, scope} {
		got, found, err := store.GetBlock(t.Context(), ref)
		if err != nil || found != want || (want && !bytes.Equal(got, p.Entries[0].Data)) {
			t.Fatalf("read found=%v want=%v err=%v", found, want, err)
		}

		// Check existence, the batch probe and the size.
		exists, err := store.GetBlockExists(t.Context(), ref)
		if err != nil || exists != want {
			t.Fatalf("exists=%v want=%v err=%v", exists, want, err)
		}
		batch, err := store.GetBlockExistsBatch(t.Context(), []*block.BlockRef{ref})
		if err != nil || len(batch) != 1 || batch[0] != want {
			t.Fatalf("exists batch=%v want=%v err=%v", batch, want, err)
		}
		stat, err := store.StatBlock(t.Context(), ref)
		if err != nil || (stat != nil) != want || (want && stat.Size != int64(len(p.Entries[0].Data))) {
			t.Fatalf("stat=%v want=%v err=%v", stat, want, err)
		}
	}
}

func assertHead(t *testing.T, v *Volume, id, want string) {
	// Read the publication head through the physical store transaction.
	t.Helper()
	tx, err := v.kvtxStore.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	got, found, err := tx.Get(t.Context(), append(v.kvKey.GetObjectStorePrefixByID(id), []byte("head")...))
	if err != nil || found != (want != "") || string(got) != want {
		t.Fatalf("head %s: %q %v %v", id, got, found, err)
	}
}

func TestPublicationGroupAtomicBlocksOwnershipAndHead(t *testing.T) {
	// Create a gated publication group with a read-only validator.
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	first := publicationFor(t, "world", "", "1")
	first.Validate = func(ctx context.Context, store block.StoreOps) error {
		// Verify the validator sees the prepared block bytes.
		got, found, err := store.GetBlock(ctx, first.Entries[0].Ref)
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(got, first.Entries[0].Data) {
			return errors.New("prepared data missing")
		}

		// Verify validation cannot mutate the publication store.
		got[0] = '!'
		if _, _, err := store.PutBlock(ctx, []byte("forbidden"), &block.PutOpts{RefsKnown: true}); !errors.Is(err, errPublicationReadOnly) {
			return errors.New("validation allowed mutation")
		}
		return nil
	}

	// Queue dependent and independent publications behind the physical gate.
	r1 := submitPublication(t, v, first)
	<-g.started
	second := publicationFor(t, "world", "1", "2")
	second.After = r1
	r2 := submitPublication(t, v, second)
	third := publicationFor(t, "independent", "", "3")
	r3 := submitPublication(t, v, third)

	// Verify queue admission leaves the durable head unchanged and serves
	// the admitted blocks of the unapplied group and the queue.
	select {
	case <-r1.Done():
		t.Fatal("admission acknowledged durability")
	default:
	}
	assertHead(t, v, "world", "")
	for _, p := range []*block.AtomicPublication{first, second, third} {
		assertReadable(t, v, p, true)
	}

	// Release the group and await every publication receipt.
	g.unblock()
	for _, r := range []*block.PublicationReceipt{r1, r2, r3} {
		if err := awaitPublication(t, r); err != nil {
			t.Fatal(err)
		}
	}

	// Verify grouped heads and owned block bytes after commit.
	assertHead(t, v, "world", "2")
	assertHead(t, v, "independent", "3")
	for _, p := range []*block.AtomicPublication{first, second, third} {
		assertPublishedBlock(t, v, p, true)
	}

	// Verify one physical commit completes the group and drains its queue.
	stats := v.GetPublicationStats()
	if stats.PhysicalCommits != 1 || stats.Completed != 3 || stats.Pending != 0 || stats.PendingBytes != 0 {
		t.Fatalf("group stats: %+v", stats)
	}
}

func TestPublicationRejectsBeforeMutationWithoutContaminatingGroup(t *testing.T) {
	// Queue a stale publication behind the physical transaction gate.
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	stale := publicationFor(t, "stale", "missing", "bad")
	r1 := submitPublication(t, v, stale)
	<-g.started

	// Queue a dependent publication, and refuse a block with corrupt bytes at
	// admission.
	child := publicationFor(t, "child", "", "bad")
	child.After = r1
	r2 := submitPublication(t, v, child)
	invalid := publicationFor(t, "invalid", "", "bad")
	invalid.Entries[0].Data = []byte("incorrect hash")
	if _, err := v.SubmitAtomic(t.Context(), invalid); err == nil {
		t.Fatal("admitted corrupt block")
	}

	// Queue a rejected validator alongside an independent valid publication.
	rejected := publicationFor(t, "validation", "", "bad")
	validationErr := errors.New("candidate cannot be decoded")
	rejected.Validate = func(context.Context, block.StoreOps) error { return validationErr }
	r4 := submitPublication(t, v, rejected)
	valid := publicationFor(t, "valid", "", "good")
	r5 := submitPublication(t, v, valid)

	// Release the group and verify stale and dependent publications fail.
	g.unblock()
	if err := awaitPublication(t, r1); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r2); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatal(err)
	}

	// Verify the invalid publication fails while the valid one commits.
	if err := awaitPublication(t, r4); !errors.Is(err, validationErr) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r5); err != nil {
		t.Fatal(err)
	}

	// Verify rejected publications leave no blocks or heads.
	for _, p := range []*block.AtomicPublication{stale, child, invalid, rejected} {
		assertPublishedBlock(t, v, p, false)
		assertHead(t, v, p.Heads[0].ObjectStoreID, "")
	}

	// Verify the independent publication survives rejection and synchronization.
	assertPublishedBlock(t, v, valid, true)
	assertHead(t, v, "valid", "good")
	if _, err := v.Sync(t.Context()); err != nil {
		t.Fatalf("isolated rejection poisoned unrelated fence: %v", err)
	}
	if stats := v.GetPublicationStats(); stats.PhysicalCommits != 1 || stats.Rejected != 3 {
		t.Fatalf("stats: %+v", stats)
	}
}

func TestPublicationPhysicalFailureRollsBackWholeGroup(t *testing.T) {
	// Exercise physical set and commit failures for publication groups.
	for _, fault := range []string{"set", "commit"} {
		t.Run(fault, func(t *testing.T) {
			// Prepare the publication Volume for the selected physical failure.
			v, s := newPublicationTestVolume(t)
			g := s.arm(t)
			failure := errors.New("injected physical failure")

			// Inject the selected failure into the physical transaction.
			s.mu.Lock()
			if fault == "set" {
				s.failSet = failure
			} else {
				s.errorCommit = failure
			}
			s.mu.Unlock()

			// Queue two dependent publications behind the failing transaction.
			first := publicationFor(t, "one", "", "1")
			r1 := submitPublication(t, v, first)
			<-g.started
			second := publicationFor(t, "two", "", "2")
			second.After = r1
			r2 := submitPublication(t, v, second)

			// Release the transaction and verify both receipts report failure.
			// The successor joins the group on a commit failure and follows in
			// the next group on a set failure; it fails through its
			// predecessor in both.
			g.unblock()
			if err := awaitPublication(t, r1); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if err := awaitPublication(t, r2); !errors.Is(err, block.ErrPublicationDependency) || !errors.Is(err, failure) {
				t.Fatal(err)
			}

			// Verify the failed group leaves neither blocks nor heads.
			assertPublishedBlock(t, v, first, false)
			assertPublishedBlock(t, v, second, false)
			assertHead(t, v, "one", "")
			assertHead(t, v, "two", "")
		})
	}
}

func TestPublicationAdmittedBlocksLeaveWithFailedGroup(t *testing.T) {
	// Fail the physical commit of a gated group.
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	failure := errors.New("injected commit failure")
	s.mu.Lock()
	s.errorCommit = failure
	s.mu.Unlock()

	// Admit a publication to the group and its successor to the queue.
	first := publicationFor(t, "world", "", "1")
	r1 := submitPublication(t, v, first)
	<-g.started
	second := publicationFor(t, "world", "1", "2")
	second.After = r1
	r2 := submitPublication(t, v, second)

	// Verify a reader sees both admitted blocks before the group applies.
	assertReadable(t, v, first, true)
	assertReadable(t, v, second, true)
	assertHead(t, v, "world", "")

	// Release the group and verify the successor fails through its predecessor.
	g.unblock()
	if err := awaitPublication(t, r1); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r2); !errors.Is(err, block.ErrPublicationDependency) || !errors.Is(err, failure) {
		t.Fatal(err)
	}

	// Verify a successor admitted after the failure is unreadable from
	// admission and fails in its own group.
	g = s.arm(t)
	third := publicationFor(t, "world", "2", "3")
	third.After = r2
	r3 := submitPublication(t, v, third)
	<-g.started
	assertReadable(t, v, third, false)
	g.unblock()
	if err := awaitPublication(t, r3); !errors.Is(err, block.ErrPublicationDependency) || !errors.Is(err, failure) {
		t.Fatal(err)
	}

	// Verify the failed groups leave no readable block and no head.
	for _, p := range []*block.AtomicPublication{first, second, third} {
		assertReadable(t, v, p, false)
		assertPublishedBlock(t, v, p, false)
	}
	assertHead(t, v, "world", "")
	if stats := v.GetPublicationStats(); stats.Pending != 0 || stats.Rejected != 3 {
		t.Fatalf("stats: %+v", stats)
	}
}

func TestPublicationCancelledWaitDoesNotCancelAcceptedCommit(t *testing.T) {
	// Submit a gated publication with a cancelable caller context.
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	ctx, cancel := context.WithCancel(t.Context())
	p := publicationFor(t, "world", "", "done")
	r, err := v.SubmitAtomic(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	// Cancel the caller wait after publication admission.
	<-g.started
	cancel()
	if err := r.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	// Release the transaction and verify the admitted publication commits.
	g.unblock()
	if err := awaitPublication(t, r); err != nil {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "done")

	// Verify an already canceled caller cannot publish a new head.
	ctx2, cancel2 := context.WithCancel(t.Context())
	cancel2()
	if _, err := v.SubmitAtomic(ctx2, publicationFor(t, "cancelled", "", "never")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertHead(t, v, "cancelled", "")
}

func TestPublicationBoundedAdmissionFenceAndJoinedClose(t *testing.T) {
	// Fill the bounded publication queue behind a physical transaction gate.
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	receipts := []*block.PublicationReceipt{submitPublication(t, v, publicationFor(t, "0", "", "done"))}
	<-g.started
	for i := 1; i < publicationMaxPending; i++ {
		receipts = append(receipts, submitPublication(t, v, publicationFor(t, fmt.Sprint(i), "", "done")))
	}

	// Verify the full publication queue blocks admission until cancellation.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := v.SubmitAtomic(ctx, publicationFor(t, "overflow", "", "no")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unbounded admission: %v", err)
	}

	// Start a durability fence and close while the publication group is blocked.
	synced := make(chan error, 1)
	go func() { _, err := v.Sync(t.Context()); synced <- err }()
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()

	// Verify close joins accepted work and rejects further publications.
	select {
	case err := <-closed:
		t.Fatalf("close did not join: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if _, err := v.SubmitAtomic(t.Context(), publicationFor(t, "closed", "", "no")); !errors.Is(err, block.ErrPublicationClosed) {
		t.Fatal(err)
	}

	// Release accepted publications and await their completion.
	g.unblock()
	for _, r := range receipts {
		if err := awaitPublication(t, r); err != nil {
			t.Fatal(err)
		}
	}

	// Verify close completes after draining the publication group.
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close stuck")
	}

	// Verify the durability fence completes after draining the group.
	select {
	case err := <-synced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fence stuck")
	}

	// Verify the publication queue releases all occupancy.
	if stats := v.GetPublicationStats(); stats.Pending != 0 || stats.PendingBytes != 0 || stats.PendingEntries != 0 {
		t.Fatalf("leaked queue: %+v", stats)
	}
}

func TestPublicationReportsUncertainCommitWithoutLosingDurableState(t *testing.T) {
	// Inject a lost acknowledgement after a durable physical commit.
	v, s := newPublicationTestVolume(t)
	failure := errors.New("ack lost after durable commit")
	s.mu.Lock()
	s.errorCommit = failure
	s.afterCommit = true
	s.mu.Unlock()

	// Publish a head and verify the receipt reports uncertainty.
	p := publicationFor(t, "world", "", "durable")
	r := submitPublication(t, v, p)
	if err := awaitPublication(t, r); !errors.Is(err, failure) {
		t.Fatal(err)
	}

	// Verify the uncertain publication retained its durable head and block.
	assertHead(t, v, "world", "durable")
	assertPublishedBlock(t, v, p, true)

	// Clear the physical fault and reject dependency on an uncertain receipt.
	s.mu.Lock()
	s.errorCommit = nil
	s.mu.Unlock()
	child := publicationFor(t, "world", "durable", "child")
	child.After = r
	if err := awaitPublication(t, submitPublication(t, v, child)); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatal(err)
	}

	// Verify the uncertain dependency leaves the durable head unchanged.
	assertHead(t, v, "world", "durable")

	// A newly acquired/reconciled writer can use the observed durable head.
	child.After = nil
	if err := awaitPublication(t, submitPublication(t, v, child)); err != nil {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "child")
}

func TestPublicationUnsupportedStoreDoesNotManufactureDurability(t *testing.T) {
	// Open a Volume whose store hides atomic commit support.
	inner := store_kvtx_inmem.NewStore()
	hidden := struct{ db_kvtx.Store }{inner}
	v, err := NewVolume(t.Context(), "unsupported", store_kvkey.NewDefaultKVKey(), hidden, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Verify the Volume rejects unadvertised atomic publication guarantees.
	if v.SupportsAtomicPublication() {
		t.Fatal("inferred unadvertised transaction semantics")
	}
	if _, err := v.SubmitAtomic(t.Context(), nil); !errors.Is(err, block.ErrAtomicPublicationUnsupported) {
		t.Fatal(err)
	}
}

func assertScopeReads(t *testing.T, scope block.StoreOps, p *block.AtomicPublication, want bool) {
	// Read the publication block through an already open read scope.
	t.Helper()
	got, found, err := scope.GetBlock(t.Context(), p.Entries[0].Ref)
	if err != nil || found != want || (want && !bytes.Equal(got, p.Entries[0].Data)) {
		t.Fatalf("scope read found=%v want=%v err=%v", found, want, err)
	}
}

func TestPublicationReadScopeKeepsAdmittedBlocksUntilItsGroupFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
	}{
		{name: "applied"},
		{name: "failed", failure: errors.New("injected commit failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Gate the next commit and inject the case's commit result.
			v, s := newPublicationTestVolume(t)
			v.Store = publicationSnapshotStore{Store: v.Store}
			g := s.arm(t)
			s.mu.Lock()
			s.errorCommit = tc.failure
			s.mu.Unlock()

			// Admit a gated publication and open a read scope over an empty snapshot.
			first := publicationFor(t, "world", "", "1")
			r1 := submitPublication(t, v, first)
			<-g.started
			scope, release, err := v.BeginReadOperation(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			assertScopeReads(t, scope, first, true)

			// Resolve the group. A block that applied is absent from the scope's
			// snapshot, so the scope keeps serving it; a failed one leaves the scope.
			g.unblock()
			if err := awaitPublication(t, r1); !errors.Is(err, tc.failure) {
				t.Fatal(err)
			}
			assertScopeReads(t, scope, first, tc.failure == nil)
		})
	}
}

func TestPublicationRejectedRequestsLeaveReadsBeforeTheirGroupResolves(t *testing.T) {
	// Gate a group whose first request is rejected as stale.
	v, s := newPublicationTestVolume(t)
	v.Store = publicationSnapshotStore{Store: v.Store}
	g := s.arm(t)
	rejected := publicationFor(t, "world", "stale", "1")
	r1 := submitPublication(t, v, rejected)
	<-g.started

	// Queue a dependent of the rejected request and a request that blocks in
	// validation, holding the group open after the first two are decided.
	dependent := publicationFor(t, "other", "", "2")
	dependent.After = r1
	r2 := submitPublication(t, v, dependent)

	// Hold the validating request until the test releases it.
	validating, releaseValidate := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseValidate:
		default:
			close(releaseValidate)
		}
	})
	slow := publicationFor(t, "third", "", "3")
	slow.Validate = func(context.Context, block.StoreOps) error {
		close(validating)
		<-releaseValidate
		return nil
	}

	// Submit it, and check that all three read before the group resolves.
	r3 := submitPublication(t, v, slow)
	for _, p := range []*block.AtomicPublication{rejected, dependent, slow} {
		assertReadable(t, v, p, true)
	}

	// Verify the rejected request and its dependent leave the read set while
	// the group they share is still applying.
	g.unblock()
	<-validating
	assertReadable(t, v, rejected, false)
	assertReadable(t, v, dependent, false)
	assertReadable(t, v, slow, true)

	// Release the group and verify each request resolves as it read.
	close(releaseValidate)
	if err := awaitPublication(t, r1); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r2); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r3); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationAdmissionRejectsUnverifiedEntries(t *testing.T) {
	// Submit an entry whose bytes do not match its reference.
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "world", "", "1")
	p.Entries[0].Data = []byte("tampered")

	// Verify admission rejects it and no reader sees its bytes.
	if _, err := v.SubmitAtomic(t.Context(), p); err == nil {
		t.Fatal("unverified entry admitted")
	}
	assertReadable(t, v, p, false)
}

func TestPublicationStoredBlockReadsThroughTheStore(t *testing.T) {
	// Read a block no publication writes from a store that reports known refs.
	v, _ := newPublicationTestVolume(t)
	v.Store = publicationSnapshotStore{Store: v.Store}
	p := publicationFor(t, "world", "", "1")

	// Verify the Volume serves the store's stored block with its refs.
	stored, err := v.GetStoredBlock(t.Context(), p.Entries[0].Ref)
	if err != nil || stored == nil || !stored.RefsKnown {
		t.Fatalf("stored=%v err=%v", stored, err)
	}
}
