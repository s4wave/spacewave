package writeticketowner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/refcount"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// TestOwnerCachesBundle checks that a resolved bundle is cached and that callers receive copies.
func TestOwnerCachesBundle(t *testing.T) {
	// Serve one bundle.
	fetcher := &testFetcher{
		bundle: &api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoCheckpointTicket:   "so-checkpoint-a",
			BstoreSyncPushTicket: "sync-a",
		},
	}
	owner := newTestOwner(fetcher)

	// Resolve it and mutate the returned copy.
	first, releaseFirst, err := owner.Resolve(context.Background())
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	first.SoOpTicket = "mutated-local-copy"
	releaseFirst()

	// Resolve again from the cache.
	second, releaseSecond, err := owner.Resolve(context.Background())
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	defer releaseSecond()

	// One fetch served both, and the cache kept its own copy.
	if fetcher.bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", fetcher.bundleCalls)
	}
	if second.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("cached bundle mutated through caller copy: got %q", second.GetSoOpTicket())
	}
	if second.GetSoCheckpointTicket() != "so-checkpoint-a" {
		t.Fatalf("unexpected so checkpoint ticket: %q", second.GetSoCheckpointTicket())
	}
	if second.GetBstoreSyncPushTicket() != "sync-a" {
		t.Fatalf("unexpected sync push ticket: %q", second.GetBstoreSyncPushTicket())
	}
}

// TestOwnerBundleSingleflight checks that concurrent resolves share one bundle fetch.
func TestOwnerBundleSingleflight(t *testing.T) {
	// Hold the first bundle fetch until a second resolve joins it.
	firstReqStarted := make(chan struct{})
	allowResponse := make(chan struct{})
	fetcher := &testFetcher{
		bundle: &api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoCheckpointTicket:   "so-checkpoint-a",
			BstoreSyncPushTicket: "sync-a",
		},
		onBundle: func(call int) {
			if call == 1 {
				close(firstReqStarted)
				<-allowResponse
			}
		},
	}
	owner := newTestOwner(fetcher)

	// Collect each resolve's result.
	type result struct {
		bundle  *api.WriteTicketBundleResponse
		release func()
		err     error
	}
	results := make(chan result, 2)
	runFetch := func() {
		bundle, release, err := owner.Resolve(context.Background())
		results <- result{bundle: bundle, release: release, err: err}
	}

	// Start a second resolve while the first fetch is in flight.
	go runFetch()
	<-firstReqStarted
	go runFetch()
	close(allowResponse)

	// Both resolves succeed.
	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first Resolve: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second Resolve: %v", second.err)
	}
	defer first.release()
	defer second.release()

	// They shared one fetch and its bundle.
	if fetcher.bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", fetcher.bundleCalls)
	}
	if first.bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected first so op ticket: %q", first.bundle.GetSoOpTicket())
	}
	if second.bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected second so op ticket: %q", second.bundle.GetSoOpTicket())
	}
}

// TestOwnerRetryBackoffAvoidsStampede checks that resolves after a failed fetch share one retry.
func TestOwnerRetryBackoffAvoidsStampede(t *testing.T) {
	// Fail the first bundle fetch, then retry after a short backoff.
	fetchErr := errors.New("retry later")
	fetcher := &testFetcher{
		bundle: &api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoCheckpointTicket:   "so-checkpoint-a",
			BstoreSyncPushTicket: "sync-a",
		},
		bundleErr: fetchErr,
	}
	opts := &refcount.Options{
		RetryBackoff: &backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 20,
				MaxInterval:     20,
				Multiplier:      1,
			},
		},
		ShouldRetry: func(error) bool { return true },
	}
	owner := NewOwner(func(context.Context) (Fetcher, error) { return fetcher, nil }, "res-1", opts, nil)

	// The first resolve fails without a release.
	_, release, err := owner.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected first Resolve to fail")
	}
	if release != nil {
		t.Fatal("unexpected release on failed Resolve")
	}
	fetcher.bundleErr = nil

	// Collect each resolve's result.
	type result struct {
		bundle  *api.WriteTicketBundleResponse
		release func()
		err     error
	}
	results := make(chan result, 2)
	runFetch := func() {
		bundle, release, err := owner.Resolve(context.Background())
		results <- result{bundle: bundle, release: release, err: err}
	}

	// Resolve twice at once after the failure.
	go runFetch()
	go runFetch()

	// Both share the single retry fetch.
	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first Resolve after retry: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second Resolve after retry: %v", second.err)
	}
	defer first.release()
	defer second.release()
	if fetcher.bundleCalls != 2 {
		t.Fatalf("bundle fetch count: got %d, want 2", fetcher.bundleCalls)
	}
}

