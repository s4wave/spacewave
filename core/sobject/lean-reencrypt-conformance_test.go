package sobject

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/fastjson"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestLeanReencryptConformance compares real decryption and clean destination construction.
func TestLeanReencryptConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(30) {
		cases = append(cases, runLeanReencryptScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanReencrypt searches source authority, plaintext and destination membership combinations.
func FuzzLeanReencrypt(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(7))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanReencryptScenario(t, createMockPeers(t, 4), seed))
	})
}

// runLeanReencryptScenario uses actual encrypted source data and verifies every destination key.
func runLeanReencryptScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	var arena fastjson.Arena
	a := &arena
	le := logrus.New().WithField("test", t.Name())
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	keys := make(map[string]crypto.PrivKey)
	for _, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[p.GetPeerID().String()] = key
	}
	owner := keys[peers[0].GetPeerID().String()]
	base := createMockSOState(peers[:3], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
		SOParticipantRole_SOParticipantRole_VALIDATOR,
	})
	base.Config.ConfigChainHash = bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)
	base.Config.ConfigChainSeqno = 12
	material, grants, _, err := RotateTransformKey(owner, mockSharedObjectID, base.Config.Participants, 4, 7)
	if err != nil {
		t.Fatal(err)
	}
	transform, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, material)
	if err != nil {
		t.Fatal(err)
	}
	plain := &SORootInner{Seqno: 7, StateData: []byte("payload " + strconv.FormatUint(seed, 10))}
	ciphertext, err := transform.EncodeBlock(mustMarshalVT(t, plain))
	if err != nil {
		t.Fatal(err)
	}
	base.Root = &SORoot{
		Inner: ciphertext, InnerSeqno: 7,
		AccountNonces: []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 9}},
	}
	if err := base.Root.SignInnerData(owner, mockSharedObjectID, 7, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	base.RootGrants = grants
	base.Invites = []*SOInvite{{InviteId: "local invitation"}}
	base.Ops = []*SOOperation{{Inner: []byte{0xff}}}
	base.QueuedAccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 10}}
	base.OpRejections = []*SOPeerOpRejections{{PeerId: peers[0].GetPeerID().String()}}

	var cases []leanCase
	for variant := range 34 {
		source := base.CloneVT()
		destination := base.Config.CloneVT().Participants[:2]
		sourceID, destinationID := mockSharedObjectID, "reencrypted-destination"
		sourceKey, destinationKey := owner, owner
		factories := sfs
		switch variant {
		case 0:
		case 1:
			sourceKey = keys[peers[1].GetPeerID().String()]
		case 2:
			sourceKey = keys[peers[3].GetPeerID().String()]
		case 3:
			destinationKey = keys[peers[1].GetPeerID().String()]
		case 4:
			destinationKey = keys[peers[3].GetPeerID().String()]
		case 5:
			destination = base.Config.CloneVT().Participants
			destinationKey = keys[peers[2].GetPeerID().String()]
		case 6:
			destination[0].Role = SOParticipantRole_SOParticipantRole_VALIDATOR
		case 7:
			destination[1].Role = SOParticipantRole(99)
		case 8:
			destination = append(destination, destination[0].CloneVT())
		case 9:
			destination = append(destination, nil)
		case 10:
			destination[1].PeerId = "invalid peer"
		case 11:
			destination = nil
		case 12:
			source.Root.ValidatorSignatures[0].SigData[0] ^= 1
		case 13:
			source.RootGrants[0].Signature.SigData[0] ^= 1
		case 14:
			source.RootGrants[0].InnerData[0] ^= 1
		case 15:
			source.RootGrants = source.RootGrants[1:]
		case 16:
			source.RootGrants = append(source.RootGrants, source.RootGrants[0].CloneVT())
		case 17:
			source.RootGrants = append(source.RootGrants, nil)
		case 18:
			source.Config.Participants = source.Config.Participants[1:]
		case 19:
			source.Config.ConsensusMode = SOConsensusMode(99)
		case 20:
			source.Root.InnerSeqno = 0
		case 21:
			source.Root.InnerSeqno++
			source.Root.ValidatorSignatures = nil
			if err := source.Root.SignInnerData(owner, mockSharedObjectID, source.Root.InnerSeqno, hash.RecommendedHashType); err != nil {
				t.Fatal(err)
			}
		case 22:
			source.Root = nil
		case 23:
			source.Config = nil
		case 24:
			source = nil
		case 25:
			sourceKey = nil
		case 26:
			destinationKey = nil
		case 27:
			sourceID = ""
		case 28:
			destinationID = ""
		case 29:
			factories = nil
		case 30:
			factories = block_transform.NewStepFactorySet()
		case 31:
			sourceID = "wrong-source-context"
		case 32:
			source.Config.Participants[2].Role = SOParticipantRole_SOParticipantRole_OWNER
			source.Config.Participants = source.Config.Participants[1:]
			source.RootGrants = source.RootGrants[1:]
			sourceKey = keys[peers[2].GetPeerID().String()]
		case 33:
			source.Config.Participants = append(source.Config.Participants, source.Config.Participants[0].CloneVT())
		}
		sourcePeer, destinationPeer := "", ""
		if sourceKey != nil {
			id, err := peer.IDFromPrivateKey(sourceKey)
			if err != nil {
				t.Fatal(err)
			}
			sourcePeer = id.String()
		}
		if destinationKey != nil {
			id, err := peer.IDFromPrivateKey(destinationKey)
			if err != nil {
				t.Fatal(err)
			}
			destinationPeer = id.String()
		}
		decoded := decodeLeanRoot(t, le, factories, sourceID, source, sourceKey, sourcePeer)
		before := source.CloneVT()
		next, callErr := ReencryptSOState(t.Context(), le, factories, sourceID, source, sourceKey,
			destinationID, destinationKey, destination)
		if !source.EqualVT(before) {
			t.Fatal("re-encryption mutated the source")
		}
		input := a.NewObject()
		input.Set("sourceId", a.NewString(sourceID))
		input.Set("destinationId", a.NewString(destinationID))
		input.Set("sourcePeer", a.NewString(sourcePeer))
		input.Set("destinationPeer", a.NewString(destinationPeer))
		input.Set("factory", leanBool(a, factories != nil))
		input.Set("source", a.NewNull())
		if source != nil {
			input.Set("source", projectLeanReencryptState(t, a, source, sourceID))
		}
		input.Set("participants", projectLeanRotationPeers(a, destination))
		input.Set("decoded", projectLeanPlainRoot(a, decoded))
		input.Set("key", a.NewString(""))
		input.Set("grantData", a.NewArray())
		input.Set("rootData", a.NewString(""))
		input.Set("rootContent", a.NewString(""))
		input.Set("rootDigest", a.NewString(""))
		// Rejected admissions still get independently generated crypto outputs,
		// so empty output bytes cannot hide a missing admission check in Lean.
		annotations := next
		if annotations == nil {
			annotations = encryptLeanDestination(t, le, factories, destinationID, destinationKey, destination, decoded)
		}
		input.Set("cryptoOK", leanBool(a, annotations != nil))
		result := a.NewNull()
		if annotations != nil {
			projected := projectLeanReencryptState(t, a, annotations, destinationID)
			root := projected.Get("root")
			input.Set("rootData", root.Get("data"))
			input.Set("rootContent", root.Get("content"))
			input.Set("rootDigest", root.Get("digest"))
			grantData, capabilities := a.NewArray(), a.NewArray()
			for i, grant := range annotations.RootGrants {
				inner, err := grant.DecryptInnerData(keys[grant.GetPeerId()], destinationID)
				if err != nil {
					t.Fatal(err)
				}
				identity := leanTransformIdentity(t, inner.GetTransformConf())
				if i == 0 {
					input.Set("key", a.NewString(identity))
				}
				capability := a.NewObject()
				capability.Set("peer", a.NewString(grant.GetPeerId()))
				capability.Set("key", a.NewString(identity))
				capabilities.SetArrayItem(i, capability)
				grantData.SetArrayItem(i, a.NewString(hex.EncodeToString(mustMarshalVT(t, grant))))
			}
			input.Set("grantData", grantData)
			if next != nil {
				result = a.NewObject()
				result.Set("state", projected)
				result.Set("keys", capabilities)
				result.Set("plain", projectLeanPlainRoot(a,
					decodeLeanRoot(t, le, factories, destinationID, next, destinationKey, destinationPeer)))
			}
		}
		req := a.NewObject()
		req.Set("op", a.NewString("reencryptState"))
		req.Set("input", input)
		cases = append(cases, leanCase{
			name:    "reencryptState seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: req.MarshalTo(nil), ok: callErr == nil, field: "reencrypted", value: result.MarshalTo(nil),
		})
	}
	return cases
}

