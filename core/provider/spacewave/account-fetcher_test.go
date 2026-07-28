package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/object"
)

func TestApplyFetchedAccountState_BootstrapDoesNotConsumeFutureServerEpoch(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.epoch = 1

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:        0,
		KeypairCount: 1,
	}, nil, nil)

	if acc.state.epoch != 0 {
		t.Fatalf("expected settled epoch 0 after bootstrap fetch, got %d", acc.state.epoch)
	}
	if acc.state.lastFetchedEpoch != 0 {
		t.Fatalf("expected last fetched epoch 0 after bootstrap fetch, got %d", acc.state.lastFetchedEpoch)
	}

	acc.setEpoch(1)
	if acc.state.epoch != 1 {
		t.Fatalf("expected remote epoch 1 to trigger after bootstrap fetch, got %d", acc.state.epoch)
	}
}

func TestApplyFetchedAccountState_PreservesConcurrentInvalidation(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.epoch = 2

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:        0,
		KeypairCount: 1,
	}, nil, nil)

	if acc.state.epoch != 2 {
		t.Fatalf("expected concurrent invalidation epoch 2 to be preserved, got %d", acc.state.epoch)
	}
	if acc.state.lastFetchedEpoch != 0 {
		t.Fatalf("expected last fetched epoch 0 after stale fetch, got %d", acc.state.lastFetchedEpoch)
	}
}

func TestApplyFetchedAccountState_PreservesAccountSObjectBindings(t *testing.T) {
	acc := &ProviderAccount{}

	state := &api.AccountStateResponse{
		Epoch: 3,
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: "account-settings",
				SoId:    "so-123",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED,
			},
		},
	}
	acc.applyFetchedAccountState(3, state, nil, nil)

	if acc.state.info == nil {
		t.Fatal("expected fetched account state to be stored")
	}
	if len(acc.state.info.GetAccountSobjectBindings()) != 1 {
		t.Fatalf("expected 1 account sobject binding, got %d", len(acc.state.info.GetAccountSobjectBindings()))
	}
	binding := acc.state.info.GetAccountSobjectBindings()[0]
	if binding.GetPurpose() != "account-settings" {
		t.Fatalf("expected binding purpose account-settings, got %q", binding.GetPurpose())
	}
	if binding.GetSoId() != "so-123" {
		t.Fatalf("expected binding so id so-123, got %q", binding.GetSoId())
	}
	if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED {
		t.Fatalf("expected reserved binding state, got %v", binding.GetState())
	}
}

func TestApplyFetchedAccountState_SetsReadyStatus(t *testing.T) {
	acc := &ProviderAccount{}

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:          1,
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}, nil, nil)

	if acc.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_READY {
		t.Fatalf("expected ready status after successful fetch, got %v", acc.state.status)
	}
}

func TestApplyFetchedAccountState_PreservesDeletedStatus(t *testing.T) {
	acc := &ProviderAccount{}

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:          1,
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED,
	}, nil, nil)

	if acc.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
		t.Fatalf("expected deleted status after deleted fetch, got %v", acc.state.status)
	}
}

