package sobject

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanStateConformance compares real signed state transitions with State.lean.
func TestLeanStateConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(120) {
		cases = append(cases, runLeanStateScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanState searches state-machine traces for Go/model disagreements.
func FuzzLeanState(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(19))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanStateScenario(t, createMockPeers(t, 4), seed))
	})
}

// runLeanStateScenario mixes valid operations, root updates, and result clearing
// with replays, conflicting IDs, stale counters, and invalid authority.
func runLeanStateScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()

	// Keep keys fixed within a trace and begin at a valid signed root.
	rng := rand.New(rand.NewPCG(seed, 0x50a7e))
	privs := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		var err error
		privs[i], err = p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
	}
	state := createMockSOState(peers[:3], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	state.Root = createMockSORoot(t, 1, peers[0])
	var arena fastjson.Arena
	var cases []leanCase
	var priorRejections []*SOOperationRejection

	// Each trace advances only through successful Go transitions. Rejected
	// working copies are discarded under SOState's documented contract.
	for step := range 35 {
		req := arena.NewObject()
		req.Set("state", projectLeanState(t, &arena, state))
		next := state.CloneVT()
		var op string
		var err error
		switch rng.IntN(5) {
		case 0, 1, 2:
			op = "queueOperation"
			signer := rng.IntN(len(peers))
			id := peers[signer].GetPeerID().String()
			if rng.IntN(10) == 0 {
				id = peers[(signer+1)%len(peers)].GetPeerID().String()
			}
			nonce := state.GetNextAccountNonce(id)
			localID := NewSOOperationLocalID()
			switch rng.IntN(8) {
			case 0:
				nonce++
			case 1:
				nonce--
			case 2:
				if len(state.Ops) != 0 {
					inner, decodeErr := state.Ops[0].UnmarshalInner()
					if decodeErr != nil {
						t.Fatal(decodeErr)
					}
					localID = inner.GetLocalId()
				}
			}
			operation := signedLeanOperation(t, privs[signer], id, nonce, localID)
			if rng.IntN(10) == 0 {
				operation.Signature.SigData[0] ^= 1
			}
			req.Set("operation", projectLeanOperation(&arena, operation))
			err = next.QueueOperation(mockSharedObjectID, operation)
		case 3:
			op = "updateRootState"
			root := state.Root.CloneVT()
			root.InnerSeqno++
			root.ValidatorSignatures = nil
			var rejected []*SOOperationRejection
			var accepted []*SOOperation
			if len(state.Ops) != 0 {
				operation := state.Ops[rng.IntN(len(state.Ops))]
				inner, decodeErr := operation.UnmarshalInner()
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				switch rng.IntN(3) {
				case 0:
					accepted = []*SOOperation{operation}
					root.updateAccountNonce(inner.GetPeerId(), inner.GetNonce())
				case 1:
					accepted = []*SOOperation{operation}
				case 2:
					submitter, parseErr := inner.ParsePeerID()
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					rejection, buildErr := BuildSOOperationRejection(privs[0], mockSharedObjectID,
						submitter, inner.GetNonce(), inner.GetLocalId(), nil)
					if buildErr != nil {
						t.Fatal(buildErr)
					}
					rejected = []*SOOperationRejection{rejection}
					priorRejections = append(priorRejections, rejection)
				}
			}
			if len(priorRejections) != 0 && rng.IntN(3) == 0 {
				replayed := priorRejections[rng.IntN(len(priorRejections))]
				rejected = append(rejected, replayed, replayed)
			}
			signer := 0
			enforce := ""
			switch rng.IntN(10) {
			case 0:
				root.InnerSeqno--
			case 1:
				root.InnerSeqno++
			case 2:
				signer = 1 + rng.IntN(3)
			case 3:
				root.AccountNonces = nil
			case 4:
				enforce = peers[1].GetPeerID().String()
			}
			if signErr := root.SignInnerData(privs[signer], mockSharedObjectID,
				root.InnerSeqno, hash.RecommendedHashType); signErr != nil {
				t.Fatal(signErr)
			}
			switch rng.IntN(10) {
			case 0:
				root.ValidatorSignatures = append(root.ValidatorSignatures, root.ValidatorSignatures[0])
			case 1:
				root.ValidatorSignatures[0].SigData[0] ^= 1
			}
			req.Set("root", projectLeanRoot(t, &arena, root))
			req.Set("enforce", arena.NewString(enforce))
			req.Set("rejected", projectLeanRejections(&arena, rejected))
			req.Set("accepted", projectLeanOperations(&arena, accepted))
			req.Set("op", arena.NewString("validateNextRootState"))
			cases = append(cases, leanCase{
				name: "validateNextRootState", request: req.MarshalTo(nil),
				ok: state.validateNextRootState(mockSharedObjectID, root, enforce) == nil,
			})
			err = next.UpdateRootState(mockSharedObjectID, root, enforce, rejected, accepted)
		case 4:
			op = "clearOperationResult"
			signer := rng.IntN(len(peers))
			localID := NewSOOperationLocalID()
			if len(state.OpRejections) != 0 && rng.IntN(4) != 0 {
				group := state.OpRejections[rng.IntN(len(state.OpRejections))]
				inner, decodeErr := group.Rejections[0].UnmarshalInner()
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				localID = inner.GetLocalId()
				signer = slices.IndexFunc(peers, func(p peer.Peer) bool { return p.GetPeerID().String() == group.GetPeerId() })
			}
			clear, buildErr := BuildSOClearOperationResult(mockSharedObjectID, privs[signer], localID)
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			if rng.IntN(8) == 0 {
				clear.Signature.SigData[0] ^= 1
			}
			inner, decodeErr := clear.UnmarshalInner()
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			req.Set("peer", arena.NewString(inner.GetPeerId()))
			req.Set("localId", arena.NewString(inner.GetLocalId()))
			req.Set("format", leanBool(&arena, clear.Validate() == nil))
			req.Set("sig", projectLeanSig(&arena, clear.GetSignature(), clear.GetInner(),
				func(id string) string {
					return BuildSOClearOperationResultSignatureContext(mockSharedObjectID, id, inner.GetLocalId())
				}))
			err = next.ClearOperationResult(mockSharedObjectID, clear)
		}

		// Compare the entire projected result, then query validation and every
		// peer's nonce so later steps cannot hide a divergent transition.
		req.Set("op", arena.NewString(op))
		c := leanCase{
			name:    op + " seed " + strconv.FormatUint(seed, 10) + " step " + strconv.Itoa(step),
			request: req.MarshalTo(nil), ok: err == nil,
		}
		if err == nil {
			c.field, c.value = "state", projectLeanState(t, &arena, next).MarshalTo(nil)
			state = next
		}
		cases = append(cases, c)
		validation := arena.NewObject()
		validation.Set("op", arena.NewString("validateState"))
		validation.Set("state", projectLeanState(t, &arena, state))
		cases = append(cases, leanCase{name: "validateState", request: validation.MarshalTo(nil), ok: state.Validate(mockSharedObjectID) == nil})
		for _, p := range peers {
			validation.Set("op", arena.NewString("getNextAccountNonce"))
			validation.Set("peer", arena.NewString(p.GetPeerID().String()))
			cases = append(cases, leanCase{
				name: "getNextAccountNonce", request: validation.MarshalTo(nil), ok: true,
				field: "nonce", value: []byte(strconv.FormatUint(state.GetNextAccountNonce(p.GetPeerID().String()), 10)),
			})
		}
	}

	// Challenge validation independently of the transition generators, which
	// normally advance from valid states. Include counter and enum boundaries.
	for mutation := range 12 {
		candidate := state.CloneVT()
		switch mutation {
		case 0:
			candidate.Config.ConsensusMode = SOConsensusMode(-1)
		case 1:
			candidate.Root.InnerSeqno = ^uint64(0)
		case 2:
			candidate.Root.InnerSeqno = 0
		case 3:
			candidate.Root.ValidatorSignatures = nil
		case 4:
			candidate.QueuedAccountNonces = []*SOAccountNonce{{PeerId: "", Nonce: 1}}
		case 5:
			candidate.QueuedAccountNonces = []*SOAccountNonce{
				{PeerId: peers[0].GetPeerID().String(), Nonce: ^uint64(0)},
				{PeerId: peers[0].GetPeerID().String(), Nonce: 1},
			}
		case 6:
			candidate.Root.AccountNonces = append(candidate.Root.AccountNonces,
				&SOAccountNonce{PeerId: peers[0].GetPeerID().String(), Nonce: ^uint64(0)})
		case 7:
			candidate.Root.Inner = nil
		case 8:
			candidate.Config.Participants = append(candidate.Config.Participants, candidate.Config.Participants[0])
		case 9:
			localID := NewSOOperationLocalID()
			candidate.Ops = []*SOOperation{
				signedLeanOperation(t, privs[0], peers[0].GetPeerID().String(), 1, localID),
				signedLeanOperation(t, privs[0], peers[0].GetPeerID().String(), 2, localID),
			}
		case 10:
			candidate.Ops = []*SOOperation{signedLeanOperation(t, privs[0], peers[0].GetPeerID().String(), 0, NewSOOperationLocalID())}
		case 11:
			candidate.QueuedAccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: ^uint64(0)}}
		}
		req := arena.NewObject()
		req.Set("state", projectLeanState(t, &arena, candidate))
		req.Set("op", arena.NewString("validateState"))
		cases = append(cases, leanCase{
			name:    "validateState malformed " + strconv.Itoa(mutation),
			request: req.MarshalTo(nil), ok: candidate.Validate(mockSharedObjectID) == nil,
		})
		req.Set("op", arena.NewString("getNextAccountNonce"))
		req.Set("peer", arena.NewString(peers[0].GetPeerID().String()))
		cases = append(cases, leanCase{
			name:    "getNextAccountNonce boundary " + strconv.Itoa(mutation),
			request: req.MarshalTo(nil), ok: true, field: "nonce",
			value: []byte(strconv.FormatUint(candidate.GetNextAccountNonce(peers[0].GetPeerID().String()), 10)),
		})
		if mutation != 11 {
			continue
		}

		// Exhaustion rejects both zero and every attempted reusable nonce.
		req.Set("op", arena.NewString("queueOperation"))
		for _, nonce := range []uint64{0, 1, ^uint64(0)} {
			operation := signedLeanOperation(t, privs[0], peers[0].GetPeerID().String(), nonce, NewSOOperationLocalID())
			req.Set("operation", projectLeanOperation(&arena, operation))
			cases = append(cases, leanCase{
				name: "queueOperation exhausted", request: req.MarshalTo(nil),
				ok: candidate.CloneVT().QueueOperation(mockSharedObjectID, operation) == nil,
			})
		}
	}

	// Clearing the last rejected nonce must retain the exhausted account even
	// when there was no local reservation before the result arrived.
	exhausted := state.CloneVT()
	exhausted.Ops = nil
	exhausted.QueuedAccountNonces = nil
	localID := NewSOOperationLocalID()
	rejection, err := BuildSOOperationRejection(privs[0], mockSharedObjectID,
		peers[0].GetPeerID(), ^uint64(0), localID, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := peers[0].GetPeerID().String()
	exhausted.OpRejections = []*SOPeerOpRejections{{PeerId: id, Rejections: []*SOOperationRejection{rejection}}}
	clear, err := BuildSOClearOperationResult(mockSharedObjectID, privs[0], localID)
	if err != nil {
		t.Fatal(err)
	}
	req := arena.NewObject()
	req.Set("op", arena.NewString("clearOperationResult"))
	req.Set("state", projectLeanState(t, &arena, exhausted))
	req.Set("peer", arena.NewString(id))
	req.Set("localId", arena.NewString(localID))
	req.Set("format", leanBool(&arena, clear.Validate() == nil))
	req.Set("sig", projectLeanSig(&arena, clear.Signature, clear.Inner, func(signer string) string {
		return BuildSOClearOperationResultSignatureContext(mockSharedObjectID, signer, localID)
	}))
	if err := exhausted.ClearOperationResult(mockSharedObjectID, clear); err != nil {
		t.Fatal(err)
	}
	cases = append(cases, leanCase{
		name: "clearOperationResult exhausted", request: req.MarshalTo(nil), ok: true,
		field: "state", value: projectLeanState(t, &arena, exhausted).MarshalTo(nil),
	})
	req.Set("op", arena.NewString("getNextAccountNonce"))
	req.Set("state", projectLeanState(t, &arena, exhausted))
	cases = append(cases, leanCase{
		name: "getNextAccountNonce after exhaustion clear", request: req.MarshalTo(nil), ok: true,
		field: "nonce", value: []byte(strconv.FormatUint(exhausted.GetNextAccountNonce(id), 10)),
	})
	return cases
}

