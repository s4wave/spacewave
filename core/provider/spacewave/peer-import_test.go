package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// TestCloudPeerImportCommitsWithoutPublication proves cloud imports use durable
// local storage, reject failed writes and stale responses, and survive reopening.
func TestCloudPeerImportCommitsWithoutPublication(t *testing.T) {
	// Any HTTP request would violate the peer-import boundary.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	// Sign a genesis state as the only owner.
	ctx := t.Context()
	priv, pid := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, priv, pid.String())
	initial, genesis, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.New()), testStepFactorySet(), testSharedObjectID, priv, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Save the verified cache in a real account store.
	cache := &api.VerifiedSOStateCache{
		GenesisHash: initial.Config.ConfigChainHash, VerifiedConfigChainHash: initial.Config.ConfigChainHash,
		CurrentConfig: initial.Config, ConfigHistory: []*sobject.SOConfigChange{genesis},
	}
	account := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	if err := account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache); err != nil {
		t.Fatal(err)
	}

	// Open a host whose cache writes fail until released.
	failed := errors.New("injected cache write failure")
	var failWrite atomic.Bool
	failWrite.Store(true)
	persist := func(ctx context.Context, next *api.VerifiedSOStateCache) error {
		if failWrite.Load() {
			return failed
		}
		return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, next)
	}
	host := newCloudSOHost(logrus.New().WithField("test", t.Name()), client, testSharedObjectID, "", nil, priv, pid, nil, cache, persist, nil)
	host.stateCtr.SetValue(initial)
	host.soHost.SetContext(ctx)
	t.Cleanup(host.soHost.ClearContext)

	// Sign a newer checkpoint under an invite change.
	change, err := sobject.BuildSOConfigChange(testSharedObjectID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := initial.CloneVT()
	candidate.Config, err = sobject.VerifyConfigChange(testSharedObjectID, initial.Config, change)
	if err != nil {
		t.Fatal(err)
	}
	candidate = advanceTestCheckpoint(t, candidate, priv, "trusted checkpoint")

	// Failure leaves the held authority, watched state and saved cache unchanged.
	if err := host.soHost.ImportPeerSnapshot(ctx, candidate, []*sobject.SOConfigChange{change}, pid, nil); !errors.Is(err, failed) {
		t.Fatalf("failed import = %v", err)
	}
	saved, err := account.loadVerifiedSOStateCache(ctx, testSharedObjectID)
	if err != nil || !saved.EqualVT(cache) || !host.stateCtr.GetValue().EqualVT(initial) {
		t.Fatalf("failed import changed accepted data: %v", err)
	}

	// Commit through the real account store.
	failWrite.Store(false)
	if err := host.soHost.ImportPeerSnapshot(ctx, candidate, []*sobject.SOConfigChange{change}, pid, nil); err != nil {
		t.Fatal(err)
	}
	saved, err = account.loadVerifiedSOStateCache(ctx, testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen without HTTP seeding.
	reopened := newCloudSOHost(logrus.New().WithField("test", t.Name()), client, testSharedObjectID, "", nil, priv, pid, nil, saved, persist, nil)
	reopened.soHost.SetContext(ctx)
	t.Cleanup(reopened.soHost.ClearContext)
	got, err := reopened.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(candidate) {
		t.Fatalf("reopened peer state mismatch: %v", err)
	}
	suffix, err := reopened.soHost.ReadConfigHistory(ctx, initial.Config.ConfigChainHash, candidate.Config.ConfigChainHash)
	if err != nil || len(suffix) != 1 || !suffix[0].EqualVT(change) {
		t.Fatalf("reopened peer history mismatch: %v", err)
	}

	// A cloud response prepared before the import cannot roll back its checkpoint or authority.
	stale := initial.CloneVT()
	stale.Config = candidate.Config.CloneVT()
	if err := reopened.handleStateDelta(ctx, &api.SOStateMessage{Seqno: 99, Content: &api.SOStateMessage_Snapshot{Snapshot: stale}}); err != nil {
		t.Fatal(err)
	}
	if !reopened.stateCtr.GetValue().EqualVT(candidate) {
		t.Fatal("stale cloud response changed accepted peer state")
	}

	// Hold peer persistence while a cloud completion tries to publish an older checkpoint.
	persisting, allowCommit := make(chan struct{}), make(chan struct{})
	var newer *sobject.SOState
	var blocked bool
	reopened.persistVerifiedStateCache = func(ctx context.Context, cache *api.VerifiedSOStateCache) error {
		if cache.GetPeerState().GetCheckpoint().EqualVT(newer.GetCheckpoint()) && !blocked {
			blocked = true
			close(persisting)
			select {
			case <-allowCommit:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return persist(ctx, cache)
	}

	// Sign an older cloud checkpoint and a newer peer checkpoint after it.
	late := advanceTestCheckpoint(t, candidate, priv, "delayed cloud checkpoint")
	newer = advanceTestCheckpoint(t, late, priv, "newer peer checkpoint")

	// The peer import blocks in persistence before it becomes visible.
	peerDone, cloudDone := make(chan error, 1), make(chan error, 1)
	go func() { peerDone <- reopened.soHost.ImportPeerSnapshot(ctx, newer, nil, pid, nil) }()
	select {
	case <-persisting:
	case <-time.After(time.Second):
		t.Fatal("peer import did not enter persistence")
	}
	if !reopened.stateCtr.GetValue().EqualVT(candidate) {
		t.Fatal("peer state became visible before persistence")
	}

	// A cloud completion races the peer commit and loses.
	cloudStarted := make(chan struct{})
	go func() {
		close(cloudStarted)
		cloudDone <- reopened.handleStateDelta(ctx, &api.SOStateMessage{Seqno: 100, Content: &api.SOStateMessage_Snapshot{Snapshot: late}})
	}()
	<-cloudStarted
	close(allowCommit)
	if err := <-peerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cloudDone; err != nil {
		t.Fatal(err)
	}
	if !reopened.stateCtr.GetValue().EqualVT(newer) || reopened.pending != nil {
		t.Fatal("delayed cloud completion overwrote the peer winner or created a publication")
	}

	// Later valid cloud progress also advances the durable checkpoint used on restart.
	cloudNext := advanceTestCheckpoint(t, newer, priv, "latest cloud checkpoint")
	if err := reopened.handleStateDelta(ctx, &api.SOStateMessage{Seqno: 101, Content: &api.SOStateMessage_Snapshot{Snapshot: cloudNext}}); err != nil {
		t.Fatal(err)
	}
	saved, err = account.loadVerifiedSOStateCache(ctx, testSharedObjectID)
	if err != nil || !saved.GetPeerState().EqualVT(cloudNext) {
		t.Fatalf("cloud progress did not update the durable peer snapshot: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("peer import made %d HTTP requests", requests.Load())
	}
}

// advanceTestCheckpoint returns state advanced to the next checkpoint, signed
// with priv, holding data.
func advanceTestCheckpoint(t *testing.T, state *sobject.SOState, priv crypto.PrivKey, data string) *sobject.SOState {
	// Adopt the next checkpoint signed by priv.
	t.Helper()
	checkpoint, err := state.BuildNextCheckpoint(testSharedObjectID, priv, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	next := state.CloneVT()
	if err := next.AdoptCheckpoint(testSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	return next
}
