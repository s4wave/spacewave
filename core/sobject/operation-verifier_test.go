package sobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// newBenchState returns a valid state holding n operations of its owner's
// chain, and the owner's key.
func newBenchState(b *testing.B, n int) (*SOState, crypto.PrivKey) {
	// Start from a valid state with one owner.
	b.Helper()
	peers := createMockPeers(b, 1)
	owner := mustPrivKeys(b, peers)[0]
	state, _ := newTestSOState(b, peers)

	// Sign each operation after its predecessor.
	var prev []byte
	for nonce := uint64(1); nonce <= uint64(n); nonce++ {
		link := &SOOperationLink{Nonce: nonce, PrevOpHash: prev, ConfigHash: state.GetConfig().GetConfigChainHash()}
		op, err := BuildSOOperation(mockSharedObjectID, owner, []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			b.Fatal(err)
		}
		if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
			b.Fatal(err)
		}
		prev = op.Hash()
	}
	return state, owner
}

// BenchmarkCommit measures one local write above a state of n operations: the
// host adds the operation under its lock, then the published snapshot of the
// written state reads its operation set.
func BenchmarkCommit(b *testing.B) {
	for _, n := range []int{32, 256, 1024} {
		b.Run(fmt.Sprintf("ops=%d", n), func(b *testing.B) {
			// Hold the state behind a lock that writes it back.
			base, owner := newBenchState(b, n)
			peerID, err := peer.IDFromPrivateKey(owner)
			if err != nil {
				b.Fatal(err)
			}
			current := base
			host := NewSOHost(nil, nil, func(context.Context, string) (SOStateLock, error) {
				return NewSOStateLock(current, func(_ context.Context, next *SOState, _ ...*SOConfigChange) error {
					current = next
					return nil
				}, func() {}), nil
			}, mockSharedObjectID)
			le := logrus.New().WithField("bench", b.Name())

			// Write one operation above the base state each iteration.
			b.ReportAllocs()
			for b.Loop() {
				current = base
				localID, _, err := host.AddLocalOperation(b.Context(), le, testStepFactorySet(), owner, []byte("data"))
				if err != nil {
					b.Fatal(err)
				}
				snap := NewSOStateParticipantHandle(le, testStepFactorySet(), mockSharedObjectID, current, owner, peerID)
				set, err := snap.GetOperationSet(b.Context())
				if err != nil {
					b.Fatal(err)
				}
				if set.Find(peerID.String(), localID) == nil || set.Len() != n+1 {
					b.Fatal("written operation missing from the set")
				}
			}
		})
	}
}

