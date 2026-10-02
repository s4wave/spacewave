package sobject

import (
	"bytes"
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanInviteConformance compares invitation admission against real signed host changes.
func TestLeanInviteConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 3)
	var cases []leanCase
	for seed := range uint64(40) {
		cases = append(cases, runLeanInviteScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// TestLeanRedeemInviteConformance compares invite redemption conditions.
func TestLeanRedeemInviteConformance(t *testing.T) {
	// The host holds Space a with one participant and Space c with none.
	held := map[string]*SharedObjectConfig{
		"a": {Participants: []*SOParticipantConfig{{PeerId: "member"}}},
		"c": {},
	}
	participations := []*SOInviteParticipation{
		nil,
		{},
		{SharedObjectIds: []string{"a"}},
		{SharedObjectIds: []string{"b"}},
		{SharedObjectIds: []string{"c"}},
		{SharedObjectIds: []string{"b", "a"}},
	}

	// Project the held configs as (Space, participants) pairs.
	var a fastjson.Arena
	heldValue := a.NewArray()
	for j, id := range []string{"a", "c"} {
		var peers []string
		for _, p := range held[id].GetParticipants() {
			peers = append(peers, p.GetPeerId())
		}
		pair := a.NewArray()
		pair.SetArrayItem(0, a.NewString(id))
		pair.SetArrayItem(1, leanStrings(&a, peers))
		heldValue.SetArrayItem(j, pair)
	}

	// Redeem every condition combination as a member and as a stranger.
	var cases []leanCase
	for _, approval := range []bool{false, true} {
		for i, participation := range participations {
			for _, peerID := range []string{"member", "stranger"} {
				req := a.NewObject()
				req.Set("op", a.NewString("redeemInvite"))
				req.Set("approvalRequired", leanBool(&a, approval))
				req.Set("participantOf", a.NewNull())
				if participation != nil {
					req.Set("participantOf", leanStrings(&a, participation.GetSharedObjectIds()))
				}
				req.Set("held", heldValue)
				req.Set("peer", a.NewString(peerID))
				inv := &SOInvite{ApprovalRequired: approval, ParticipantOf: participation}
				redemption := RedeemInvite(inv, peerID, held)
				cases = append(cases, leanCase{
					name:    "redeemInvite approval " + strconv.FormatBool(approval) + " participation " + strconv.Itoa(i) + " " + peerID,
					request: req.MarshalTo(nil), ok: true,
					field: "redemption", value: strconv.AppendInt(nil, int64(redemption), 10),
				})
			}
		}
	}

	// The Lean oracle must decide each redemption the same way.
	checkLeanCases(t, leanOracle(t), cases)
}

// leanStrings projects a string list.
func leanStrings(a *fastjson.Arena, values []string) *fastjson.Value {
	out := a.NewArray()
	for i, v := range values {
		out.SetArrayItem(i, a.NewString(v))
	}
	return out
}

// FuzzLeanInvite searches finite capacity, stale checkpoints and authority boundaries.
func FuzzLeanInvite(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(17))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanInviteScenario(t, createMockPeers(t, 3), seed))
	})
}