// signedLeanOperation can sign structurally invalid inputs for adversarial cases.
func signedLeanOperation(t *testing.T, priv crypto.PrivKey, id string, nonce uint64, localID string) *SOOperation {
	t.Helper()
	inner := &SOOperationInner{PeerId: id, LocalId: localID, Nonce: nonce, OpData: []byte("operation")}
	data := mustMarshalVT(t, inner)
	signer, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := peer.NewSignature(BuildSOOperationSignatureContext(mockSharedObjectID, signer.String(), nonce, localID),
		priv, hash.RecommendedHashType, data, true)
	if err != nil {
		t.Fatal(err)
	}
	return &SOOperation{Inner: data, Signature: sig}
}

// projectLeanSig projects key parsing and raw cryptographic verification, leaving
// membership and consensus decisions to the Lean model.
func projectLeanSig(a *fastjson.Arena, sig *peer.Signature, data []byte, context func(string) string) *fastjson.Value {
	// ParsePubKey returns nil without error when the envelope omits its key.
	id, valid := "", false
	pub, err := sig.ParsePubKey()
	if err == nil && pub != nil {
		parsed, parseErr := peer.IDFromPublicKey(pub)
		if parseErr == nil {
			id = parsed.String()
			verified, verifyErr := sig.VerifyWithPublic(context(id), pub, data)
			valid = verifyErr == nil && verified
		}
	}

	// Preserve the signer even when the message signature is invalid.
	v := a.NewObject()
	v.Set("signer", a.NewString(id))
	v.Set("valid", leanBool(a, valid))
	return v
}

