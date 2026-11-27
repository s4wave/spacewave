package sobject

import (
	"bytes"
	"cmp"
	"os"
	"slices"
	"strings"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
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

// carriedBudget bounds the encoded size of a checkpoint's carried entries,
// about ten thousand refusals, so they never crowd out the state data. A
// signer over budget drops whole entries, the oldest by Height first. By
// invariant 3 that costs only unknown outcomes.
const carriedBudget = MaxSyncStateSize / 10

// carriedReserve is the part of MaxSyncStateSize that carriedRoom leaves for
// what is added to a checkpoint body after its carried entries: the
// signatures, and the growth of the enclosing length prefixes. A signature
// takes about a hundred bytes for an ed25519 key and about a kilobyte for
// RSA-4096, so the reserve holds hundreds of ed25519 co-signers or about sixty
// RSA-4096 ones.
const carriedReserve = 64 * 1024

// carriedRoom returns the encoded size the carried entries of inner may take.
// inner holds none yet. CoSign and Verify refuse a checkpoint larger than
// MaxSyncStateSize, and the state data alone may take nearly all of it, so
// the entries get only what the rest of the signed checkpoint and
// carriedReserve leave, and at most carriedBudget. They are the one part a
// signer can drop safely, so they never stop a checkpoint. The result is
// negative when nothing is left.
func carriedRoom(inner *SOCheckpointInner) int {
	rest := protobuf_go_lite.SizeMessage(1, inner.SizeVT())
	return min(carriedBudget, MaxSyncStateSize-rest-carriedReserve)
}

// carryOutcomes returns the carried outcomes of the checkpoint after prev that
// covers prefix, at most room bytes encoded: for each author of prev that no operation in prefix
// acknowledges, the span of its operations the checkpoints from its entry
// through prev covered, with every refusal they recorded in it, ordered by
// nonce, then hash. An author that equivocated has each of its refused
// operations at that nonce listed. get returns
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
//     dropped early, by an acknowledgment or for room, therefore costs
//     an unknown outcome, never a wrong one.
//  4. Signed: every entry is copied from prev, a verified checkpoint, or from
//     its Refused and PrevAuthors, and the signer signs the result.
//
// Author A acknowledges prev when prefix holds an operation of A that built
// on every author head of prev, so A held every operation prev covers when
// it wrote it. A's entry then ends. An acknowledgment shows that A held those
// operations, not that every device signing as A decided them: when two
// devices share one identity, one's acknowledgment ends the entry the other
// still needed, which costs that device unknown outcomes only. An author that
// never acknowledges, such as one that left, keeps its entry, which grows only
// with its own refusals, until it no longer fits in room.
//
// A group checkpoint records no outcomes, so the checkpoint after it starts and
// carries no entry: no record says which operations in the group's span were
// refused, so no entry could stay complete across it. A device that missed a
// group checkpoint decides the operations of that span, and of the entries open
// before it, as unknown.
//
// Cost: one pass over prefix and the links of each operation, with a word of
// bits per 64 authors of prev, once per signed checkpoint.
func carryOutcomes(prev *SOCheckpointInner, get func(h []byte) *SOOperationInner, prefix [][]byte, room int) []*SOCheckpointCarried {
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

	// Extend each unacknowledged author's span through prev. A new entry
	// starts at prev's height.
	acked := acknowledged(prev.GetAuthors(), get, prefix)
	var out []*SOCheckpointCarried
	for _, head := range prev.GetAuthors() {
		peerID := head.GetPeerId()
		if _, ok := acked[peerID]; ok {
			continue
		}
		from, refused, height := prevFrom[peerID], []*SOOperationPosition(nil), prev.GetHeight()
		if entry := carried[peerID]; entry != nil {
			from, height = entry.GetFrom(), entry.GetHeight()
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
		slices.SortFunc(refused, comparePosition)
		refused = slices.CompactFunc(refused, func(a, b *SOOperationPosition) bool { return comparePosition(a, b) == 0 })
		out = append(out, &SOCheckpointCarried{PeerId: peerID, From: from.CloneVT(), Refused: refused, Height: height})
	}
	return trimCarried(out, room)
}

// trimCarried drops the oldest of carried, by Height and then peer ID, until
// they encode, as the repeated field they fill, in at most room bytes. It
// keeps peer ID order.
func trimCarried(carried []*SOCheckpointCarried, room int) []*SOCheckpointCarried {
	// Keep every entry that fits.
	size := 0
	for _, entry := range carried {
		size += carriedSize(entry)
	}
	if size <= room {
		return carried
	}

	// Drop the oldest until the rest fit.
	byAge := slices.Clone(carried)
	slices.SortStableFunc(byAge, func(a, b *SOCheckpointCarried) int { return cmp.Compare(a.GetHeight(), b.GetHeight()) })
	drop := make(map[*SOCheckpointCarried]struct{})
	for _, entry := range byAge {
		if size <= room {
			break
		}
		size -= carriedSize(entry)
		drop[entry] = struct{}{}
	}
	return slices.DeleteFunc(carried, func(entry *SOCheckpointCarried) bool {
		_, ok := drop[entry]
		return ok
	})
}

// carriedSize returns the encoded size of entry as an element of the carried
// field, its tag and length prefix included.
func carriedSize(entry *SOCheckpointCarried) int {
	return protobuf_go_lite.SizeMessage(1, entry.SizeVT())
}

// comparePosition orders positions of one author by nonce, then hash.
func comparePosition(a, b *SOOperationPosition) int {
	if c := cmp.Compare(a.GetNonce(), b.GetNonce()); c != 0 {
		return c
	}
	return bytes.Compare(a.GetOpHash(), b.GetOpHash())
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

// validateCarried checks that each carried entry of the checkpoint at height
// names one author, in peer ID order, starts at an earlier checkpoint, and
// lists that author's refusals above From once each, ordered by nonce, then
// hash.
func validateCarried(height uint64, carried []*SOCheckpointCarried) error {
	for j, entry := range carried {
		// Check the author and its order.
		if _, err := parsePeerIDField(entry.GetPeerId()); err != nil {
			return errors.Wrapf(err, "outcomes carried[%d]", j)
		}
		if j > 0 && strings.Compare(carried[j-1].GetPeerId(), entry.GetPeerId()) >= 0 {
			return errors.New("outcomes carried must be strictly sorted by peer_id")
		}
		if entry.GetHeight() >= height {
			return errors.Errorf("outcomes carried[%d] height must be below the checkpoint's", j)
		}

		// Check the span's start and refusals.
		if from := entry.GetFrom(); from != nil {
			if err := validatePosition(from); err != nil || from.GetPeerId() != entry.GetPeerId() {
				return errors.Errorf("outcomes carried[%d] from must name an operation of its author", j)
			}
		}
		from := entry.GetFrom().GetNonce()
		for k, pos := range entry.GetRefused() {
			if err := validatePosition(pos); err != nil || pos.GetPeerId() != entry.GetPeerId() || pos.GetNonce() <= from ||
				(k > 0 && comparePosition(entry.GetRefused()[k-1], pos) >= 0) {
				return errors.Errorf("outcomes carried[%d] refused must name operations of its author above from, once each, in order", j)
			}
		}
	}
	return nil
}
