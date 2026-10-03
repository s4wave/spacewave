package provider_local_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

const p2pSyncTestTimeout = 2 * time.Minute

// connectSessionTransports connects two session transports via inproc
// transport so they can exchange bifrost traffic in-process.
func connectSessionTransports(ctx context.Context, t *testing.T, stA, stB *transport.SessionTransport) {
	t.Helper()
	prepareSessionTransportBackends(ctx, t, stA, stB)
	addEstablishLink(ctx, t, stA.GetChildBus(), stA.GetPeerID(), stB.GetPeerID())
}

func prepareSessionTransportBackends(ctx context.Context, t *testing.T, stA, stB *transport.SessionTransport) {
	t.Helper()

	peerIDA := stA.GetPeerID()
	peerIDB := stB.GetPeerID()
	childBusA := stA.GetChildBus()
	childBusB := stB.GetChildBus()

	le := logrus.NewEntry(logrus.New())

	// Build inproc transport controllers with dialers pointing at each other.
	inprocCtrlA := inproc.BuildInprocController(le, childBusA, "", &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerIDB.String(): {Address: inproc.NewAddr(peerIDB).String()},
		},
	})
	inprocCtrlB := inproc.BuildInprocController(le, childBusB, "", &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerIDA.String(): {Address: inproc.NewAddr(peerIDA).String()},
		},
	})

	if _, err := childBusA.AddController(ctx, inprocCtrlA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := childBusB.AddController(ctx, inprocCtrlB, nil); err != nil {
		t.Fatal(err)
	}

	// Wait for both transports to be ready, then connect them.
	tptA, err := inprocCtrlA.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tptB, err := inprocCtrlB.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ipA := tptA.(*inproc.Inproc)
	ipB := tptB.(*inproc.Inproc)
	ipA.ConnectToInproc(ctx, ipB)
	ipB.ConnectToInproc(ctx, ipA)
}

// addEstablishLink adds an EstablishLinkWithPeer directive to the bus.
func addEstablishLink(ctx context.Context, t *testing.T, b bus.Bus, src, dst peer.ID) {
	t.Helper()
	_, diRef, err := b.AddDirective(link.NewEstablishLinkWithPeer(src, dst), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { diRef.Release() })
}

// TestBlockSyncDEX verifies that StartP2PSync starts DEX solicit controllers
// for each block store bucket, and that the DEX solicit directives resolve
// when peers are connected.
func TestBlockSyncDEX(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), p2pSyncTestTimeout)
	defer cancel()

	_, _, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	defer releaseA()
	_, _, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	defer releaseB()

	// Create transports.
	if err := accA.CreateSessionTransport(ctx, sessA.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accA.StopSessionTransport()
	if err := accB.CreateSessionTransport(ctx, sessB.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accB.StopSessionTransport()

	stA := accA.GetSessionTransport()
	stB := accB.GetSessionTransport()

	// Connect via inproc.
	connectSessionTransports(ctx, t, stA, stB)

	// Start P2P sync on both (this starts SOSync + DEX solicit for each SO).
	if err := accA.StartP2PSync(ctx, stA); err != nil {
		t.Fatal(err)
	}
	defer accA.StopP2PSync()
	if err := accB.StartP2PSync(ctx, stB); err != nil {
		t.Fatal(err)
	}
	defer accB.StopP2PSync()

	// Verify both sides have sync running (SOSync + DEX solicit registered).
	if !accA.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync running on A")
	}
	if !accB.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync running on B")
	}

}

