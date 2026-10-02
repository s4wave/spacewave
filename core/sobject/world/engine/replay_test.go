package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// replayTestObjectID is the SharedObject the replay test's operations name.
const replayTestObjectID = "test-replay"

// replayTestOp is one signed World operation in the replay test.
type replayTestOp struct {
	// name labels the operation in failures.
	name string
	// op is the signed envelope.
	op *sobject.SOOperation
	// author is the signing peer.
	author peer.ID
	// data is the encoded SOWorldOp.
	data []byte
}

// hash returns the operation identity.
func (o *replayTestOp) hash() []byte {
	return o.op.Hash()
}

// replayTestResult is a World replayed from an ordered operation prefix.
type replayTestResult struct {
	// state is the World state after the prefix.
	state *InnerState
	// outcomes names each operation in the prefix with "applied" or its
	// rejection reason.
	outcomes []string
}

// replayTestMember replays the operations it knows to a local World. The
// processing peer comes from its SharedObject, as in the running engine.
type replayTestMember struct {
	t       *testing.T
	c       *Controller
	so      *testSharedObject
	genesis *InnerState
	// warm caches the result of every replayed prefix, keyed by the prefix's
	// concatenated operation hashes. A cold member replays from genesis.
	warm  bool
	cache map[string]*replayTestResult
	known []*replayTestOp
}

// deliver adds a batch of operations and replays the known set once.
func (m *replayTestMember) deliver(ops ...*replayTestOp) *replayTestResult {
	// Replay the grown set in its deterministic order.
	m.t.Helper()
	m.known = append(m.known, ops...)
	return m.replay(replayTestOrder(m.t, m.known))
}

// replay replays order from the longest cached prefix.
func (m *replayTestMember) replay(order []*replayTestOp) *replayTestResult {
	// Start from the longest cached prefix, or from genesis.
	m.t.Helper()
	start, res := 0, &replayTestResult{state: m.genesis}
	for i := len(order); i > 0 && m.warm; i-- {
		if cached := m.cache[replayTestPrefixKey(order[:i])]; cached != nil {
			start, res = i, cached
			break
		}
	}

	// Replay the remaining operations in order.
	for i := start; i < len(order); i++ {
		op := order[i]
		next, opRes, err := m.c.processOp(
			context.Background(),
			m.c.le,
			m.so,
			op.data,
			op.name,
			op.author,
			1,
			i,
			res.state,
		)
		if err != nil {
			m.t.Fatalf("replay %s: %v", op.name, err)
		}
		outcome := "applied"
		if !opRes.GetSuccess() {
			outcome = opRes.GetErrorDetails().GetErrorMsg()
			next = res.state
		}
		res = &replayTestResult{
			state:    next,
			outcomes: append(slices.Clone(res.outcomes), op.name+": "+outcome),
		}
		if m.warm {
			m.cache[replayTestPrefixKey(order[:i+1])] = res
		}
	}
	return res
}

// replayTestOrder returns ops in topological order of their causal links, with
// concurrent operations ordered by hash.
func replayTestOrder(t *testing.T, ops []*replayTestOp) []*replayTestOp {
	// Count the known parents of every operation.
	t.Helper()
	byHash := make(map[string]*replayTestOp, len(ops))
	for _, op := range ops {
		byHash[string(op.hash())] = op
	}
	pending := make(map[*replayTestOp]int, len(ops))
	for _, op := range ops {
		pending[op] = 0
		for _, parent := range replayTestParents(t, op) {
			if byHash[string(parent)] != nil {
				pending[op]++
			}
		}
	}

	// Emit the lowest-hash operation whose parents are all emitted.
	order := make([]*replayTestOp, 0, len(ops))
	for len(order) < len(ops) {
		var next *replayTestOp
		for op, n := range pending {
			if n == 0 && (next == nil || bytes.Compare(op.hash(), next.hash()) < 0) {
				next = op
			}
		}
		if next == nil {
			t.Fatal("operation set has a cycle")
		}
		delete(pending, next)
		order = append(order, next)
		for op := range pending {
			for _, parent := range replayTestParents(t, op) {
				if bytes.Equal(parent, next.hash()) {
					pending[op]--
				}
			}
		}
	}
	return order
}

// replayTestParents returns the previous operation and causal parents of op.
func replayTestParents(t *testing.T, op *replayTestOp) [][]byte {
	// Read the causal links from the signed envelope.
	t.Helper()
	inner, err := op.op.UnmarshalInner()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Combine the parents with the previous operation.
	parents := slices.Clone(inner.GetParentHashes())
	if prev := inner.GetPrevOpHash(); len(prev) != 0 {
		parents = append(parents, prev)
	}
	return parents
}

