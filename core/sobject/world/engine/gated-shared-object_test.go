//go:build !js

package sobject_world_engine_test

import (
	"context"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/bldr/storage"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	bstore "github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// gatedSharedObject is a local SharedObject that can hold the physical
// commit of the next publication a World write admits, then apply or fail it.
type gatedSharedObject struct {
	*provider_local.SharedObject
	// ctrl is the World engine controller serving the SharedObject.
	ctrl *sobject_world_engine.Controller
	// stopServe stops the engine serve started.
	stopServe func()

	// mtx guards the fields below.
	mtx sync.Mutex
	// next wraps the state head of the next admitted publication, or is nil.
	next func(head *block.AtomicHeadUpdate) *block.AtomicHeadUpdate
	// held is closed to resolve the receipts holdReceipts holds, or is nil
	// when it holds none.
	held chan struct{}
	// last is the last receipt holdReceipts held.
	last *block.PublicationReceipt
	// committed maps each held receipt to the store's.
	committed map[*block.PublicationReceipt]*block.PublicationReceipt
}

// startGatedWorld starts a local provider on s4db storage under dir and a
// World engine serving the SharedObject soRef through a gatedSharedObject,
// creating the SharedObject when soRef is nil. Test cleanup stops them.
func startGatedWorld(t *testing.T, dir string, soRef *sobject.SharedObjectRef) (*localWorld, *gatedSharedObject) {
	t.Helper()
	return startGatedWorldConf(t, dir, soRef, nil)
}

// startGatedWorldConf starts a gated World as startGatedWorld does, with
// configure, when set, adjusting the engine Config first.
func startGatedWorldConf(t *testing.T, dir string, soRef *sobject.SharedObjectRef, configure func(*sobject_world_engine.Config)) (*localWorld, *gatedSharedObject) {
	t.Helper()
	return startGatedWorldOn(t, storage_native.NewS4db(false, dir), soRef, configure)
}

// startGatedWorldOn starts a gated World as startGatedWorldConf does, on st.
func startGatedWorldOn(t *testing.T, st storage.Storage, soRef *sobject.SharedObjectRef, configure func(*sobject_world_engine.Config)) (*localWorld, *gatedSharedObject) {
	// Mount the SharedObject and wrap it.
	t.Helper()
	tb, acc, soRef := openLocalAccountOn(t, st, soRef)
	so, soMountRef, err := sobject.ExMountSharedObject(t.Context(), tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(soMountRef.Release)

	// Build the World engine over the local SharedObject, starting without a
	// change log as a Space World does.
	local, ok := so.(*provider_local.SharedObject)
	if !ok {
		t.Fatalf("SharedObject is %T, not a local SharedObject", so)
	}
	conf := sobject_world_engine.NewConfig("gated-world-engine", soRef)
	conf.InitWorldOp = &sobject_world_engine.InitWorldOp{LastChangeDisable: true}
	if configure != nil {
		configure(conf)
	}

	// Start its controller and gate the SharedObject's publications.
	ctrl, err := sobject_world_engine.NewController(tb.Logger, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err)
	}
	ctrl.SetOpTypes(sobject_world_engine.OpTypes{Lookup: world_mock.LookupMockOp})
	gated := &gatedSharedObject{SharedObject: local, ctrl: ctrl}

	// Serve its World with the mock operations until cleanup.
	gated.serve(t)
	eng, err := ctrl.GetWorldEngine(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return newLocalWorld(t, tb, acc, eng, soRef), gated
}

// serve starts serving the World of s with its controller until stop or test
// cleanup. A controller that served before replaces its engine.
func (s *gatedSharedObject) serve(t *testing.T) {
	// Run the engine until its context is canceled.
	ctx, cancel := context.WithCancel(t.Context())
	exited := make(chan error, 1)
	go func() {
		exited <- sobject_world_engine.ExecuteWorld(ctx, s.ctrl, s)
	}()
	var once sync.Once
	s.stopServe = func() {
		once.Do(func() {
			cancel()
			<-exited
		})
	}
	t.Cleanup(s.stopServe)
}

// stop ends the engine serve started, as losing read access does.
func (s *gatedSharedObject) stop() {
	s.stopServe()
}

// hold holds the physical commit of the next publication a World write
// admits. applying receives once the commit reaches the state head. release
// lets it go on: it fails with failure when set, and applies otherwise. It
// may be called more than once, so a test can defer it.
func (s *gatedSharedObject) hold(failure error) (applying <-chan struct{}, release func()) {
	// Gate the state head's replacement on release.
	reached := make(chan struct{}, 1)
	released := make(chan struct{})
	s.mtx.Lock()
	s.next = func(head *block.AtomicHeadUpdate) *block.AtomicHeadUpdate {
		return &block.AtomicHeadUpdate{
			ObjectStoreID: head.ObjectStoreID,
			Key:           head.Key,
			Replace: func(ctx context.Context, current []byte, found bool) ([]byte, error) {
				select {
				case reached <- struct{}{}:
				default:
				}
				select {
				case <-released:
				case <-ctx.Done():
					return nil, context.Cause(ctx)
				}
				if failure != nil {
					return nil, failure
				}
				return head.Replace(ctx, current, found)
			},
		}
	}
	s.mtx.Unlock()
	var once sync.Once
	return reached, func() { once.Do(func() { close(released) }) }
}

// holdReceipts holds the receipts of the publications World writes submit
// until release: the store commits each at once, and its receipt resolves
// with the store's result, in submission order, once released. The engine
// then sees the publications pending while the volume is free for a sweep.
// release may be called more than once.
func (s *gatedSharedObject) holdReceipts() (release func()) {
	// Arm the gate, and disarm it and resolve the held receipts on release.
	held := make(chan struct{})
	s.mtx.Lock()
	s.held = held
	s.mtx.Unlock()
	return sync.OnceFunc(func() {
		s.mtx.Lock()
		s.held = nil
		s.mtx.Unlock()
		close(held)
	})
}

// GetBlockStore returns the SharedObject's block store, whose publications
// holdReceipts can hold.
func (s *gatedSharedObject) GetBlockStore() bstore.BlockStore {
	// Wrap the local block store.
	store, ok := s.SharedObject.GetBlockStore().(*provider_local.BlockStore)
	if !ok {
		panic("SharedObject block store is not a local provider block store")
	}
	return &gatedBlockStore{BlockStore: store, s: s}
}

// gatedBlockStore is a local block store whose publication receipts the
// gatedSharedObject can hold.
type gatedBlockStore struct {
	*provider_local.BlockStore
	s *gatedSharedObject
}

// SubmitAtomic submits p after the store's receipt of a held After, and
// holds its receipt when holdReceipts armed the gate.
func (g *gatedBlockStore) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Order p after the store's receipt of a held one.
	g.s.mtx.Lock()
	if committed, ok := g.s.committed[p.After]; ok {
		cp := *p
		cp.After = committed
		p = &cp
	}
	held, prev := g.s.held, g.s.last
	g.s.mtx.Unlock()

	// Submit it, and hold its receipt while armed.
	receipt, err := g.BlockStore.SubmitAtomic(ctx, p)
	if err != nil || held == nil {
		return receipt, err
	}

	// Return a proxy the store's receipt and the gate resolve, after the
	// proxy before it.
	proxy := block.NewRetainedPublicationReceipt(receipt.Release)
	g.s.mtx.Lock()
	if g.s.committed == nil {
		g.s.committed = make(map[*block.PublicationReceipt]*block.PublicationReceipt)
	}
	g.s.committed[proxy] = receipt
	g.s.last = proxy
	g.s.mtx.Unlock()

	// Resolve it once the store's receipt resolves and the gate opens.
	go func() {
		<-receipt.Done()
		<-held
		if prev != nil {
			<-prev.Done()
		}
		proxy.Resolve(receipt.Err())
	}()
	return proxy, nil
}

// QueueOperation queues op, holding its publication when hold armed it.
func (s *gatedSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	// Take the gate for an admitted write.
	publish := sobject.GetPublishState(ctx)
	if publish == nil {
		return s.SharedObject.QueueOperation(ctx, op)
	}
	s.mtx.Lock()
	wrap := s.next
	s.next = nil
	s.mtx.Unlock()

	// Publish through it.
	if wrap != nil {
		ctx = sobject.WithPublishState(ctx, func(ctx context.Context, head *block.AtomicHeadUpdate) (*block.PublicationReceipt, error) {
			return publish(ctx, wrap(head))
		})
	}
	return s.SharedObject.QueueOperation(ctx, op)
}

// _ is a type assertion
var (
	_ sobject.SharedObject  = (*gatedSharedObject)(nil)
	_ block.AtomicPublisher = (*gatedBlockStore)(nil)
)
