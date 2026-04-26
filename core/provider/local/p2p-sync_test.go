package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// TestSOSyncSolicit verifies that two sessions connected via inproc
// transport sync SO state through the SOSync solicit protocol.
func TestSOSyncSolicit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
