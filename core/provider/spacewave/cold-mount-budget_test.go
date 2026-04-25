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
// when an iteration removes an expected fetch; every tightening is
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
// Rejoin and mailbox fan-out are covered by their own unit tests; this test
// pins only the guaranteed per-SO fetches the cloudSOHost cold mount path
// issues on its own.
func TestColdMountBudget(t *testing.T) {
	t.Run("warm_per_so_mount", func(t *testing.T) {
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

		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			nil,
			nil,
		)

		if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
			t.Fatalf("pullState: %v", err)
		}

		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
	})

	t.Run("cold_per_so_mount", func(t *testing.T) {
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

		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			nil,
			nil,
			nil,
		)

		if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
			t.Fatalf("pullState: %v", err)
		}
		select {
		case <-host.configChangedCh:
		default:
			t.Fatal("cold mount did not signal configChangedCh; verifier would not run")
		}
		host.handleConfigChanged(context.Background())

		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 1)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 1)
	})

	t.Run("execute_cold_seeds_when_state_missing", func(t *testing.T) {
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

		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			nil,
			nil,
		)

		execCtx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.Execute(execCtx)
		}()

		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		if _, err := host.snapCtr.WaitValue(waitCtx, nil); err != nil {
			t.Fatalf("wait for snapshot: %v", err)
		}
		cancel()
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() = %v, want context canceled", err)
		}

		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonColdSeed, 1)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
		counter.assertReason(t, SeedReasonRejoin, 0)
	})

	t.Run("execute_reuses_rejoin_seed", func(t *testing.T) {
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

		host := newCloudSOHost(
			logrus.New().WithField("test", t.Name()),
			NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
			soID,
			"",
			newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
			priv,
			pid,
			nil,
			&api.VerifiedSOStateCache{
				VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
				VerifiedConfigChainSeqno: 5,
			},
			nil,
			nil,
		)

		if err := host.ensureInitialState(context.Background(), SeedReasonRejoin); err != nil {
			t.Fatalf("ensureInitialState(rejoin): %v", err)
		}

		execCtx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.Execute(execCtx)
		}()

		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		if _, err := host.snapCtr.WaitValue(waitCtx, nil); err != nil {
			t.Fatalf("wait for snapshot: %v", err)
		}
		cancel()
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() = %v, want context canceled", err)
		}

		counter.assertPath(t, "/api/sobject/"+soID+"/state", 1)
		counter.assertPath(t, "/api/sobject/"+soID+"/config-chain", 0)
		counter.assertReason(t, SeedReasonRejoin, 1)
		counter.assertReason(t, SeedReasonColdSeed, 0)
		counter.assertReason(t, SeedReasonConfigChainVerify, 0)
	})

	t.Run("list_bootstrap", func(t *testing.T) {
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
	mu       sync.Mutex
	byReason map[SeedReason]int
	byPath   map[string]int
}

func newBudgetCounter() *budgetCounter {
	return &budgetCounter{
		byReason: make(map[SeedReason]int),
		byPath:   make(map[string]int),
	}
}

func (b *budgetCounter) record(r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byReason[SeedReason(r.Header.Get(SeedReasonHeader))]++
	b.byPath[r.URL.Path]++
}

func (b *budgetCounter) assertPath(t *testing.T, path string, want int) {
	t.Helper()
	b.mu.Lock()
	got := b.byPath[path]
	b.mu.Unlock()
	if got != want {
		t.Errorf("path %s hits = %d, want %d", path, got, want)
	}
}

func (b *budgetCounter) assertReason(t *testing.T, reason SeedReason, want int) {
	t.Helper()
	b.mu.Lock()
	got := b.byReason[reason]
	b.mu.Unlock()
	if got != want {
		t.Errorf("reason %q hits = %d, want %d", reason, got, want)
	}
}
