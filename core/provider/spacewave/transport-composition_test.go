package provider_spacewave

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
)

type compositionTestLinkSource struct {
	bcast broadcast.Broadcast
	links []transport_controller.LinkSnapshot
}

func (s *compositionTestLinkSource) GetLinkSnapshotsWithWait() ([]transport_controller.LinkSnapshot, <-chan struct{}) {
	var links []transport_controller.LinkSnapshot
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		links = append([]transport_controller.LinkSnapshot(nil), s.links...)
		waitCh = getWaitCh()
	})
	return links, waitCh
}

func (s *compositionTestLinkSource) setLinkCount(count int) {
	s.bcast.HoldLock(func(notify func(), _ func() <-chan struct{}) {
		s.links = make([]transport_controller.LinkSnapshot, count)
		notify()
	})
}

func TestExistingSessionMetadataDefaultsDirectP2PEnabled(t *testing.T) {
	t.Parallel()

	if !directP2PEnabledFromMetadata(nil) {
		t.Fatal("missing Session metadata disabled direct P2P")
	}
	if !directP2PEnabledFromMetadata(&session.SessionMetadata{}) {
		t.Fatal("zero-value Session metadata disabled direct P2P")
	}
	if directP2PEnabledFromMetadata(&session.SessionMetadata{DirectP2PDisabled: true}) {
		t.Fatal("persisted direct-P2P disable policy was ignored")
	}
}

func TestTransportCompositionDisabledConstructsNoDirectMechanics(t *testing.T) {
	t.Parallel()

	acc := &ProviderAccount{}
	owner := &acc.transportComposition
	owner.init(acc)
	starts := 0
	stops := 0
	owner.startDirect = func(context.Context, string, crypto.PrivKey, string) (transportCompositionLinkSource, error) {
		starts++
		return &compositionTestLinkSource{}, nil
	}
	owner.stopDirect = func(string) { stops++ }

	if err := owner.configure(&transportCompositionConfig{
		ctx:       t.Context(),
		sessionID: "session",
		enabled:   false,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := owner.snapshotWithWait("session")
	if starts != 0 || stops != 0 {
		t.Fatalf("disabled composition started/stopped direct mechanics: %d/%d", starts, stops)
	}
	if snapshot.DirectP2PEnabled || snapshot.P2PState != TransportCompositionP2PStateDisabled {
		t.Fatalf("unexpected disabled snapshot: %+v", snapshot)
	}
}

func TestTransportCompositionUnknownLifecycleDoesNotCreateState(t *testing.T) {
	t.Parallel()

	acc := &ProviderAccount{}
	owner := &acc.transportComposition
	owner.init(acc)

	owner.demandStarted("unknown")
	owner.demandFinished("unknown")
	owner.stop("unknown")
	snapshot, waitCh := owner.snapshotWithWait("unknown")
	if snapshot.DirectP2PEnabled != true || snapshot.P2PState != TransportCompositionP2PStateNoPeers || waitCh != nil {
		t.Fatalf("unknown lifecycle projection = snapshot=%+v wait=%v", snapshot, waitCh)
	}
	if len(owner.sessions) != 0 {
		t.Fatalf("unknown lifecycle created %d states", len(owner.sessions))
	}

	for _, sessionID := range []string{"A", "B"} {
		if err := owner.configure(&transportCompositionConfig{
			ctx:       t.Context(),
			sessionID: sessionID,
			enabled:   false,
		}); err != nil {
			t.Fatal(err)
		}
	}
	owner.stop("A")
	if _, ok := owner.sessions["A"]; ok {
		t.Fatal("stopped Session A state was retained")
	}
	if _, ok := owner.sessions["B"]; !ok {
		t.Fatal("live Session B state was removed with Session A")
	}
	snapshot, _ = owner.snapshotWithWait("B")
	if snapshot.P2PState != TransportCompositionP2PStateDisabled {
		t.Fatalf("Session B snapshot changed after Session A stop: %+v", snapshot)
	}
}

func TestTransportCompositionTracksPeerLossAndReconnectFromLinkEvents(t *testing.T) {
	t.Parallel()

	acc := &ProviderAccount{}
	owner := &acc.transportComposition
	owner.init(acc)
	links := &compositionTestLinkSource{}
	starts := 0
	stops := 0
	owner.startDirect = func(context.Context, string, crypto.PrivKey, string) (transportCompositionLinkSource, error) {
		starts++
		return links, nil
	}
	owner.stopDirect = func(string) { stops++ }
	transportRunning := true
	var transportBcast broadcast.Broadcast
	owner.transportState = func(string) (bool, <-chan struct{}) {
		var running bool
		var waitCh <-chan struct{}
		transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			running = transportRunning
			waitCh = getWaitCh()
		})
		return running, waitCh
	}

	if err := owner.configure(&transportCompositionConfig{
		ctx:       t.Context(),
		sessionID: "session",
		enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	awaitCompositionState(t, owner, TransportCompositionP2PStateNoPeers, 0)

	links.setLinkCount(1)
	awaitCompositionState(t, owner, TransportCompositionP2PStateIdle, 1)

	links.setLinkCount(0)
	awaitCompositionState(t, owner, TransportCompositionP2PStateFallbackNoPeer, 0)

	links.setLinkCount(1)
	awaitCompositionState(t, owner, TransportCompositionP2PStateIdle, 1)

	transportBcast.HoldLock(func(notify func(), _ func() <-chan struct{}) {
		transportRunning = false
		notify()
	})
	awaitCompositionState(t, owner, TransportCompositionP2PStateError, 0)

	owner.stop("session")
	if starts != 1 || stops != 1 {
		t.Fatalf("direct mechanics lifecycle = %d starts/%d stops, want 1/1", starts, stops)
	}
}

func awaitCompositionState(
	t *testing.T,
	owner *transportCompositionOwner,
	state TransportCompositionP2PState,
	peers uint32,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		snapshot, waitCh := owner.snapshotWithWait("session")
		if snapshot.P2PState == state && snapshot.ActivePeerCount == peers {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("composition snapshot = %+v, want state=%d peers=%d", snapshot, state, peers)
		case <-waitCh:
		}
	}
}