// runLeanInviteScenario retains real invitation bytes, signatures and provider publication.
func runLeanInviteScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	// Hold an owner and a reader with one invitation.
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 0x1a71))
	projection := &configChainScenario{t: t}
	a := &projection.arena
	base := createMockSOState(peers[:2], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	base.Config.ConfigChainHash = bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)
	base.Root = createMockSORoot(t, 1, peers[0])
	base.Invites = []*SOInvite{{InviteId: "target", TokenHash: []byte{1}, MaxUses: 2}}

	// Try each variant against fresh copies.
	var cases []leanCase
	for variant := range 30 {
		previous, snapshot := base.CloneVT(), base.CloneVT()
		input := &SOInvite{InviteId: "created", TokenHash: []byte{2}, MaxUses: rng.Uint32N(4)}
		id, signer := "target", peers[0]
		buildOK, lockOK, writeOK := true, true, true
		switch variant {
		case 0:
		case 1:
			previous.Invites[0].Uses = 1
		case 2:
			previous.Invites[0].Uses = 2
		case 3:
			previous.Invites[0].Uses = 3
		case 4:
			previous.Invites[0].Revoked = true
		case 5:
			previous.Invites[0].ExpiresAt = timestamppb.New(time.Unix(1, 0))
		case 6:
			previous.Invites[0].ExpiresAt = timestamppb.New(time.Unix(4102444800, 0))
		case 7:
			previous.Invites[0].MaxUses = 0
			previous.Invites[0].Uses = math.MaxUint32
		case 8:
			previous.Invites[0].MaxUses = math.MaxUint32
			previous.Invites[0].Uses = math.MaxUint32 - 1
		case 9:
			previous.Invites = append(previous.Invites, input.CloneVT())
		case 10:
			previous.Invites = nil
		case 11:
			previous.Invites = append([]*SOInvite{nil}, previous.Invites...)
		case 12:
			previous.Invites = append(previous.Invites, previous.Invites[0].CloneVT())
			previous.Invites[0].Uses = 2
		case 13:
			id = ""
			input.InviteId = ""
		case 14:
			input.TokenHash = nil
		case 15:
			input = nil
		case 16:
			input.MaxUses, input.Uses = 1, 2
		case 17:
			signer = peers[1]
		case 18:
			signer = peers[2]
		case 19:
			previous.Config.ConfigChainHash[0] ^= 1
		case 20:
			buildOK = false
		case 21:
			lockOK = false
		case 22:
			writeOK = false
		case 23:
			previous.Config.ConfigChainSeqno = math.MaxUint64
			snapshot.Config = previous.Config.CloneVT()
		case 24:
			input.Uses = input.MaxUses
		case 25:
			id = ""
			previous.Invites = []*SOInvite{nil, {}}
		case 26:
			id = ""
			previous.Invites = []*SOInvite{{}, nil}
		case 27:
			snapshot.Invites[0].Uses = 2
		case 28:
			snapshot.Invites[0].Revoked = true
		case 29:
			previous.Invites = append(previous.Invites, previous.Invites[0].CloneVT())
			previous.Invites[1].Uses = 2
		}
		key, err := signer.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// The clock boundary is projected directly; acceptance is decided independently.
		current := FindInvite(previous, id)
		expired := current.GetExpiresAt() != nil && time.Now().After(current.GetExpiresAt().AsTime())
		for _, kind := range []SOConfigChangeType{
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES,
		} {
			store := &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
			watch := ccontainer.NewCContainer[*SOState](snapshot.CloneVT())
			host := NewSOHost(t.Context(), func(context.Context, string, func()) (ccontainer.Watchable[*SOState], func(), error) {
				if !buildOK {
					return nil, nil, errors.New("injected watch failure")
				}
				return watch, func() {}, nil
			}, store.lock, mockSharedObjectID)
			entry, err := BuildSOConfigChange(mockSharedObjectID, snapshot.Config, snapshot.Config, kind, key, nil)
			if err != nil {
				t.Fatal(err)
			}
			projected := projection.entryJSON(entry)
			req := a.NewObject()
			req.Set("op", a.NewString("mutateInvite"))
			req.Set("previous", projectLeanState(t, a, previous))
			req.Set("snapshot", projectLeanConfig(snapshot.Config).json(a))
			req.Set("kind", a.NewNumberInt(int(kind)))
			req.Set("invite", projectLeanInvite(t, a, input))
			req.Set("id", a.NewString(id))
			req.Set("expired", leanBool(a, expired))
			req.Set("sig", projected.Get("sig"))
			req.Set("hash", projected.Get("hash"))
			req.Set("buildOK", leanBool(a, buildOK))
			req.Set("lockOK", leanBool(a, lockOK))
			req.Set("writeOK", leanBool(a, writeOK))
			switch kind {
			case SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE:
				err = host.CreateInvite(t.Context(), key, input)
			case SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE:
				err = host.RevokeInvite(t.Context(), key, id)
			case SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES:
				err = host.IncrementInviteUses(t.Context(), key, id)
			}
			cases = append(cases, leanHostCase(t, a, req, store, err, seed, variant))
		}

		name := " seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		req := a.NewObject()
		req.Set("op", a.NewString("validateInviteUsable"))
		req.Set("invite", projectLeanInvite(t, a, current))
		req.Set("expired", leanBool(a, expired))
		cases = append(cases, leanCase{
			name:    "validateInviteUsable" + name,
			request: req.MarshalTo(nil), ok: ValidateInviteUsable(current) == nil,
		})

		req = a.NewObject()
		req.Set("op", a.NewString("findInvite"))
		req.Set("invites", projectLeanState(t, a, previous).Get("invites"))
		req.Set("id", a.NewString(id))
		value := a.NewNull()
		if current != nil {
			value = projectLeanInvite(t, a, current)
		}
		cases = append(cases, leanCase{
			name: "findInvite" + name, request: req.MarshalTo(nil),
			ok: current != nil, field: "invite", value: value.MarshalTo(nil),
		})
	}
	return cases
}
