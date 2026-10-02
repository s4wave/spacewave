package sobject

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/fastjson"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestLeanProcessConformance compares the validator's root construction from queued operations.
func TestLeanProcessConformance(t *testing.T) {
	t.Skip("the operation-set rewrite replaces this model")

	// Collect every scenario, then check them in one oracle session.
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(200) {
		cases = append(cases, runLeanProcessScenario(t, peers, seed))
	}
	checkLeanCases(t, oracle, cases)
}

// runLeanProcessScenario builds a random queue and application result for one
// validator pass, projecting primitive decode outcomes and leaving decisions to the model.
func runLeanProcessScenario(t *testing.T, peers []peer.Peer, seed uint64) leanCase {
	// Keep signing identities for an owner, a validator, a writer and an outsider.
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 0x9e0c))
	keys := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}

	// Grant the shared transform to the owner and validator only.
	transformConf := &block_transform.Config{Steps: []*block_transform.StepConfig{{
		Id: transform_blockenc.ConfigID,
		Config: mustMarshalVT(t, &transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		}),
	}}}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	le := logrus.New().WithField("test", t.Name())
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, transformConf)
	if err != nil {
		t.Fatal(err)
	}
	state := createMockSOState(peers[:3], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_VALIDATOR,
		SOParticipantRole_SOParticipantRole_WRITER,
	})
	for _, recipient := range peers[:2] {
		pub, err := recipient.GetPeerID().ExtractPublicKey()
		if err != nil {
			t.Fatal(err)
		}
		grant, err := EncryptSOGrant(keys[0], pub, mockSharedObjectID, &SOGrantInner{TransformConf: transformConf})
		if err != nil {
			t.Fatal(err)
		}
		state.RootGrants = append(state.RootGrants, grant)
	}

	// Start from no root or an encrypted root, sometimes with a mismatched sequence.
	if seed%3 != 0 {
		seqno := 1 + seed%5
		inner, err := xfrm.EncodeBlock(mustMarshalVT(t, &SORootInner{Seqno: seqno, StateData: []byte("current")}))
		if err != nil {
			t.Fatal(err)
		}
		state.Root = &SORoot{Inner: inner, InnerSeqno: seqno}
		if nonce := rng.Uint64N(3); nonce != 0 {
			state.Root.AccountNonces = advanceAccountNonce(state.Root.AccountNonces, peers[2].GetPeerID().String(), nonce, mockPrevOpHash)
		}
		if rng.IntN(8) == 0 {
			state.Root.InnerSeqno++
		}
	}

	// Queue operations whose data may not decode, including a repeated nonce.
	var ops []*SOOperation
	for i := range rng.IntN(4) {
		submitter := rng.IntN(3)
		nonce := uint64(1 + rng.IntN(3))
		data := []byte("not encrypted")
		if rng.IntN(3) != 0 {
			data, err = xfrm.EncodeBlock([]byte("operation " + strconv.Itoa(i)))
			if err != nil {
				t.Fatal(err)
			}
		}
		op, err := BuildSOOperation(mockSharedObjectID, keys[submitter], data, linkAt(state, keys[submitter], nonce), NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, op)
	}
	if rng.IntN(12) == 0 {
		ops = append(ops, signedLeanOperation(t, keys[0], peers[0].GetPeerID().String(), linkAt(state, keys[0], 0), NewSOOperationLocalID()))
	}

	// Answer with accepted, rejected, absent, malformed and unknown references.
	var results []*SOOperationResult
	for range rng.IntN(4) {
		id, nonce := peers[rng.IntN(3)].GetPeerID().String(), uint64(1+rng.IntN(3))
		switch rng.IntN(8) {
		case 0:
			results = append(results, nil)
		case 1:
			results = append(results, BuildSOOperationResult("", nonce, true, nil))
		default:
			details := &SOOperationRejectionErrorDetails{ErrorMsg: "rejected"}
			results = append(results, BuildSOOperationResult(id, nonce, rng.IntN(3) != 0, details))
		}
	}
	callbackOK, changed := rng.IntN(10) != 0, rng.IntN(2) == 0
	cb := func(context.Context, []byte, []*SOOperationInner) (*[]byte, []*SOOperationResult, error) {
		if !callbackOK {
			return nil, nil, errors.New("injected application failure")
		}
		var next *[]byte
		if changed {
			data := []byte("next")
			next = &data
		}
		return next, results, nil
	}

	// Run the pass as a validator, owner, writer or outsider.
	validator := []int{0, 1, 1, 2, 3}[rng.IntN(5)]
	snap := NewSOStateParticipantHandle(le, sfs, mockSharedObjectID, state, keys[validator], peers[validator].GetPeerID())
	root, rejected, accepted, err := snap.ProcessOperations(t.Context(), ops, cb)

	// Observe the transform and root decode outcomes the model takes as inputs.
	var arena fastjson.Arena
	a := &arena
	held, transformErr := snap.GetTransformer(t.Context())
	rootOK := false
	if transformErr == nil {
		_, rootErr := snap.decodeRootInnerWithTransformer(held)
		rootOK = rootErr == nil
	}

	// Project the validator identity and each queued operation's decode outcome.
	input := a.NewObject()
	input.Set("validator", a.NewString(peers[validator].GetPeerID().String()))
	input.Set("transformOK", leanBool(a, transformErr == nil))
	projectedOps := a.NewArray()
	for i, op := range ops {
		decodeOK := false
		if inner, err := op.UnmarshalInner(); err == nil {
			_, decodeErr := xfrm.DecodeBlock(inner.GetOpData())
			decodeOK = decodeErr == nil
		}
		v := a.NewObject()
		v.Set("op", projectLeanOperation(t, a, op))
		v.Set("decodeOK", leanBool(a, decodeOK))
		projectedOps.SetArrayItem(i, v)
	}
	input.Set("ops", projectedOps)

	// Project the application's output and each result reference.
	input.Set("rootOK", leanBool(a, rootOK))
	input.Set("callbackOK", leanBool(a, callbackOK))
	input.Set("changed", leanBool(a, changed))
	projectedResults := a.NewArray()
	for i, result := range results {
		ref := result.GetOpRef()
		_, isError := result.GetBody().(*SOOperationResult_ErrorDetails)
		v := a.NewObject()
		v.Set("present", leanBool(a, ref != nil))
		v.Set("refValid", leanBool(a, ref.Validate() == nil))
		v.Set("peer", a.NewString(ref.GetPeerId()))
		v.Set("nonce", a.NewNumberString(strconv.FormatUint(ref.GetNonce(), 10)))
		v.Set("accepted", leanBool(a, !isError))
		projectedResults.SetArrayItem(i, v)
	}
	input.Set("results", projectedResults)
	input.Set("signOK", leanBool(a, true))

	// Request the model's decision for the same state and input.
	req := a.NewObject()
	req.Set("op", a.NewString("processOperations"))
	req.Set("state", projectLeanState(t, a, state))
	req.Set("input", input)

	// Project Go's root decision by the identities its rejections bind.
	processed := a.NewNull()
	if err == nil {
		processed = a.NewObject()
		processed.Set("advance", leanBool(a, !root.EqualVT(state.GetRoot())))
		refs := a.NewArray()
		for i, rejection := range rejected {
			inner, err := rejection.UnmarshalInner()
			if err != nil {
				t.Fatal(err)
			}
			ref := a.NewObject()
			ref.Set("peer", a.NewString(inner.GetPeerId()))
			ref.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetOpNonce(), 10)))
			ref.Set("localId", a.NewString(inner.GetLocalId()))
			refs.SetArrayItem(i, ref)
		}
		processed.Set("rejected", refs)
		processed.Set("accepted", projectLeanOperations(t, a, accepted))
		processed.Set("nonces", projectLeanNonces(a, root.GetAccountNonces()))
	}
	return leanCase{
		name:    "processOperations seed " + strconv.FormatUint(seed, 10),
		request: req.MarshalTo(nil), ok: err == nil, field: "processed", value: processed.MarshalTo(nil),
	}
}
