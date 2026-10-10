package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestCatchupBudgetsRejectWithoutMutation sends excessive history over an
// authenticated stream and checks that neither a prefix nor its head is adopted.
func TestCatchupBudgetsRejectWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		// name identifies the independently enforced protocol budget.
		name string
		// count is the number of untrusted linked entries sent.
		count int
		// padding enlarges each entry without changing cursor continuity.
		padding int
		// perPage chooses a page size, including an intentionally excessive one.
		perPage int
		// oversizedFrame sends only an excessive length prefix, with no payload allocation.
		oversizedFrame bool
	}{
		{name: "page entries", count: maxHistoryPageEntries + 1, perPage: maxHistoryPageEntries + 1},
		{name: "page bytes", count: 1, padding: maxHistoryPageBytes, perPage: 1},
		{name: "suffix entries", count: sobject.MaxConfigSuffixEntries + 1, perPage: maxHistoryPageEntries},
		{name: "suffix bytes", count: 33, padding: 256 * 1024, perPage: 3},
		{name: "snapshot frame", oversizedFrame: true},
	} {
		t.Run(test.name, func(t *testing.T) {

			// withTimeout ctx,cancel via context.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			const soID = "catchup-budget"
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
			head := &SOSyncMessage{}
			if err := session.RecvMsg(head); err != nil {
				t.Fatal(err)
			}
			if err := session.SendMsg(syncAcknowledgment(head.GetHead().GetRevision())); err != nil {
				t.Fatal(err)
			}

			// Bytes beyond the frame cap must be rejected before a body is read.
			if test.oversizedFrame {
				var prefix [4]byte
				binary.LittleEndian.PutUint32(prefix[:], maxMessageSize+1)
				if _, err := right.Write(prefix[:]); err != nil {
					t.Fatal(err)
				}
			} else {
				// Entries are linked but untrusted; budget rejection precedes signature work.
				cursor := initial.Config.ConfigChainHash
				changes := make([]*sobject.SOConfigChange, 0, test.count)
				for i := range test.count {
					change := &sobject.SOConfigChange{
						ConfigSeqno: uint64(i + 1), PreviousHash: cursor,
						Config: &sobject.SharedObjectConfig{ConfigChainHash: bytes.Repeat([]byte{1}, test.padding)},
					}
					var err error
					cursor, err = sobject.HashSOConfigChange(change)
					if err != nil {
						t.Fatal(err)
					}
					changes = append(changes, change)
				}
				if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
					Revision: 1, ConfigHash: cursor, ConfigSeqno: uint64(test.count), StateHash: bytes.Repeat([]byte{1}, 32),
				}}}); err != nil {
					t.Fatal(err)
				}
				request := &SOSyncMessage{}
				if err := session.RecvMsg(request); err != nil {
					t.Fatal(err)
				}

				// The last page crosses exactly the selected budget and must terminate catch-up.
				cursor = initial.Config.ConfigChainHash
				for len(changes) != 0 {
					count := min(test.perPage, len(changes))
					page := &SOSyncHistoryPage{Revision: 1, Cursor: cursor, Changes: changes[:count]}
					if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: page}}); err != nil {
						t.Fatal(err)
					}
					var err error
					cursor, err = sobject.HashSOConfigChange(changes[count-1])
					if err != nil {
						t.Fatal(err)
					}
					changes = changes[count:]
				}
				response := &SOSyncMessage{}
				if err := session.RecvMsg(response); err != nil || response.GetRecoveryRequired() == nil {
					t.Fatalf("budget recovery response = %v: %v", response, err)
				}
			}
			select {
			case err := <-done:
				if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
					t.Fatalf("budget result = %v", err)
				}
			case <-ctx.Done():
				t.Fatal("budget rejection did not stop the stream")
			}
			got, err := local.soHost.GetHostState(ctx)
			if err != nil || !got.EqualVT(initial) {
				t.Fatalf("budget failure changed held state: %v", err)
			}
		})
	}
}