// projectLeanNonces retains nonce order, duplicates, and all uint64 bits.
func projectLeanNonces(a *fastjson.Arena, nonces []*SOAccountNonce) *fastjson.Value {
	result := a.NewArray()
	for i, nonce := range nonces {
		v := a.NewObject()
		v.Set("peer", a.NewString(nonce.GetPeerId()))
		v.Set("nonce", a.NewNumberString(strconv.FormatUint(nonce.GetNonce(), 10)))
		result.SetArrayItem(i, v)
	}
	return result
}

// projectLeanRoot abstracts encoding and signature structure, retaining all root
// progression, nonce-order, signer-authorization, and consensus decisions.
func projectLeanRoot(t *testing.T, a *fastjson.Arena, root *SORoot) *fastjson.Value {
	t.Helper()
	data, err := root.BuildSignatureData()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := DigestSOAuthoritativeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	format := len(root.GetInner()) <= MaxInnerDataSize
	for _, nonce := range root.GetAccountNonces() {
		if _, err := nonce.ParsePeerID(); err != nil {
			format = false
		}
	}
	sigs := a.NewArray()
	for i, sig := range root.GetValidatorSignatures() {
		format = format && sig.Validate() == nil
		sigs.SetArrayItem(i, projectLeanSig(a, sig, data, func(string) string {
			return BuildValidatorRootSignatureContext(mockSharedObjectID, root.GetInnerSeqno())
		}))
	}

	v := a.NewObject()
	v.Set("seqno", a.NewNumberString(strconv.FormatUint(root.GetInnerSeqno(), 10)))
	v.Set("digest", a.NewString(hex.EncodeToString(digest)))
	v.Set("format", leanBool(a, format))
	v.Set("hasInner", leanBool(a, len(root.GetInner()) != 0))
	v.Set("nonces", projectLeanNonces(a, root.GetAccountNonces()))
	v.Set("sigs", sigs)
	return v
}

