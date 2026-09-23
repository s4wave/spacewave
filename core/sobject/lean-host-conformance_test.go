package sobject

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanHostConformance exercises the real host, locks, signed snapshots and callbacks.
func TestLeanHostConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(40) {
		cases = append(cases, runLeanHostScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanHost searches snapshot, configuration and provider boundary combinations.
func FuzzLeanHost(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(13))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanHostScenario(t, createMockPeers(t, 4), seed))
	})
}

// leanHostStore supplies an atomic write callback to the real SOStateLock.
type leanHostStore struct {
	// state is the published checkpoint observed by the next lock.
	state *SOState
	// lockOK controls whether a lock can be acquired.
	lockOK bool
	// writeOK controls whether the atomic provider write succeeds.
	writeOK bool
	// writes counts successful publications for oracle comparison.
	writes int
}

// lock opens the same host boundary with independently injected lock/write failures.
func (s *leanHostStore) lock(context.Context, string) (SOStateLock, error) {
	if !s.lockOK {
		return nil, errors.New("injected lock failure")
	}
	return NewSOStateLock(s.state, func(_ context.Context, next *SOState, _ ...*SOConfigChange) error {
		if !s.writeOK {
			return errors.New("injected atomic write failure")
		}
		s.state = next
		s.writes++
		return nil
	}, func() {}), nil
}

