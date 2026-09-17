//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

type sessionGate struct {
	entered, release chan struct{}
	once             sync.Once
	err              error
}

func (g *sessionGate) open() { g.once.Do(func() { close(g.release) }) }

type sessionPublisher struct {
	block.AtomicPublisher
	mu          sync.Mutex
	gate        *sessionGate
	unsupported bool
}

func (p *sessionPublisher) arm(t *testing.T, err error) *sessionGate {
	t.Helper()
	g := &sessionGate{entered: make(chan struct{}), release: make(chan struct{}), err: err}
	p.mu.Lock()
	p.gate = g
	p.mu.Unlock()
	t.Cleanup(g.open)
	return g
}
func (p *sessionPublisher) SubmitAtomic(ctx context.Context, publication *block.AtomicPublication) (*block.PublicationReceipt, error) {
	p.mu.Lock()
	g, unsupported := p.gate, p.unsupported
	p.gate = nil
	p.mu.Unlock()
	if unsupported {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	if g != nil {
		copy := *publication
		validate := publication.Validate
		copy.Validate = func(ctx context.Context, store block.StoreOps) error {
			close(g.entered)
			select {
			case <-g.release:
			case <-ctx.Done():
				return ctx.Err()
			}
			if g.err != nil {
				return g.err
			}
			if validate != nil {
				return validate(ctx, store)
			}
			return nil
		}
		publication = &copy
	}
	return p.AtomicPublisher.SubmitAtomic(ctx, publication)
}

type sessionFixture struct {
	engine    *Engine
	publisher *sessionPublisher
	db        *bdb.DB
	load      func(context.Context) (*bucket.ObjectRef, error)
	legacy    atomic.Int64
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.WithVolumeConfig(&volume_bolt.Config{
		Path: filepath.Join(t.TempDir(), "session.bolt"), VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	publisher, ok := cursor.GetBucket().(block.AtomicPublisher)
	if !ok || !publisher.SupportsAtomicPublication() {
		t.Fatal("native synced bucket must support publication")
	}
	store, rel, err := tb.Volume.AccessObjectStore(ctx, "session-head", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)
	f := &sessionFixture{publisher: &sessionPublisher{AtomicPublisher: publisher}, db: volume_bolt.GetBoltDB(tb.Volume)}
	if f.db == nil || f.db.NoSync || f.db.NoFreelistSync {
		t.Fatal("test requires durable native Bolt")
	}
	f.load = func(ctx context.Context) (*bucket.ObjectRef, error) {
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			return nil, err
		}
		defer tx.Discard()
		data, found, err := tx.Get(ctx, []byte("head"))
		if err != nil || !found {
			return nil, err
		}
		r := &bucket.ObjectRef{}
		if err := r.UnmarshalVT(data); err != nil {
			return nil, err
		}
		return r, nil
	}
	head := func(base, next *bucket.ObjectRef) *block.AtomicHeadUpdate {
		base, next = base.Clone(), next.Clone()
		return &block.AtomicHeadUpdate{ObjectStoreID: "session-head", Key: []byte("head"), Replace: func(_ context.Context, data []byte, found bool) ([]byte, error) {
			if found {
				old := &bucket.ObjectRef{}
				if err := old.UnmarshalVT(data); err != nil {
					return nil, err
				}
				if !old.EqualsRef(base) {
					return nil, coord.ErrStaleGeneration
				}
			} else if !base.GetRootRef().GetEmpty() {
				return nil, coord.ErrStaleGeneration
			}
			return next.MarshalVT()
		}}
	}
	legacy := func(ctx context.Context, base, next *bucket.ObjectRef) error {
		f.legacy.Add(1)
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		data, found, err := tx.Get(ctx, []byte("head"))
		if err != nil {
			return err
		}
		data, err = head(base, next).Replace(ctx, data, found)
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, []byte("head"), data); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	f.engine, err = NewEngine(ctx, logrus.NewEntry(log), cursor, world_mock.LookupMockOp, legacy, false,
		WithWriteCoordinator(tb.Volume, coord.Scope{VolumeID: tb.Volume.GetID(), ObjectStoreID: "session-head", ParticipantID: "session-producer"}, []byte("head"), f.load),
		WithAtomicPublication(f.publisher, head))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.engine.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func sessionWriter(t *testing.T, f *sessionFixture) *EngineTx {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	w, err := f.engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Discard)
	return w
}
func sessionObject(t *testing.T, w *EngineTx, key string) (*bucket_lookup.Cursor, *bucket.ObjectRef) {
	t.Helper()
	c, err := w.BuildStorageCursor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Release)
	btx, bcs := c.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample("payload "+key), true)
	ref, _, err := btx.Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	next := c.GetRef().Clone()
	next.RootRef = ref
	obj, err := w.CreateObject(t.Context(), key, next)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	return c, next
}
func sessionSubmit(t *testing.T, w *EngineTx) world.CommitReceipt {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	r, err := w.Submit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func sessionWait(t *testing.T, r world.CommitReceipt) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return r.Wait(ctx)
}
func sessionHas(t *testing.T, w world.WorldState, key string, want bool) {
	t.Helper()
	got, err := w.HasObject(t.Context(), key)
	if err != nil || got != want {
		t.Fatalf("HasObject(%s)=%v,%v want %v", key, got, err, want)
	}
}
func sessionEntered(t *testing.T, g *sessionGate) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("publication never reached physical preflight")
	}
}

