package sobject

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanQueueConformance compares queueing with an optional same-write validator pass.
func TestLeanQueueConformance(t *testing.T) {
	// Collect every scenario, then check them in one oracle session.
	oracle := leanOracle(t)
	peers := createMockPeers(t, 3)
	var cases []leanCase
	for seed := range uint64(12) {
		cases = append(cases, runLeanQueueScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// runLeanQueueScenario queues one operation per variant and projects the root the
// processor returned, leaving admission and the fallback write to the model.
func runLeanQueueScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	// Keep signing identities for the two owners and the reader.
	t.Helper()
	keys := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}

	// Start from a root signed by the first owner, optionally with a pending operation.
	base := createMockSOState(peers, []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	base.Config.ConfigChainHash = bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)
	base.Root = createMockSORoot(t, 1+seed%5, peers[0])
	if seed%2 == 1 {
		pending := signedLeanOperation(t, keys[1], peers[1].GetPeerID().String(), linkAt(base, keys[1], 1), NewSOOperationLocalID())
		if err := base.QueueOperation(mockSharedObjectID, pending); err != nil {
			t.Fatal(err)
		}
	}

	// Compare each processor outcome against the same queued operation.
	var cases []leanCase
	for variant := range 12 {
		cases = append(cases, runLeanQueueVariant(t, peers, keys, base, seed, variant))
	}
	return cases
}

// runLeanQueueVariant runs one real QueueOperationAndProcess call and its projection.
func runLeanQueueVariant(t *testing.T, peers []peer.Peer, keys []crypto.PrivKey,
	base *SOState, seed uint64, variant int,
) leanCase {
	// Select the queuing identity, callback and provider outcomes.
	t.Helper()
	queuer := 0
	lockOK, writeOK, callbackOK, nonceDelta := true, true, true, uint64(0)
	switch variant {
	case 6:
		callbackOK = false
	case 7:
		nonceDelta = 1
	case 8:
		lockOK = false
	case 9:
		writeOK = false
	case 10:
		queuer = 2
	}

	// Build the operation for the nonce the host selects.
	var arena fastjson.Arena
	a := &arena
	var built *SOOperation
	cb := func(link *SOOperationLink) (*SOOperation, error) {
		if !callbackOK {
			return nil, errors.New("injected callback failure")
		}
		link.Nonce += nonceDelta
		built = signedLeanOperation(t, keys[queuer], peers[queuer].GetPeerID().String(), link, NewSOOperationLocalID())
		return built, nil
	}

	// Validate the queued state the way a validator pass would, recording its result.
	var processed *fastjson.Value
	process := func(_ context.Context, state *SOState) (*SORoot, []*SOOperationRejection, []*SOOperation, error) {
		// Fail the pass or advance the root, sometimes to a stale sequence number.
		if variant == 2 {
			return nil, nil, nil, errors.New("injected processing failure")
		}
		root := state.GetRoot().CloneVT()
		root.InnerSeqno++
		if variant == 5 {
			root.InnerSeqno--
		}
		root.ValidatorSignatures = nil

		// Reject or accept every queued operation, recording accepted nonces.
		var rejected []*SOOperationRejection
		var accepted []*SOOperation
		for _, operation := range state.GetOps() {
			inner, err := operation.UnmarshalInner()
			if err != nil {
				t.Fatal(err)
			}
			if variant == 4 {
				submitter, err := inner.ParsePeerID()
				if err != nil {
					t.Fatal(err)
				}
				rejection, err := BuildSOOperationRejection(keys[0], mockSharedObjectID, submitter, inner.GetNonce(), inner.GetLocalId(), nil)
				if err != nil {
					t.Fatal(err)
				}
				rejected = append(rejected, rejection)
				continue
			}
			accepted = append(accepted, operation)
			root.AccountNonces = advanceAccountNonce(root.AccountNonces, inner.GetPeerId(), inner.GetNonce(), operation.Hash())
		}

		// Sign the root, sometimes by a non-validator.
		signer := keys[0]
		if variant == 3 {
			signer = keys[1]
		}
		if err := root.SignInnerData(signer, mockSharedObjectID, root.InnerSeqno, hash.RecommendedHashType); err != nil {
			t.Fatal(err)
		}

		// Record the processor output the model admits or rejects.
		processed = a.NewObject()
		processed.Set("root", projectLeanRoot(t, a, root))
		processed.Set("rejected", projectLeanRejections(t, a, rejected))
		processed.Set("accepted", projectLeanOperations(t, a, accepted))
		return root, rejected, accepted, nil
	}
	if variant == 0 {
		process = nil
	}

	// Run the host call against a store with injected lock and write outcomes.
	previous := base.CloneVT()
	store := &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
	host := NewSOHost(nil, nil, store.lock, mockSharedObjectID)
	err := host.QueueOperationAndProcess(t.Context(), peers[queuer].GetPeerID(), cb, process)
	if variant == 1 && store.state.GetRoot().GetInnerSeqno() != previous.GetRoot().GetInnerSeqno()+1 {
		t.Fatalf("validator pass did not advance the root: %v", err)
	}

	// Project the callback's operation and the processor's result, if either ran.
	req := a.NewObject()
	req.Set("op", a.NewString("hostQueueOperation"))
	req.Set("previous", projectLeanState(t, a, previous))
	req.Set("operation", a.NewNull())
	if built != nil {
		req.Set("operation", projectLeanOperation(t, a, built))
	}
	req.Set("validator", a.NewString(peers[queuer].GetPeerID().String()))

	// Project the processor's result and the injected provider outcomes.
	req.Set("processed", a.NewNull())
	if processed != nil {
		req.Set("processed", processed)
	}
	req.Set("lockOK", leanBool(a, lockOK))
	req.Set("writeOK", leanBool(a, writeOK))
	c := leanHostCase(t, a, req, store, err, seed, variant)
	c.name = "hostQueueOperation seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
	return c
}
