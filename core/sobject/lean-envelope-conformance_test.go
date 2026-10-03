package sobject

import (
	"encoding/hex"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// leanEnvelopeSeeds is the number of random operations and checkpoints
// compared with the Lean envelope model.
const leanEnvelopeSeeds = 400

// TestLeanEnvelopeConformance checks that SOOperation.Verify,
// ValidateSignature, SOCheckpoint.Verify and ValidateAuthority make the same
// decisions as the Lean model in lean/Spacewave/SObject/Envelope.lean, on
// signed envelopes with one field broken at a time.
func TestLeanEnvelopeConformance(t *testing.T) {
	// Sign with an owner, a writer, a reader and an outsider.
	oracle := leanOracle(t)
	e := &envelopeScenario{t: t}
	for _, name := range []string{"envelope owner", "envelope writer", "envelope reader", "envelope outsider"} {
		priv, id := vectorKey(t, name)
		e.privs = append(e.privs, priv)
		e.ids = append(e.ids, id)
	}
	e.participants = []*SOParticipantConfig{
		{PeerId: e.ids[0], Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: e.ids[1], Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: e.ids[2], Role: SOParticipantRole_SOParticipantRole_READER},
	}

	// Compare one operation and one checkpoint per seed.
	var cases []leanCase
	for seed := range uint64(leanEnvelopeSeeds) {
		e.rng = rand.New(rand.NewPCG(seed, 0xe57e))
		name := " seed " + strconv.FormatUint(seed, 10)
		cases = append(cases, e.operationCase("authorizeOperation"+name), e.checkpointCase("authorizeCheckpoint"+name))
	}
	checkLeanCases(t, oracle, cases)
}

// envelopeScenario generates signed envelopes from fixed keys.
type envelopeScenario struct {
	// t reports generation failures.
	t *testing.T
	// rng selects the envelope and the broken field.
	rng *rand.Rand
	// privs are the signing keys.
	privs []crypto.PrivKey
	// ids are the peer IDs of privs.
	ids []string
	// participants grants the first three keys OWNER, WRITER and READER.
	participants []*SOParticipantConfig
	// a owns the projected JSON.
	a fastjson.Arena
}

// operationCase signs one operation, usually with one field broken, and
// records Go's verification and authorization.
func (e *envelopeScenario) operationCase(name string) leanCase {
	// Build a well-formed body for a random author.
	author := e.rng.IntN(len(e.privs))
	inner := &SOOperationInner{
		PeerId:          e.ids[author],
		LocalId:         NewSOOperationLocalID(),
		Nonce:           1,
		OpData:          []byte("data"),
		SharedObjectId:  vectorObjectID,
		ProtocolVersion: SOOperationProtocolVersion,
		ConfigHash:      e.digest(),
	}
	if e.rng.IntN(2) == 0 {
		inner.Nonce = 2 + uint64(e.rng.IntN(5))
		inner.PrevOpHash = e.digest()
	}
	for range e.rng.IntN(4) {
		inner.Parents = append(inner.Parents, e.position())
	}
	slices.SortFunc(inner.Parents, compareOpHash)

	// Break one field, sign the body and break the envelope.
	e.breakOperation(inner)
	data, err := inner.MarshalVT()
	if err != nil {
		e.t.Fatal(err)
	}
	op := &SOOperation{Inner: data, Signature: e.sign(SOOperationSignatureContext, author, data)}
	e.breakEnvelope(&op.Inner, &op.Signature)

	// Record Go's decisions.
	verifyErr := func() error { _, err := op.Verify(vectorObjectID); return err }()
	result := e.a.NewObject()
	result.Set("authorized", leanBool(&e.a, op.ValidateSignature(vectorObjectID, e.participants) == nil))
	req := e.request("authorizeOperation", "operation", e.operationJSON(op))
	return leanCase{name: name, request: req, ok: verifyErr == nil, field: "result", value: result.MarshalTo(nil)}
}

// breakOperation breaks at most one field of a well-formed operation body.
func (e *envelopeScenario) breakOperation(inner *SOOperationInner) {
	switch e.rng.IntN(40) {
	case 0:
		inner.PeerId = "not-a-peer-id"
	case 1:
		inner.PeerId = e.ids[(slices.Index(e.ids, inner.PeerId)+1)%len(e.ids)]
	case 2:
		inner.LocalId = "not-a-ulid"
	case 3:
		inner.Nonce = 0
	case 4:
		inner.Nonce, inner.PrevOpHash = 1, e.digest()
	case 5:
		inner.Nonce, inner.PrevOpHash = 2, nil
	case 6:
		inner.OpData = make([]byte, MaxInnerDataSize+1)
	case 7:
		inner.SharedObjectId = ""
	case 8:
		inner.SharedObjectId = "other object"
	case 9:
		inner.ProtocolVersion = 2
	case 10:
		inner.ConfigHash = inner.ConfigHash[:31]
	default:
		e.breakParents(inner)
	}
}

// breakParents breaks at most one rule of an operation's parent list.
func (e *envelopeScenario) breakParents(inner *SOOperationInner) {
	// Add parents for the rules that need them.
	parents := inner.Parents
	for len(parents) < 2 {
		parents = append(parents, e.position())
	}
	slices.SortFunc(parents, compareOpHash)
	switch e.rng.IntN(16) {
	case 0:
		parents[0], parents[1] = parents[1], parents[0]
	case 1:
		parents[1] = parents[0].CloneVT()
	case 2:
		parents[0].PeerId = "not-a-peer-id"
	case 3:
		parents[0].Nonce = 0
	case 4:
		parents[0].OpHash = parents[0].GetOpHash()[:31]
	case 5:
		if len(inner.PrevOpHash) == 0 {
			return
		}
		parents[0] = &SOOperationPosition{PeerId: inner.PeerId, Nonce: inner.Nonce - 1, OpHash: inner.PrevOpHash}
		slices.SortFunc(parents, compareOpHash)
	case 6:
		for len(parents) <= MaxSOOperationParents {
			parents = append(parents, e.position())
		}
		slices.SortFunc(parents, compareOpHash)
	default:
		return
	}
	inner.Parents = parents
}

// checkpointCase signs one checkpoint, usually with one field broken, and
// records Go's verification and authority.
func (e *envelopeScenario) checkpointCase(name string) leanCase {
	// Build a well-formed body, a genesis or a later checkpoint.
	inner := &SOCheckpointInner{
		SharedObjectId: vectorObjectID,
		ConfigHash:     e.digest(),
		ReplayVersion:  SOReplayVersion,
		StateData:      []byte("state"),
	}
	if e.rng.IntN(3) != 0 {
		inner.Height = 1 + uint64(e.rng.IntN(5))
		inner.PrevCheckpointHash = e.digest()
		inner.Authors = e.authors()
	}

	// Break one field and sign the body by one to three signers.
	e.breakCheckpoint(inner)
	data, err := inner.MarshalVT()
	if err != nil {
		e.t.Fatal(err)
	}
	cp := &SOCheckpoint{Inner: data}
	for range e.signatureCount() {
		cp.Signatures = append(cp.Signatures, e.sign(SOCheckpointSignatureContext, e.rng.IntN(len(e.privs)), data))
	}

	// Break the envelope or one signature.
	if len(cp.Signatures) != 0 {
		i := e.rng.IntN(len(cp.Signatures))
		e.breakEnvelope(&cp.Inner, &cp.Signatures[i])
	}
	cp.Signatures = slices.DeleteFunc(cp.Signatures, func(sig *peer.Signature) bool { return sig == nil })

	// Record Go's decisions.
	_, signers, verifyErr := cp.Verify(vectorObjectID)
	_, authErr := cp.ValidateAuthority(vectorObjectID, &SharedObjectConfig{Participants: e.participants})
	result := e.a.NewObject()
	result.Set("signers", e.strings(signers))
	result.Set("authorized", leanBool(&e.a, authErr == nil))
	req := e.request("authorizeCheckpoint", "checkpoint", e.checkpointJSON(cp))
	return leanCase{name: name, request: req, ok: verifyErr == nil, field: "result", value: result.MarshalTo(nil)}
}

// breakCheckpoint breaks at most one field of a well-formed checkpoint body.
func (e *envelopeScenario) breakCheckpoint(inner *SOCheckpointInner) {
	switch e.rng.IntN(32) {
	case 0:
		inner.SharedObjectId = ""
	case 1:
		inner.SharedObjectId = "other object"
	case 2:
		inner.ConfigHash = nil
	case 3:
		inner.ReplayVersion = 2
	case 4:
		inner.StateData = make([]byte, MaxStateDataSize+1)
	case 5:
		inner.PrevCheckpointHash = e.digest()
	case 6:
		inner.Authors = e.authors()
	case 7:
		inner.PrevCheckpointHash = nil
	default:
		e.breakAuthors(inner)
	}
}

// breakAuthors breaks at most one rule of a later checkpoint's author heads.
func (e *envelopeScenario) breakAuthors(inner *SOCheckpointInner) {
	authors := inner.GetAuthors()
	if len(authors) < 2 {
		return
	}
	switch e.rng.IntN(8) {
	case 0:
		authors[0], authors[1] = authors[1], authors[0]
	case 1:
		authors[1].PeerId = authors[0].GetPeerId()
	case 2:
		authors[0].PeerId = "not-a-peer-id"
	case 3:
		authors[0].Nonce = 0
	case 4:
		authors[0].OpHash = nil
	}
}

// breakEnvelope breaks at most one property of a signed envelope: its body
// bytes, its signature or the signature's key.
func (e *envelopeScenario) breakEnvelope(inner *[]byte, sig **peer.Signature) {
	switch e.rng.IntN(24) {
	case 0:
		*inner = nil
	case 1:
		*inner = []byte{0xff, 0xff, 0xff}
	case 2:
		*sig = nil
	case 3:
		(*sig).PubKey = nil
	case 4:
		(*sig).PubKey = []byte{1, 2, 3}
	case 5:
		(*sig).SigData = slices.Clone((*sig).GetSigData())
		(*sig).SigData[0] ^= 1
	case 6:
		(*sig).SigData = nil
	}
}

// signatureCount returns how many signatures a checkpoint carries: usually
// one to three, rarely none or more than the participant limit.
func (e *envelopeScenario) signatureCount() int {
	switch e.rng.IntN(32) {
	case 0:
		return 0
	case 1:
		return MaxParticipants + 1
	}
	return 1 + e.rng.IntN(3)
}

// sign signs data under context with key index signer.
func (e *envelopeScenario) sign(context string, signer int, data []byte) *peer.Signature {
	sig, err := peer.NewSignature(context, e.privs[signer], hash.RecommendedHashType, data, true)
	if err != nil {
		e.t.Fatal(err)
	}
	return sig
}

// digest returns a random 32-byte hash.
func (e *envelopeScenario) digest() []byte {
	h := make([]byte, 32)
	for i := range h {
		h[i] = byte(e.rng.IntN(256))
	}
	return h
}

// position returns a random well-formed position.
func (e *envelopeScenario) position() *SOOperationPosition {
	return &SOOperationPosition{PeerId: e.ids[e.rng.IntN(len(e.ids))], Nonce: 1 + uint64(e.rng.IntN(5)), OpHash: e.digest()}
}

// authors returns well-formed author heads for some of the keys, in peer ID
// order.
func (e *envelopeScenario) authors() []*SOOperationPosition {
	var authors []*SOOperationPosition
	for _, id := range e.ids {
		if e.rng.IntN(2) == 0 {
			authors = append(authors, &SOOperationPosition{PeerId: id, Nonce: 1 + uint64(e.rng.IntN(5)), OpHash: e.digest()})
		}
	}
	slices.SortFunc(authors, func(a, b *SOOperationPosition) int { return strings.Compare(a.GetPeerId(), b.GetPeerId()) })
	return authors
}

// request writes an oracle request for op with the envelope under field.
func (e *envelopeScenario) request(op, field string, envelope *fastjson.Value) []byte {
	// Name the object and the participants the envelope is checked under.
	a := &e.a
	req := a.NewObject()
	req.Set("op", a.NewString(op))
	req.Set("object", a.NewString(vectorObjectID))
	req.Set("participants", projectLeanConfig(&SharedObjectConfig{Participants: e.participants}).json(a).Get("participants"))
	req.Set(field, envelope)
	return req.MarshalTo(nil)
}

// operationJSON projects an operation into the Lean Operation structure.
func (e *envelopeScenario) operationJSON(op *SOOperation) *fastjson.Value {
	// Project the envelope and the signature over the encoded body.
	a := &e.a
	v := a.NewObject()
	v.Set("innerBytes", a.NewNumberInt(len(op.GetInner())))
	v.Set("sigFormat", leanBool(a, op.GetSignature().Validate() == nil))
	v.Set("sig", projectLeanSig(a, op.GetSignature(), op.GetInner(), func(string) string {
		return SOOperationSignatureContext
	}))

	// Project the decoded body, or null when it does not decode.
	inner := &SOOperationInner{}
	if err := inner.UnmarshalVT(op.GetInner()); err != nil {
		v.Set("body", a.NewNull())
		return v
	}
	_, localErr := ParseSOOperationLocalID(inner.GetLocalId())
	body := a.NewObject()
	body.Set("object", a.NewString(inner.GetSharedObjectId()))
	body.Set("version", a.NewNumberInt(int(inner.GetProtocolVersion())))
	body.Set("peer", e.peer(inner.GetPeerId()))
	body.Set("localID", leanBool(a, localErr == nil))

	// Project the sequence, payload size and links.
	body.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetNonce(), 10)))
	body.Set("dataBytes", a.NewNumberInt(len(inner.GetOpData())))
	body.Set("prev", a.NewString(hex.EncodeToString(inner.GetPrevOpHash())))
	body.Set("parents", e.positions(inner.GetParents()))
	body.Set("configHash", a.NewString(hex.EncodeToString(inner.GetConfigHash())))
	v.Set("body", body)
	return v
}