func TestEngineSessionRunAheadGroupsWithoutPublishingPrivateHead(t *testing.T) {
	f := newSessionFixture(t)
	initial := f.engine.GetRootRef()
	g := f.publisher.arm(t, nil)
	before := f.db.CommitCounter()
	first := sessionWriter(t, f)
	sessionObject(t, first, "one")
	r1 := sessionSubmit(t, first)
	sessionEntered(t, g)
	second := sessionWriter(t, f)
	sessionHas(t, second, "one", true)
	sessionObject(t, second, "two")
	if err := second.SetGraphQuad(t.Context(), world.NewGraphQuadWithKeys("one", "next", "two", "")); err != nil {
		t.Fatal(err)
	}
	r2 := sessionSubmit(t, second)
	for _, r := range []world.CommitReceipt{r1, r2} {
		select {
		case <-r.Done():
			t.Fatal("admission acknowledged undurable publication")
		default:
		}
	}
	if !f.engine.GetRootRef().EqualsRef(initial) {
		t.Fatal("private head became canonical before durability")
	}
	if head, err := f.load(t.Context()); err != nil || head != nil {
		t.Fatalf("undurable metadata %v %v", head, err)
	}
	reader, err := f.engine.NewBlockEngineTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	sessionHas(t, reader, "one", false)
	sessionHas(t, reader, "two", false)
	g.open()
	if err := sessionWait(t, r1); err != nil {
		t.Fatal(err)
	}
	if err := sessionWait(t, r2); err != nil {
		t.Fatal(err)
	}
	if n := f.db.CommitCounter() - before; n != 1 {
		t.Fatalf("two revisions used %d physical commits, want 1", n)
	}
	reader2, err := f.engine.NewBlockEngineTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader2.Discard()
	sessionHas(t, reader2, "one", true)
	sessionHas(t, reader2, "two", true)
	qs, err := reader2.LookupGraphQuads(t.Context(), world.NewGraphQuadWithKeys("one", "next", "two", ""), 0)
	if err != nil || len(qs) != 1 {
		t.Fatalf("relationship lost: %v %v", qs, err)
	}
}

