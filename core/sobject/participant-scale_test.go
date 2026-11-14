package sobject

import (
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// scaleSpace is a Space held by one host, with its participants' keys.
type scaleSpace struct {
	// host is the owner's host of the Space.
	host *SOHost
	// state is the host's current state.
	state **SOState
	// peers are the participants in join order; peers[0] is the owner.
	peers []peer.Peer
	// keys are the private keys of peers.
	keys []crypto.PrivKey
}

// newScaleSpace returns a Space of count new peers, owned by the first. The
// first members peers are writers holding the current key; the rest may join.
// The genesis state grants all members at once, since joining each through the
// API costs more as the Space grows.
func newScaleSpace(t testing.TB, count, members int) *scaleSpace {
	t.Helper()

	// Create the peers and their keys.
	peers := createMockPeers(t, uint64(count))
	keys := mustPrivKeys(t, peers)

	// Hold the genesis state, which grants the key to every member.
	initial, genesis := newTestSOState(t, peers[:members])
	host, state := newLeaveTestHost(t, initial.GetConfig(), genesis)
	*state = initial
	return &scaleSpace{host: host, state: state, peers: peers, keys: keys}
}

// join adds a peer as a writer through AddSOParticipant, as the owner.
func (s *scaleSpace) join(t testing.TB, p peer.Peer) {
	t.Helper()

	// Add the peer as the owner.
	id := p.GetPeerID().String()
	grant, outcome, _, err := AddSOParticipant(
		t.Context(), s.host, mockSharedObjectID, s.keys[0], s.peers[0].GetPeerID().String(),
		id, p.GetPubKey(), SOParticipantRole_SOParticipantRole_WRITER, "", "",
	)
	if err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
	if grant == nil || outcome != SOParticipantAddOutcome_SOParticipantAddOutcome_ADDED {
		t.Fatalf("join %s: outcome %v", id, outcome)
	}
}

// depart removes a participant as the owner and rotates the transform key to
// the remaining readers, as the provider does after NeedsKeyRotation.
func (s *scaleSpace) depart(t testing.TB, id string) {
	t.Helper()

	// Remove the peer, which drops its grants.
	removed, err := RemoveSOParticipant(t.Context(), s.host, id, s.keys[0], nil)
	if err != nil {
		t.Fatalf("remove %s: %v", id, err)
	}
	if !removed {
		t.Fatalf("remove %s: peer was not a participant", id)
	}

	// Grant a new key to the remaining readers.
	state := *s.state
	_, epoch, err := RotateTransformKey(
		s.keys[0], mockSharedObjectID, state.GetConfig().GetParticipants(),
		state.CurrentKeyEpoch().GetEpoch(), state.GetConfig().GetConfigChainSeqno(),
	)
	if err != nil {
		t.Fatal(err)
	}
	state.SetKeyEpoch(epoch)
}

// TestSpaceReachesThreeHundredParticipants joins 300 peers one after another
// through AddSOParticipant, then removes one and re-keys. Every join succeeds,
// the state validates, and every remaining participant reads the new key.
func TestSpaceReachesThreeHundredParticipants(t *testing.T) {
	// The owner alone creates the Space.
	const count = 300
	space := newScaleSpace(t, count, 1)
	peers, keys, state := space.peers, space.keys, space.state

	// Every join succeeds, one after another.
	for i := 1; i < count; i++ {
		space.join(t, peers[i])
	}
	joined := *state
	if got := len(joined.GetConfig().GetParticipants()); got != count {
		t.Fatalf("space has %d participants, want %d", got, count)
	}
	if err := joined.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("300 participants do not validate: %v", err)
	}
	if err := joined.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("300 participants do not authorize: %v", err)
	}

	// A departure re-keys the Space.
	departed := peers[count/2].GetPeerID().String()
	oldEpoch := joined.CurrentKeyEpoch().GetEpoch()
	space.depart(t, departed)
	next := *state
	if err := next.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("state after departure does not validate: %v", err)
	}
	epoch := next.CurrentKeyEpoch()
	if epoch.GetEpoch() != oldEpoch+1 {
		t.Fatalf("key epoch is %d, want %d", epoch.GetEpoch(), oldEpoch+1)
	}
	if epoch.FindGrant(departed) != nil {
		t.Fatal("departed participant holds a grant to the new key")
	}

	// Every remaining participant reads the new key.
	if got := len(epoch.GetGrants()); got != count-1 {
		t.Fatalf("new epoch grants %d participants, want %d", got, count-1)
	}
	for i, p := range peers {
		id := p.GetPeerID().String()
		if id == departed {
			continue
		}
		grant := epoch.FindGrant(id)
		if grant == nil {
			t.Fatalf("participant %d has no grant to the new key", i)
		}
		if _, err := grant.DecryptInnerData(keys[i], mockSharedObjectID); err != nil {
			t.Fatalf("participant %d cannot read the new key: %v", i, err)
		}
	}
}

