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
	commits              int
}

func (s *publicationTestStore) SupportsAtomicCommit() bool { return true }
func (s *publicationTestStore) arm(t *testing.T) *publicationGate {
	g := &publicationGate{started: make(chan struct{}), release: make(chan struct{})}
	s.mu.Lock()
	s.gate = g
	s.mu.Unlock()
	t.Cleanup(g.unblock)
	return g
}

func (s *publicationTestStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	var g *publicationGate
	var setErr, commitErr error
	var after bool
	if write {
		s.mu.Lock()
		g = s.gate
		s.gate = nil
		setErr = s.failSet
		commitErr = s.errorCommit
		after = s.afterCommit
		s.mu.Unlock()
	}
	if g != nil {
		close(g.started)
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write {
		return tx, nil
	}
	return &publicationTestTx{Tx: tx, store: s, failSet: setErr, errorCommit: commitErr, afterCommit: after}, nil
}

type publicationTestTx struct {
	db_kvtx.Tx
	store                *publicationTestStore
	failSet, errorCommit error
	afterCommit          bool
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
	if t.errorCommit != nil && !t.afterCommit {
		return t.errorCommit
	}
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	t.store.mu.Lock()
	t.store.commits++
	t.store.mu.Unlock()
	return t.errorCommit
}

func newPublicationTestVolume(t *testing.T) (*Volume, *publicationTestStore) {
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
	t.Helper()
	data := []byte("block for " + id + "/" + next)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.AtomicPublication{
		Entries: []*block.PutBatchEntry{{Ref: ref, Data: data}}, BucketID: "owner", TrackGC: true,
		Head: &block.AtomicHeadUpdate{ObjectStoreID: id, Key: []byte("head"), Replace: func(_ context.Context, current []byte, found bool) ([]byte, error) {
			if (base != "") != found || !bytes.Equal(current, []byte(base)) {
				return nil, coord.ErrStaleGeneration
			}
			return []byte(next), nil
		}},
	}
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
	t.Helper()
	ref := p.Entries[0].Ref
	got, found, err := v.GetBlock(t.Context(), ref)
	if err != nil || found != want {
		t.Fatalf("block found=%v want=%v err=%v", found, want, err)
	}
	if want && !bytes.Equal(got, p.Entries[0].Data) {
		t.Fatal("block bytes changed")
	}
	owners, err := v.GetRefGraph().GetIncomingRefs(t.Context(), block_gc.BlockIRI(ref))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(owners, block_gc.BucketIRI(p.BucketID)) != want {
		t.Fatalf("owners=%v want=%v", owners, want)
	}
}

func assertHead(t *testing.T, v *Volume, id, want string) {
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
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	first := publicationFor(t, "world", "", "1")
	first.Validate = func(ctx context.Context, store block.StoreOps) error {
		got, found, err := store.GetBlock(ctx, first.Entries[0].Ref)
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(got, first.Entries[0].Data) {
			return errors.New("prepared data missing")
		}
		got[0] = '!'
		if _, _, err := store.PutBlock(ctx, []byte("forbidden"), nil); !errors.Is(err, errPublicationReadOnly) {
			return errors.New("validation allowed mutation")
		}
		return nil
	}
	r1 := submitPublication(t, v, first)
	<-g.started
	second := publicationFor(t, "world", "1", "2")
	second.After = r1
	r2 := submitPublication(t, v, second)
	third := publicationFor(t, "independent", "", "3")
	r3 := submitPublication(t, v, third)
	select {
	case <-r1.Done():
		t.Fatal("admission acknowledged durability")
	default:
	}
	assertHead(t, v, "world", "")
	g.unblock()
	for _, r := range []*block.PublicationReceipt{r1, r2, r3} {
		if err := awaitPublication(t, r); err != nil {
			t.Fatal(err)
		}
	}
	assertHead(t, v, "world", "2")
	assertHead(t, v, "independent", "3")
	for _, p := range []*block.AtomicPublication{first, second, third} {
		assertPublishedBlock(t, v, p, true)
	}
	stats := v.GetPublicationStats()
	if stats.PhysicalCommits != 1 || stats.Completed != 3 || stats.Pending != 0 || stats.PendingBytes != 0 {
		t.Fatalf("group stats: %+v", stats)
	}
}

func TestPublicationRejectsBeforeMutationWithoutContaminatingGroup(t *testing.T) {
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	stale := publicationFor(t, "stale", "missing", "bad")
	r1 := submitPublication(t, v, stale)
	<-g.started
	child := publicationFor(t, "child", "", "bad")
	child.After = r1
	r2 := submitPublication(t, v, child)
	invalid := publicationFor(t, "invalid", "", "bad")
	invalid.Entries[0].Data = []byte("incorrect hash")
	r3 := submitPublication(t, v, invalid)
	rejected := publicationFor(t, "validation", "", "bad")
	validationErr := errors.New("candidate cannot be decoded")
	rejected.Validate = func(context.Context, block.StoreOps) error { return validationErr }
	r4 := submitPublication(t, v, rejected)
	valid := publicationFor(t, "valid", "", "good")
	r5 := submitPublication(t, v, valid)
	g.unblock()
	if err := awaitPublication(t, r1); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r2); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r3); err == nil {
		t.Fatal("accepted corrupt block")
	}
	if err := awaitPublication(t, r4); !errors.Is(err, validationErr) {
		t.Fatal(err)
	}
	if err := awaitPublication(t, r5); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*block.AtomicPublication{stale, child, invalid, rejected} {
		assertPublishedBlock(t, v, p, false)
		assertHead(t, v, p.Head.ObjectStoreID, "")
	}
	assertPublishedBlock(t, v, valid, true)
	assertHead(t, v, "valid", "good")
	if _, err := v.Sync(t.Context()); err != nil {
		t.Fatalf("isolated rejection poisoned unrelated fence: %v", err)
	}
	if stats := v.GetPublicationStats(); stats.PhysicalCommits != 1 || stats.Rejected != 4 {
		t.Fatalf("stats: %+v", stats)
	}
}

