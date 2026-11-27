package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// TestColdMountBudget asserts the documented per-SO cold-mount request budget.
//
// Budget: when a previously-unknown SO is mounted from a fresh session, HTTP
// fan-out is bounded and classifiable. Counting middleware buckets requests
// by the X-Alpha-Seed-Reason header and by URL path. Tighten the budget here
// when a change removes an expected fetch. Every tightening is
// intentional.
//
// Per-SO cold mount (warm verified-state cache):
//   - /api/sobject/{id}/state             x1   cold-seed
//   - /api/sobject/{id}/config-chain      x0
//
// Per-SO cold mount (no verified-state cache):
//   - /api/sobject/{id}/state             x1   cold-seed
//   - /api/sobject/{id}/config-chain      x1   config-chain-verify
//
// Account-level cold SO list bootstrap:
//   - /api/sobject/list                   x1   list-bootstrap
//
// Rejoin and mailbox fan-out are covered by their own unit tests. This test
// pins only the guaranteed per-SO fetches the cloudSOHost cold mount path
// issues on its own.
func TestColdMountBudget(t *testing.T) {
	// A warm verified cache pulls only /state.
	t.Run("warm_per_so_mount", func(t *testing.T) {
		// Serve a state at the cached head.
		const soID = "so-warm"
		priv, pid := generateTestKeypair(t)
		warmHash := []byte("warm-config-chain-hash")
		state := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				ConfigChainHash:  append([]byte(nil), warmHash...),
				ConfigChainSeqno: 5,
			},
		}
		stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
		counter := newBudgetCounter()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.record(r)
			switch r.URL.Path {
			case "/api/sobject/" + soID + "/state":
				_, _ = w.Write(stateJSON)
			case "/api/sobject/" + soID + "/config-chain":
				_, _ = w.Write([]byte("{}"))
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer srv.Close()

		// Pull through a host over the cache.
		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			cloudSOHostHooks{},
			nil,
		)
		if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
			t.Fatalf("pullState: %v", err)
		}

		// Only the cold seed was fetched.
		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
	})

	// A cold host pulls /state and then verifies /config-chain.
	t.Run("cold_per_so_mount", func(t *testing.T) {
		// Serve a state and an empty chain.
		const soID = "so-cold"
		priv, pid := generateTestKeypair(t)
		state := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				ConfigChainHash:  []byte("server-hash"),
				ConfigChainSeqno: 5,
			},
		}
		stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
		counter := newBudgetCounter()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.record(r)
			switch r.URL.Path {
			case "/api/sobject/" + soID + "/state":
				_, _ = w.Write(stateJSON)
			case "/api/sobject/" + soID + "/config-chain":
				// Empty response keeps syncConfigChain from failing on hash
				// verification while still counting the HTTP fan-out.
				_, _ = w.Write([]byte("{}"))
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer srv.Close()

		// Pull through a host over the cache.
		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			nil,
			cloudSOHostHooks{},
			nil,
		)
		if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
			t.Fatalf("pullState: %v", err)
		}
		if !host.configChangedRoutine.Pending() {
			t.Fatal("cold mount did not signal config verifier; verifier would not run")
		}
		host.handleConfigChanged(context.Background())

		// The seed and the verification were fetched once each.
		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 1)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 1)
	})

	// Execute cold-seeds a host with no state, subscribed before the fetch.
	t.Run("execute_cold_seeds_when_state_missing", func(t *testing.T) {
		// Expose the host's notification callback.
		const soID = "so-execute-cold"
		priv, pid := generateTestKeypair(t)
		warmHash := []byte("warm-config-chain-hash")
		state := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				ConfigChainHash:  append([]byte(nil), warmHash...),
				ConfigChainSeqno: 5,
			},
		}
		stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
		tracker := newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil })
		notify := func() soEvents {
			var events soEvents
			tracker.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				events = tracker.soEvents[soID]
			})
			return events
		}

		// Serve the state only after the subscription exists.
		counter := newBudgetCounter()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.record(r)
			switch r.URL.Path {
			case "/api/sobject/" + soID + "/state":
				if notify() == nil {
					t.Error("initial state fetch began before notification subscription")
				}
				_, _ = w.Write(stateJSON)
			case "/api/sobject/" + soID + "/config-chain":
				_, _ = w.Write([]byte("{}"))
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer srv.Close()

		// Pull through a host over the cache.
		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			tracker,
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			cloudSOHostHooks{},
			nil,
		)

		// Execute, delivering a notification once the mount publishes. The
		// peer host starts with the subscription, before the initial state,
		// and lives as long as the run.
		execCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		ready := make(chan struct{})
		var runCtx context.Context
		started := func(ctx context.Context) {
			if notify() == nil {
				t.Error("peer host started before notification subscription")
			}
			if host.stateCtr.GetValue() != nil {
				t.Error("peer host started after initial state")
			}
			runCtx = ctx
		}
		go func() {
			errCh <- host.execute(execCtx, started, func(context.Context) error {
				if runCtx == nil {
					return errors.New("mount published before its peer host started")
				}
				// The mount publishes only with a state and a subscription.
				if host.stateCtr.GetValue() == nil {
					return errors.New("mount published before initial state")
				}
				events := notify()
				if events == nil {
					return errors.New("mount published without notification subscription")
				}
				events.soNotify(&api.SONotifyEventPayload{
					Seqno: 2,
					StateMessage: &api.SOStateMessage{
						Seqno:   2,
						Content: &api.SOStateMessage_Snapshot{Snapshot: state.CloneVT()},
					},
				})
				close(ready)
				return nil
			})
		}()

		// The mount becomes ready and keeps the notification.
		select {
		case <-ready:
		case err := <-errCh:
			t.Fatalf("host did not become ready: %v", err)
		case <-time.After(time.Second):
			t.Fatal("host did not become ready")
		}
		// The state routine applies the notification after the mount.
		applyCtx, applyCancel := context.WithTimeout(context.Background(), time.Second)
		defer applyCancel()
		for {
			var seqno uint64
			var waitCh <-chan struct{}
			host.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				seqno, waitCh = host.lastSeqno, getWaitCh()
			})
			if seqno == 2 {
				break
			}
			select {
			case <-waitCh:
			case <-applyCtx.Done():
				t.Fatalf("notification during mount was lost: sequence %d", seqno)
			}
		}

		// Stop the host once its snapshot is published.
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		if _, err := host.snapCtr.WaitValue(waitCtx, nil); err != nil {
			t.Fatalf("wait for snapshot: %v", err)
		}
		cancel()
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() = %v, want context canceled", err)
		}
		if runCtx.Err() == nil {
			t.Fatal("peer host outlived its run")
		}

		// Only the cold seed was fetched.
		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
		counter.assertReason(t, SeedReasonRejoin, 0)
	})

	// Execute reuses a state seeded by rejoin instead of seeding again.
	t.Run("execute_reuses_rejoin_seed", func(t *testing.T) {
		// Serve a state at the cached head.
		const soID = "so-execute-rejoin"
		priv, pid := generateTestKeypair(t)
		warmHash := []byte("warm-config-chain-hash")
		state := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				ConfigChainHash:  append([]byte(nil), warmHash...),
				ConfigChainSeqno: 5,
			},
		}
		stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
		counter := newBudgetCounter()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.record(r)
			switch r.URL.Path {
			case "/api/sobject/" + soID + "/state":
				_, _ = w.Write(stateJSON)
			case "/api/sobject/" + soID + "/config-chain":
				_, _ = w.Write([]byte("{}"))
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer srv.Close()

		// Pull through a host over the cache.
		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			cloudSOHostHooks{},
			nil,
		)

		// Seed through rejoin, then execute.
		if err := host.ensureInitialState(context.Background(), SeedReasonRejoin); err != nil {
			t.Fatalf("ensureInitialState(rejoin): %v", err)
		}
		execCtx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.Execute(execCtx)
		}()

		// Stop the host once its snapshot is published.
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		if _, err := host.snapCtr.WaitValue(waitCtx, nil); err != nil {
			t.Fatalf("wait for snapshot: %v", err)
		}
		cancel()
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() = %v, want context canceled", err)
		}

		// Only the rejoin seed was fetched.
		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonRejoin, 1)
		counter.assertReason(t, SeedReasonColdSeed, 0)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
	})

	// Listing SharedObjects fetches the list once.
	t.Run("list_bootstrap", func(t *testing.T) {
		// Serve only the list.
		priv, pid := generateTestKeypair(t)
		counter := newBudgetCounter()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.record(r)
			if r.URL.Path != "/api/sobject/list" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		// List once and count the request.
		cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
		if _, err := cli.ListSharedObjects(context.Background()); err != nil {
			t.Fatalf("ListSharedObjects: %v", err)
		}
		counter.assertPath(t, "/api/sobject/list", 1)
		counter.assertReason(t, SeedReasonListBootstrap, 1)
	})
}

