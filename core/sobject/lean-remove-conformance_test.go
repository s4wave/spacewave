package sobject

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanRemovalConformance compares real atomic removal and encrypted proof replacement.
func TestLeanRemovalConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(40) {
		cases = append(cases, runLeanRemovalScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanRemoval searches grant, root, authority and checkpoint combinations.
func FuzzLeanRemoval(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(11))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanRemovalScenario(t, createMockPeers(t, 4), seed))
	})
}

// runLeanRemovalScenario projects primitive crypto results independently of membership admission.
func runLeanRemovalScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	projection := &configChainScenario{t: t}
	a := &projection.arena
	creator, err := peers[0].GetPrivKey(t.Context())
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
	material, grants, _, err := RotateTransformKey(creator, mockSharedObjectID, base.Config.Participants, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	base.RootGrants = grants
	var cases []leanCase
	for variant := range 30 {
		previous := base.CloneVT()
		targets := []string{peers[0].GetPeerID().String()}
		signer := peers[1]
		watchOK, lockOK, writeOK := true, true, true
		switch variant {
		case 0:
		case 1:
			targets = []string{peers[2].GetPeerID().String()}
		case 2:
			targets = []string{peers[0].GetPeerID().String(), peers[2].GetPeerID().String()}
		case 3:
			targets = []string{"", peers[3].GetPeerID().String()}
		case 4:
			targets = []string{"", ""}
			watchOK = false
		case 5:
			targets = append(targets, targets[0])
		case 6:
			targets = []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String()}
		case 7:
			signer = peers[2]
		case 8:
			signer = peers[3]
		case 9:
			previous.RootGrants = previous.RootGrants[:1]
		case 10:
			previous.RootGrants[1].Signature.SigData[0] ^= 1
		case 11:
			previous.RootGrants[1].InnerData[0] ^= 1
		case 12:
			previous.RootGrants[2].Signature.PubKey = []byte{0xff}
		case 13:
			previous.RootGrants[2].PeerId = "invalid peer"
		case 14:
			previous.RootGrants = append(previous.RootGrants, previous.RootGrants[2].CloneVT())
		case 15:
			previous.Root.ValidatorSignatures[0].SigData[0] ^= 1
		case 16:
			previous.Root.ValidatorSignatures[0].PubKey = []byte{0xff}
		case 17:
			previous.Root.ValidatorSignatures = nil
		case 18:
			previous.Root = nil
		case 19:
			previous.Root.Inner = nil
		case 20:
			previous.Config.ConsensusMode = SOConsensusMode(99)
		case 21:
			watchOK = false
		case 22:
			lockOK = false
		case 23:
			writeOK = false
		case 24:
			previous.Config.ConfigChainSeqno = math.MaxUint64
		case 25:
			previous.Config = nil
		case 26:
			previous.RootGrants = append(previous.RootGrants, nil)
		case 27:
			targets = []string{peers[2].GetPeerID().String()}
			previous.RootGrants[0].Signature.SigData[0] ^= 1
		case 28:
			targets = []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String()}
			previous.Root = nil
		case 29:
			previous.Root.InnerSeqno += seed % 7
		}
		snapshot := previous.CloneVT()
		key, err := signer.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		targeted := func(id string) bool { return id != "" && slices.Contains(targets, id) }
		nextCfg := snapshot.Config.CloneVT()
		if nextCfg != nil {
			nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(p *SOParticipantConfig) bool {
				return targeted(p.GetPeerId())
			})
		}
		entry, err := BuildSOConfigChange(snapshot.Config, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, key, nil)
		if err != nil {
			t.Fatal(err)
		}
		projected := projection.entryJSON(entry)

		// Decryption and recipient-key parsing are independent primitive calls.
		retained := slices.DeleteFunc(slices.Clone(previous.RootGrants), func(g *SOGrant) bool {
			return targeted(g.GetPeerId())
		})
		var inner *SOGrantInner
		for _, grant := range retained {
			if grant.GetPeerId() == signer.GetPeerID().String() {
				inner, _ = grant.DecryptInnerData(key, mockSharedObjectID)
				break
			}
		}
		store := &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		watch := ccontainer.NewCContainer[*SOState](snapshot)
		host := NewSOHost(t.Context(), func(context.Context, string, func()) (ccontainer.Watchable[*SOState], func(), error) {
			if !watchOK {
				return nil, nil, errors.New("injected watch failure")
			}
			return watch, func() {}, nil
		}, store.lock, mockSharedObjectID)
		removed, callErr := RemoveSOParticipants(t.Context(), host, targets, key, nil)

		// Fresh encryption and root serialization stay opaque. The model computes
		// recipients, authority, replacement selection and the entire visible state.
		wraps := a.NewArray()
		for i, grant := range retained {
			_, _, pubErr := peer.ParsePeerIDWithPubKey(grant.GetPeerId())
			wrap := a.NewObject()
			wrap.Set("publicKey", leanBool(a, pubErr == nil))
			wrap.Set("encryptOK", leanBool(a, inner != nil && inner.Validate() == nil))
			data := ""
			if i < len(store.state.RootGrants) && store.state.RootGrants[i] != nil {
				data = hex.EncodeToString(mustMarshalVT(t, store.state.RootGrants[i]))
			}
			wrap.Set("data", a.NewString(data))
			wraps.SetArrayItem(i, wrap)
		}
		root := projectLeanRoot(t, a, store.state.Root)
		crypto := a.NewObject()
		crypto.Set("decrypted", leanBool(a, inner != nil))
		crypto.Set("wraps", wraps)
		crypto.Set("rootSignOK", leanBool(a, true))
		crypto.Set("rootData", root.Get("data"))
		crypto.Set("rootFormat", root.Get("format"))
		req := a.NewObject()
		req.Set("op", a.NewString("removeParticipants"))
		req.Set("previous", projectLeanState(t, a, previous))
		req.Set("snapshot", a.NewNull())
		if snapshot.Config != nil {
			req.Set("snapshot", projectLeanConfig(snapshot.Config).json(a))
		}
		targetJSON := a.NewArray()
		for i, target := range targets {
			targetJSON.SetArrayItem(i, a.NewString(target))
		}
		req.Set("targets", targetJSON)
		req.Set("sig", projected.Get("sig"))
		req.Set("hash", projected.Get("hash"))
		req.Set("crypto", crypto)
		req.Set("watchOK", leanBool(a, watchOK))
		req.Set("buildOK", leanBool(a, true))
		req.Set("lockOK", leanBool(a, lockOK))
		req.Set("writeOK", leanBool(a, writeOK))
		outcome := a.NewObject()
		outcome.Set("state", projectLeanState(t, a, store.state))
		outcome.Set("revoked", leanBool(a, false))
		outcome.Set("wrote", leanBool(a, store.writes != 0))
		result := a.NewObject()
		result.Set("outcome", outcome)
		removedJSON := a.NewArray()
		for i, id := range removed {
			removedJSON.SetArrayItem(i, a.NewString(id))
		}
		result.Set("removed", removedJSON)
		cases = append(cases, leanCase{
			name:    "removeParticipants seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: req.MarshalTo(nil), ok: callErr == nil, field: "removal", value: result.MarshalTo(nil),
		})
		if callErr == nil && store.writes != 0 && variant == 0 {
			for _, grant := range store.state.RootGrants {
				for _, recipient := range peers {
					if recipient.GetPeerID().String() != grant.GetPeerId() {
						continue
					}
					private, err := recipient.GetPrivKey(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					decoded, err := grant.DecryptInnerData(private, mockSharedObjectID)
					if err != nil || !decoded.GetTransformConf().EqualVT(material) {
						t.Fatalf("removal did not preserve the retained reader's key: %v", err)
					}
				}
			}
		}
	}
	return cases
}