// TestCatchupRecoveryFencesTrailingSnapshot rejects further imports while its recovery response is blocked.
func TestCatchupRecoveryFencesTrailingSnapshot(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	const soID = "catchup-terminal-snapshot"
	owner, reader := mustKeyPair(t), mustKeyPair(t)

	// authenticationState initial.
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	left, right := net.Pipe()
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}

	// Perform the action.
	var streamErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		streamErr = local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b")
	}()
	t.Cleanup(func() {
		cancel()
		left.Close()
		right.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("catch-up workers survived cancellation")
		}
	})

	// Abort on the error.
	if _, err := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a"); err != nil {
		t.Fatal(err)
	}
	session := stream_packet.NewSession(right, maxMessageSize)
	head := &SOSyncMessage{}
	if err := session.RecvMsg(head); err != nil {
		t.Fatal(err)
	}
	if err := session.SendMsg(syncAcknowledgment(head.GetHead().GetRevision())); err != nil {
		t.Fatal(err)
	}

	// The advertised newer checkpoint is admissible, so only the terminal stream decision can fence it.
	candidate := initial.CloneVT()
	advanceSnapshotCheckpoint(t, soID, candidate, owner)
	data, err := candidate.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)

	// Check the condition before continuing.
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
		Revision: 1, ConfigHash: candidate.Config.ConfigChainHash, ConfigSeqno: candidate.Config.ConfigChainSeqno,
		StateHash: digest[:],
	}}}); err != nil {
		t.Fatal(err)
	}
	request := &SOSyncMessage{}
	if err := session.RecvMsg(request); err != nil || request.GetHistoryRequest() == nil {
		t.Fatalf("newer checkpoint was not requested: %v", err)
	}
	changes := make([]*sobject.SOConfigChange, maxHistoryPageEntries+1)
	for index := range changes {
		changes[index] = &sobject.SOConfigChange{}
	}
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
		Revision: 1, Cursor: initial.Config.ConfigChainHash, Changes: changes,
	}}}); err != nil {
		t.Fatal(err)
	}

	// Observe the actual recovery write before admitting trailing input; the peer is not reading it yet.
	for {
		select {
		case message := <-observed.messages:
			if message.GetRecoveryRequired() == nil {
				continue
			}
		case <-ctx.Done():
			t.Fatal("budget error did not queue recovery")
		}
		break
	}
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{
		SoState: data, Revision: 1, BaseHash: initial.Config.ConfigChainHash,
	}}}); err != nil {
		t.Fatal(err)
	}

	// Three discarded frames exceed the reader's one queued and one in-progress frame.
	// Completing these writes proves the owner handled the earlier snapshot before recovery can finish.
	for range 3 {
		if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Ack{Ack: &SOSyncAck{}}}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := local.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) {
		t.Fatalf("trailing snapshot changed state after budget rejection: %v", err)
	}
	recovery := &SOSyncMessage{}
	if err := session.RecvMsg(recovery); err != nil || recovery.GetRecoveryRequired().GetRevision() != 1 {
		t.Fatalf("terminal recovery response: %v", err)
	}
	select {
	case <-done:
		if !errors.Is(streamErr, sobject.ErrConfigHistoryUnavailable) {
			t.Fatalf("terminal stream result: %v", streamErr)
		}
	case <-ctx.Done():
		t.Fatal("recovery delivery did not finish the stream")
	}
}

// TestOversizedReceiverAdoptsTrimmedPeer holds a receiver whose state is larger
// than one frame and checks that it still catches up from a peer that is ahead
// in configuration and has trimmed to a newer checkpoint.
func TestOversizedReceiverAdoptsTrimmedPeer(t *testing.T) {
	// Bound the test and create the keys.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const soID = "oversized-receiver"
	owner, reader := mustKeyPair(t), mustKeyPair(t)

	// Both sides hold more operations than one frame carries.
	initial := authenticationState(t, soID, owner, reader)
	for range 21 {
		writeSyncOp(t, soID, initial, owner, strings.Repeat("x", 512<<10))
	}
	if initial.SizeVT() <= maxMessageSize {
		t.Fatalf("state is %d bytes, want more than %d", initial.SizeVT(), maxMessageSize)
	}
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)

	// The owner changes the configuration.
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

	// The owner trims to a new checkpoint.
	if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		advanceSnapshotCheckpoint(t, soID, state, owner)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, err := local.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Watch the receiver's state.
	states, release, err := remote.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The receiver adopts the trimmed state instead of ending the stream.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)
	go func() { localDone <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	accepted, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return state.GetCheckpoint().EqualVT(target.GetCheckpoint()), nil
	}, remoteDone)
	if err != nil {
		t.Fatalf("oversized receiver did not adopt the trimmed peer: %v", err)
	}
	if accepted.GetConfig().GetConfigChainSeqno() != target.GetConfig().GetConfigChainSeqno() || accepted.SizeVT() > maxMessageSize {
		t.Fatalf("accepted %d bytes at config seqno %d", accepted.SizeVT(), accepted.GetConfig().GetConfigChainSeqno())
	}
}

// TestOversizedSnapshotEndsStreamAsTooLarge asks a sender whose state is larger
// than one frame for a snapshot. The sender cannot serve it, so it ends the
// stream with ErrStateTooLarge and tells the requester to recover.
func TestOversizedSnapshotEndsStreamAsTooLarge(t *testing.T) {
	// Bound the test and create the keys.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const soID = "oversized-snapshot"
	owner, reader := mustKeyPair(t), mustKeyPair(t)

	// The sender holds a configuration change and more operations than a frame.
	initial := authenticationState(t, soID, owner, reader)
	big := initial.CloneVT()
	for range 21 {
		writeSyncOp(t, soID, big, owner, strings.Repeat("x", 512<<10))
	}
	local := newAuthenticationPeer(t, soID, owner, big)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	change, err := sobject.BuildSOConfigChange(soID, big.Config, big.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}

	// Watch for the requester being told to recover.
	recovery := make(chan peer.ID, 1)
	remote.SetPeerRecoveryObserver(func(id peer.ID, required bool) {
		if required {
			recovery <- id
		}
	})
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)
	go func() { localDone <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()

	// The sender ends the stream as too large.
	select {
	case err := <-localDone:
		if !errors.Is(err, sobject.ErrStateTooLarge) || !isTerminalSyncError(err) {
			t.Fatalf("oversized snapshot result = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("oversized snapshot did not end the stream")
	}
	select {
	case <-recovery:
	case <-ctx.Done():
		t.Fatal("requester was not told to recover")
	}
}