// TestOwnerRefreshAudiencePreservesOthers checks that refreshing one audience keeps the other tickets.
func TestOwnerRefreshAudiencePreservesOthers(t *testing.T) {
	// Serve a bundle and a fresh checkpoint ticket.
	fetcher := &testFetcher{
		bundle: &api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoCheckpointTicket:   "so-checkpoint-a",
			BstoreSyncPushTicket: "sync-a",
		},
		tickets: map[Audience]string{AudienceSOCheckpoint: "so-checkpoint-b"},
	}
	owner := newTestOwner(fetcher)

	// Resolve the initial bundle.
	bundle, release, err := owner.Resolve(context.Background())
	if err != nil {
		t.Fatalf("initial Resolve: %v", err)
	}
	release()
	if bundle.GetSoCheckpointTicket() != "so-checkpoint-a" {
		t.Fatalf("unexpected initial so checkpoint ticket: %q", bundle.GetSoCheckpointTicket())
	}

	// Invalidate and refresh the checkpoint audience.
	if err := owner.InvalidateAudience(AudienceSOCheckpoint); err != nil {
		t.Fatalf("InvalidateAudience: %v", err)
	}
	refreshed, err := owner.RefreshAudience(context.Background(), AudienceSOCheckpoint)
	if err != nil {
		t.Fatalf("RefreshAudience: %v", err)
	}
	if refreshed != "so-checkpoint-b" {
		t.Fatalf("unexpected refreshed ticket: %q", refreshed)
	}

	// Resolve the bundle again.
	bundle, release, err = owner.Resolve(context.Background())
	if err != nil {
		t.Fatalf("final Resolve: %v", err)
	}
	defer release()

	// Only the checkpoint ticket changed, without another bundle fetch.
	if fetcher.bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", fetcher.bundleCalls)
	}
	if fetcher.ticketCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", fetcher.ticketCalls)
	}
	if bundle.GetSoOpTicket() != "so-op-a" {
		t.Fatalf("unexpected so op ticket after refresh: %q", bundle.GetSoOpTicket())
	}
	if bundle.GetSoCheckpointTicket() != "so-checkpoint-b" {
		t.Fatalf("unexpected so checkpoint ticket after refresh: %q", bundle.GetSoCheckpointTicket())
	}
	if bundle.GetBstoreSyncPushTicket() != "sync-a" {
		t.Fatalf("unexpected sync push ticket: %q", bundle.GetBstoreSyncPushTicket())
	}
}

// TestOwnerRefreshAudienceSingleflight checks that concurrent refreshes of one audience share one fetch.
func TestOwnerRefreshAudienceSingleflight(t *testing.T) {
	// Hold the first ticket refresh until a second refresh joins it.
	refreshStarted := make(chan struct{})
	allowRefresh := make(chan struct{})
	fetcher := &testFetcher{
		bundle: &api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-a",
			SoCheckpointTicket:   "so-checkpoint-a",
			BstoreSyncPushTicket: "sync-a",
		},
		tickets: map[Audience]string{AudienceSOCheckpoint: "so-checkpoint-b"},
		onTicket: func(call int) {
			if call == 1 {
				close(refreshStarted)
				<-allowRefresh
			}
		},
	}
	owner := newTestOwner(fetcher)

	// Resolve the initial bundle.
	bundle, release, err := owner.Resolve(context.Background())
	if err != nil {
		t.Fatalf("initial Resolve: %v", err)
	}
	release()
	if bundle.GetSoCheckpointTicket() != "so-checkpoint-a" {
		t.Fatalf("unexpected initial so checkpoint ticket: %q", bundle.GetSoCheckpointTicket())
	}

	// Invalidate the checkpoint audience.
	if err := owner.InvalidateAudience(AudienceSOCheckpoint); err != nil {
		t.Fatalf("InvalidateAudience: %v", err)
	}

	// Collect each refresh's result.
	type result struct {
		ticket string
		err    error
	}
	results := make(chan result, 2)
	runRefresh := func() {
		ticket, err := owner.RefreshAudience(context.Background(), AudienceSOCheckpoint)
		results <- result{ticket: ticket, err: err}
	}

	// Start a second refresh while the first is in flight.
	go runRefresh()
	<-refreshStarted
	go runRefresh()
	time.Sleep(20 * time.Millisecond)
	close(allowRefresh)

	// Both refreshes share one fetch and its ticket.
	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("first RefreshAudience: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second RefreshAudience: %v", second.err)
	}
	if first.ticket != "so-checkpoint-b" {
		t.Fatalf("unexpected first refreshed ticket: %q", first.ticket)
	}
	if second.ticket != "so-checkpoint-b" {
		t.Fatalf("unexpected second refreshed ticket: %q", second.ticket)
	}
	if fetcher.ticketCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", fetcher.ticketCalls)
	}
}

