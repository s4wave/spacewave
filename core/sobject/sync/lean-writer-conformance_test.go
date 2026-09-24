package sobject_sync

import (
	"context"
	"encoding/hex"
	"math/rand/v2"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestLeanSyncWriterConformance checks actual worker admission, transport and cancellation traces.
func TestLeanSyncWriterConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(8) {
		cases = append(cases, leanSyncWriterCases(t, seed)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncWriter varies current roles, serialized frame kinds and worker exit boundaries.
func FuzzLeanSyncWriter(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(17))
	f.Fuzz(func(t *testing.T, seed uint64) {
		oracle := leanSyncOracle(t)
		checkLeanSync(t, oracle, leanSyncWriterCases(t, seed))
	})
}

// leanSyncWriterCases drives the same writer routine used by synchronize over a real pipe.
func leanSyncWriterCases(t *testing.T, seed uint64) []leanSyncCase {
	t.Helper()
	const soID = "lean-sync-writer"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	localID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}
	local := &SOSync{localObjectPeerID: localID}
	rng := rand.New(rand.NewPCG(seed, seed^0x738a))
	var cases []leanSyncCase
	for variant := range 12 {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		t.Cleanup(cancel)
		host, ctr := newMemHost(soID, initial.CloneVT())
		t.Cleanup(host.ClearContext)
		states, release, err := host.GetSOStateCtr(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		left, right := net.Pipe()
		observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 8)}
		outbound, sent := make(chan *SOSyncMessage), make(chan error)
		done := make(chan error, 1)
		stop := context.AfterFunc(ctx, func() { left.Close(); right.Close() })
		go func() {
			done <- local.writeMessages(ctx, stream_packet.NewSession(observed, maxMessageSize), remote, states, outbound, sent)
		}()

		// Keep all later inputs in the oracle trace, including attempted regrant after failure.
		var arena fastjson.Arena
		attempts, frames, results := arena.NewArray(), arena.NewArray(), arena.NewArray()
		frameCount, resultCount := 0, 0
		waiting, joined := true, false
		t.Cleanup(func() {
			cancel()
			left.Close()
			right.Close()
			if !joined {
				<-done
			}
		})
		for index := range 4 {
			current := initial.CloneVT()
			selected, writeOK, reported := true, true, true
			if index == 1 {
				switch variant {
				case 1:
					current.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
				case 2:
					current.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
				case 3:
					current.Config.ConfigChainHash = nil
				case 4:
					writeOK = false
				case 5:
					selected = false
				case 6:
					reported = false
				case 7:
					writeOK, reported = false, false
				case 8:
					current.Config.Participants[1].Role = sobject.SOParticipantRole(-1 - rng.IntN(8))
					writeOK = false
				case 9:
					current.Config.Participants = append(current.Config.Participants,
						participantCfg(remote.String(), sobject.SOParticipantRole_SOParticipantRole_UNKNOWN))
				case 10:
					current.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_WRITER
				case 11:
					current.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
					reported = false
				}
			}
			kinds := []int{1, 3, 7, 9, 10}
			kind := kinds[rng.IntN(len(kinds))]
			message := leanSyncWriterMessage(kind)
			attempt := arena.NewObject()
			attempt.Set("selected", leanSyncBool(&arena, selected))
			attempt.Set("participants", leanSyncParticipants(&arena, current.Config.Participants))
			attempt.Set("hash", arena.NewString(hex.EncodeToString(current.Config.ConfigChainHash)))
			attempt.Set("kind", arena.NewNumberInt(kind))
			attempt.Set("writeOK", leanSyncBool(&arena, writeOK))
			attempt.Set("reported", leanSyncBool(&arena, reported))
			attempts.SetArrayItem(index, attempt)
			if !waiting {
				continue
			}
			ctr.SetValue(current)
			if !selected {
				cancel()
				<-done
				waiting, joined = false, true
				continue
			}
			select {
			case outbound <- message:
			case <-ctx.Done():
				t.Fatal("writer did not accept selected handoff")
			}
			var frame *SOSyncMessage
			select {
			case frame = <-observed.messages:
			case <-ctx.Done():
				t.Fatal("writer did not admit a transport frame")
			}
			frames.SetArrayItem(frameCount, arena.NewNumberInt(leanSyncWriterKind(frame)))
			frameCount++
			if writeOK {
				received := &SOSyncMessage{}
				if err := stream_packet.NewSession(right, maxMessageSize).RecvMsg(received); err != nil {
					t.Fatal(err)
				}
			} else {
				right.Close()
			}
			if !reported {
				cancel()
				<-done
				waiting, joined = false, true
				continue
			}
			select {
			case err := <-sent:
				results.SetArrayItem(resultCount, leanSyncBool(&arena, err == nil))
				resultCount++
				if err != nil {
					<-done
					waiting, joined = false, true
				}
			case <-ctx.Done():
				t.Fatal("writer did not report completed transport operation")
			}
		}

		// Join the real worker before testing that no later channel handoff is admitted.
		cancel()
		left.Close()
		right.Close()
		if !joined {
			<-done
			joined = true
		}
		select {
		case outbound <- leanSyncWriterMessage(7):
			t.Fatal("joined writer accepted another frame")
		default:
		}
		stop()
		release()
		host.ClearContext()
		request, expected, result := arena.NewObject(), arena.NewObject(), arena.NewObject()
		request.Set("op", arena.NewString("writeSyncFrames"))
		request.Set("local", arena.NewString(local.localObjectPeerID.String()))
		request.Set("remote", arena.NewString(remote.String()))
		request.Set("attempts", attempts)
		result.Set("frames", frames)
		result.Set("results", results)
		result.Set("waiting", leanSyncBool(&arena, waiting))
		expected.Set("ok", leanSyncBool(&arena, waiting))
		expected.Set("writer", result)
		cases = append(cases, leanSyncCase{
			name:    "writeSyncFrames seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: request.MarshalTo(nil), expected: expected.MarshalTo(nil),
		})
	}
	return cases
}

// leanSyncWriterMessage supplies real bodies for each modeled data/control frame kind.
func leanSyncWriterMessage(kind int) *SOSyncMessage {
	switch kind {
	case 1:
		return &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: []byte("snapshot")}}}
	case 3:
		return syncAcknowledgment(1)
	case 7:
		return &SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{Revision: 1}}}
	case 9:
		return &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{Revision: 1}}}
	default:
		return &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: 1}}}
	}
}

// leanSyncWriterKind reads the serialized body tag from an observed real write.
func leanSyncWriterKind(message *SOSyncMessage) int {
	switch message.GetBody().(type) {
	case *SOSyncMessage_Snapshot:
		return 1
	case *SOSyncMessage_Ack:
		return 3
	case *SOSyncMessage_Authorization:
		return 6
	case *SOSyncMessage_Head:
		return 7
	case *SOSyncMessage_HistoryPage:
		return 9
	case *SOSyncMessage_RecoveryRequired:
		return 10
	default:
		return 0
	}
}
