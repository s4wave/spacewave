package sobject

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanRecoveryConformance compares recovery construction, decoding and provider authority.
func TestLeanRecoveryConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(30) {
		cases = append(cases, runLeanRecoveryScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanRecovery searches entity, role, credential and provider outcome combinations.
func FuzzLeanRecovery(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(13))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanRecoveryScenario(t, createMockPeers(t, 4), seed))
	})
}

// runLeanRecoveryScenario retains real encrypted envelopes and each recovered plaintext.
func runLeanRecoveryScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	projection := &configChainScenario{t: t}
	a := &projection.arena
	keys := make([]crypto.PrivKey, len(peers))
	pubs := make([]crypto.PubKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[i], pubs[i] = key, key.GetPublic()
	}
	config := &SharedObjectConfig{ConfigChainSeqno: seed%23 + 1, ConfigChainHash: bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)}
	transform, _, _, err := RotateTransformKey(keys[0], mockSharedObjectID, []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
	}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	base := &SOEntityRecoveryMaterial{
		EntityId: "entity", Role: SOParticipantRole(seed%4 + 1),
		GrantInner: &SOGrantInner{TransformConf: transform},
	}
	var cases []leanCase
	for variant := range 25 {
		cfg, material := config.CloneVT(), base.CloneVT()
		entity, recipients := "entity", pubs[:2]
		switch variant {
		case 16:
			entity = ""
		case 17:
			cfg = nil
		case 18:
			material = nil
		case 19:
			recipients = nil
		case 20:
			recipients = []crypto.PubKey{nil}
		case 21:
			recipients = []crypto.PubKey{pubs[0], pubs[0]}
		case 22:
			material = &SOEntityRecoveryMaterial{}
		case 23:
			cfg.ConfigChainHash = nil
		case 24:
			material.EntityId = "different entity"
		}
		epoch := seed%17 + 1
		env, err := BuildSOEntityRecoveryEnvelope(entity, epoch, cfg, material, recipients)
		encoded := env.GetEnvelopeData()
		cryptoOK := err == nil
		if err != nil {
			var cryptoErr error
			encoded, cryptoErr = encryptLeanRecovery(t, material, recipients, soEntityRecoveryEnvelopeContext)
			cryptoOK = cryptoErr == nil
		}
		req := a.NewObject()
		req.Set("op", a.NewString("buildRecoveryEnvelope"))
		req.Set("entity", a.NewString(entity))
		req.Set("epoch", a.NewNumberString(strconv.FormatUint(epoch, 10)))
		req.Set("config", a.NewNull())
		if cfg != nil {
			req.Set("config", projectLeanConfig(cfg).json(a))
		}
		req.Set("material", projectLeanRecoveryMaterial(t, a, material))
		req.Set("recipients", projectLeanRecoveryPublicKeys(t, a, recipients))
		req.Set("cryptoOK", leanBool(a, cryptoOK))
		req.Set("encoded", a.NewString(hex.EncodeToString(encoded)))
		name := " seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		cases = append(cases, leanCase{
			name: "buildRecoveryEnvelope" + name, request: req.MarshalTo(nil),
			ok: err == nil, field: "envelope", value: projectLeanRecoveryEnvelope(a, env).MarshalTo(nil),
		})

		if err == nil {
			for _, pub := range recipients {
				for i, candidate := range pubs {
					if !candidate.Equals(pub) {
						continue
					}
					decoded, err := UnlockSOEntityRecoveryEnvelope([]crypto.PrivKey{keys[i]}, env)
					if err != nil || !decoded.EqualVT(material) {
						t.Fatalf("recipient failed to recover exact material: %v", err)
					}
				}
			}
		}
		unlockKeys := keys[:1]
		unlockEnv := env.CloneVT()
		switch variant {
		case 6:
			unlockKeys = keys[1:2]
		case 7:
			unlockKeys = keys[2:3]
		case 8:
			unlockKeys = nil
		case 9:
			unlockKeys = []crypto.PrivKey{keys[2], keys[0]}
		case 10:
			unlockEnv = nil
		case 11:
			unlockEnv.EnvelopeData = nil
		case 12:
			unlockEnv.EnvelopeData[len(unlockEnv.EnvelopeData)/2] ^= 1
		case 13:
			wrong, err := encryptLeanRecovery(t, material, recipients, "wrong recovery context")
			if err != nil {
				t.Fatal(err)
			}
			unlockEnv.EnvelopeData = wrong
		case 14:
			unlockEnv.EnvelopeData = []byte{0xff}
		case 15:
			unlockEnv.EntityId = "metadata is not decoded material"
		}
		decoded := decodeLeanRecovery(t, unlockKeys, unlockEnv)
		actual, unlockErr := UnlockSOEntityRecoveryEnvelope(unlockKeys, unlockEnv)
		req = a.NewObject()
		req.Set("op", a.NewString("unlockRecovery"))
		keyIDs := a.NewArray()
		for i, key := range unlockKeys {
			id, err := peer.IDFromPrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			keyIDs.SetArrayItem(i, a.NewString(id.String()))
		}
		req.Set("keys", keyIDs)
		req.Set("envelope", projectLeanRecoveryEnvelope(a, unlockEnv))
		req.Set("decoded", projectLeanRecoveryMaterial(t, a, decoded))
		cases = append(cases, leanCase{
			name: "unlockRecovery" + name, request: req.MarshalTo(nil),
			ok: unlockErr == nil, field: "material", value: projectLeanRecoveryMaterial(t, a, actual).MarshalTo(nil),
		})
	}
	cases = append(cases, runLeanRecoveryResolver(t, projection, base, seed)...)
	cases = append(cases, runLeanRecoveryEnrollment(t, projection, peers, keys, config, base, seed)...)
	return cases
}

