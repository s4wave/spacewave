package sobject

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestStablePoint checks that the stable point waits for every roster member
// to build on an operation, that members acknowledge only edits, and that a
// checkpoint at the stable point keeps every later operation placed.
func TestStablePoint(t *testing.T) {
	// Owner A edits twice under writers B and C.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	roster := state.GetConfig().TrimRoster()
	a1 := writeTestOp(t, state, keys[0], "a1")
	a2 := writeTestOp(t, state, keys[0], "a2")
	set := mustOperationSet(t, state)

	// Nothing is stable until B and C build on A's edits, and only they lag.
	if got := set.StablePoint(roster); len(got) != 0 {
		t.Fatalf("stable point before acknowledgments: %d operations", len(got))
	}
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String()}
	if set.NeedsAcknowledgment(ids[0], ids[0], 1) || !set.NeedsAcknowledgment(ids[0], ids[1], 2) || set.NeedsAcknowledgment(ids[0], ids[1], 3) {
		t.Fatal("acknowledgment lag is not the unbuilt edit count")
	}

	// After B and C acknowledge, A's edits are stable and no acknowledgment
	// asks for another.
	writeTestOp(t, state, keys[1], "")
	writeTestOp(t, state, keys[2], "")
	set = mustOperationSet(t, state)
	if got := set.StablePoint(roster); !slices.EqualFunc(got, [][]byte{a1.Hash(), a2.Hash()}, bytes.Equal) {
		t.Fatalf("stable point %x; want a1, a2", got)
	}
	for _, id := range ids {
		if set.NeedsAcknowledgment(ids[0], id, 1) {
			t.Fatal("an acknowledgment asked for another")
		}
	}

	// An acknowledgment of the checkpointer asks B and C to answer at once.
	writeTestOp(t, state, keys[0], "")
	set = mustOperationSet(t, state)
	if set.NeedsAcknowledgment(ids[0], ids[0], 1) || !set.NeedsAcknowledgment(ids[0], ids[1], AcknowledgmentLag) {
		t.Fatal("the checkpointer's acknowledgment did not ask the members to answer")
	}
	writeTestOp(t, state, keys[1], "")
	writeTestOp(t, state, keys[2], "")
	set = mustOperationSet(t, state)
	for _, id := range ids {
		if set.NeedsAcknowledgment(ids[0], id, 1) {
			t.Fatal("an answer asked for another")
		}
	}

	// A checkpoint at the stable point keeps the answers placed.
	prefix := set.StablePoint(roster)
	checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, keys[0], prefix, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if got := mustOperationSet(t, state).Order(); len(got) != 2 || len(state.GetOps()) != 2 {
		t.Fatalf("checkpoint left %d placed of %d held; want 2 of 2", len(got), len(state.GetOps()))
	}
}

// TestCheckpointCoverKeepsLinks checks that a checkpoint covering any prefix
// of the order keeps the rest of the order placed, even when a later
// operation references a covered operation that is not a head of the prefix.
func TestCheckpointCoverKeepsLinks(t *testing.T) {
	// Sign each operation with a fixed config hash.
	privA, _ := vectorKey(t, "cover a")
	privB, _ := vectorKey(t, "cover b")
	build := func(priv crypto.PrivKey, link *SOOperationLink) *SOOperation {
		// Sign the operation under a fixed config.
		t.Helper()
		link.ConfigHash = bytes.Repeat([]byte{3}, 32)
		op, err := BuildSOOperation(vectorObjectID, priv, []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}
		return op
	}

	// a1 <- a2 <- a3, with b1 naming a1 and b2 naming a2.
	a1 := build(privA, &SOOperationLink{Nonce: 1})
	a2 := build(privA, &SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := build(privA, &SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})
	b1 := build(privB, &SOOperationLink{Nonce: 1, Parents: []*SOOperationPosition{opPosition(t, a1)}})
	b2 := build(privB, &SOOperationLink{Nonce: 2, PrevOpHash: b1.Hash(), Parents: []*SOOperationPosition{opPosition(t, a2)}})

	// Hold them in one set.
	ops := []*SOOperation{a1, a2, a3, b1, b2}
	set := NewSOOperationSet(vectorObjectID, nil)
	for _, op := range ops {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err)
		}
	}

	// Cover each prefix and replay the rest above the checkpoint.
	order := set.Order()
	for n := range order {
		above := NewSOOperationSet(vectorObjectID, &SOCheckpointInner{Authors: set.cover(order[:n])})
		for _, op := range ops {
			if _, err := above.Add(op); err != nil {
				t.Fatal(err)
			}
		}
		if got := above.Order(); !slices.EqualFunc(got, order[n:], bytes.Equal) {
			t.Fatalf("covering %d operations placed %d of %d", n, len(got), len(order)-n)
		}
	}
}