// replayTestPrefixKey identifies an ordered operation prefix.
func replayTestPrefixKey(prefix []*replayTestOp) string {
	var key []byte
	for _, op := range prefix {
		key = append(key, op.hash()...)
	}
	return string(key)
}

// TestReplayConvergesAcrossDeliveryOrders replays concurrent transactions from
// two members that receive them in opposite orders, cold and with a warm cache.
// Both members create the same object concurrently. Every member must reach
// the same World with the same outcomes: the creation that sorts first applies
// and the other is rejected.
func TestReplayConvergesAcrossDeliveryOrders(t *testing.T) {
	// Build the shared genesis World and two member devices.
	ctx := context.Background()
	c, so, genesis := newProcessTestWorld(t, ctx)
	privA, pidA := newReplayTestKey(t)
	privB, pidB := newReplayTestKey(t)
	configHash := bytes.Repeat([]byte{7}, 32)

	// sign signs data as the next operation of an author.
	sign := func(name string, priv crypto.PrivKey, pid peer.ID, data []byte, link *sobject.SOOperationLink) *replayTestOp {
		// Sign the operation under the shared config.
		t.Helper()
		link.ConfigHash = configHash
		op, err := sobject.BuildSOOperation(replayTestObjectID, priv, data, link, sobject.NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err.Error())
		}
		return &replayTestOp{name: name, op: op, author: pid, data: data}
	}
	createObject := func(key string) []byte {
		t.Helper()
		tx, err := world_block_tx.NewTxCreateObject(key, genesis.GetHeadRef().CloneVT())
		if err != nil {
			t.Fatal(err.Error())
		}
		return marshalApplyTxOpForProcessTest(t, tx)
	}

	// Member A creates its own object and then the shared one. Member B
	// creates the shared object concurrently.
	opA := sign("A own", privA, pidA, createObject("object-a"), &sobject.SOOperationLink{Nonce: 1})
	opAShared := sign("A shared", privA, pidA, createObject("object-shared"), &sobject.SOOperationLink{Nonce: 2, PrevOpHash: opA.hash()})
	opBShared := sign("B shared", privB, pidB, createObject("object-shared"), &sobject.SOOperationLink{Nonce: 1})

	// Each member delivers its own operations first, one at a time, then the
	// other member's operations as one batch.
	type run struct {
		name string
		res  *replayTestResult
	}
	var runs []run
	for _, warm := range []bool{false, true} {
		mode := "cold"
		if warm {
			mode = "warm"
		}
		memberA := &replayTestMember{t: t, c: c, so: &testSharedObject{peerID: pidA, blockStore: so.blockStore}, genesis: genesis, warm: warm, cache: map[string]*replayTestResult{}}
		memberA.deliver(opA)
		memberA.deliver(opAShared)
		runs = append(runs, run{"member A " + mode, memberA.deliver(opBShared)})

		memberB := &replayTestMember{t: t, c: c, so: &testSharedObject{peerID: pidB, blockStore: so.blockStore}, genesis: genesis, warm: warm, cache: map[string]*replayTestResult{}}
		memberB.deliver(opBShared)
		runs = append(runs, run{"member B " + mode, memberB.deliver(opA, opAShared)})
	}

	// Every run reaches the same World with the same outcomes.
	want := runs[0]
	for _, got := range runs[1:] {
		if !got.res.state.EqualVT(want.res.state) || !slices.Equal(got.res.outcomes, want.res.outcomes) {
			t.Errorf("%s diverged from %s:\n  %q\n  %q", got.name, want.name, want.res.outcomes, got.res.outcomes)
		}
	}

	// Only the later of the two shared creations is rejected.
	order := replayTestOrder(t, []*replayTestOp{opA, opAShared, opBShared})
	for i, op := range order {
		applied := want.res.outcomes[i] == op.name+": applied"
		if wantApplied := op != order[2]; applied != wantApplied {
			t.Errorf("outcome %q; want applied=%v", want.res.outcomes[i], wantApplied)
		}
	}
}

// newReplayTestKey generates a device key and its peer ID.
func newReplayTestKey(t *testing.T) (crypto.PrivKey, peer.ID) {
	// Generate the device key.
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Derive the peer ID from the key.
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return priv, pid
}
