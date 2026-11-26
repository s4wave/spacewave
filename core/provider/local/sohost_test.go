package provider_local

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

const testSharedObjectID = "test-shared-object"

// newTestLocalSOHost returns a local host over the genesis state of a new
// owner, bound to lifetime, without publishing snapshots.
func newTestLocalSOHost(t *testing.T, lifetime context.Context) *LocalSOHost {
	// Build a host over a new owner's genesis.
	t.Helper()
	soHost, priv := newTestGenesisHost(t, store_kvtx_inmem.NewStore())
	host, err := NewLocalSOHost(lifetime, logrus.NewEntry(logrus.New()), priv, soHost, testSharedObjectID, testStepFactorySet())
	if err != nil {
		t.Fatal(err)
	}
	return host
}

// TestWaitPublishedConfigFencesBodySnapshot checks that a config wait returns
// only once body readers can observe the target config.
func TestWaitPublishedConfigFencesBodySnapshot(t *testing.T) {
	// Build a host and a target config.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	host := newTestLocalSOHost(t, t.Context())
	target := &sobject.SharedObjectConfig{ConfigChainSeqno: 7, ConfigChainHash: []byte("accepted")}

	// A stale config does not release the wait.
	done := make(chan error, 1)
	go func() { done <- host.waitPublishedConfig(ctx, target) }()
	host.publishedConfigCtr.SetValue(&sobject.SharedObjectConfig{ConfigChainSeqno: 6, ConfigChainHash: []byte("stale")})
	select {
	case err := <-done:
		t.Fatalf("wait returned before the body snapshot reached admission: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	// The target config releases it.
	host.publishedConfigCtr.SetValue(target.CloneVT())
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestQueueOperationFencesPublishedSnapshot checks that QueueOperation returns
// only once the snapshot body readers see holds the operation, while its
// receipt already names the operation in the state the signer wrote.
func TestQueueOperationFencesPublishedSnapshot(t *testing.T) {
	// Queue an operation while no snapshot is published.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	host := newTestLocalSOHost(t, t.Context())
	type queued struct {
		// localID identifies the operation in the published snapshot.
		localID string
		// err is the error the queue returned.
		err error
	}
	receipt := &sobject.SignedOperationReceipt{}
	done := make(chan queued, 1)
	go func() {
		localID, _, err := host.QueueOperationWithReceipt(ctx, []byte("op"), receipt)
		done <- queued{localID, err}
	}()

	// The state already holds the signed operation and the receipt names it,
	// yet the call has not returned.
	states, rel, err := host.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	state, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return len(state.GetOps()) != 0, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if op := state.GetOps()[0]; !receipt.MatchesHash(op.Hash()) {
		t.Fatal("receipt does not identify the operation written by the signer")
	}
	select {
	case res := <-done:
		t.Fatalf("operation returned before its snapshot was published: %+v", res)
	default:
	}

	// Publishing releases the call, and the published snapshot holds the operation.
	go func() { _ = host.Execute(ctx) }()
	res := <-done
	if res.err != nil {
		t.Fatal(res.err)
	}
	snap, err := host.stateSnapCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if set.Find(host.peerID.String(), res.localID) == nil {
		t.Fatal("published snapshot does not hold the queued operation")
	}
}

// TestQueueOperationEndsWithLifetime checks that a queued operation whose
// snapshot never arrives stops waiting when the host's lifetime ends, even when
// the caller's context stays live.
func TestQueueOperationEndsWithLifetime(t *testing.T) {
	// Bind the host to a lifetime that has already ended.
	lifetime, cancel := context.WithCancel(t.Context())
	cancel()
	host := newTestLocalSOHost(t, lifetime)

	// The operation is added, and the wait for its snapshot ends with the lifetime.
	ctx, cancelCaller := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelCaller()
	if _, _, err := host.QueueOperation(ctx, []byte("op")); !errors.Is(err, context.Canceled) {
		t.Fatalf("queue after lifetime ended = %v; want context.Canceled", err)
	}
}

// TestSOLinkPeersLinksWritersToEveryone checks the block-peer rule: every
// participant keeps links to the owners and writers, a writer also keeps links
// to the readers, and a reader keeps no link to another reader.
func TestSOLinkPeersLinksWritersToEveryone(t *testing.T) {
	config := &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		{PeerId: "owner", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: "device", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: "reader-a", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
		{PeerId: "reader-b", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
	}}
	owners := []string{"owner"}
	for _, tc := range []struct {
		self string
		want []string
	}{
		{"owner", []string{"device", "reader-a", "reader-b"}},
		{"device", []string{"owner", "reader-a", "reader-b"}},
		{"reader-a", []string{"owner", "device"}},
	} {
		got := soLinkPeers(config, owners, map[string]crypto.PrivKey{tc.self: nil})
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%s links %v, want %v", tc.self, got, tc.want)
		}
	}
}