// runLeanHostScenario compares all four host admission paths on related real states.
func runLeanHostScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 0x4057))
	projection := &configChainScenario{t: t}
	a := &projection.arena
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	base := createMockSOState(peers[:3], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	base.Config.ConfigChainHash = bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)
	base.Root = createMockSORoot(t, 1, peers[0])
	_, base.RootGrants, _, err = RotateTransformKey(owner, mockSharedObjectID, base.Config.Participants, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	base.Invites = []*SOInvite{{InviteId: "local invitation"}}
	operation := signedLeanOperation(t, owner, peers[0].GetPeerID().String(), 1, NewSOOperationLocalID())
	if err := base.QueueOperation(mockSharedObjectID, operation); err != nil {
		t.Fatal(err)
	}

	// Each case keeps its original checkpoint to check rejection immutability.
	var cases []leanCase
	for variant := range 26 {
		previous, candidate := base.CloneVT(), base.CloneVT()
		candidate.Invites = []*SOInvite{{InviteId: "remote invitation"}}
		localPeer := peers[2].GetPeerID()
		lockOK, writeOK, accessOK := true, true, true
		var entries []*SOConfigChange
		change := func(config *SharedObjectConfig) {
			t.Helper()
			entry, err := BuildSOConfigChange(previous.Config, config,
				SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
			if err != nil {
				t.Fatal(err)
			}
			candidate.Config, err = VerifyConfigChange(previous.Config, entry)
			if err != nil {
				t.Fatal(err)
			}
			entries = []*SOConfigChange{entry}
		}
		signRoot := func(root *SORoot) {
			t.Helper()
			root.ValidatorSignatures = nil
			if err := root.SignInnerData(owner, mockSharedObjectID, root.InnerSeqno, hash.RecommendedHashType); err != nil {
				t.Fatal(err)
			}
		}
		switch variant {
		case 0:
			// Exact held proof and local operations reconstruct a no-op.
		case 1:
			candidate.Root.InnerSeqno += 1 + rng.Uint64N(3)
			signRoot(candidate.Root)
		case 2:
			candidate.Root.InnerSeqno--
		case 3:
			candidate.Root.Inner = []byte("equal-sequence conflict")
			signRoot(candidate.Root)
		case 4:
			candidate.Root.ValidatorSignatures[0].SigData[0] ^= 1
		case 5:
			candidate.Config.Participants = candidate.Config.Participants[1:]
			candidate.RootGrants, candidate.Ops = nil, nil
			change(candidate.Config)
		case 6:
			candidate.Config.Participants = candidate.Config.Participants[:2]
			change(candidate.Config)
			candidate.Root = nil
		case 7:
			candidate.Config.ConfigChainHash[0] ^= 1
		case 8:
			change(candidate.Config)
			entries = nil
		case 9:
			change(candidate.Config)
			entries[0].Signature.SigData[0] ^= 1
		case 10:
			accessOK = false
		case 11:
			lockOK = false
		case 12:
			writeOK = false
			candidate.Root.InnerSeqno++
			signRoot(candidate.Root)
		case 13:
			candidate.Root.InnerSeqno++
			candidate.Root.ValidatorSignatures[0].SigData[0] ^= 1
		case 14:
			candidate.Ops = nil
			candidate.QueuedAccountNonces = nil
		case 15:
			candidate.Ops[0].Signature.SigData[0] ^= 1
		case 16:
			candidate.Ops = append(candidate.Ops, candidate.Ops[0].CloneVT())
		case 17:
			candidate.RootGrants = append(candidate.RootGrants, candidate.RootGrants[0].CloneVT())
		case 18:
			candidate.RootGrants[0].Signature.SigData[0] ^= 1
		case 19:
			previous.Root.AccountNonces[0].Nonce = 7
			signRoot(previous.Root)
			candidate.Root.InnerSeqno++
			candidate.Root.AccountNonces[0].Nonce = rng.Uint64N(7)
			signRoot(candidate.Root)
		case 20:
			localPeer = peers[3].GetPeerID()
		case 21:
			candidate.Ops = []*SOOperation{signedLeanOperation(t, owner, peers[0].GetPeerID().String(), 2, NewSOOperationLocalID())}
		case 22:
			previous.Ops[0].Inner = []byte{0xff}
			candidate.Ops, candidate.QueuedAccountNonces = nil, nil
		case 23:
			candidate.Root.InnerSeqno++
			candidate.Root.AccountNonces[0].Nonce = 1
			candidate.Ops, candidate.QueuedAccountNonces = nil, nil
			signRoot(candidate.Root)
		case 24:
			candidate.OpRejections = []*SOPeerOpRejections{{PeerId: peers[0].GetPeerID().String()}}
		case 25:
			candidate.Config.ConsensusMode = SOConsensusMode(99)
			change(candidate.Config)
			candidate.Root.InnerSeqno++
			signRoot(candidate.Root)
		}

		// Import exposes committed revocation separately from ordinary errors.
		store := &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		host := NewSOHost(nil, nil, store.lock, mockSharedObjectID)
		req := a.NewObject()
		req.Set("op", a.NewString("importPeerSnapshot"))
		req.Set("previous", projectLeanState(t, a, previous))
		req.Set("candidate", projectLeanState(t, a, candidate))
		req.Set("entries", projection.entriesJSON(entries))
		req.Set("localPeer", a.NewString(localPeer.String()))
		req.Set("candidateBytes", a.NewNumberInt(candidate.SizeVT()))
		historyBytes := 0
		for _, entry := range entries {
			historyBytes += entry.SizeVT()
		}
		req.Set("historyBytes", a.NewNumberInt(historyBytes))
		req.Set("lockOK", leanBool(a, lockOK))
		req.Set("writeOK", leanBool(a, writeOK))
		req.Set("accessOK", leanBool(a, accessOK))
		err = host.ImportPeerSnapshot(t.Context(), candidate, entries, localPeer, func(context.Context, *SOState) error {
			if !accessOK {
				return errors.New("injected access failure")
			}
			return nil
		})
		cases = append(cases, leanHostCase(t, a, req, store, err, seed, variant))

		// Local callbacks may fail after modifying their independent working copy.
		store = &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		host = NewSOHost(nil, nil, store.lock, mockSharedObjectID)
		entry, buildErr := BuildSOConfigChange(previous.Config, previous.Config,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		switch variant % 5 {
		case 1:
			entry.ConfigSeqno--
			signConfigChange(t, entry, owner)
		case 2:
			entry.Signature.SigData[0] ^= 1
		case 3:
			entry = nil
		}
		callbackOK := variant%4 != 1
		var replacement *SOState
		if variant%4 == 2 {
			replacement = candidate.CloneVT()
		}
		req = a.NewObject()
		req.Set("op", a.NewString("applyConfigChange"))
		req.Set("previous", projectLeanState(t, a, previous))
		req.Set("entry", a.NewNull())
		if entry != nil {
			req.Set("entry", projection.entryJSON(entry))
		}
		req.Set("replacement", a.NewNull())
		if replacement != nil {
			req.Set("replacement", projectLeanState(t, a, replacement))
		}
		req.Set("callbackOK", leanBool(a, callbackOK))
		req.Set("lockOK", leanBool(a, lockOK))
		req.Set("writeOK", leanBool(a, writeOK))
		err = host.ApplyConfigChange(t.Context(), entry, func(next *SOState) error {
			if !callbackOK {
				next.Root, next.Config = nil, nil
				return errors.New("injected callback failure")
			}
			if replacement != nil {
				*next = *replacement.CloneVT()
			}
			return nil
		})
		cases = append(cases, leanHostCase(t, a, req, store, err, seed, variant))

		// Invitation installation uses the explicit checkpoint boundary.
		store = &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		sync := &SOHostSyncFuncs{CheckpointLock: store.lock}
		checkpoint := variant%7 != 0
		if !checkpoint {
			sync.CheckpointLock = nil
		}
		host = NewSOHost(nil, nil, store.lock, mockSharedObjectID, sync)
		req = a.NewObject()
		req.Set("op", a.NewString("installInviteSnapshot"))
		req.Set("previous", projectLeanState(t, a, previous))
		req.Set("candidate", projectLeanState(t, a, candidate))
		req.Set("checkpoint", leanBool(a, checkpoint))
		req.Set("lockOK", leanBool(a, lockOK))
		req.Set("writeOK", leanBool(a, writeOK))
		err = host.InstallInviteSnapshot(t.Context(), candidate)
		cases = append(cases, leanHostCase(t, a, req, store, err, seed, variant))

		// Host root admission discards mutations rejected by the state validator.
		store = &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		host = NewSOHost(nil, nil, store.lock, mockSharedObjectID)
		req = a.NewObject()
		req.Set("op", a.NewString("hostUpdateRootState"))
		req.Set("previous", projectLeanState(t, a, previous))
		req.Set("root", projectLeanRoot(t, a, candidate.Root))
		req.Set("enforce", a.NewString(""))
		req.Set("rejected", a.NewArray())
		req.Set("accepted", a.NewArray())
		req.Set("lockOK", leanBool(a, lockOK))
		req.Set("writeOK", leanBool(a, writeOK))
		err = host.UpdateRootState(t.Context(), candidate.Root, "", nil, nil)
		cases = append(cases, leanHostCase(t, a, req, store, err, seed, variant))
	}
	return cases
}

// leanHostCase records every visible state, including ordinary failed calls.
func leanHostCase(t *testing.T, a *fastjson.Arena, request *fastjson.Value,
	store *leanHostStore, err error, seed uint64, variant int,
) leanCase {
	t.Helper()
	revoked := errors.Is(err, ErrParticipantRevoked)
	outcome := a.NewObject()
	outcome.Set("state", projectLeanState(t, a, store.state))
	outcome.Set("revoked", leanBool(a, revoked))
	outcome.Set("wrote", leanBool(a, store.writes != 0))
	return leanCase{
		name:    string(request.GetStringBytes("op")) + " seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
		request: request.MarshalTo(nil), ok: err == nil || revoked,
		field: "outcome", value: outcome.MarshalTo(nil),
	}
}
