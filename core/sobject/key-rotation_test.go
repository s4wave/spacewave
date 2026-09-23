package sobject

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestRotateTransformKeyExhaustion rejects counters that would reuse the zero epoch or root range.
func TestRotateTransformKeyExhaustion(t *testing.T) {
	owner := createMockPeers(t, 1)[0]
	key, err := owner.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	participants := createMockSOState([]peer.Peer{owner}, nil).Config.Participants
	for _, counters := range [][2]uint64{{^uint64(0), 1}, {1, ^uint64(0)}} {
		if _, _, _, err := RotateTransformKey(key, mockSharedObjectID, participants, counters[0], counters[1]); err == nil {
			t.Fatalf("rotation wrapped exhausted counters %v", counters)
		}
	}
}

// TestFindCoveringEpochAmbiguity requires one unambiguous epoch for the requested root.
func TestFindCoveringEpochAmbiguity(t *testing.T) {
	first := &SOKeyEpoch{Epoch: 1, SeqnoStart: 1, SeqnoEnd: 5}
	second := &SOKeyEpoch{Epoch: 2, SeqnoStart: 5}
	if got := FindCoveringEpoch([]*SOKeyEpoch{first, second}, 5); got != nil {
		t.Fatalf("overlapping epochs selected %d", got.GetEpoch())
	}
	if got := FindCoveringEpoch([]*SOKeyEpoch{first, first}, 2); got != nil {
		t.Fatal("duplicate covering entries are ambiguous")
	}
	if got := FindCoveringEpoch([]*SOKeyEpoch{first, second}, 4); got != first {
		t.Fatal("unambiguous earlier range was lost")
	}
	if got := FindCoveringEpoch([]*SOKeyEpoch{first, second}, 6); got != second {
		t.Fatal("unambiguous current range was lost")
	}
	if got := FindCoveringEpoch([]*SOKeyEpoch{nil, first}, 4); got != first {
		t.Fatal("absent entry hid the unique covering epoch")
	}
}
