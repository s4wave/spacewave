package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/refcount"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestProviderAccountGetWriteTicketBundleCachesOwner(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/session/write-tickets/res-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		calls++

		body, err := (&api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoRootTicket:         "so-root-a",
			BstoreSyncPushTicket: "sync-a",
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	first, releaseFirst, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("first GetWriteTicketBundle: %v", err)
	}
	first.SoOpTicket = "mutated-local-copy"
	releaseFirst()

	second, releaseSecond, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("second GetWriteTicketBundle: %v", err)
	}
	defer releaseSecond()

	if calls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", calls)
	}
	if second.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("cached bundle mutated through caller copy: got %q", second.GetSoOpTicket())
	}
	if second.GetSoRootTicket() != "so-root-a" {
		t.Fatalf("unexpected so root ticket: %q", second.GetSoRootTicket())
	}
	if second.GetBstoreSyncPushTicket() != "sync-a" {
		t.Fatalf("unexpected sync push ticket: %q", second.GetBstoreSyncPushTicket())
	}
}

func TestProviderAccountGetWriteTicketBundleSingleflight(t *testing.T) {
	firstReqStarted := make(chan struct{})
	allowResponse := make(chan struct{})

	var calls int
	var callsMtx sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/session/write-tickets/res-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		callsMtx.Lock()
		calls++
		callNum := calls
		callsMtx.Unlock()

		if callNum == 1 {
			close(firstReqStarted)
			<-allowResponse
		}

		body, err := (&api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoRootTicket:         "so-root-a",
			BstoreSyncPushTicket: "sync-a",
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	type result struct {
		bundle  *api.WriteTicketBundleResponse
		release func()
		err     error
	}
	results := make(chan result, 2)
	runFetch := func() {
		bundle, release, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
		results <- result{bundle: bundle, release: release, err: err}
	}

	go runFetch()
	<-firstReqStarted
	go runFetch()
	close(allowResponse)

	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first GetWriteTicketBundle: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second GetWriteTicketBundle: %v", second.err)
	}
	defer first.release()
	defer second.release()

	callsMtx.Lock()
	gotCalls := calls
	callsMtx.Unlock()
	if gotCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", gotCalls)
	}
	if first.bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected first so op ticket: %q", first.bundle.GetSoOpTicket())
	}
	if second.bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected second so op ticket: %q", second.bundle.GetSoOpTicket())
	}
}

func TestProviderAccountGetWriteTicketBundleRetryBackoffAvoidsStampede(t *testing.T) {
	prevOpts := writeTicketBundleRefCountOptions
	writeTicketBundleRefCountOptions = &refcount.Options{
		RetryBackoff: &backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 20,
				MaxInterval:     20,
				Multiplier:      1,
			},
		},
		ShouldRetry: func(err error) bool {
			return !isNonRetryableCloudError(err)
		},
	}
	defer func() {
		writeTicketBundleRefCountOptions = prevOpts
	}()

	var calls int
	var callsMtx sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/session/write-tickets/res-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		callsMtx.Lock()
		calls++
		callNum := calls
		callsMtx.Unlock()

		if callNum == 1 {
			body, err := (&api.ErrorResponse{
				Code:      "temporary_unavailable",
				Message:   "retry later",
				Retryable: true,
			}).MarshalJSON()
			if err != nil {
				t.Fatalf("marshal error response: %v", err)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write(body)
			return
		}
		if callNum != 2 {
			t.Fatalf("unexpected bundle fetch count: %d", callNum)
		}

		body, err := (&api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoRootTicket:         "so-root-a",
			BstoreSyncPushTicket: "sync-a",
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	_, release, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err == nil {
		t.Fatal("expected first GetWriteTicketBundle to fail")
	}
	if release != nil {
		t.Fatal("unexpected release on failed GetWriteTicketBundle")
	}

	type result struct {
		bundle  *api.WriteTicketBundleResponse
		release func()
		err     error
	}
	results := make(chan result, 2)
	runFetch := func() {
		bundle, release, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
		results <- result{bundle: bundle, release: release, err: err}
	}

	go runFetch()
	go runFetch()

	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first GetWriteTicketBundle: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second GetWriteTicketBundle: %v", second.err)
	}
	defer first.release()
	defer second.release()

	callsMtx.Lock()
	gotCalls := calls
	callsMtx.Unlock()
	if gotCalls != 2 {
		t.Fatalf("bundle fetch count: got %d, want 2", gotCalls)
	}
}