// TestStartP2PSyncSameTransportRestartAddsDesiredWork proves that a
// same-transport start arriving while the first startup pass is gated causes
// the pass to restart and load a shared object added after its SO snapshot.
func TestStartP2PSyncSameTransportRestartAddsDesiredWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	_, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected non-nil session transport")
	}

	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	dexLoads := make(chan string, 8)
	var gate sync.Once
	removeHandler, err := st.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(handlerCtx context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			loadConfig, ok := load.GetLoadControllerConfig().(*dex_solicit.Config)
			if !ok {
				return nil, nil
			}
			select {
			case dexLoads <- loadConfig.GetBucketId():
			default:
			}
			gate.Do(func() {
				close(loadStarted)
				select {
				case <-releaseLoad:
				case <-handlerCtx.Done():
				}
			})
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- acc.StartP2PSync(ctx, st)
	}()

	select {
	case <-loadStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for DEX solicit controller load")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- acc.StartP2PSync(ctx, st)
	}()

	newRef, err := acc.CreateSharedObject(
		ctx,
		"pending-start-space",
		&sobject.SharedObjectMeta{BodyType: "space"},
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	newBucketID := provider_local.BlockStoreBucketID(
		newRef.GetProviderResourceRef().GetProviderId(),
		newRef.GetProviderResourceRef().GetProviderAccountId(),
		newRef.GetBlockStoreId(),
	)

	if acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to remain starting while DEX controller load is blocked")
	}
	_, p2pWaitCh := acc.GetP2PSyncSnapshotWithWait()

	close(releaseLoad)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-p2pWaitCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for P2P startup broadcast")
	}
	if running, _ := acc.GetP2PSyncSnapshotWithWait(); !running {
		t.Fatal("expected P2P sync running after startup broadcast")
	}

	defer acc.StopP2PSync()

	for {
		select {
		case bucketID := <-dexLoads:
			if bucketID == newBucketID {
				if !acc.IsP2PSyncRunning() {
					t.Fatal("expected P2P sync running after pending shared object start")
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for DEX solicit controller for %s", newBucketID)
		}
	}
}

// TestStartP2PSyncAddsSharedObjectAfterStartup proves that a shared object
// created after startup is picked up by the running SO-list watcher.
func TestStartP2PSyncAddsSharedObjectAfterStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected non-nil session transport")
	}

	dexLoads := make(chan string, 8)
	removeHandler, err := st.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			loadConfig, ok := load.GetLoadControllerConfig().(*dex_solicit.Config)
			if !ok {
				return nil, nil
			}
			select {
			case dexLoads <- loadConfig.GetBucketId():
			default:
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	if err := acc.StartP2PSync(ctx, st); err != nil {
		t.Fatal(err)
	}
	defer acc.StopP2PSync()
	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync running after startup")
	}

	newRef, err := acc.CreateSharedObject(
		ctx,
		"post-start-space",
		&sobject.SharedObjectMeta{BodyType: "space"},
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	newBucketID := provider_local.BlockStoreBucketID(
		sessRef.GetProviderResourceRef().GetProviderId(),
		newRef.GetProviderResourceRef().GetProviderAccountId(),
		newRef.GetBlockStoreId(),
	)

	for {
		select {
		case bucketID := <-dexLoads:
			if bucketID == newBucketID {
				if !acc.IsP2PSyncRunning() {
					t.Fatal("expected P2P sync running after shared object start")
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for DEX solicit controller for %s", newBucketID)
		}
	}
}

func TestStartP2PSyncCoalescedCallerOwnsLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	_, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	var gate sync.Once
	removeHandler, err := st.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(handlerCtx context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			if _, ok := load.GetLoadControllerConfig().(*dex_solicit.Config); !ok {
				return nil, nil
			}
			gate.Do(func() {
				close(loadStarted)
				select {
				case <-releaseLoad:
				case <-handlerCtx.Done():
				}
			})
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	firstCtx, firstCancel := context.WithCancel(ctx)
	defer firstCancel()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- acc.StartP2PSync(firstCtx, st)
	}()

	select {
	case <-loadStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first P2P startup load")
	}

	secondBaseCtx, secondCancel := context.WithCancel(ctx)
	defer secondCancel()
	secondCtx := &observedP2PStartContext{
		Context:         secondBaseCtx,
		awaitReady:      make(chan struct{}),
		allowAwait:      make(chan struct{}),
		ownerRegistered: make(chan struct{}),
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- acc.StartP2PSync(secondCtx, st)
	}()

	select {
	case <-secondCtx.awaitReady:
	case <-ctx.Done():
		t.Fatal("timed out waiting for coalesced P2P caller readiness")
	}
	close(secondCtx.allowAwait)
	select {
	case <-secondCtx.ownerRegistered:
	case <-ctx.Done():
		t.Fatal("coalesced P2P caller did not retain the lifecycle")
	}

	// Cancel the first caller while its startup is still gated. The startup
	// belongs to whoever owns the state, so a caller that gives up returns on
	// its own context instead of finishing work it no longer has a stake in.
	firstCancel()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected the canceled first caller to return context.Canceled, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("canceled first P2P caller did not return while startup was still gated")
	}
	close(releaseLoad)

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for coalesced P2P start")
	}
	defer acc.StopP2PSync()
	if !acc.IsP2PSyncRunning() {
		t.Fatal("expected P2P sync to remain running under the coalesced caller")
	}
}