// projectLeanOperation retains decoded identity and verifies the signed bytes.
func projectLeanOperation(a *fastjson.Arena, operation *SOOperation) *fastjson.Value {
	inner := &SOOperationInner{}
	parsed := inner.UnmarshalVT(operation.GetInner()) == nil
	v := a.NewObject()
	v.Set("peer", a.NewString(inner.GetPeerId()))
	v.Set("localId", a.NewString(inner.GetLocalId()))
	v.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetNonce(), 10)))
	v.Set("parsed", leanBool(a, parsed))
	v.Set("innerValid", leanBool(a, parsed && inner.Validate() == nil))
	v.Set("format", leanBool(a, operation.Validate() == nil))
	v.Set("sig", projectLeanSig(a, operation.GetSignature(), operation.GetInner(), func(id string) string {
		return BuildSOOperationSignatureContext(mockSharedObjectID, id, inner.GetNonce(), inner.GetLocalId())
	}))
	return v
}

// projectLeanOperations projects a pending or explicitly accepted batch.
func projectLeanOperations(a *fastjson.Arena, operations []*SOOperation) *fastjson.Value {
	v := a.NewArray()
	for i, operation := range operations {
		v.SetArrayItem(i, projectLeanOperation(a, operation))
	}
	return v
}

// projectLeanRejections projects signed rejections without deciding their authority.
func projectLeanRejections(a *fastjson.Arena, rejections []*SOOperationRejection) *fastjson.Value {
	v := a.NewArray()
	for i, rejection := range rejections {
		inner := &SOOperationRejectionInner{}
		parsed := inner.UnmarshalVT(rejection.GetInner()) == nil
		r := a.NewObject()
		r.Set("peer", a.NewString(inner.GetPeerId()))
		r.Set("localId", a.NewString(inner.GetLocalId()))
		r.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetOpNonce(), 10)))
		r.Set("parsed", leanBool(a, parsed))
		r.Set("innerValid", leanBool(a, parsed && inner.Validate() == nil))
		r.Set("format", leanBool(a, rejection.Validate() == nil))
		r.Set("sig", projectLeanSig(a, rejection.GetSignature(), rejection.GetInner(), func(id string) string {
			return BuildSOOperationRejectionSignatureContext(mockSharedObjectID, id, inner.GetPeerId(), inner.GetOpNonce(), inner.GetLocalId())
		}))
		v.SetArrayItem(i, r)
	}
	return v
}

