package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// lookupTestID is the valid Cloud resource used by descriptor lookup fixtures.
const lookupTestID = "01j00000000000000000000000"

// TestLookupSharedObjectRouteDetection confines legacy fallback to the old
// dispatcher's typed 404 and forgets it at the authenticated reconnect boundary.
func TestLookupSharedObjectRouteDetection(t *testing.T) {
	for _, outcome := range []string{"absent", "refused", "unauth", "throttled", "blocked", "gateway", "bare404", "malformed", "wrong-account", "wrong-id", "empty-ref"} {
		t.Run(outcome, func(t *testing.T) {
			// Answer the actual signed lookup request with one rollout outcome.
			var calls atomic.Int32
			var upgraded atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/sobject/"+lookupTestID+"/lookup" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if upgraded.Load() {
					data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
					_, _ = w.Write(data)
					return
				}
				switch outcome {
				case "absent":
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"code":"not_found"}`))
				case "refused":
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"code":"sobject_not_accessible"}`))
				case "unauth":
					w.WriteHeader(http.StatusUnauthorized)
				case "throttled":
					w.WriteHeader(http.StatusTooManyRequests)
				case "blocked":
					w.WriteHeader(http.StatusUnavailableForLegalReasons)
				case "gateway":
					w.WriteHeader(http.StatusBadGateway)
					_, _ = w.Write([]byte("<html>gateway</html>"))
				case "bare404":
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("<html>not found</html>"))
				case "malformed":
					_, _ = w.Write([]byte{255})
				case "wrong-account", "wrong-id", "empty-ref":
					entry := &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}
					switch outcome {
					case "wrong-account":
						entry.Ref.ProviderResourceRef.ProviderAccountId = "foreign"
					case "wrong-id":
						entry.Ref.ProviderResourceRef.Id = "foreign"
					case "empty-ref":
						entry.Ref = nil
					}
					data, _ := (&api.LookupSObjectResponse{Entry: entry}).MarshalVT()
					_, _ = w.Write(data)
				}
			}))
			defer srv.Close()
			acc := NewTestProviderAccount(t, srv.URL)

			// Only typed action absence suppresses subsequent probes.
			for range 2 {
				entry, err := acc.LookupSharedObject(t.Context(), lookupTestID)
				if entry != nil || err == nil || errors.Is(err, sobject.ErrSharedObjectLookupUnsupported) != (outcome == "absent") {
					t.Fatalf("outcome %s returned %v %v", outcome, entry, err)
				}
			}
			want := int32(2)
			if outcome == "absent" {
				want = 1
			}
			if calls.Load() != want {
				t.Fatalf("got %d probes, want %d", calls.Load(), want)
			}
			acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if len(acc.soLookup.demand) != 0 || len(acc.soLookup.admitted) != 0 || acc.state.sharedObjectMetadata[lookupTestID] != nil {
					t.Errorf("a failed lookup left state: demand=%d admitted=%d", len(acc.soLookup.demand), len(acc.soLookup.admitted))
				}
			})

			// A reconnect discovers an upgraded server with the next needed lookup.
			upgraded.Store(true)
			acc.resetSharedObjectLookup()
			entry, err := acc.LookupSharedObject(t.Context(), lookupTestID)
			if err != nil || entry.GetRef().GetProviderResourceRef().GetId() != lookupTestID {
				t.Fatalf("lookup after upgrade: %v %v", entry, err)
			}
		})
	}
}

// TestLookupSharedObjectInvalidation repeats a request superseded by access
// invalidation, a reconnect or a replaced signer while the caller still waits,
// and fails one superseded by the object's or the account's deletion.
func TestLookupSharedObjectInvalidation(t *testing.T) {
	for _, change := range []string{"delete", "access", "reconnect", "signer", "account"} {
		t.Run(change, func(t *testing.T) {
			// Hold the first successful response after its request has started.
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
				}
				data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
				_, _ = w.Write(data)
			}))
			defer srv.Close()
			acc := NewTestProviderAccount(t, srv.URL)
			done := make(chan error, 1)
			go func() {
				_, err := acc.LookupSharedObject(t.Context(), lookupTestID)
				done <- err
			}()
			<-started

			// Supersede its authority before the response can be admitted.
			want := error(nil)
			switch change {
			case "delete":
				acc.DeleteSharedObjectMetadata(lookupTestID)
				want = ErrSharedObjectMetadataDeleted
			case "access":
				acc.invalidateSharedObjectLookup()
			case "reconnect":
				acc.resetSharedObjectLookup()
			case "account":
				acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
					acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
				})
				want = ErrProviderAccountDeleted
			case "signer":
				priv, pid := generateTestKeypair(t)
				acc.ReplaceSessionClient("replacement", priv, pid)
			}
			close(release)

			// A superseded reply repeats once under the current generation.
			err := <-done
			if want == nil && (err != nil || calls.Load() != 2) {
				t.Fatalf("superseded lookup: calls=%d error=%v", calls.Load(), err)
			}
			if want != nil && !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if acc.soListCtr.GetValue() != nil {
				t.Fatal("a lookup published a catalog")
			}
		})
	}
}

