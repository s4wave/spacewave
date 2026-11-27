package sobject

import (
	"os"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// CarriedOutcomesEnv names the environment variable that, set to "off", stops
// carried outcomes: a signer records none, and a device decides nothing from
// them.
const CarriedOutcomesEnv = "SPACEWAVE_SOBJECT_CARRIED_OUTCOMES"

// CarriedOutcomesOff reports whether CarriedOutcomesEnv turns carried outcomes
// off.
func CarriedOutcomesOff() bool {
	return os.Getenv(CarriedOutcomesEnv) == "off"
}

// carryOutcomes returns the carried outcomes of the checkpoint after prev that
// covers prefix: for each author of prev that no operation in prefix
// acknowledges, the span of its operations the checkpoints from its entry
// through prev covered, with every refusal they recorded in it. get returns
// the verified body of a held operation by hash. prefix lists the operations
// in replay order, each after the operations it links to.
//
// The entries keep these invariants, which the device that decides from them
// relies on:
//
//  1. Complete: an entry for author A with From f lists every operation of A
//     that a checkpoint refused above f and at or below A's head in prev. A
//     device that reads every other operation of A in that span as applied
//     reads it correctly.
//  2. Built only from recorded spans: an entry starts or grows only from a
//     prev that records its Refused and its PrevAuthors, so the span it adds,
//     PrevAuthors to Authors, is exactly the one prev's Refused lists. A prev
//     signed before those fields starts no entry.
//  3. Dropping is safe: a device that finds no entry for its author decides
//     nothing below prev's head, as before carried outcomes existed. An entry
//     dropped early therefore costs an unknown outcome, never a wrong one.
//  4. Signed: every entry is copied from prev, a verified checkpoint, or from
//     its Refused and PrevAuthors, and the signer signs the result.
//
// Author A acknowledges prev when prefix holds an operation of A that built
// on every author head of prev, so A held every operation prev covers when
// it wrote it. A's entry then ends. An author that never acknowledges, such
// as one that left, keeps its entry, which grows only with its own
// refusals.
//
// Cost: one pass over prefix and the links of each operation, with a word of
// bits per 64 authors of prev, once per signed checkpoint.
func carryOutcomes(prev *SOCheckpointInner, get func(h []byte) *SOOperationInner, prefix [][]byte) []*SOCheckpointCarried {
	// Carry only from a checkpoint that records its span.
	prevOutcomes := prev.GetOutcomes()
	if prevOutcomes.GetPrevAuthors() == nil {
		return nil
	}
	prevFrom := make(map[string]*SOOperationPosition, len(prevOutcomes.GetPrevAuthors().GetAuthors()))
	for _, author := range prevOutcomes.GetPrevAuthors().GetAuthors() {
		prevFrom[author.GetPeerId()] = author
	}
	carried := make(map[string]*SOCheckpointCarried, len(prevOutcomes.GetCarried()))
	for _, entry := range prevOutcomes.GetCarried() {
		carried[entry.GetPeerId()] = entry
	}

	// Extend each unacknowledged author's span through prev.
	acked := acknowledged(prev.GetAuthors(), get, prefix)
	var out []*SOCheckpointCarried
	for _, head := range prev.GetAuthors() {
		peerID := head.GetPeerId()
		if _, ok := acked[peerID]; ok {
			continue
		}
		from, refused := prevFrom[peerID], []*SOOperationPosition(nil)
		if entry := carried[peerID]; entry != nil {
			from = entry.GetFrom()
			for _, pos := range entry.GetRefused() {
				refused = append(refused, pos.CloneVT())
			}
		}
		for _, pos := range prevOutcomes.GetRefused() {
			if pos.GetPeerId() == peerID {
				refused = append(refused, pos.CloneVT())
			}
		}
		if from.GetNonce() >= head.GetNonce() {
			continue
		}
		out = append(out, &SOCheckpointCarried{PeerId: peerID, From: from.CloneVT(), Refused: refused})
	}
	return out
}

// acknowledged returns the authors with an operation in prefix that built on
// every one of heads: one whose links reach, through prefix, an operation of
// each head's author at or above that head's nonce. prefix lists operations
// each after the ones it links to, so one pass carries the reached heads
// forward as bits.
func acknowledged(heads []*SOOperationPosition, get func(h []byte) *SOOperationInner, prefix [][]byte) map[string]struct{} {
	// Give each head its own bit.
	bit := make(map[string]int, len(heads))
	for i, head := range heads {
		bit[head.GetPeerId()] = i
	}
	floor := func(pos *SOOperationPosition) (int, bool) {
		i, ok := bit[pos.GetPeerId()]
		return i, ok && pos.GetNonce() >= heads[i].GetNonce()
	}
	words := (len(heads) + 63) / 64
	full := make([]uint64, words)
	for i := range heads {
		full[i/64] |= 1 << (i % 64)
	}

	// Carry the reached heads from each link to the operation, from the
	// first operation of prefix on.
	acked := make(map[string]struct{})
	index := make(map[string]int, len(prefix))
	masks := make([]uint64, len(prefix)*words)
	for i, h := range prefix {
		inner := get(h)
		index[string(h)] = i
		mask := masks[i*words : (i+1)*words]
		own := &SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce()}
		if b, ok := floor(own); ok {
			mask[b/64] |= 1 << (b % 64)
		}
		for _, link := range links(inner) {
			if j, ok := index[string(link.GetOpHash())]; ok {
				for w, bits := range masks[j*words : (j+1)*words] {
					mask[w] |= bits
				}
			} else if b, ok := floor(link); ok {
				mask[b/64] |= 1 << (b % 64)
			}
		}
		if slices.Equal(mask, full) {
			acked[inner.GetPeerId()] = struct{}{}
		}
	}
	return acked
}

// validateCarried checks that each carried entry names one author, in peer ID
// order, and lists that author's refusals above From in nonce order.
func validateCarried(carried []*SOCheckpointCarried) error {
	for j, entry := range carried {
		// Check the author and its order.
		if _, err := parsePeerIDField(entry.GetPeerId()); err != nil {
			return errors.Wrapf(err, "outcomes carried[%d]", j)
		}
		if j > 0 && strings.Compare(carried[j-1].GetPeerId(), entry.GetPeerId()) >= 0 {
			return errors.New("outcomes carried must be strictly sorted by peer_id")
		}

		// Check the span's start and refusals.
		if from := entry.GetFrom(); from != nil {
			if err := validatePosition(from); err != nil || from.GetPeerId() != entry.GetPeerId() {
				return errors.Errorf("outcomes carried[%d] from must name an operation of its author", j)
			}
		}
		last := entry.GetFrom().GetNonce()
		for _, pos := range entry.GetRefused() {
			if err := validatePosition(pos); err != nil || pos.GetPeerId() != entry.GetPeerId() || pos.GetNonce() <= last {
				return errors.Errorf("outcomes carried[%d] refused must name operations of its author above from, in order", j)
			}
			last = pos.GetNonce()
		}
	}
	return nil
}