// budgetCounter buckets observed HTTP requests by X-Alpha-Seed-Reason header
// and by URL path. Goroutine-safe.
type budgetCounter struct {
	// mu guards byReason and byPath.
	mu sync.Mutex
	// byReason counts requests by seed reason.
	byReason map[SeedReason]int
	// byPath counts requests by URL path.
	byPath map[string]int
}

// newBudgetCounter returns an empty budgetCounter.
func newBudgetCounter() *budgetCounter {
	return &budgetCounter{
		byReason: make(map[SeedReason]int),
		byPath:   make(map[string]int),
	}
}

// record counts r under its seed reason and path.
func (b *budgetCounter) record(r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byReason[SeedReason(r.Header.Get(SeedReasonHeader))]++
	b.byPath[r.URL.Path]++
}

// assertPath fails t unless path was requested want times.
func (b *budgetCounter) assertPath(t *testing.T, path string, want int) {
	// Read the count under the lock, then compare.
	t.Helper()
	b.mu.Lock()
	got := b.byPath[path]
	b.mu.Unlock()
	if got != want {
		t.Errorf("path %s hits = %d, want %d", path, got, want)
	}
}

// assertReason fails t unless reason was sent want times.
func (b *budgetCounter) assertReason(t *testing.T, reason SeedReason, want int) {
	// Read the count under the lock, then compare.
	t.Helper()
	b.mu.Lock()
	got := b.byReason[reason]
	b.mu.Unlock()
	if got != want {
		t.Errorf("reason %q hits = %d, want %d", reason, got, want)
	}
}