// TestLookupSharedObjectSupersededBackoff backs off a lookup whose every reply
// is superseded, and ends it at the caller's deadline instead of a repeat cap.
func TestLookupSharedObjectSupersededBackoff(t *testing.T) {
	// Supersede each reply before the client can admit it.
	var calls atomic.Int32
	var acc *ProviderAccount
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		acc.invalidateSharedObjectLookup()
		data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	acc = NewTestProviderAccount(t, srv.URL)

	// The backoff waits 0.5s then 0.7s after the immediate repeat, so 1.5s
	// holds four requests where an unpaced loop would send thousands.
	ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
	defer cancel()
	_, err := acc.LookupSharedObject(ctx, lookupTestID)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("lookup ended with %v before the caller's deadline", err)
	}
	if n := calls.Load(); n < 3 || n > 5 {
		t.Fatalf("superseded lookup sent %d requests in 1.5s, want about 4", n)
	}
}

// TestLookupSharedObjectCatalogApart keeps an admitted descriptor out of the
// catalog, and keeps it through a catalog snapshot fetched before admission.
func TestLookupSharedObjectCatalogApart(t *testing.T) {
	// Hold an empty catalog snapshot until the lookup has been admitted.
	listStarted, listRelease := make(chan struct{}), make(chan struct{})
	var lookups atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sobject/list" {
			close(listStarted)
			<-listRelease
			data, _ := (&sobject.SharedObjectList{}).MarshalVT()
			_, _ = w.Write(data)
			return
		}
		lookups.Add(1)
		data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	fetched := make(chan error, 1)
	go func() { fetched <- acc.fetchSharedObjectList(t.Context()) }()
	<-listStarted

	// The lookup leaves the catalog still loading.
	if _, err := acc.LookupSharedObject(t.Context(), lookupTestID); err != nil {
		t.Fatal(err)
	}
	if acc.soListCtr.GetValue() != nil {
		t.Fatal("a lookup published a partial catalog")
	}

	// The stale snapshot lands without the descriptor, which stays admitted.
	close(listRelease)
	if err := <-fetched; err != nil {
		t.Fatal(err)
	}
	if acc.HasCachedSharedObject(lookupTestID) {
		t.Fatal("the catalog holds a lookup result")
	}
	if _, err := acc.LookupSharedObject(t.Context(), lookupTestID); err != nil || lookups.Load() != 1 {
		t.Fatalf("admitted descriptor after a stale snapshot: lookups=%d error=%v", lookups.Load(), err)
	}

	// An access change drops the admitted descriptor.
	acc.invalidateSharedObjectLookup()
	if _, err := acc.LookupSharedObject(t.Context(), lookupTestID); err != nil || lookups.Load() != 2 {
		t.Fatalf("lookup after invalidation: lookups=%d error=%v", lookups.Load(), err)
	}
}

// TestLookupSharedObjectDemand shares a request and keeps it alive after one
// caller cancels, while an admitted descriptor needs no subsequent request.
func TestLookupSharedObjectDemand(t *testing.T) {
	// Hold the request while two independently cancellable callers attach.
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	firstCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := acc.LookupSharedObject(firstCtx, lookupTestID); first <- err }()
	<-started

	// Add a second ref directly to the production demand primitive before cancelling.
	var lookup *refcount.RefCount[*sobject.SharedObjectListEntry]
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lookup = acc.soLookup.demand[lookupTestID].rc
	})
	secondPromise, secondRef := lookup.WaitWithReleased(t.Context(), nil)
	go func() { _, err := secondPromise.Await(t.Context()); second <- err; secondRef.Release() }()
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller: %v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}

	// Reuse the admitted descriptor even after the HTTP server closes.
	srv.Close()
	if _, err := acc.LookupSharedObject(t.Context(), lookupTestID); err != nil || calls.Load() != 1 {
		t.Fatalf("cached lookup: calls=%d error=%v", calls.Load(), err)
	}
}

// TestLookupSharedObjectLastCancellation releases the HTTP request when no
// caller still needs its descriptor.
func TestLookupSharedObjectLastCancellation(t *testing.T) {
	// Keep the server waiting until the final demand cancels the request.
	started, stopped := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := acc.LookupSharedObject(ctx, lookupTestID); done <- err }()
	<-started

	// The caller finishes and the HTTP server observes transport cancellation.
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lookup: %v", err)
	}
	select {
	case <-stopped:
	case <-t.Context().Done():
		t.Fatal("last demand did not cancel the HTTP request")
	}
}

// TestLookupSharedObjectRetryDeadline waits out a typed retry hint while retaining
// the original demand, then resolves the descriptor without a catalog refresh.
func TestLookupSharedObjectRetryDeadline(t *testing.T) {
	// Return one throttled response followed by the descriptor.
	var calls atomic.Int32
	var firstAt atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			firstAt.Store(time.Now().UnixNano())
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"rate_limited","retryable":true,"retryAfterSeconds":1}`))
			return
		}
		if time.Since(time.Unix(0, firstAt.Load())) < time.Second {
			t.Error("retried before the server deadline")
		}
		data, _ := (&api.LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("", "test-account", lookupTestID, lookupTestID)}}).MarshalVT()
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Resolve through the account's existing demand and retry primitives.
	acc := NewTestProviderAccount(t, srv.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	entry, err := acc.LookupSharedObject(ctx, lookupTestID)
	if err != nil || entry == nil || calls.Load() != 2 {
		t.Fatalf("retry: calls=%d entry=%v error=%v", calls.Load(), entry, err)
	}
}