// BenchmarkParticipantScale times one join and one departure with re-keying
// in a Space of 100, 300 and 1000 participants. Run it with -benchtime=1x; each
// iteration builds a fresh Space outside the timer.
func BenchmarkParticipantScale(b *testing.B) {
	for _, size := range []int{100, 300, 1000} {
		b.Run("join/"+strconv.Itoa(size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				space := newScaleSpace(b, size+1, size)
				joiner := space.peers[size]
				b.StartTimer()
				start := time.Now()
				space.join(b, joiner)
				b.ReportMetric(time.Since(start).Seconds(), "s/join")
			}
		})
		b.Run("depart/"+strconv.Itoa(size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				space := newScaleSpace(b, size, size)
				departed := space.peers[size-1].GetPeerID().String()
				b.StartTimer()
				start := time.Now()
				space.depart(b, departed)
				b.ReportMetric(time.Since(start).Seconds(), "s/depart")
			}
		})
	}
}

// TestAddSOParticipantRaiseNeedsOwner proves that only an owner raises a
// participant: a writer's attempt to raise a reader to writer fails at the
// host's verification and leaves the config as it was, and the owner's
// succeeds and reports RAISED.
func TestAddSOParticipantRaiseNeedsOwner(t *testing.T) {
	// Hold a Space of an owner, a writer and a reader.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	initial, genesis := newTestSOState(t, peers, SOParticipantRole_SOParticipantRole_OWNER, SOParticipantRole_SOParticipantRole_WRITER, SOParticipantRole_SOParticipantRole_READER)
	host, state := newLeaveTestHost(t, initial.GetConfig(), genesis)
	*state = initial
	reader := peers[2].GetPeerID().String()
	writer := SOParticipantRole_SOParticipantRole_WRITER

	// The writer cannot raise the reader.
	before := (*state).GetConfig().GetConfigChainSeqno()
	_, outcome, _, err := AddSOParticipant(t.Context(), host, mockSharedObjectID, keys[1], peers[1].GetPeerID().String(), reader, peers[2].GetPubKey(), writer, "", "")
	if err == nil {
		t.Fatalf("writer raised the reader: outcome %v", outcome)
	}
	if got := (*state).GetConfig().GetParticipantRole(reader); got != SOParticipantRole_SOParticipantRole_READER {
		t.Fatalf("reader role after a refused raise is %v", got)
	}
	if got := (*state).GetConfig().GetConfigChainSeqno(); got != before {
		t.Fatalf("refused raise moved the config from %d to %d", before, got)
	}

	// The owner raises it.
	grant, outcome, held, err := AddSOParticipant(t.Context(), host, mockSharedObjectID, keys[0], peers[0].GetPeerID().String(), reader, peers[2].GetPubKey(), writer, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != SOParticipantAddOutcome_SOParticipantAddOutcome_RAISED || held != writer || grant != nil {
		t.Fatalf("owner raise: outcome %v held %v grant %v", outcome, held, grant != nil)
	}
	if got := (*state).GetConfig().GetParticipantRole(reader); got != writer {
		t.Fatalf("reader role after the raise is %v", got)
	}
	if err := (*state).Validate(mockSharedObjectID); err != nil {
		t.Fatalf("state after the raise does not validate: %v", err)
	}
}

// TestAddSOParticipantLowerRoleUnchanged proves that adding a participant at a
// lower role than it holds reports UNCHANGED with the held role and writes
// nothing.
func TestAddSOParticipantLowerRoleUnchanged(t *testing.T) {
	// Hold a Space of an owner and a writer.
	space := newScaleSpace(t, 2, 2)
	before := *space.state
	target := space.peers[1].GetPeerID().String()

	// Add the writer again as a reader.
	grant, outcome, held, err := AddSOParticipant(
		t.Context(), space.host, mockSharedObjectID, space.keys[0], space.peers[0].GetPeerID().String(),
		target, space.peers[1].GetPubKey(), SOParticipantRole_SOParticipantRole_READER, "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != SOParticipantAddOutcome_SOParticipantAddOutcome_UNCHANGED || held != SOParticipantRole_SOParticipantRole_WRITER || grant != nil {
		t.Fatalf("lower add: outcome %v held %v grant %v", outcome, held, grant != nil)
	}
	if *space.state != before {
		t.Fatal("lower add wrote a new state")
	}
}
