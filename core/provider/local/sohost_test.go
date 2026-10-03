package provider_local

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

const testSharedObjectID = "test-shared-object"

// newTestLocalSOHost returns a local host over the genesis state of a new
// owner, without publishing snapshots.
func newTestLocalSOHost(t *testing.T) *LocalSOHost {
	// Build a host over a new owner's genesis.
	t.Helper()
	soHost, priv := newTestGenesisHost(t, store_kvtx_inmem.NewStore())
	host, err := NewLocalSOHost(logrus.NewEntry(logrus.New()), priv, soHost, testSharedObjectID, testStepFactorySet())
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
	host := newTestLocalSOHost(t)
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
// only once the snapshot body readers see holds the operation.
func TestQueueOperationFencesPublishedSnapshot(t *testing.T) {
	// Queue an operation while no snapshot is published.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	host := newTestLocalSOHost(t)
	type queued struct {
		localID string
		err     error
	}
	done := make(chan queued, 1)
	go func() {
		localID, err := host.QueueOperation(ctx, []byte("op"))
		done <- queued{localID, err}
	}()

	// The host state holds the operation, yet the call has not returned.
	states, rel, err := host.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return len(state.GetOps()) != 0, nil
	}, nil); err != nil {
		t.Fatal(err)
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