func TestPublicationPhysicalFailureRollsBackWholeGroup(t *testing.T) {
	for _, fault := range []string{"set", "commit"} {
		t.Run(fault, func(t *testing.T) {
			v, s := newPublicationTestVolume(t)
			g := s.arm(t)
			failure := errors.New("injected physical failure")
			s.mu.Lock()
			if fault == "set" {
				s.failSet = failure
			} else {
				s.errorCommit = failure
			}
			s.mu.Unlock()
			first := publicationFor(t, "one", "", "1")
			r1 := submitPublication(t, v, first)
			<-g.started
			second := publicationFor(t, "two", "", "2")
			second.After = r1
			r2 := submitPublication(t, v, second)
			g.unblock()
			if err := awaitPublication(t, r1); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if err := awaitPublication(t, r2); err == nil {
				t.Fatal("dependent publication survived physical failure")
			}
			assertPublishedBlock(t, v, first, false)
			assertPublishedBlock(t, v, second, false)
			assertHead(t, v, "one", "")
			assertHead(t, v, "two", "")
		})
	}
}

func TestPublicationCancelledWaitDoesNotCancelAcceptedCommit(t *testing.T) {
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	ctx, cancel := context.WithCancel(t.Context())
	p := publicationFor(t, "world", "", "done")
	r, err := v.SubmitAtomic(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	<-g.started
	cancel()
	if err := r.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	g.unblock()
	if err := awaitPublication(t, r); err != nil {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "done")
	ctx2, cancel2 := context.WithCancel(t.Context())
	cancel2()
	if _, err := v.SubmitAtomic(ctx2, publicationFor(t, "cancelled", "", "never")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertHead(t, v, "cancelled", "")
}

func TestPublicationBoundedAdmissionFenceAndJoinedClose(t *testing.T) {
	v, s := newPublicationTestVolume(t)
	g := s.arm(t)
	receipts := []*block.PublicationReceipt{submitPublication(t, v, publicationFor(t, "0", "", "done"))}
	<-g.started
	for i := 1; i < publicationMaxPending; i++ {
		receipts = append(receipts, submitPublication(t, v, publicationFor(t, fmt.Sprint(i), "", "done")))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := v.SubmitAtomic(ctx, publicationFor(t, "overflow", "", "no")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unbounded admission: %v", err)
	}
	synced := make(chan error, 1)
	go func() { _, err := v.Sync(t.Context()); synced <- err }()
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("close did not join: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if _, err := v.SubmitAtomic(t.Context(), publicationFor(t, "closed", "", "no")); !errors.Is(err, block.ErrPublicationClosed) {
		t.Fatal(err)
	}
	g.unblock()
	for _, r := range receipts {
		if err := awaitPublication(t, r); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close stuck")
	}
	select {
	case err := <-synced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fence stuck")
	}
	if stats := v.GetPublicationStats(); stats.Pending != 0 || stats.PendingBytes != 0 || stats.PendingEntries != 0 {
		t.Fatalf("leaked queue: %+v", stats)
	}
}

func TestPublicationReportsUncertainCommitWithoutLosingDurableState(t *testing.T) {
	v, s := newPublicationTestVolume(t)
	failure := errors.New("ack lost after durable commit")
	s.mu.Lock()
	s.errorCommit = failure
	s.afterCommit = true
	s.mu.Unlock()
	p := publicationFor(t, "world", "", "durable")
	r := submitPublication(t, v, p)
	if err := awaitPublication(t, r); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "durable")
	assertPublishedBlock(t, v, p, true)
	s.mu.Lock()
	s.errorCommit = nil
	s.mu.Unlock()
	child := publicationFor(t, "world", "durable", "child")
	child.After = r
	if err := awaitPublication(t, submitPublication(t, v, child)); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "durable")
	// A newly acquired/reconciled writer can use the observed durable head.
	child.After = nil
	if err := awaitPublication(t, submitPublication(t, v, child)); err != nil {
		t.Fatal(err)
	}
	assertHead(t, v, "world", "child")
}

func TestPublicationUnsupportedStoreDoesNotManufactureDurability(t *testing.T) {
	inner := store_kvtx_inmem.NewStore()
	hidden := struct{ db_kvtx.Store }{inner}
	v, err := NewVolume(t.Context(), "unsupported", store_kvkey.NewDefaultKVKey(), hidden, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if v.SupportsAtomicPublication() {
		t.Fatal("inferred unadvertised transaction semantics")
	}
	if _, err := v.SubmitAtomic(t.Context(), nil); !errors.Is(err, block.ErrAtomicPublicationUnsupported) {
		t.Fatal(err)
	}
}
