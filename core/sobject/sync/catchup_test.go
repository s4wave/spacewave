package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestAuthenticatedCatchupPinsPagesAndContinues proves real paired streams catch
// up across multiple history pages and then propagate configuration-only changes.
func TestAuthenticatedCatchupPinsPagesAndContinues(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const soID = "paged-authenticated-catchup"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)

	// Author a suffix larger than one page while the receiver remains offline.
	for range maxHistoryPageEntries + 2 {
		current, err := local.soHost.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		change, err := sobject.BuildSOConfigChange(soID, current.Config, current.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := local.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		state.Root = &sobject.SORoot{InnerSeqno: 7, Inner: []byte("visible content after offline work")}
		return state.Root.SignInnerData(owner, soID, 7, hash.RecommendedHashType)
	}); err != nil {
		t.Fatal(err)
	}
	target, err := local.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	states, release, err := remote.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Both endpoints authenticate and negotiate simultaneously over an unbuffered stream.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 64)}
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)

	// func.
	go func() { localDone <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	accepted, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return state.EqualVT(target), nil
	}, remoteDone)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(accepted.GetRoot().GetInner(), target.GetRoot().GetInner()) {
		t.Fatal("accepted configuration did not bring its signed visible content")
	}

	// Every page stays within both limits, and the receiver can serve the retained suffix.
	var pageCount int
	for len(observed.messages) != 0 {
		message := <-observed.messages
		if page := message.GetHistoryPage(); page != nil {
			pageCount++
			if len(page.GetChanges()) > maxHistoryPageEntries || message.SizeVT() > maxHistoryPageBytes {
				t.Fatal("sender exceeded a history page budget")
			}
		}
	}
	if pageCount != 2 {
		t.Fatalf("history pages = %d, want 2", pageCount)
	}
	retained, err := remote.soHost.ReadConfigHistory(ctx, initial.Config.ConfigChainHash, target.Config.ConfigChainHash)
	if err != nil || len(retained) != maxHistoryPageEntries+2 {
		t.Fatalf("receiver retained %d changes: %v", len(retained), err)
	}

	// The same open stream must notice a configuration change without a newer root.
	change, err := sobject.BuildSOConfigChange(soID, target.Config, target.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}
	accepted, err = states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return state.GetConfig().GetConfigChainSeqno() == target.Config.ConfigChainSeqno+1, nil
	}, remoteDone)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Root.EqualVT(target.Root) {
		t.Fatal("configuration-only catch-up changed the root")
	}

	// Cancellation closes transport and joins all workers in both directions.
	cancel()
	for _, done := range []<-chan error{localDone, remoteDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("catch-up workers survived cancellation")
		}
	}
}

// TestAuthenticatedReaderCannotPromoteSnapshot submits the escalation over the
// actual authenticated protocol and checks that authority and content stay held.
func TestAuthenticatedReaderCannotPromoteSnapshot(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const soID = "authenticated-reader-forgery"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	left, right := net.Pipe()

	// cleanup.
	t.Cleanup(func() { left.Close(); right.Close() })
	done := make(chan error, 1)
	go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	if _, err := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a"); err != nil {
		t.Fatal(err)
	}
	session := stream_packet.NewSession(right, maxMessageSize)
	localHead := &SOSyncMessage{}
	if err := session.RecvMsg(localHead); err != nil {
		t.Fatal(err)
	}
	if err := session.SendMsg(syncAcknowledgment(localHead.GetHead().GetRevision())); err != nil {
		t.Fatal(err)
	}

	// The reader signs a higher root under its self-promoted, same-head configuration.
	forged := initial.CloneVT()
	forged.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_OWNER
	forged.Root = &sobject.SORoot{InnerSeqno: 20, Inner: []byte("attacker replacement content")}
	if err := forged.Root.SignInnerData(reader, soID, 20, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	data, err := forged.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
		Revision: 1, StateHash: digest[:], ConfigHash: forged.Config.ConfigChainHash, ConfigSeqno: forged.Config.ConfigChainSeqno, RootSeqno: 20,
	}}}); err != nil {
		t.Fatal(err)
	}

	// Record request.
	request := &SOSyncMessage{}
	if err := session.RecvMsg(request); err != nil {
		t.Fatal(err)
	}
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{
		Revision: 1, BaseHash: request.GetHistoryRequest().GetBaseHash(), SoState: data, RootSeqno: 20,
	}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "configuration authority") {
			t.Fatalf("forged reader result = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("forged reader did not terminate the stream")
	}
	got, err := local.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) {
		t.Fatalf("reader forgery changed held state: %v", err)
	}
}