func TestAccountFetcherResumesAfterUnauthStatusClears(t *testing.T) {
	var stateHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			if stateHits.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"unknown_session","message":"Session not found"}`))
				return
			}
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:          1,
				LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
			}))
		case "/api/account/emails":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountEmailsResponse{}))
		case "/api/account/sessions":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountSessionsResponse{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc.state.epoch = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- acc.accountFetcher(ctx)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	for {
		if waitCtx.Err() != nil {
			t.Fatalf("timed out waiting for unauthenticated status; stateHits=%d", stateHits.Load())
		}
		var status provider.ProviderAccountStatus
		var ch <-chan struct{}
		acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status = acc.state.status
			ch = getWaitCh()
		})
		if status == provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
			break
		}
		select {
		case <-waitCtx.Done():
		case <-ch:
		}
	}

	acc.SetAccountStatus(provider.ProviderAccountStatus_ProviderAccountStatus_READY)
	for {
		if waitCtx.Err() != nil {
			t.Fatalf("timed out waiting for bootstrap fetch; stateHits=%d", stateHits.Load())
		}
		var fetched bool
		var ch <-chan struct{}
		acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			fetched = acc.state.accountBootstrapFetched
			ch = getWaitCh()
		})
		if fetched {
			break
		}
		select {
		case <-waitCtx.Done():
		case <-ch:
		}
	}

	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("accountFetcher() = %v, want context canceled", err)
	}
	if got := stateHits.Load(); got != 2 {
		t.Fatalf("stateHits = %d, want 2", got)
	}
}

func TestAccountFetcherRetryWakesOnEpochChangeAndWritesCache(t *testing.T) {
	var stateHits atomic.Int32
	firstStateHit := make(chan struct{})
	secondStateHit := make(chan struct{})
	var firstOnce sync.Once
	var secondOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			hit := stateHits.Add(1)
			if hit == 1 {
				firstOnce.Do(func() { close(firstStateHit) })
				writeRetryableAccountFetcherError(w)
				return
			}
			secondOnce.Do(func() { close(secondStateHit) })
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:          2,
				LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
			}))
		case "/api/account/emails":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountEmailsResponse{}))
		case "/api/account/sessions":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountSessionsResponse{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	cacheStore := newAccountFetcherCacheStore()
	acc.objStore = cacheStore
	acc.state.epoch = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- acc.accountFetcher(ctx)
	}()
	defer func() {
		cancel()
		if err := <-done; err != context.Canceled {
			t.Fatalf("accountFetcher() = %v, want context canceled", err)
		}
	}()

	waitForAccountFetcherSignal(t, firstStateHit, time.Second, "first account-state hit")
	acc.setEpoch(2)
	waitForAccountFetcherSignal(t, secondStateHit, 500*time.Millisecond, "second account-state hit")
	waitForAccountBootstrapFetched(t, acc, time.Second)
	waitForAccountFetcherSignal(t, cacheStore.committed, time.Second, "account state cache commit")

	cache, err := acc.loadAccountStateCache(context.Background())
	if err != nil {
		t.Fatalf("load account state cache: %v", err)
	}
	if cache == nil {
		t.Fatal("expected account state cache to be written")
	}
	if got := cache.GetFetchedEpoch(); got != 2 {
		t.Fatalf("cached fetched epoch = %d, want 2", got)
	}
}

func TestAccountFetcherRetryWakesOnSessionClientChange(t *testing.T) {
	var stateHits atomic.Int32
	firstStateHit := make(chan struct{})
	secondStateHit := make(chan struct{})
	var firstOnce sync.Once
	var secondOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			hit := stateHits.Add(1)
			if hit == 1 {
				firstOnce.Do(func() { close(firstStateHit) })
				writeRetryableAccountFetcherError(w)
				return
			}
			secondOnce.Do(func() { close(secondStateHit) })
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:          1,
				LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
			}))
		case "/api/account/emails":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountEmailsResponse{}))
		case "/api/account/sessions":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountSessionsResponse{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	cacheStore := newAccountFetcherCacheStore()
	acc.objStore = cacheStore
	acc.state.epoch = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- acc.accountFetcher(ctx)
	}()
	defer func() {
		cancel()
		if err := <-done; err != context.Canceled {
			t.Fatalf("accountFetcher() = %v, want context canceled", err)
		}
	}()

	waitForAccountFetcherSignal(t, firstStateHit, time.Second, "first account-state hit")
	priv, pid := generateTestKeypair(t)
	acc.ReplaceSessionClient(NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()))
	waitForAccountFetcherSignal(t, secondStateHit, 500*time.Millisecond, "second account-state hit")
	waitForAccountBootstrapFetched(t, acc, time.Second)
	waitForAccountFetcherSignal(t, cacheStore.committed, time.Second, "account state cache commit")
}

func TestAccountFetcherRetryDelayBacksOffAndCancels(t *testing.T) {
	start := time.Now()
	if err := waitAccountFetcherRetryDelay(context.Background(), nil, 25*time.Millisecond); err != nil {
		t.Fatalf("retry wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("retry wait returned after %s, want at least 25ms", elapsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- waitAccountFetcherRetryDelay(ctx, nil, 2*time.Second)
	}()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("retry wait after cancel = %v, want context canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for retry wait cancellation")
	}
}

func writeRetryableAccountFetcherError(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "2")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"code":"temporary","message":"retry later","retryable":true}`))
}

func waitForAccountFetcherSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitForAccountBootstrapFetched(t *testing.T, acc *ProviderAccount, timeout time.Duration) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		var fetched bool
		var ch <-chan struct{}
		acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			fetched = acc.state.accountBootstrapFetched
			ch = getWaitCh()
		})
		if fetched {
			return
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("timed out waiting for bootstrap fetch")
		case <-ch:
		}
	}
}

type accountFetcherCacheStore struct {
	object.ObjectStore
	committed chan struct{}
	once      sync.Once
}

func newAccountFetcherCacheStore() *accountFetcherCacheStore {
	return &accountFetcherCacheStore{
		ObjectStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		committed:   make(chan struct{}),
	}
}

func (s *accountFetcherCacheStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.ObjectStore.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &accountFetcherCacheTx{
		Tx: tx,
		committed: func() {
			s.once.Do(func() { close(s.committed) })
		},
	}, nil
}

type accountFetcherCacheTx struct {
	kvtx.Tx
	committed func()
}

func (tx *accountFetcherCacheTx) Commit(ctx context.Context) error {
	if err := tx.Tx.Commit(ctx); err != nil {
		return err
	}
	tx.committed()
	return nil
}

func TestGetAccountStateWaitsForSigningSession(t *testing.T) {
	acc := NewTestProviderAccount(t, "https://example.com")
	acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.sessionClient = NewSessionClient(
			http.DefaultClient,
			"https://example.com",
			DefaultSigningEnvPrefix,
			nil,
			"",
		)
		broadcast()
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := acc.GetAccountState(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetAccountState error = %v, want context cancellation while signer is unavailable", err)
	}
}

func TestGetAccountStateWaitsForSigningSessionThenFetches(t *testing.T) {
	var stateHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/state" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		stateHits.Add(1)
		_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
			Epoch:          1,
			LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
		}))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	_, initialPID := generateTestKeypair(t)
	acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.sessionClient = NewSessionClientSigner(
			http.DefaultClient,
			srv.URL,
			DefaultSigningEnvPrefix,
			initialPID.String(),
			func(context.Context, []byte) ([]byte, error) {
				return nil, ErrSigningUnavailable
			},
		)
		broadcast()
	})

	ctx := t.Context()
	resultCh := make(chan struct {
		state *api.AccountStateResponse
		err   error
	}, 1)
	go func() {
		state, err := acc.GetAccountState(ctx)
		resultCh <- struct {
			state *api.AccountStateResponse
			err   error
		}{state: state, err: err}
	}()

	select {
	case <-time.After(25 * time.Millisecond):
		if got := stateHits.Load(); got != 0 {
			t.Fatalf("account-state requests without signer = %d, want 0", got)
		}
	case result := <-resultCh:
		t.Fatalf("GetAccountState returned before signer: state=%v err=%v", result.state, result.err)
	}

	priv, pid := generateTestKeypair(t)
	acc.ReplaceSessionClient(NewSessionClient(
		http.DefaultClient,
		srv.URL,
		DefaultSigningEnvPrefix,
		priv,
		pid.String(),
	))

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("GetAccountState after signer: %v", result.err)
		}
		if result.state == nil || result.state.GetEpoch() != 1 {
			t.Fatalf("GetAccountState state = %v, want epoch 1", result.state)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for account state after signer installation")
	}
	if got := stateHits.Load(); got != 1 {
		t.Fatalf("account-state requests = %d, want 1", got)
	}
}