func TestProviderAccountRefreshWriteTicketAudiencePreservesOthers(t *testing.T) {
	var bundleCalls int
	var refreshCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-tickets/res-1":
			bundleCalls++
			body, err := (&api.WriteTicketBundleResponse{
				SoOpTicket:           "so-op-a",
				SoRootTicket:         "so-root-a",
				BstoreSyncPushTicket: "sync-a",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal bundle response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/api/session/write-ticket/res-1/so-root":
			refreshCalls++
			body, err := (&api.TicketResponse{Ticket: "so-root-b"}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal refresh response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	bundle, release, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("initial GetWriteTicketBundle: %v", err)
	}
	release()
	if bundle.GetSoRootTicket() != "so-root-a" {
		t.Fatalf("unexpected initial so root ticket: %q", bundle.GetSoRootTicket())
	}

	if err := acc.InvalidateWriteTicketAudience("res-1", writeTicketAudienceSORoot); err != nil {
		t.Fatalf("InvalidateWriteTicketAudience: %v", err)
	}
	refreshed, err := acc.RefreshWriteTicketAudience(
		context.Background(),
		"res-1",
		writeTicketAudienceSORoot,
	)
	if err != nil {
		t.Fatalf("RefreshWriteTicketAudience: %v", err)
	}
	if refreshed != "so-root-b" {
		t.Fatalf("unexpected refreshed ticket: %q", refreshed)
	}

	bundle, release, err = acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("final GetWriteTicketBundle: %v", err)
	}
	defer release()

	if bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", bundleCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", refreshCalls)
	}
	if bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected so op ticket after refresh: %q", bundle.GetSoOpTicket())
	}
	if bundle.GetSoRootTicket() != "so-root-b" {
		t.Fatalf("unexpected so root ticket after refresh: %q", bundle.GetSoRootTicket())
	}
	if bundle.GetBstoreSyncPushTicket() != "sync-a" {
		t.Fatalf("unexpected sync push ticket after refresh: %q", bundle.GetBstoreSyncPushTicket())
	}
}

func TestProviderAccountRefreshWriteTicketAudienceSingleflight(t *testing.T) {
	refreshStarted := make(chan struct{})
	allowRefresh := make(chan struct{})

	var refreshCalls int
	var refreshCallsMtx sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-tickets/res-1":
			body, err := (&api.WriteTicketBundleResponse{
				SoOpTicket:           "so-op-a",
				SoRootTicket:         "so-root-a",
				BstoreSyncPushTicket: "sync-a",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal bundle response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/api/session/write-ticket/res-1/so-root":
			refreshCallsMtx.Lock()
			refreshCalls++
			callNum := refreshCalls
			refreshCallsMtx.Unlock()
			if callNum == 1 {
				close(refreshStarted)
				<-allowRefresh
			}
			body, err := (&api.TicketResponse{Ticket: "so-root-b"}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal refresh response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	bundle, release, err := acc.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("initial GetWriteTicketBundle: %v", err)
	}
	release()
	if bundle.GetSoRootTicket() != "so-root-a" {
		t.Fatalf("unexpected initial so root ticket: %q", bundle.GetSoRootTicket())
	}

	if err := acc.InvalidateWriteTicketAudience("res-1", writeTicketAudienceSORoot); err != nil {
		t.Fatalf("InvalidateWriteTicketAudience: %v", err)
	}

	type result struct {
		ticket string
		err    error
	}
	results := make(chan result, 2)
	runRefresh := func() {
		ticket, err := acc.RefreshWriteTicketAudience(
			context.Background(),
			"res-1",
			writeTicketAudienceSORoot,
		)
		results <- result{ticket: ticket, err: err}
	}

	go runRefresh()
	<-refreshStarted
	go runRefresh()
	// Give the second goroutine a tick to park on the in-flight singleflight
	// promise before letting the owner finish.
	time.Sleep(20 * time.Millisecond)
	close(allowRefresh)

	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first RefreshWriteTicketAudience: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second RefreshWriteTicketAudience: %v", second.err)
	}
	if first.ticket != "so-root-b" {
		t.Fatalf("unexpected first refreshed ticket: %q", first.ticket)
	}
	if second.ticket != "so-root-b" {
		t.Fatalf("unexpected second refreshed ticket: %q", second.ticket)
	}

	refreshCallsMtx.Lock()
	gotCalls := refreshCalls
	refreshCallsMtx.Unlock()
	if gotCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", gotCalls)
	}
}

