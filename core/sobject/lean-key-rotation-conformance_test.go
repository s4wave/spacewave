package sobject

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanKeyRotationConformance checks real encrypted grants and epoch lookup against Lean.
func TestLeanKeyRotationConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 5)
	var cases []leanCase
	for seed := range uint64(80) {
		cases = append(cases, runLeanKeyRotationScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanKeyRotation searches role, recipient, counter and interval combinations.
func FuzzLeanKeyRotation(f *testing.F) {
	f.Add(uint64(0))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanKeyRotationScenario(t, createMockPeers(t, 5), seed))
	})
}

// runLeanKeyRotationScenario retains random key draws as opaque identities while
// comparing recipient selection and each recipient's actual decrypted material.
func runLeanKeyRotationScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 0xe90c4))
	keys := make(map[string]crypto.PrivKey, len(peers))
	for _, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[p.GetPeerID().String()] = key
	}
	owner := keys[peers[0].GetPeerID().String()]
	var a fastjson.Arena
	var cases []leanCase
	for variant := range 24 {
		// Include invalid roles, duplicate recipients and unreadable malformed IDs.
		participants := make([]*SOParticipantConfig, rng.IntN(7))
		for i := range participants {
			participants[i] = &SOParticipantConfig{
				PeerId: peers[rng.IntN(len(peers))].GetPeerID().String(),
				Role:   SOParticipantRole(rng.IntN(8) - 1),
			}
		}
		if len(participants) != 0 {
			switch variant % 6 {
			case 1:
				participants[0].PeerId = "malformed peer"
			case 2:
				participants[0] = nil
			case 3:
				participants[0].Role = SOParticipantRole_SOParticipantRole_READER
			}
		}
		current, seqno := rng.Uint64N(100), rng.Uint64N(100)
		switch variant % 8 {
		case 1:
			current = ^uint64(0)
		case 2:
			seqno = ^uint64(0)
		case 3:
			current, seqno = ^uint64(0)-1, ^uint64(0)-1
		}
		request := a.NewObject()
		request.Set("op", a.NewString("rotateTransformKey"))
		request.Set("participants", projectLeanRotationPeers(&a, participants))
		request.Set("epoch", a.NewNumberString(strconv.FormatUint(current, 10)))
		request.Set("seqno", a.NewNumberString(strconv.FormatUint(seqno, 10)))
		request.Set("cryptoOK", a.NewTrue())
		transform, grants, epoch, err := RotateTransformKey(owner, mockSharedObjectID, participants, current, seqno)
		key := "unobserved random draw"
		result := a.NewNull()
		if err == nil {
			key = leanTransformIdentity(t, transform)
			result = a.NewObject()
			result.Set("key", a.NewString(key))
			result.Set("grants", projectLeanKeyGrants(t, &a, grants, keys))
			result.Set("epoch", projectLeanKeyEpoch(t, &a, epoch, keys))
		}
		request.Set("key", a.NewString(key))
		cases = append(cases, leanCase{
			name:    "rotateTransformKey seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: request.MarshalTo(nil), ok: err == nil, field: "rotation", value: result.MarshalTo(nil),
		})

		// Lookup compares complete selected epochs, including retained grant identities.
		epochs := []*SOKeyEpoch{
			{Epoch: rng.Uint64N(10), SeqnoStart: rng.Uint64N(10), SeqnoEnd: rng.Uint64N(12)},
			{Epoch: rng.Uint64N(10), SeqnoStart: rng.Uint64N(10)},
			epoch,
		}
		if variant%3 == 0 {
			epochs = append(epochs, epochs[0])
		}
		if variant%5 == 0 {
			epochs[0].Epoch = ^uint64(0)
		}
		epochValues := a.NewArray()
		for i, e := range epochs {
			epochValues.SetArrayItem(i, projectLeanKeyEpoch(t, &a, e, keys))
		}
		for _, lookup := range []uint64{0, 1, 5, 10, seqno + 1, ^uint64(0)} {
			request := a.NewObject()
			request.Set("op", a.NewString("findCoveringEpoch"))
			request.Set("epochs", epochValues)
			request.Set("seqno", a.NewNumberString(strconv.FormatUint(lookup, 10)))
			found := FindCoveringEpoch(epochs, lookup)
			cases = append(cases, leanCase{
				name:    "findCoveringEpoch seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
				request: request.MarshalTo(nil), ok: found != nil, field: "epoch",
				value: projectLeanKeyEpoch(t, &a, found, keys).MarshalTo(nil),
			})
		}
		request = a.NewObject()
		request.Set("op", a.NewString("currentEpochNumber"))
		request.Set("epochs", epochValues)
		cases = append(cases, leanCase{
			name: "currentEpochNumber", request: request.MarshalTo(nil), ok: true, field: "epoch",
			value: []byte(strconv.FormatUint(CurrentEpochNumber(epochs), 10)),
		})
	}
	return cases
}

// projectLeanRotationPeers checks public key extraction independently of role selection.
func projectLeanRotationPeers(a *fastjson.Arena, participants []*SOParticipantConfig) *fastjson.Value {
	projected := projectLeanConfig(&SharedObjectConfig{Participants: participants}).json(a).GetArray("participants")
	result := a.NewArray()
	for i, participant := range participants {
		id, err := peer.IDB58Decode(participant.GetPeerId())
		valid := err == nil
		if valid {
			_, err = id.ExtractPublicKey()
			valid = err == nil
		}
		p := a.NewObject()
		p.Set("participant", projected[i])
		p.Set("publicKey", leanBool(a, valid))
		result.SetArrayItem(i, p)
	}
	return result
}

// leanTransformIdentity names generated transform material without exposing its key bytes.
func leanTransformIdentity(t *testing.T, transform *block_transform.Config) string {
	t.Helper()
	digest := sha256.Sum256(mustMarshalVT(t, transform))
	return hex.EncodeToString(digest[:])
}

// projectLeanKeyGrants projects the material each actual recipient can decrypt.
func projectLeanKeyGrants(t *testing.T, a *fastjson.Arena, grants []*SOGrant, keys map[string]crypto.PrivKey) *fastjson.Value {
	t.Helper()
	result := a.NewArray()
	for i, grant := range grants {
		key, ok := keys[grant.GetPeerId()]
		if !ok {
			t.Fatalf("grant for unexpected peer %s", grant.GetPeerId())
		}
		inner, err := grant.DecryptInnerData(key, mockSharedObjectID)
		if err != nil {
			t.Fatal(err)
		}
		g := a.NewObject()
		g.Set("peer", a.NewString(grant.GetPeerId()))
		g.Set("key", a.NewString(leanTransformIdentity(t, inner.GetTransformConf())))
		result.SetArrayItem(i, g)
	}
	return result
}

// projectLeanKeyEpoch retains interval endpoints and every decrypted grant.
func projectLeanKeyEpoch(t *testing.T, a *fastjson.Arena, epoch *SOKeyEpoch, keys map[string]crypto.PrivKey) *fastjson.Value {
	t.Helper()
	if epoch == nil {
		return a.NewNull()
	}
	result := a.NewObject()
	result.Set("epoch", a.NewNumberString(strconv.FormatUint(epoch.GetEpoch(), 10)))
	result.Set("start", a.NewNumberString(strconv.FormatUint(epoch.GetSeqnoStart(), 10)))
	result.Set("finish", a.NewNumberString(strconv.FormatUint(epoch.GetSeqnoEnd(), 10)))
	result.Set("grants", projectLeanKeyGrants(t, a, epoch.GetGrants(), keys))
	return result
}