// encryptLeanDestination supplies primitive crypto outcomes without deciding source or signer admission.
func encryptLeanDestination(t *testing.T, le *logrus.Entry, sfs *block_transform.StepFactorySet,
	objectID string, key crypto.PrivKey, participants []*SOParticipantConfig, plain *SORootInner,
) *SOState {
	t.Helper()
	if sfs == nil || key == nil || plain == nil {
		return nil
	}
	material, grants, _, err := RotateTransformKey(key, objectID, participants, 0, 0)
	if err != nil {
		return nil
	}
	transform, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, material)
	if err != nil {
		return nil
	}
	encoded, err := transform.EncodeBlock(mustMarshalVT(t, &SORootInner{Seqno: 1, StateData: plain.GetStateData()}))
	if err != nil {
		return nil
	}
	root := &SORoot{Inner: encoded, InnerSeqno: 1}
	if err := root.SignInnerData(key, objectID, 1, hash.RecommendedHashType); err != nil {
		return nil
	}
	return &SOState{Config: &SharedObjectConfig{Participants: participants}, Root: root, RootGrants: grants}
}

// decodeLeanRoot performs only grant/block decryption and parsing, leaving admission to the model.
func decodeLeanRoot(t *testing.T, le *logrus.Entry, sfs *block_transform.StepFactorySet,
	objectID string, state *SOState, key crypto.PrivKey, id string,
) *SORootInner {
	t.Helper()
	if sfs == nil || key == nil {
		return nil
	}
	for _, grant := range state.GetRootGrants() {
		if grant.GetPeerId() != id {
			continue
		}
		inner, err := grant.DecryptInnerData(key, objectID)
		if err != nil || inner.GetTransformConf().Validate() != nil {
			return nil
		}
		transform, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, inner.GetTransformConf())
		if err != nil {
			return nil
		}
		data, err := transform.DecodeBlock(state.GetRoot().GetInner())
		if err != nil {
			return nil
		}
		plain := &SORootInner{}
		if plain.UnmarshalVT(data) != nil {
			return nil
		}
		return plain
	}
	return nil
}