func TestEngineSessionCompletionPreservesActiveSuccessor(t *testing.T) {
	f := newSessionFixture(t)
	g := f.publisher.arm(t, nil)
	first := sessionWriter(t, f)
	sessionObject(t, first, "one")
	r := sessionSubmit(t, first)
	sessionEntered(t, g)
	second := sessionWriter(t, f)
	sessionObject(t, second, "two")
	g.open()
	if err := sessionWait(t, r); err != nil {
		t.Fatal(err)
	}
	sessionHas(t, second, "one", true)
	sessionHas(t, second, "two", true)
	if err := second.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEngineSessionFailureInvalidatesDescendantsAndRetainsCursorForRetry(t *testing.T) {
	f := newSessionFixture(t)
	failure := errors.New("reject prepared candidate")
	g := f.publisher.arm(t, failure)
	first := sessionWriter(t, f)
	cursor, body := sessionObject(t, first, "one")
	r1 := sessionSubmit(t, first)
	sessionEntered(t, g)
	second := sessionWriter(t, f)
	sessionObject(t, second, "two")
	r2 := sessionSubmit(t, second)
	third := sessionWriter(t, f)
	sessionObject(t, third, "three")
	g.open()
	if err := sessionWait(t, r1); !errors.Is(err, failure) {
		t.Fatalf("first result: %v", err)
	}
	if err := sessionWait(t, r2); !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatalf("dependent result: %v", err)
	}
	if _, err := third.HasObject(t.Context(), "three"); !errors.Is(err, tx.ErrDiscarded) {
		t.Fatalf("active dependent survived: %v", err)
	}
	if head, err := f.load(t.Context()); err != nil || head != nil {
		t.Fatalf("rejected head persisted: %v %v", head, err)
	}
	if f.legacy.Load() != 0 {
		t.Fatal("failure retried through legacy publication")
	}
	retained, err := cursor.FollowRef(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	_, bcs := retained.BuildTransaction(nil)
	got, err := block_mock.UnmarshalExample(t.Context(), bcs)
	if err != nil || got.GetMsg() != "payload one" {
		t.Fatalf("retained cursor lost bytes: %v %v", got, err)
	}
	retry := sessionWriter(t, f)
	obj, err := retry.CreateObject(t.Context(), "retry", body)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEngineSessionCommitCancellationDoesNotAbandonAcceptedWrite(t *testing.T) {
	f := newSessionFixture(t)
	g := f.publisher.arm(t, nil)
	writer := sessionWriter(t, f)
	sessionObject(t, writer, "one")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- writer.Commit(ctx) }()
	sessionEntered(t, g)
	cancel()
	select {
	case err := <-done:
		t.Fatalf("Commit returned before durable result: %v", err)
	default:
	}
	second := sessionWriter(t, f)
	sessionHas(t, second, "one", true)
	second.Discard()
	g.open()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Commit did not finish")
	}
}

func TestEngineSessionCloseJoinsAcceptedPublication(t *testing.T) {
	f := newSessionFixture(t)
	g := f.publisher.arm(t, nil)
	writer := sessionWriter(t, f)
	sessionObject(t, writer, "one")
	r := sessionSubmit(t, writer)
	sessionEntered(t, g)
	successor := sessionWriter(t, f)
	sessionObject(t, successor, "two")
	done := make(chan error, 2)
	go func() { done <- f.engine.Close() }()
	go func() { done <- f.engine.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := r.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait cancellation: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("Close returned before receipt: %v", err)
	default:
	}
	g.open()
	if err := sessionWait(t, r); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Close failed to join")
		}
	}
	if head, err := f.load(t.Context()); err != nil || head == nil {
		t.Fatalf("accepted write was lost at Close: %v %v", head, err)
	}
	if _, err := f.engine.NewBlockEngineTransaction(t.Context(), true); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("closed writer admission: %v", err)
	}
}

func TestEngineSessionBoundedAdmissionAndSyncFence(t *testing.T) {
	f := newSessionFixture(t)
	g := f.publisher.arm(t, nil)
	var receipts []world.CommitReceipt
	for i := 0; i < maxEnginePublications; i++ {
		w := sessionWriter(t, f)
		sessionObject(t, w, fmt.Sprintf("object/%02d", i))
		receipts = append(receipts, sessionSubmit(t, w))
		if i == 0 {
			sessionEntered(t, g)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	w, err := f.engine.NewBlockEngineTransaction(ctx, true)
	cancel()
	if w != nil {
		w.Discard()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded admission: %v", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	_, err = f.engine.Sync(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Sync crossed undurable fence: %v", err)
	}
	g.open()
	for _, r := range receipts {
		if err := sessionWait(t, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.engine.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	locked := f.engine.bcast.Lock()
	pending := f.engine.writeSession
	locked.Unlock()
	if pending != nil {
		t.Fatal("completed queue retained its session")
	}
}

func TestEngineSessionOnlyUnsupportedFallsBack(t *testing.T) {
	f := newSessionFixture(t)
	f.publisher.mu.Lock()
	f.publisher.unsupported = true
	f.publisher.mu.Unlock()
	w := sessionWriter(t, f)
	sessionObject(t, w, "legacy")
	if err := w.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.legacy.Load() != 1 {
		t.Fatal("unsupported capability did not use legacy CAS")
	}
	reader, err := f.engine.NewBlockEngineTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	sessionHas(t, reader, "legacy", true)
}
