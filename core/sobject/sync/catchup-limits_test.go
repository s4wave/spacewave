package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
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
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			const soID = "catchup-budget"
			owner, reader := mustKeyPair(t), mustKeyPair(t)
			initial := authenticationState(t, soID, owner, reader)
			local := newAuthenticationPeer(t, soID, owner, initial)
			remote := newAuthenticationPeer(t, soID, reader, initial)
			left, right := net.Pipe()
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
					Revision: 1, ConfigHash: cursor, ConfigSeqno: uint64(test.count), RootSeqno: 1, StateHash: bytes.Repeat([]byte{1}, 32),
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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	const soID = "catchup-terminal-snapshot"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	left, right := net.Pipe()
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}
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

	// The advertised newer root is admissible, so only the terminal stream decision can fence it.
	candidate := initial.CloneVT()
	candidate.Root.InnerSeqno++
	signSnapshotRoot(t, soID, candidate, owner)
	data, err := candidate.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
		Revision: 1, ConfigHash: candidate.Config.ConfigChainHash, ConfigSeqno: candidate.Config.ConfigChainSeqno,
		RootSeqno: candidate.Root.InnerSeqno, StateHash: digest[:],
	}}}); err != nil {
		t.Fatal(err)
	}
	request := &SOSyncMessage{}
	if err := session.RecvMsg(request); err != nil || request.GetHistoryRequest() == nil {
		t.Fatalf("newer root was not requested: %v", err)
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
		SoState: data, RootSeqno: candidate.Root.InnerSeqno, Revision: 1, BaseHash: initial.Config.ConfigChainHash,
	}}}); err != nil {
		t.Fatal(err)
	}

	// Three empty operations exceed the reader's one queued and one in-progress frame.
	// Completing these writes proves the owner handled the earlier snapshot before recovery can finish.
	for range 3 {
		if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Op{Op: &SOSyncOp{}}}); err != nil {
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