// encryptLeanRecovery supplies the envelope primitive without deciding recovery constructor admission.
func encryptLeanRecovery(t *testing.T, material *SOEntityRecoveryMaterial, recipients []crypto.PubKey, context string) ([]byte, error) {
	t.Helper()
	grants := make([]*envelope.EnvelopeGrantConfig, len(recipients))
	for i := range recipients {
		grants[i] = &envelope.EnvelopeGrantConfig{ShareCount: 1, KeypairIndexes: []uint32{uint32(i)}}
	}
	env, err := envelope.BuildEnvelope(rand.Reader, context, mustMarshalVT(t, material), recipients,
		&envelope.EnvelopeConfig{Threshold: 0, GrantConfigs: grants})
	if err != nil {
		return nil, err
	}
	return env.MarshalVT()
}

// decodeLeanRecovery performs only primitive envelope parsing, decryption and material parsing.
func decodeLeanRecovery(t *testing.T, keys []crypto.PrivKey, env *SOEntityRecoveryEnvelope) *SOEntityRecoveryMaterial {
	t.Helper()
	parsed := &envelope.Envelope{}
	if parsed.UnmarshalVT(env.GetEnvelopeData()) != nil {
		return nil
	}
	data, result, err := envelope.UnlockEnvelope(soEntityRecoveryEnvelopeContext, parsed, keys)
	if err != nil || !result.GetSuccess() {
		return nil
	}
	material := &SOEntityRecoveryMaterial{}
	if material.UnmarshalVT(data) != nil {
		return nil
	}
	return material
}