// projectLeanState projects the state fields consumed by state.go. Grants are
// unchanged by these transitions and retain their validation under this config.
func projectLeanState(t *testing.T, a *fastjson.Arena, state *SOState) *fastjson.Value {
	t.Helper()
	grantsValid := true
	seen := make(map[string]bool)
	roles := make(map[string]SOParticipantRole)
	for _, p := range state.GetConfig().GetParticipants() {
		roles[p.GetPeerId()] = p.GetRole()
	}
	for _, g := range state.GetRootGrants() {
		id := g.GetPeerId()
		grantsValid = grantsValid && !seen[id] && g.Validate() == nil &&
			g.ValidateSignature(mockSharedObjectID, state.GetConfig().GetParticipants()) == nil && CanReadState(roles[id])
		seen[id] = true
	}
	groups := a.NewArray()
	for i, group := range state.GetOpRejections() {
		g := a.NewObject()
		g.Set("peer", a.NewString(group.GetPeerId()))
		g.Set("entries", projectLeanRejections(a, group.GetRejections()))
		groups.SetArrayItem(i, g)
	}

	v := a.NewObject()
	v.Set("config", projectLeanConfig(state.GetConfig()).json(a))
	v.Set("root", projectLeanRoot(t, a, state.GetRoot()))
	v.Set("grantsValid", leanBool(a, grantsValid))
	v.Set("ops", projectLeanOperations(a, state.GetOps()))
	v.Set("queued", projectLeanNonces(a, state.GetQueuedAccountNonces()))
	v.Set("rejections", groups)
	return v
}

// equalLeanJSON compares JSON structurally without depending on object key order.
func equalLeanJSON(a, b *fastjson.Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case fastjson.TypeObject:
		x, y := a.GetObject(), b.GetObject()
		if x.Len() != y.Len() {
			return false
		}
		equal := true
		x.Visit(func(key []byte, value *fastjson.Value) {
			equal = equal && equalLeanJSON(value, y.Get(string(key)))
		})
		return equal
	case fastjson.TypeArray:
		return slices.EqualFunc(a.GetArray(), b.GetArray(), equalLeanJSON)
	default:
		return bytes.Equal(a.MarshalTo(nil), b.MarshalTo(nil))
	}
}
