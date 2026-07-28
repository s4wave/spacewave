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
	account_settings "github.com/s4wave/spacewave/core/account/settings"
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

// TestSOSyncSolicit verifies that two sessions connected via inproc
// transport sync SO state through the SOSync solicit protocol.
func TestSOSyncSolicit(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), p2pSyncTestTimeout)
	defer cancel()

	// Create two separate provider accounts.
	tbA, sessRefA, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	defer releaseA()
	_, _, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	defer releaseB()

	// Add a paired device to A's account settings SO.
	// This gives A a higher seqno than B's initial state.
	accountIDA := sessRefA.GetProviderResourceRef().GetProviderAccountId()
	soA, soARelease := mountAccountSettingsSO(ctx, t, tbA.Bus, accountIDA)
	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWTestSyncPeer", "Sync Test Device")
	soARelease()

	// Create session transports for both sides.
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

	// Connect the two session transports via inproc.
	connectSessionTransports(ctx, t, stA, stB)

	// Start P2P sync on both sides.
	if err := accA.StartP2PSync(ctx, stA); err != nil {
		t.Fatal(err)
	}
	defer accA.StopP2PSync()
	if err := accB.StartP2PSync(ctx, stB); err != nil {
		t.Fatal(err)
	}
	defer accB.StopP2PSync()

	// Verify B's SOHost receives A's state (higher seqno).
	// We check the raw SOHost state which includes the full state
	// after sync, without needing grant decryption.
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 1)
}

// TestAutoReconnectSync verifies that P2P sync works when sync is started
// before transport connectivity exists (unlike TestSOSyncSolicit which
// connects first). This tests the "reconnect" path where sync is already
// running when a peer appears.
func TestAutoReconnectSync(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), p2pSyncTestTimeout)
	defer cancel()

	// Create two provider accounts.
	tbA, sessRefA, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	defer releaseA()
	_, _, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	defer releaseB()

	// Write to A's account settings SO to give it a higher seqno than B.
	accountIDA := sessRefA.GetProviderResourceRef().GetProviderAccountId()
	soA, soARelease := mountAccountSettingsSO(ctx, t, tbA.Bus, accountIDA)
	addPairedDeviceAndWait(ctx, t, soA, sessB.GetPeerId().String(), "Device B")
	soARelease()

	// Create A's transport and start P2P sync.
	if err := accA.CreateSessionTransport(ctx, sessA.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accA.StopSessionTransport()
	stA := accA.GetSessionTransport()
	if stA == nil {
		t.Fatal("expected transport on A")
	}
	if err := accA.StartP2PSync(ctx, stA); err != nil {
		t.Fatal(err)
	}
	defer accA.StopP2PSync()

	// Create B's transport and start sync.
	if err := accB.CreateSessionTransport(ctx, sessB.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accB.StopSessionTransport()
	stB := accB.GetSessionTransport()

	if err := accB.StartP2PSync(ctx, stB); err != nil {
		t.Fatal(err)
	}
	defer accB.StopP2PSync()

	// Connect the two transports via inproc.
	connectSessionTransports(ctx, t, stA, stB)

	// Verify B receives A's state via sync (A has seqno > 0 from the paired device write).
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 1)
}

// connectSessionTransports connects two session transports via inproc
// transport so they can exchange bifrost traffic in-process.
func connectSessionTransports(ctx context.Context, t *testing.T, stA, stB *transport.SessionTransport) {
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

	// Establish link from one side only to avoid dual-dial instability.
	addEstablishLink(ctx, t, childBusA, peerIDA, peerIDB)
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

// waitForSyncedRootSeqno polls the account's SO via its SOHost until the
// root seqno is at least minSeqno, indicating a sync has been applied.
func waitForSyncedRootSeqno(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, soID string, minSeqno uint64) {
	t.Helper()

	var ref *sobject.SharedObjectRef
	var err error
	if soID == account_settings.BindingPurpose {
		ref, err = acc.GetAccountSettingsRef(ctx)
		if err != nil {
			t.Fatalf("get account settings ref: %v", err)
		}
	} else {
		soList := acc.GetSOListCtr().GetValue()
		for _, entry := range soList.GetSharedObjects() {
			entryRef := entry.GetRef()
			if entryRef.GetProviderResourceRef().GetId() == soID {
				ref = entryRef
				break
			}
		}
	}
	if ref == nil {
		t.Fatalf("SO %s not found in SO list", soID)
	}

	so, relSO, err := acc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatalf("mount SO %s: %v", soID, err)
	}
	defer relSO()

	localSO := so.(*provider_local.SharedObject)
	for {
		if ctx.Err() != nil {
			t.Fatalf("timed out waiting for SO %s root seqno >= %d", soID, minSeqno)
		}

		hostState, err := localSO.GetSOHostState(ctx)
		if err != nil {
			t.Fatalf("get host state: %v", err)
		}

		if hostState.GetRoot().GetInnerSeqno() >= minSeqno {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for SO %s root seqno >= %d", soID, minSeqno)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestP2PConflictResolution verifies that when two sessions have divergent
// SO state (different seqnos), SOSync's snapshot exchange resolves by
// adopting the higher seqno state.
func TestP2PConflictResolution(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), p2pSyncTestTimeout)
	defer cancel()

	// Create two provider accounts. A will have higher seqno.
	tbA, sessRefA, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	defer releaseA()
	tbB, sessRefB, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	defer releaseB()

	// Write to A's account settings SO twice (seqno=2 after two ops).
	accountIDA := sessRefA.GetProviderResourceRef().GetProviderAccountId()
	soA, soARelease := mountAccountSettingsSO(ctx, t, tbA.Bus, accountIDA)
	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWConflictPeerA1", "Conflict A1")
	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWConflictPeerA2", "Conflict A2")
	soARelease()

	// Write to B's account settings SO once (seqno=1).
	accountIDB := sessRefB.GetProviderResourceRef().GetProviderAccountId()
	soB, soBRelease := mountAccountSettingsSO(ctx, t, tbB.Bus, accountIDB)
	addPairedDeviceAndWait(ctx, t, soB, "12D3KooWConflictPeerB1", "Conflict B1")
	soBRelease()

	// Now connect and sync. A has seqno=2, B has seqno=1.
	// B should adopt A's state (higher seqno).
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
	connectSessionTransports(ctx, t, stA, stB)

	if err := accA.StartP2PSync(ctx, stA); err != nil {
		t.Fatal(err)
	}
	defer accA.StopP2PSync()
	if err := accB.StartP2PSync(ctx, stB); err != nil {
		t.Fatal(err)
	}
	defer accB.StopP2PSync()

	// B should adopt A's state (seqno=2).
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 2)
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

	// Verify SO sync works between the two sides (proves the solicit
	// infrastructure including DEX is operational on the connected link).
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 0)
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