func TestProviderAccountExecuteWriteTicketAudienceRefreshesAndRetriesOnce(t *testing.T) {
	var bundleCalls int
	var refreshCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-tickets/res-1":
			bundleCalls++
			body, err := (&api.WriteTicketBundleResponse{
				SoOpTicket:           "so-op-a",
				SoRootTicket:         "so-root-a",
				BstoreSyncPushTicket: "sync-a",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal bundle response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/api/session/write-ticket/res-1/so-op":
			refreshCalls++
			body, err := (&api.TicketResponse{Ticket: "so-op-b"}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal refresh response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	var seen []string
	err := acc.ExecuteWriteTicketAudience(
		context.Background(),
		"res-1",
		writeTicketAudienceSOOp,
		func(ticket string) error {
			seen = append(seen, ticket)
			if len(seen) == 1 {
				return &cloudError{
					StatusCode: http.StatusUnauthorized,
					Code:       "expired_write_ticket",
					Message:    "expired",
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ExecuteWriteTicketAudience: %v", err)
	}
	if bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", bundleCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", refreshCalls)
	}
	if len(seen) != 2 {
		t.Fatalf("callback call count: got %d, want 2", len(seen))
	}
	if seen[0] != "so-op-a" {
		t.Fatalf("first ticket: got %q, want %q", seen[0], "so-op-a")
	}
	if seen[1] != "so-op-b" {
		t.Fatalf("second ticket: got %q, want %q", seen[1], "so-op-b")
	}
}

func TestProviderAccountExecuteWriteTicketAudienceReturnsSecondErrorUnchanged(t *testing.T) {
	var refreshCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-tickets/res-1":
			body, err := (&api.WriteTicketBundleResponse{
				SoOpTicket:           "so-op-a",
				SoRootTicket:         "so-root-a",
				BstoreSyncPushTicket: "sync-a",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal bundle response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/api/session/write-ticket/res-1/so-op":
			refreshCalls++
			body, err := (&api.TicketResponse{Ticket: "so-op-b"}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal refresh response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	firstErr := &cloudError{
		StatusCode: http.StatusUnauthorized,
		Code:       "expired_write_ticket",
		Message:    "expired",
	}
	secondErr := errors.New("retry failed")
	var calls int
	err := acc.ExecuteWriteTicketAudience(
		context.Background(),
		"res-1",
		writeTicketAudienceSOOp,
		func(ticket string) error {
			calls++
			if calls == 1 {
				return firstErr
			}
			return secondErr
		},
	)
	if !errors.Is(err, secondErr) {
		t.Fatalf("got err %v, want second retry err %v", err, secondErr)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", refreshCalls)
	}
}

func TestProviderAccountExecuteWriteTicketAudienceDoesNotRetryNonRefreshableError(t *testing.T) {
	var refreshCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-tickets/res-1":
			body, err := (&api.WriteTicketBundleResponse{
				SoOpTicket:           "so-op-a",
				SoRootTicket:         "so-root-a",
				BstoreSyncPushTicket: "sync-a",
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal bundle response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/api/session/write-ticket/res-1/so-op":
			refreshCalls++
			t.Fatalf("unexpected refresh request")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	wantErr := errors.New("permanent failure")
	var calls int
	err := acc.ExecuteWriteTicketAudience(
		context.Background(),
		"res-1",
		writeTicketAudienceSOOp,
		func(ticket string) error {
			calls++
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("callback call count: got %d, want 1", calls)
	}
	if refreshCalls != 0 {
		t.Fatalf("refresh fetch count: got %d, want 0", refreshCalls)
	}
}