// TestStartP2PSyncDoesNotCoalesceAcrossTransports checks that a start for a
// replacement session transport gets its own controllers.
//
// A startup pass loads controllers onto the child bus of the transport it began
// with. A start for a different transport that joined an in-flight one would be
// told sync had started while its own transport carried no DEX solicit, SO
// sync, or invite controllers at all.
func TestStartP2PSyncDoesNotCoalesceAcrossTransports(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	_, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()
	first := acc.GetSessionTransport()
	if first == nil {
		t.Fatal("expected non-nil session transport")
	}

	firstLoadStarted := make(chan struct{})
	releaseFirstLoad := make(chan struct{})
	var gate sync.Once
	removeFirst, err := first.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(handlerCtx context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			if _, ok := load.GetLoadControllerConfig().(*dex_solicit.Config); !ok {
				return nil, nil
			}
			// Hold the first startup open past the replacement of its own
			// transport, so the second start meets a startup genuinely in
			// flight rather than one that already finished.
			gate.Do(func() {
				close(firstLoadStarted)
				select {
				case <-releaseFirstLoad:
				case <-handlerCtx.Done():
				}
			})
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeFirst()
	defer close(releaseFirstLoad)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- acc.StartP2PSync(ctx, first)
	}()
	select {
	case <-firstLoadStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for the first P2P startup load")
	}

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	second := acc.GetSessionTransport()
	if second == first {
		t.Fatal("expected a replacement session transport")
	}

	secondLoads := make(chan struct{}, 1)
	removeSecond, err := second.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			if _, ok := load.GetLoadControllerConfig().(*dex_solicit.Config); !ok {
				return nil, nil
			}
			select {
			case secondLoads <- struct{}{}:
			default:
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeSecond()

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- acc.StartP2PSync(ctx, second)
	}()
	defer acc.StopP2PSync()

	select {
	case <-secondLoads:
	case err := <-secondDone:
		t.Fatalf("start for the replacement transport finished without loading controllers on it: %v", err)
	case <-ctx.Done():
		t.Fatal("the replacement transport never received its DEX solicit controller")
	}
}

func TestP2PSyncRetiresWhenFinalOwnerExits(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	_, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()
	defer acc.StopP2PSync()

	st := acc.GetSessionTransport()
	ownerCtx, ownerCancel := context.WithCancel(ctx)
	if err := acc.StartP2PSync(ownerCtx, st); err != nil {
		t.Fatal(err)
	}

	running, lifecycleChanged := acc.GetP2PSyncSnapshotWithWait()
	if !running {
		t.Fatal("expected P2P sync to be running before its final owner exits")
	}
	ownerCancel()

	select {
	case <-lifecycleChanged:
	case <-ctx.Done():
		t.Fatal("P2P sync did not retire after its final owner exited")
	}
	if running, _ := acc.GetP2PSyncSnapshotWithWait(); running {
		t.Fatal("expected P2P sync to stop after its final owner exited")
	}
}