// TestCatchupMissingHistoryRequiresRecovery preserves divergent legacy state.
func TestCatchupMissingHistoryRequiresRecovery(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const soID = "missing-history-recovery"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)

	// cloneVT candidate via initial.
	candidate := initial.CloneVT()
	change, err := sobject.BuildSOConfigChange(soID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Config, err = sobject.VerifyConfigChange(soID, initial.Config, change)
	if err != nil {
		t.Fatal(err)
	}

	// The newer provider has its checkpoint but no retained path back to the receiver.
	local := newAuthenticationPeer(t, soID, owner, candidate)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)

	// func.
	go func() { localDone <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	for _, done := range []<-chan error{localDone, remoteDone} {
		select {
		case err := <-done:
			if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
				t.Fatalf("missing history result = %v", err)
			}
		case <-ctx.Done():
			t.Fatal("missing history did not return recovery")
		}
	}
	got, err := remote.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) {
		t.Fatalf("missing history changed receiver state: %v", err)
	}
}

// TestLegacyCatchupRequiresRecovery preserves local data and reports recovery
// for an empty checkpoint or an older authenticated snapshot exchange.
func TestLegacyCatchupRequiresRecovery(t *testing.T) {
	for _, mode := range []string{"empty checkpoint", "unrequested snapshot"} {
		t.Run(mode, func(t *testing.T) {

			// withTimeout ctx,cancel via context.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			const soID = "legacy-catchup-recovery"
			owner, reader := mustKeyPair(t), mustKeyPair(t)
			initial := authenticationState(t, soID, owner, reader)
			if mode == "empty checkpoint" {
				initial.Config.ConfigChainHash = nil
				initial.Config.ConfigChainSeqno = 0
			}

			// newAuthenticationPeer local.
			local := newAuthenticationPeer(t, soID, owner, initial)
			remote := newAuthenticationPeer(t, soID, reader, initial)
			recovery := make(chan bool, 1)
			local.SetPeerRecoveryObserver(func(_ peer.ID, needed bool) { recovery <- needed })
			left, right := net.Pipe()
			t.Cleanup(func() { left.Close(); right.Close() })
			done := make(chan error, 1)

			// func.
			go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
			_, authErr := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a")
			if mode == "empty checkpoint" && !errors.Is(authErr, sobject.ErrConfigHistoryUnavailable) {
				t.Fatalf("empty checkpoint admission = %v", authErr)
			}

			// Check the condition before continuing.
			if mode != "empty checkpoint" && authErr != nil {
				t.Fatal(authErr)
			}
			if mode == "unrequested snapshot" {
				session := stream_packet.NewSession(right, maxMessageSize)
				if err := session.RecvMsg(&SOSyncMessage{}); err != nil {
					t.Fatal(err)
				}
				data, err := initial.MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: data, RootSeqno: 1}}}); err != nil {
					t.Fatal(err)
				}
			}
			select {

			// Perform the action.
			case err := <-done:
				if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
					t.Fatalf("legacy exchange result = %v", err)
				}
			case <-ctx.Done():
				t.Fatal("legacy exchange did not terminate")
			}
			select {
			case needed := <-recovery:
				if !needed {
					t.Fatal("legacy exchange cleared recovery")
				}
			default:
				t.Fatal("legacy exchange did not report recovery")
			}
			got, err := local.soHost.GetHostState(ctx)
			if err != nil || !got.EqualVT(initial) {
				t.Fatalf("legacy exchange changed local state: %v", err)
			}
		})
	}
}