// checkpointJSON projects a checkpoint into the Lean Checkpoint structure.
func (e *envelopeScenario) checkpointJSON(cp *SOCheckpoint) *fastjson.Value {
	// Project the envelope and each signature over the encoded body.
	a := &e.a
	v := a.NewObject()
	v.Set("innerBytes", a.NewNumberInt(len(cp.GetInner())))
	sigs := a.NewArray()
	for i, sig := range cp.GetSignatures() {
		sigs.SetArrayItem(i, projectLeanSig(a, sig, cp.GetInner(), func(string) string {
			return SOCheckpointSignatureContext
		}))
	}
	v.Set("sigs", sigs)

	// Project the decoded body, or null when it does not decode.
	inner := &SOCheckpointInner{}
	if err := inner.UnmarshalVT(cp.GetInner()); err != nil {
		v.Set("body", a.NewNull())
		return v
	}
	body := a.NewObject()
	body.Set("object", a.NewString(inner.GetSharedObjectId()))
	body.Set("configHash", a.NewString(hex.EncodeToString(inner.GetConfigHash())))
	body.Set("replayVersion", a.NewNumberInt(int(inner.GetReplayVersion())))
	body.Set("stateBytes", a.NewNumberInt(len(inner.GetStateData())))

	// Project the chain position.
	body.Set("height", a.NewNumberString(strconv.FormatUint(inner.GetHeight(), 10)))
	body.Set("prev", a.NewString(hex.EncodeToString(inner.GetPrevCheckpointHash())))
	body.Set("authors", e.positions(inner.GetAuthors()))
	v.Set("body", body)
	return v
}

// positions projects positions, writing a peer ID that does not parse as the
// empty string.
func (e *envelopeScenario) positions(list []*SOOperationPosition) *fastjson.Value {
	a := &e.a
	arr := a.NewArray()
	for i, pos := range list {
		v := a.NewObject()
		v.Set("peer", e.peer(pos.GetPeerId()))
		v.Set("nonce", a.NewNumberString(strconv.FormatUint(pos.GetNonce(), 10)))
		v.Set("hash", a.NewString(hex.EncodeToString(pos.GetOpHash())))
		arr.SetArrayItem(i, v)
	}
	return arr
}

// peer projects a peer ID, or the empty string when it does not parse.
func (e *envelopeScenario) peer(id string) *fastjson.Value {
	if _, err := parsePeerIDField(id); err != nil {
		id = ""
	}
	return e.a.NewString(id)
}

// strings projects a list of strings.
func (e *envelopeScenario) strings(list []string) *fastjson.Value {
	arr := e.a.NewArray()
	for i, s := range list {
		arr.SetArrayItem(i, e.a.NewString(s))
	}
	return arr
}