// runLeanRecoveryResolver reuses the provider fixtures to vary each independent provider result.
func runLeanRecoveryResolver(t *testing.T, projection *configChainScenario, base *SOEntityRecoveryMaterial, seed uint64) []leanCase {
	t.Helper()
	a := &projection.arena
	injected := errors.New("injected recovery provider failure")
	var cases []leanCase
	for variant := range 13 {
		decoder := &fakeSharedObjectRecoveryDecoder{material: base.CloneVT()}
		provider := &fakeSharedObjectRecoveryProvider{
			entityID: "entity", env: &SOEntityRecoveryEnvelope{EntityId: "entity"}, dec: decoder,
		}
		account := &fakeProviderAccount{recoveryProv: provider}
		switch variant {
		case 1:
			account.featureErr = injected
		case 2:
			provider.entityErr = injected
		case 3:
			provider.entityID = ""
		case 4:
			provider.envErr = injected
		case 5:
			provider.env.EntityId = "foreign entity"
		case 6:
			provider.env.EntityId = ""
		case 7:
			provider.decoderErr = injected
		case 8:
			decoder.err = injected
		case 9:
			decoder.material.EntityId = "foreign entity"
		case 10:
			decoder.material.EntityId = ""
		case 11:
			provider.env = nil
		case 12:
			decoder.material = nil
		}
		result, err := ResolveSharedObjectRecoveryMaterial(t.Context(), account, &SharedObjectRef{})
		req := a.NewObject()
		req.Set("op", a.NewString("resolveRecovery"))
		req.Set("featureOK", leanBool(a, account.featureErr == nil))
		req.Set("entity", a.NewNull())
		if provider.entityErr == nil {
			req.Set("entity", a.NewString(provider.entityID))
		}
		req.Set("readOK", leanBool(a, provider.envErr == nil))
		req.Set("envelope", projectLeanRecoveryEnvelope(a, provider.env))
		req.Set("decoderOK", leanBool(a, provider.decoderErr == nil))
		req.Set("decodeOK", leanBool(a, decoder.err == nil))
		req.Set("material", projectLeanRecoveryMaterial(t, a, decoder.material))
		cases = append(cases, leanCase{
			name:    "resolveRecovery seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: req.MarshalTo(nil), ok: err == nil, field: "material", value: projectLeanRecoveryMaterial(t, a, result).MarshalTo(nil),
		})
	}
	return cases
}

// projectLeanRecoveryMaterial retains exact material bytes and the separately inspected entity/role.
func projectLeanRecoveryMaterial(t *testing.T, a *fastjson.Arena, material *SOEntityRecoveryMaterial) *fastjson.Value {
	t.Helper()
	if material == nil {
		return a.NewNull()
	}
	result := a.NewObject()
	result.Set("data", a.NewString(hex.EncodeToString(mustMarshalVT(t, material))))
	result.Set("entity", a.NewString(material.GetEntityId()))
	result.Set("role", a.NewNumberInt(int(material.GetRole())))
	result.Set("grant", a.NewNull())
	if material.GetGrantInner() != nil {
		result.Set("grant", a.NewString(hex.EncodeToString(mustMarshalVT(t, material.GetGrantInner()))))
	}
	return result
}

// projectLeanRecoveryEnvelope retains the actual encrypted payload and all envelope metadata.
func projectLeanRecoveryEnvelope(a *fastjson.Arena, env *SOEntityRecoveryEnvelope) *fastjson.Value {
	if env == nil {
		return a.NewNull()
	}
	result := a.NewObject()
	result.Set("entity", a.NewString(env.GetEntityId()))
	result.Set("epoch", a.NewNumberString(strconv.FormatUint(env.GetKeyEpoch(), 10)))
	result.Set("seqno", a.NewNumberString(strconv.FormatUint(env.GetConfigChainSeqno(), 10)))
	result.Set("hash", a.NewString(hex.EncodeToString(env.GetConfigChainHash())))
	result.Set("data", a.NewString(hex.EncodeToString(env.GetEnvelopeData())))
	return result
}

// projectLeanRecoveryPublicKeys preserves recipient multiplicity and omitted-key slots.
func projectLeanRecoveryPublicKeys(t *testing.T, a *fastjson.Arena, keys []crypto.PubKey) *fastjson.Value {
	t.Helper()
	result := a.NewArray()
	for i, key := range keys {
		id := ""
		if key != nil {
			parsed, err := peer.IDFromPublicKey(key)
			if err != nil {
				t.Fatal(err)
			}
			id = parsed.String()
		}
		result.SetArrayItem(i, a.NewString(id))
	}
	return result
}