func TestOwnerExecuteAudienceRefreshesAndRetriesOnce(t *testing.T) {
	refreshErr := errors.New("expired")
	fetcher := &testFetcher{
		bundle:  &api.WriteTicketBundleResponse{SoOpTicket: "so-op-a"},
		tickets: map[Audience]string{AudienceSOOp: "so-op-b"},
	}
	owner := NewOwner(
		func(context.Context) (Fetcher, error) { return fetcher, nil },
		"res-1",
		nil,
		func(err error) bool { return errors.Is(err, refreshErr) },
	)

	var seen []string
	err := owner.ExecuteAudience(
		context.Background(),
		AudienceSOOp,
		func(ticket string) error {
			seen = append(seen, ticket)
			if len(seen) == 1 {
				return refreshErr
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ExecuteAudience: %v", err)
	}
	if fetcher.bundleCalls != 1 {
		t.Fatalf("bundle fetch count: got %d, want 1", fetcher.bundleCalls)
	}
	if fetcher.ticketCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", fetcher.ticketCalls)
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

func TestOwnerExecuteAudienceReturnsSecondErrorUnchanged(t *testing.T) {
	refreshErr := errors.New("expired")
	secondErr := errors.New("retry failed")
	fetcher := &testFetcher{
		bundle:  &api.WriteTicketBundleResponse{SoOpTicket: "so-op-a"},
		tickets: map[Audience]string{AudienceSOOp: "so-op-b"},
	}
	owner := NewOwner(
		func(context.Context) (Fetcher, error) { return fetcher, nil },
		"res-1",
		nil,
		func(err error) bool { return errors.Is(err, refreshErr) },
	)

	var calls int
	err := owner.ExecuteAudience(
		context.Background(),
		AudienceSOOp,
		func(string) error {
			calls++
			if calls == 1 {
				return refreshErr
			}
			return secondErr
		},
	)
	if !errors.Is(err, secondErr) {
		t.Fatalf("got err %v, want second retry err %v", err, secondErr)
	}
	if fetcher.ticketCalls != 1 {
		t.Fatalf("refresh fetch count: got %d, want 1", fetcher.ticketCalls)
	}
}

func TestOwnerExecuteAudienceDoesNotRetryNonRefreshableError(t *testing.T) {
	refreshErr := errors.New("expired")
	wantErr := errors.New("permanent failure")
	fetcher := &testFetcher{bundle: &api.WriteTicketBundleResponse{SoOpTicket: "so-op-a"}}
	owner := NewOwner(
		func(context.Context) (Fetcher, error) { return fetcher, nil },
		"res-1",
		nil,
		func(err error) bool { return errors.Is(err, refreshErr) },
	)

	var calls int
	err := owner.ExecuteAudience(
		context.Background(),
		AudienceSOOp,
		func(string) error {
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
	if fetcher.ticketCalls != 0 {
		t.Fatalf("refresh fetch count: got %d, want 0", fetcher.ticketCalls)
	}
}

func newTestOwner(fetcher *testFetcher) *Owner {
	return NewOwner(
		func(context.Context) (Fetcher, error) { return fetcher, nil },
		"res-1",
		nil,
		nil,
	)
}

type testFetcher struct {
	bundle    *api.WriteTicketBundleResponse
	bundleErr error
	tickets   map[Audience]string
	ticketErr error

	onBundle func(call int)
	onTicket func(call int)

	mtx         sync.Mutex
	bundleCalls int
	ticketCalls int
}

func (f *testFetcher) GetWriteTicketBundle(
	context.Context,
	string,
) (*api.WriteTicketBundleResponse, error) {
	f.mtx.Lock()
	f.bundleCalls++
	call := f.bundleCalls
	onBundle := f.onBundle
	err := f.bundleErr
	var bundle *api.WriteTicketBundleResponse
	if f.bundle != nil {
		bundle = f.bundle.CloneVT()
	}
	f.mtx.Unlock()

	if onBundle != nil {
		onBundle(call)
	}
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

func (f *testFetcher) GetWriteTicket(
	_ context.Context,
	_ string,
	audience Audience,
) (string, error) {
	f.mtx.Lock()
	f.ticketCalls++
	call := f.ticketCalls
	onTicket := f.onTicket
	err := f.ticketErr
	ticket := f.tickets[audience]
	f.mtx.Unlock()

	if onTicket != nil {
		onTicket(call)
	}
	if err != nil {
		return "", err
	}
	return ticket, nil
}