// projectLeanPlainRoot retains the exact decoded state data and its claimed sequence.
func projectLeanPlainRoot(a *fastjson.Arena, plain *SORootInner) *fastjson.Value {
	if plain == nil {
		return a.NewNull()
	}
	result := a.NewObject()
	result.Set("seqno", a.NewNumberString(strconv.FormatUint(plain.GetSeqno(), 10)))
	result.Set("data", a.NewString(hex.EncodeToString(plain.GetStateData())))
	return result
}

// projectLeanReencryptState verifies root/grant signatures in the actual object's context.
// Other state fields retain the shared projection so unexpected copied history still differs.
func projectLeanReencryptState(t *testing.T, a *fastjson.Arena, state *SOState, objectID string) *fastjson.Value {
	t.Helper()
	result := projectLeanState(t, a, state)
	root := state.GetRoot()
	data, err := root.BuildSignatureData()
	if err != nil {
		t.Fatal(err)
	}
	sigs := a.NewArray()
	for i, sig := range root.GetValidatorSignatures() {
		sigs.SetArrayItem(i, projectLeanSig(a, sig, data, func(string) string {
			return BuildValidatorRootSignatureContext(objectID, root.GetInnerSeqno())
		}))
	}
	result.Get("root").Set("sigs", sigs)
	for i, grant := range state.GetRootGrants() {
		result.GetArray("grants")[i].Set("sig", projectLeanSig(a, grant.GetSignature(), grant.GetInnerData(), func(signer string) string {
			return BuildSOGrantSignatureContext(objectID, signer, grant.GetPeerId())
		}))
	}
	return result
}
