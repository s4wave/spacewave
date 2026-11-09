//go:build !goscript

package sobject

import (
	"bytes"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestStateChangeScales checks at two sizes the three costs the body driver
// meets on a state change and a checkpoint: the stable point after the order
// is one pass over the order rather than another order and a walk per roster
// member, adopting a checkpoint through the host's verifier checks no
// signature it already checked, and the snapshots sharing a keyring decode a
// checkpoint once. The timing bounds compare two costs measured in the same
// run, so they hold on a slow or loaded machine.
func TestStateChangeScales(t *testing.T) {
	// Ten writers, as a roster of ten devices.
	peers := createMockPeers(t, 10)
	keys := mustPrivKeys(t, peers)
	for _, size := range []struct{ rounds, dataBytes int }{{10, 512}, {200, 64 << 10}} {
		state := buildRoundsState(t, peers, keys, size.rounds)
		roster := state.GetConfig().TrimRoster()
		n := len(state.GetOps())

		// The stable point is every round but the last, as the walk per
		// member finds, and costs less than the order it follows.
		set := mustOperationSet(t, state)
		stable := set.StablePoint(roster)
		if want := referenceStablePoint(set, roster); len(stable) != n-len(peers) || !slices.EqualFunc(stable, want, bytes.Equal) {
			t.Fatalf("over %d operations the stable point holds %d, the walk per member %d; want %d", n, len(stable), len(want), n-len(peers))
		}
		orderDur := fastest(func() { set.placeOrder() })
		stableDur := fastest(func() { set.StablePoint(roster) })
		if stableDur >= orderDur {
			t.Fatalf("over %d operations the stable point took %v after an order of %v; want less", n, stableDur, orderDur)
		}

		// A checkpoint over half the stable prefix, adopted through the
		// host's verifier, leaves the state an adoption verifying every
		// operation leaves, in a small part of that adoption's time.
		xfrm, err := testHandle(t, state, keys[0]).GetTransformer(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		enc, err := xfrm.EncodeBlock(bytes.Repeat([]byte{'s'}, size.dataBytes))
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, keys[0], stable[:len(stable)/2], enc)
		if err != nil {
			t.Fatal(err)
		}
		host := NewSOOperationVerifier(mockSharedObjectID)
		if _, err := host.OperationSet(state); err != nil {
			t.Fatal(err)
		}
		// A stall only lengthens a run, so the host's adoption takes its
		// fastest run and the verifying adoption, which a stall only helps
		// pass, takes one.
		hostDur, viaHost := fastestOn(state.Clone, func(s *SOState) {
			if err := host.AdoptCheckpoint(s, checkpoint); err != nil {
				t.Fatal(err)
			}
		})
		verifying := state.Clone()
		start := time.Now()
		if err := verifying.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
			t.Fatal(err)
		}
		verifyDur := time.Since(start)
		if !viaHost.EqualVT(verifying) || len(viaHost.GetOps()) != n-len(stable)/2 {
			t.Fatalf("over %d operations adoption through the host kept %d, verifying kept %d; want %d", n, len(viaHost.GetOps()), len(verifying.GetOps()), n-len(stable)/2)
		}
		if hostDur*4 >= verifyDur {
			t.Fatalf("over %d operations adoption through the host took %v, verifying every operation %v; want under a quarter", n, hostDur, verifyDur)
		}

		// Snapshots sharing a keyring decode the new checkpoint once: the
		// next state, one operation later, shares the decoded body.
		keyring := NewSOGrantKeyring(mockSharedObjectID)
		snap := func(state *SOState) *SOCheckpointInner {
			t.Helper()
			inner, err := testHandle(t, state, keys[0]).WithGrantKeyring(keyring).GetCheckpoint(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			return inner
		}
		before := snap(state)
		adopted := snap(viaHost)
		writeTestOp(t, viaHost, keys[1], "")
		if before == adopted || len(adopted.GetStateData()) != size.dataBytes || snap(viaHost) != adopted {
			t.Fatalf("over %d operations the keyring did not decode the %d-byte checkpoint once", n, size.dataBytes)
		}
	}
}

// buildRoundsState returns a state whose writers each write rounds
// acknowledgments, every operation of a round naming every operation of the
// round before, as concurrent devices that each saw the last round do.
func buildRoundsState(t *testing.T, peers []peer.Peer, keys []crypto.PrivKey, rounds int) *SOState {
	// Start from the genesis state.
	t.Helper()
	state, _ := newTestSOState(t, peers)
	ids := make([]string, len(peers))
	for i, p := range peers {
		ids[i] = p.GetPeerID().String()
	}

	// Write each round over the one before.
	var prev []*SOOperationPosition
	last := make(map[string][]byte, len(ids))
	for r := range rounds {
		cur := make([]*SOOperationPosition, 0, len(ids))
		for i, id := range ids {
			link := &SOOperationLink{
				Nonce:      uint64(r + 1),
				PrevOpHash: last[id],
				ConfigHash: state.GetConfig().GetConfigChainHash(),
				KeyEpoch:   state.CurrentKeyEpoch().GetEpoch(),
			}
			for _, pos := range prev {
				if pos.GetPeerId() != id {
					link.Parents = append(link.Parents, pos)
				}
			}
			op, err := BuildSOOperation(mockSharedObjectID, keys[i], nil, link, NewSOOperationLocalID())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
				t.Fatal(err)
			}
			last[id] = op.Hash()
			cur = append(cur, &SOOperationPosition{PeerId: id, Nonce: uint64(r + 1), OpHash: op.Hash()})
		}
		slices.SortFunc(cur, compareOpHash)
		prev = cur
	}
	return state
}

// referenceStablePoint is the stable point as a walk of each roster member's
// ancestry finds it.
func referenceStablePoint(set *SOOperationSet, roster []string) [][]byte {
	// Intersect what each member has built on.
	order := set.Order()
	stable := set.builtOn(order, roster[0])
	for _, peerID := range roster[1:] {
		built := set.builtOn(order, peerID)
		maps.DeleteFunc(stable, func(key string, _ struct{}) bool {
			_, ok := built[key]
			return !ok
		})
	}

	// Take the longest prefix inside it.
	for i, h := range order {
		if _, ok := stable[string(h)]; !ok {
			return order[:i]
		}
	}
	return order
}

// fastestRuns is how many times fastest runs a function.
const fastestRuns = 40

// fastest returns the shortest of fastestRuns runs of fn. The costs compared
// are about a millisecond, so a loaded CI runner needs many runs for one to
// land outside a stall.
func fastest(fn func()) time.Duration {
	best := time.Duration(1<<63 - 1)
	for range fastestRuns {
		start := time.Now()
		fn()
		best = min(best, time.Since(start))
	}
	return best
}

// fastestOn returns the shortest of fastestRuns runs of fn, each on a fresh
// value from prepare outside the timing, and the value of the last run. An
// adoption changes the state it adopts into, so each run takes its own copy.
func fastestOn[T any](prepare func() T, fn func(T)) (time.Duration, T) {
	best := time.Duration(1<<63 - 1)
	var last T
	for range fastestRuns {
		last = prepare()
		start := time.Now()
		fn(last)
		best = min(best, time.Since(start))
	}
	return best, last
}