// mustOperationSet returns the operation set of state.
func mustOperationSet(t *testing.T, state *SOState) *SOOperationSet {
	t.Helper()
	set, err := state.OperationSet(mockSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// TestCarryOutcomes checks that a checkpoint carries an author's refusals and
// span start until the author acknowledges the previous checkpoint by building
// on all of its heads, and that it starts no entry from a previous checkpoint
// that records no span.
func TestCarryOutcomes(t *testing.T) {
	// Authors a and b. The previous checkpoint covers a to 5 and b to 2,
	// newly from a@3 and b@1, refused a@4, and carries a's entry from a@1
	// with a@2 refused.
	at := func(peer string, nonce uint64) *SOOperationPosition {
		return &SOOperationPosition{PeerId: peer, Nonce: nonce, OpHash: []byte(peer + strconv.FormatUint(nonce, 10))}
	}
	prev := &SOCheckpointInner{
		Height:  3,
		Authors: []*SOOperationPosition{at("a", 5), at("b", 2)},
		Outcomes: &SOCheckpointOutcomes{
			Refused:     []*SOOperationPosition{at("a", 4)},
			PrevAuthors: &SOCheckpointAuthors{Authors: []*SOOperationPosition{at("a", 3), at("b", 1)}},
			Carried:     []*SOCheckpointCarried{{PeerId: "a", From: at("a", 1), Refused: []*SOOperationPosition{at("a", 2)}, Height: 2}},
		},
	}
	op := func(peer string, nonce uint64, parents ...*SOOperationPosition) *SOOperationInner {
		return &SOOperationInner{PeerId: peer, Nonce: nonce, PrevOpHash: at(peer, nonce-1).GetOpHash(), Parents: parents}
	}
	carry := func(ops ...*SOOperationInner) []*SOCheckpointCarried {
		byHash := make(map[string]*SOOperationInner, len(ops))
		prefix := make([][]byte, 0, len(ops))
		for _, inner := range ops {
			h := at(inner.GetPeerId(), inner.GetNonce()).GetOpHash()
			byHash[string(h)] = inner
			prefix = append(prefix, h)
		}
		return carryOutcomes(prev, func(h []byte) *SOOperationInner { return byHash[string(h)] }, prefix)
	}
	want := func(name string, got []*SOCheckpointCarried, want ...*SOCheckpointCarried) {
		t.Helper()
		if !slices.EqualFunc(got, want, (*SOCheckpointCarried).EqualVT) {
			t.Fatalf("%s: carried %v, want %v", name, got, want)
		}
	}
	aEntry := &SOCheckpointCarried{PeerId: "a", From: at("a", 1), Refused: []*SOOperationPosition{at("a", 2), at("a", 4)}, Height: 2}
	bEntry := &SOCheckpointCarried{PeerId: "b", From: at("b", 1), Height: 3}

	// With no acknowledgment, a's entry grows by prev's refusal and b's
	// starts at its previous head.
	want("none acknowledge", carry(), aEntry, bEntry)

	// b builds on a@5 and its own b@2, so only a's entry remains. a building
	// on b's operation reaches b@2 through it and ends a's entry as well.
	b3 := op("b", 3, at("a", 5))
	want("b acknowledges", carry(b3), aEntry)
	want("both acknowledge", carry(b3, op("a", 6, at("b", 3))))

	// An operation that builds on its own chain alone acknowledges nothing.
	want("own chain only", carry(op("a", 6)), aEntry, bEntry)

	// An author whose span is empty owes nothing.
	prev.Outcomes.PrevAuthors.Authors[1] = at("b", 2)
	want("nothing owed", carry(), aEntry)

	// A previous checkpoint that records no span starts no entry.
	prev.Outcomes.PrevAuthors = nil
	want("no recorded span", carry())
}

// TestCarriedEquivocation checks that a signer carries both refused operations
// of an author that equivocated at one nonce, in hash order, and that the
// checkpoint holding them signs and verifies, so an equivocation cannot stop
// checkpoints. An entry listing one refusal twice is refused.
func TestCarriedEquivocation(t *testing.T) {
	// The writer signed two operations at nonce 5, and the previous
	// checkpoint refused both, listed against hash order.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	owner, writer := peers[0].GetPeerID().String(), peers[1].GetPeerID().String()
	at := func(peer string, nonce uint64, tag string) *SOOperationPosition {
		h := sha256.Sum256([]byte(peer + tag))
		return &SOOperationPosition{PeerId: peer, Nonce: nonce, OpHash: h[:]}
	}
	heads := func(positions ...*SOOperationPosition) []*SOOperationPosition {
		slices.SortFunc(positions, func(a, b *SOOperationPosition) int { return strings.Compare(a.GetPeerId(), b.GetPeerId()) })
		return positions
	}
	first, second := at(writer, 5, "first"), at(writer, 5, "second")
	if comparePosition(first, second) < 0 {
		first, second = second, first
	}
	prev := &SOCheckpointInner{
		Height:  3,
		Authors: heads(at(owner, 1, "owner"), at(writer, 5, "first")),
		Outcomes: &SOCheckpointOutcomes{
			Refused:     []*SOOperationPosition{first, second},
			PrevAuthors: &SOCheckpointAuthors{Authors: heads(at(owner, 1, "owner"), at(writer, 4, "four"))},
		},
	}

	// The next checkpoint carries the writer's entry with both refusals.
	carried := carryOutcomes(prev, func([]byte) *SOOperationInner { return nil }, nil)
	if len(carried) != 1 || len(carried[0].GetRefused()) != 2 || comparePosition(carried[0].GetRefused()[0], carried[0].GetRefused()[1]) >= 0 {
		t.Fatalf("carried %v, want the writer's two refusals in hash order", carried)
	}
	configHash, prevHash := sha256.Sum256([]byte("config")), sha256.Sum256([]byte("prev"))
	next := &SOCheckpointInner{
		SharedObjectId:     mockSharedObjectID,
		Height:             4,
		PrevCheckpointHash: prevHash[:],
		ConfigHash:         configHash[:],
		ReplayVersion:      SOReplayVersion,
		Authors:            prev.GetAuthors(),
		Outcomes:           &SOCheckpointOutcomes{PrevAuthors: &SOCheckpointAuthors{Authors: prev.GetAuthors()}, Carried: carried},
	}
	checkpoint, err := BuildSOCheckpoint(keys[0], next)
	if err != nil {
		t.Fatalf("sign the checkpoint carrying an equivocation: %v", err)
	}
	if inner, _, err := checkpoint.Verify(mockSharedObjectID); err != nil || len(inner.GetOutcomes().GetCarried()) != 1 {
		t.Fatalf("verify the checkpoint carrying an equivocation: %v", err)
	}

	// A refusal listed twice is malformed.
	carried[0].Refused = append(carried[0].Refused, carried[0].GetRefused()[1])
	if _, err := BuildSOCheckpoint(keys[0], next); err == nil {
		t.Fatal("signed a carried entry that lists a refusal twice")
	}
}

// TestCarriedBudget checks that carried entries that outgrow carriedBudget
// lose the oldest whole entries first, and that the rest fit.
func TestCarriedBudget(t *testing.T) {
	// Thirty authors that never acknowledge, each owed two thousand
	// refusals, about three times the budget.
	const authors, refusals = 30, 2000
	prev := &SOCheckpointInner{Height: 100, Outcomes: &SOCheckpointOutcomes{PrevAuthors: &SOCheckpointAuthors{}}}
	for i := range authors {
		peer := "p" + strconv.Itoa(10+i)
		h := sha256.Sum256([]byte(peer))
		pos := func(nonce uint64) *SOOperationPosition {
			return &SOOperationPosition{PeerId: peer, Nonce: nonce, OpHash: h[:]}
		}
		entry := &SOCheckpointCarried{PeerId: peer, From: pos(1), Height: uint64(1 + (i*7)%authors)}
		for n := range uint64(refusals) {
			entry.Refused = append(entry.Refused, pos(2+n))
		}
		prev.Authors = append(prev.Authors, pos(3000))
		prev.Outcomes.PrevAuthors.Authors = append(prev.Outcomes.PrevAuthors.Authors, pos(2999))
		prev.Outcomes.Carried = append(prev.Outcomes.Carried, entry)
	}

	// The kept entries fit, are newer than every dropped one, and keep
	// peer ID order.
	carried := carryOutcomes(prev, func([]byte) *SOOperationInner { return nil }, nil)
	size, oldestKept := 0, uint64(authors+1)
	kept := make(map[string]struct{}, len(carried))
	for j, entry := range carried {
		size += entry.SizeVT()
		oldestKept = min(oldestKept, entry.GetHeight())
		kept[entry.GetPeerId()] = struct{}{}
		if j > 0 && carried[j-1].GetPeerId() >= entry.GetPeerId() {
			t.Fatal("trimmed entries left peer ID order")
		}
	}
	if len(carried) == 0 || len(carried) == authors || size > carriedBudget {
		t.Fatalf("kept %d of %d entries, %d bytes, want some dropped and at most %d bytes", len(carried), authors, size, carriedBudget)
	}
	for _, entry := range prev.GetOutcomes().GetCarried() {
		if _, ok := kept[entry.GetPeerId()]; !ok && entry.GetHeight() >= oldestKept {
			t.Fatalf("dropped the entry from height %d while keeping one from %d", entry.GetHeight(), oldestKept)
		}
	}
}