func TestStopP2PSyncReleasesResourceRegisteredDuringStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, sessRef, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	_, soRelease := mountAccountSettingsSO(ctx, t, tb.Bus, accountID)
	soRelease()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	st := acc.GetSessionTransport()
	childBus := st.GetChildBus()
	var factoryResolver controller.Controller
	for _, ctrl := range childBus.GetControllers() {
		if _, ok := ctrl.(*resolver.Controller); ok {
			childBus.RemoveController(ctrl)
			factoryResolver = ctrl
			break
		}
	}
	if factoryResolver == nil {
		t.Fatal("expected the transport child bus factory resolver")
	}
	defer func() {
		if err := factoryResolver.Close(); err != nil {
			t.Errorf("close transport child bus factory resolver: %v", err)
		}
	}()

	controllerSelected := make(chan struct{})
	allowRegistration := make(chan struct{})
	resourceReleased := make(chan struct{})
	var selectedOnce, releasedOnce sync.Once
	removeHandler, err := childBus.AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			loadConfig, ok := load.GetLoadControllerConfig().(*dex_solicit.Config)
			if !ok {
				return nil, nil
			}
			ctrl, err := dex_solicit.NewController(logrus.NewEntry(logrus.New()), childBus, loadConfig)
			if err != nil {
				return nil, err
			}
			value := &gatedP2PSyncExecValue{
				ExecControllerValue: loader.NewExecControllerValue(time.Now(), time.Time{}, ctrl, nil),
				ctrl:                ctrl,
				selected:            controllerSelected,
				allow:               allowRegistration,
				selectedOnce:        &selectedOnce,
			}
			return directive.Resolvers(directive.NewFuncResolver(
				func(resolverCtx context.Context, handler directive.ResolverHandler) error {
					if _, accepted := handler.AddValue(value); !accepted {
						return errors.New("P2P startup resource was rejected")
					}
					<-resolverCtx.Done()
					releasedOnce.Do(func() {
						close(resourceReleased)
					})
					return resolverCtx.Err()
				},
			)), nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	startDone := make(chan error, 1)
	go func() {
		startDone <- acc.StartP2PSync(ctx, st)
	}()
	select {
	case <-controllerSelected:
	case <-ctx.Done():
		t.Fatal("P2P startup did not select the gated controller resource")
	}

	_, lifecycleChanged := acc.GetP2PSyncSnapshotWithWait()
	stopDone := make(chan struct{})
	go func() {
		acc.StopP2PSync()
		close(stopDone)
	}()
	select {
	case <-lifecycleChanged:
	case <-ctx.Done():
		t.Fatal("P2P stop did not retire the starting state")
	}

	// Registration proceeds only after stop has retired the state. Cleanup must
	// wait for the startup pass to retain this reference before releasing it.
	close(allowRegistration)
	select {
	case <-resourceReleased:
	case <-ctx.Done():
		t.Fatal("P2P startup resource registered after stop began was not released")
	}
	select {
	case <-stopDone:
	case <-ctx.Done():
		t.Fatal("P2P stop did not finish after releasing the startup resource")
	}
	select {
	case err := <-startDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected stopped P2P startup to return context.Canceled, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("stopped P2P startup did not return")
	}
}

type gatedP2PSyncExecValue struct {
	loader.ExecControllerValue
	ctrl         controller.Controller
	selected     chan struct{}
	allow        chan struct{}
	selectedOnce *sync.Once
}

func (v *gatedP2PSyncExecValue) GetController() controller.Controller {
	v.selectedOnce.Do(func() {
		close(v.selected)
	})
	<-v.allow
	return v.ctrl
}

type observedP2PStartContext struct {
	context.Context
	awaitReady      chan struct{}
	allowAwait      chan struct{}
	ownerRegistered chan struct{}
	awaitOnce       sync.Once
	ownerOnce       sync.Once
}

func (c *observedP2PStartContext) Done() <-chan struct{} {
	c.awaitOnce.Do(func() {
		close(c.awaitReady)
		<-c.allowAwait
	})
	c.ownerOnce.Do(func() {
		close(c.ownerRegistered)
	})
	return c.Context.Done()
}

func skipFullP2PSyncUnderGoScript(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("full two-peer P2P sync is too costly under GoScript; direct DEX startup coverage runs separately")
	}
}