// runLeanRecoveryEnrollment compares proposals, current role bounds and actual recovered grants.
func runLeanRecoveryEnrollment(t *testing.T, projection *configChainScenario, peers []peer.Peer,
	keys []crypto.PrivKey, checkpoint *SharedObjectConfig, base *SOEntityRecoveryMaterial, seed uint64,
) []leanCase {
	t.Helper()
	a := &projection.arena
	var cases []leanCase
	for variant := range 26 {
		current := checkpoint.CloneVT()
		current.Participants = []*SOParticipantConfig{
			{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER, EntityId: "owner"},
			{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole(seed%3 + 1), EntityId: "entity"},
		}
		signer, grantSigner := keys[2], keys[2]
		peerID, grantPeer := peers[2].GetPeerID().String(), peers[2].GetPeerID()
		entity, objectID := "entity", mockSharedObjectID
		role := SOParticipantRole(seed%4 + 1)
		material := base.CloneVT()
		switch variant {
		case 1:
			entity, role = "owner", SOParticipantRole_SOParticipantRole_OWNER
		case 2:
			entity = "foreign entity"
		case 3:
			entity = ""
		case 4:
			peerID = ""
		case 5:
			signer, grantSigner = nil, nil
		case 6:
			current = nil
		case 7:
			role = SOParticipantRole(-1)
		case 8:
			role = SOParticipantRole_SOParticipantRole_UNKNOWN
		case 9:
			role = SOParticipantRole(99)
		case 10:
			peerID = peers[1].GetPeerID().String()
		case 11:
			peerID = peers[3].GetPeerID().String()
		case 12:
			signer = keys[0]
		case 13:
			current.ConfigChainSeqno = math.MaxUint64
		case 14:
			current.ConfigChainHash = nil
		case 15:
			role = SOParticipantRole_SOParticipantRole_OWNER
		case 16:
			current.Participants[0].EntityId = "entity"
			role = SOParticipantRole_SOParticipantRole_OWNER
		case 17:
			current.ConsensusMode = SOConsensusMode(99)
		case 18:
			current.Participants = append(current.Participants, current.Participants[1].CloneVT())
		case 19:
			material = nil
		case 20:
			material.GrantInner = nil
		case 21:
			objectID = ""
		case 22:
			grantPeer = peer.ID("invalid public key encoding")
		case 23:
			grantSigner = keys[3]
		case 24:
			grantSigner = keys[0]
		case 25:
			material.GrantInner.TransformConf = &block_transform.Config{}
		}
		entry, buildErr := BuildSelfEnrollPeerConfigChange(current, signer, peerID, entity, role)
		// Generate the signature/hash primitive even when argument admission rejected.
		primitive := entry
		if primitive == nil && signer != nil {
			previous := current
			if previous == nil {
				previous = &SharedObjectConfig{}
			}
			next := previous.CloneVT()
			next.Participants = append(next.Participants, &SOParticipantConfig{PeerId: peerID, Role: role, EntityId: entity})
			var err error
			primitive, err = BuildSOConfigChange(previous, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER, signer, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := a.NewObject()
		req.Set("op", a.NewString("buildSelfEnroll"))
		req.Set("current", a.NewNull())
		if current != nil {
			req.Set("current", projectLeanConfig(current).json(a))
		}
		req.Set("sig", a.NewNull())
		req.Set("hash", a.NewString(""))
		if primitive != nil {
			projected := projection.entryJSON(primitive)
			req.Set("sig", projected.Get("sig"))
			req.Set("hash", projected.Get("hash"))
		}
		req.Set("peer", a.NewString(peerID))
		req.Set("entity", a.NewString(entity))
		req.Set("role", a.NewNumberInt(int(role)))
		req.Set("signOK", leanBool(a, primitive != nil))
		value := a.NewNull()
		if entry != nil {
			value = projection.entryJSON(entry)
		}
		name := " seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		cases = append(cases, leanCase{
			name: "buildSelfEnroll" + name, request: req.MarshalTo(nil),
			ok: buildErr == nil, field: "entry", value: value.MarshalTo(nil),
		})

		var admitted *SharedObjectConfig
		admitErr := buildErr
		if buildErr == nil {
			admitted, admitErr = VerifyConfigChange(current, entry)
			// An omitted embedded key must reject without changing the checkpoint.
			missingSigner := entry.CloneVT()
			missingSigner.Signature.PubKey = nil
			next, err := VerifyConfigChange(current, missingSigner)
			projection.checkChange(variant, current, missingSigner, next, err)
		}
		if current != nil && signer != nil {
			req.Set("op", a.NewString("enrollRecovery"))
			value = a.NewNull()
			if admitted != nil {
				value = projectLeanConfig(admitted).json(a)
			}
			cases = append(cases, leanCase{
				name: "enrollRecovery" + name, request: req.MarshalTo(nil),
				ok: admitErr == nil, field: "config", value: value.MarshalTo(nil),
			})
		}

		grant, grantErr := BuildSelfEnrollPeerGrant(grantSigner, grantPeer, objectID, material)
		pub, pubErr := grantPeer.ExtractPublicKey()
		primitiveGrant := grant
		if primitiveGrant == nil && grantSigner != nil && pubErr == nil && material.GetGrantInner() != nil {
			primitiveGrant, _ = EncryptSOGrant(grantSigner, pub, objectID, material.GrantInner)
		}
		signerID := ""
		if grantSigner != nil {
			id, err := peer.IDFromPrivateKey(grantSigner)
			if err != nil {
				t.Fatal(err)
			}
			signerID = id.String()
		}
		req = a.NewObject()
		req.Set("op", a.NewString("buildSelfEnrollGrant"))
		req.Set("signer", a.NewString(signerID))
		req.Set("peer", a.NewString(grantPeer.String()))
		req.Set("object", a.NewString(objectID))
		req.Set("material", projectLeanRecoveryMaterial(t, a, material))
		req.Set("publicKey", leanBool(a, pubErr == nil && pub != nil))
		req.Set("cryptoOK", leanBool(a, primitiveGrant != nil))
		req.Set("encoded", a.NewString(hex.EncodeToString(mustMarshalVT(t, primitiveGrant))))
		req.Set("inner", a.NewString(hex.EncodeToString(primitiveGrant.GetInnerData())))
		value = a.NewNull()
		if grant != nil {
			plain, err := grant.DecryptInnerData(keys[2], objectID)
			if err != nil || !plain.EqualVT(material.GrantInner) {
				t.Fatalf("self-enrollment grant changed recovered key material: %v", err)
			}
			value = a.NewObject()
			projected := projectLeanReencryptState(t, a, &SOState{RootGrants: []*SOGrant{grant}}, objectID).GetArray("grants")[0]
			value.Set("grant", projected)
			value.Set("material", a.NewString(hex.EncodeToString(mustMarshalVT(t, plain))))
		}
		cases = append(cases, leanCase{
			name: "buildSelfEnrollGrant" + name, request: req.MarshalTo(nil),
			ok: grantErr == nil, field: "grant", value: value.MarshalTo(nil),
		})
		if grant != nil {
			authority := admitted
			if authority == nil {
				authority = current
			}
			req = a.NewObject()
			req.Set("op", a.NewString("validateRecoveryGrant"))
			req.Set("grant", value.Get("grant"))
			req.Set("config", projectLeanConfig(authority).json(a))
			cases = append(cases, leanCase{
				name: "validateRecoveryGrant" + name, request: req.MarshalTo(nil),
				ok: grant.ValidateSignature(objectID, authority.GetParticipants()) == nil,
			})
		}
	}
	return append(cases, projection.cases...)
}
