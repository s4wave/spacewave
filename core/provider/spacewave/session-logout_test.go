//go:build !goscript

package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestLogoutReleasesCloudSessionForNextLogin checks that deleting an attachment
// stops its mounted lifetime and lets the same account use a new Cloud signer.
func TestLogoutReleasesCloudSessionForNextLogin(t *testing.T) {
	// Serve Cloud registration and reject the old signer once its session is revoked.
	var revokedPeer atomic.Value
	revokedPeer.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject requests signed by the Session revoked by the Cloud.
		if r.Header.Get("X-Peer-ID") == revokedPeer.Load().(string) {
			http.Error(w, `{"code":"unknown_session"}`, http.StatusUnauthorized)
			return
		}

		// Return typed responses for registration and the transition watcher's first read.
		var data []byte
		var err error
		switch r.URL.Path {
		case "/api/account/session/register":
			data, err = (&api.RegisterSessionResponse{AccountId: "test-account"}).MarshalVT()
		case "/api/account/info":
			data, err = (&api.AccountInfoResponse{AccountId: "test-account"}).MarshalVT()
		case "/api/billing/test-billing/usage-query":
			data = []byte(r.Header.Get("X-Peer-ID"))
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Send the Cloud response through the real HTTP client.
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write(data); err != nil {
			t.Errorf("write Cloud response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	// Load the production Session controller with an in-memory storage volume.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Attach the Session controller factory and its configured storage.
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, loadRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.Volume.GetID(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(loadRef.Release)

	// Acquire the Session registry used by logout and login.
	controller, controllerRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controllerRef.Release)

	// Keep one ProviderAccount on the daemon across both logins.
	priv, pid := generateTestKeypair(t)
	le := logrus.New().WithField("test", t.Name())
	prov := NewProvider(le, tb.Bus, &Config{Endpoint: srv.URL}, NewProviderInfo("spacewave"), nil, nil)
	account := &ProviderAccount{
		le:        le,
		p:         prov,
		accountID: "test-account",
		vol:       tb.Volume,
		conf:      prov.conf,
		sfs:       prov.sfs,
		soListCtr: ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
		entityCli: NewEntityClientDirect(prov.httpCli, srv.URL, DefaultSigningEnvPrefix, priv, pid),
	}
	account.sessions = keyed.NewKeyedRefCount(account.buildSessionTracker)
	account.sessions.SetContext(ctx, false)
	t.Cleanup(account.sessions.ClearContext)

	// Register and mount the first attachment through production interfaces.
	oldRef := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "old-session", ProviderId: "spacewave", ProviderAccountId: "test-account",
	}}
	if _, err := controller.RegisterSession(ctx, oldRef, &session.SessionMetadata{DirectP2PDisabled: true}); err != nil {
		t.Fatal(err)
	}
	oldSession, releaseOld, err := account.MountSession(ctx, oldRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOld)
	revokedPeer.Store(oldSession.GetPeerId().String())

	// Logout must release the account's signer even while an external mount remains held.
	if err := controller.DeleteSession(ctx, oldRef); err != nil {
		t.Fatal(err)
	}
	if _, err := account.GetSessionClient(); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatal("logged-out Session still holds the account Cloud client")
	}
	if got := account.sessions.GetKeys(); len(got) != 0 {
		t.Fatalf("logged-out Session trackers = %v, want none", got)
	}

	// Log in again without rebuilding the ProviderAccount or daemon.
	newRef := oldRef.CloneVT()
	newRef.ProviderResourceRef.Id = "new-session"
	if _, err := controller.RegisterSession(ctx, newRef, &session.SessionMetadata{DirectP2PDisabled: true}); err != nil {
		t.Fatal(err)
	}
	newSession, releaseNew, err := account.MountSession(ctx, newRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseNew)
	client, err := account.GetSessionClient()
	if err != nil {
		t.Fatal("new Session did not install the account Cloud client")
	}

	// A billing request must use the new registered peer, not the revoked Session.
	usage, err := client.GetBillingUsage(ctx, "test-billing")
	if err != nil {
		t.Fatalf("billing after logout and login: %v", err)
	}
	if got, want := string(usage), newSession.GetPeerId().String(); got != want {
		t.Fatalf("billing signer = %q, want new Session %q", got, want)
	}
	if err := controller.DeleteSession(ctx, newRef); err != nil {
		t.Fatal(err)
	}
}

// mountTransportTestSession mounts the production provider Session on in-memory storage.
func mountTransportTestSession(t *testing.T, endpoint string, configure func(*transportCompositionOwner)) (*Session, *ProviderAccount) {
	// Load the production Session registry on the testbed volume.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, loadRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.Volume.GetID()}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(loadRef.Release)
	controller, controllerRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controllerRef.Release)

	// Bind a real provider account while replacing only direct transport mechanics.
	priv, pid := generateTestKeypair(t)
	le := logrus.New().WithField("test", t.Name())
	prov := NewProvider(le, tb.Bus, &Config{Endpoint: endpoint}, NewProviderInfo("spacewave"), nil, nil)
	account := &ProviderAccount{
		le: le, p: prov, accountID: "test-account", vol: tb.Volume, conf: prov.conf, sfs: prov.sfs,
		soListCtr: ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
		entityCli: NewEntityClientDirect(prov.httpCli, endpoint, DefaultSigningEnvPrefix, priv, pid),
	}
	account.sessions = keyed.NewKeyedRefCount(account.buildSessionTracker)
	account.sessions.SetContext(ctx, false)
	t.Cleanup(account.sessions.ClearContext)
	account.transportComposition.init(account)
	configure(&account.transportComposition)

	// Register and mount through the same interfaces used by login.
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: "session", ProviderId: "spacewave", ProviderAccountId: "test-account",
	}}
	if _, err := controller.RegisterSession(ctx, ref, nil); err != nil {
		t.Fatal(err)
	}
	mounted, release, err := account.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	t.Cleanup(func() {
		if err := controller.DeleteSession(context.Background(), ref); err != nil {
			t.Error(err)
		}
	})
	return mounted.(*Session), account
}

// TestSessionTransportLockLatency times the real lock path with an 80 ms signaling fixture.
func TestSessionTransportLockLatency(t *testing.T) {
	// Serve registration and account reads with production binary responses.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return the registration and account snapshots needed by Session startup.
		var data []byte
		switch r.URL.Path {
		case "/api/account/session/register":
			data, _ = (&api.RegisterSessionResponse{AccountId: "test-account"}).MarshalVT()
		case "/api/account/info":
			data, _ = (&api.AccountInfoResponse{AccountId: "test-account"}).MarshalVT()
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	// Delay direct setup by a representative signaling round trip.
	started := make(chan struct{}, 4)
	sess, _ := mountTransportTestSession(t, srv.URL, func(owner *transportCompositionOwner) {
		owner.startDirect = func(context.Context, string, crypto.PrivKey, string) (transportCompositionLinkSource, error) {
			started <- struct{}{}
			<-time.After(80 * time.Millisecond)
			return &compositionTestLinkSource{}, nil
		}
		owner.stopDirect = func(string) {}
		owner.transportState = func(string) (bool, <-chan struct{}) { return true, nil }
	})
	<-started
	pin := []byte("123456")
	if err := sess.SetLockMode(t.Context(), session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED, pin); err != nil {
		t.Fatal(err)
	}
	if err := sess.LockSession(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Lock again while unlock's transport setup is pending, timing both callers.
	unlocked := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		if err := sess.UnlockSession(t.Context(), pin); err != nil {
			t.Error(err)
		}
		unlocked <- time.Since(start)
	}()
	<-started
	start := time.Now()
	if err := sess.LockSession(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Logf("signaling=80ms lock=%s unlock=%s", time.Since(start), <-unlocked)
}

// TestSessionLockFencesTransportGeneration locks during setup and registration
// repair, then requires each unlocked successor to restore direct transport.
func TestSessionLockFencesTransportGeneration(t *testing.T) {
	// Hold the second registration response after the real HTTP request arrives.
	registration := make(chan struct{})
	releaseRegistration := make(chan struct{})
	var registrations atomic.Uint32
	var signedRequests atomic.Uint32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count signed Session calls separately from entity-signed registration.
		var data []byte
		switch r.URL.Path {
		case "/api/account/session/register":
			if registrations.Add(1) == 2 {
				close(registration)
				<-releaseRegistration
			}
			data, _ = (&api.RegisterSessionResponse{AccountId: "test-account"}).MarshalVT()
		case "/api/account/info":
			data, _ = (&api.AccountInfoResponse{AccountId: "test-account"}).MarshalVT()
		case "/api/billing/test-billing/usage-query":
			signedRequests.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	defer func() {
		select {
		case <-releaseRegistration:
		default:
			close(releaseRegistration)
		}
	}()

	// Hold setup even after cancellation so lock cannot rely on worker exit.
	setupStarted := make(chan crypto.PrivKey, 4)
	releaseSetup := make(chan struct{})
	afterSetup := make(chan error, 1)
	var setups atomic.Uint32
	stopped := make(chan struct{}, 4)
	sess, account := mountTransportTestSession(t, srv.URL, func(owner *transportCompositionOwner) {
		// Let the HTTP response arrive after cancellation to test late repair completion.
		client := *owner.account.entityCli.httpCli
		client.Transport = sessionRegistrationRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/account/session/register" {
				req = req.Clone(context.WithoutCancel(req.Context()))
			}
			return http.DefaultTransport.RoundTrip(req)
		})
		owner.account.entityCli.httpCli = &client

		// Leave setup suspended until the test has checked the revoked handle.
		owner.startDirect = func(ctx context.Context, _ string, key crypto.PrivKey, _ string) (transportCompositionLinkSource, error) {
			// Retain the generation across a setup that ignores cancellation.
			setupStarted <- key
			if setups.Add(1) == 1 {
				<-releaseSetup
				_, err := key.Sign([]byte("late setup"))
				afterSetup <- err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &compositionTestLinkSource{}, nil
		}
		owner.stopDirect = func(string) { stopped <- struct{}{} }
		owner.transportState = func(string) (bool, <-chan struct{}) { return true, nil }
	})
	t.Cleanup(func() {
		select {
		case <-releaseSetup:
		default:
			close(releaseSetup)
		}
	})
	first := <-setupStarted
	pin := []byte("123456")
	if err := sess.SetLockMode(t.Context(), session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED, pin); err != nil {
		t.Fatal(err)
	}

	// Lock while setup is held and prove retained clients cannot send afterwards.
	retained, err := account.GetSessionClient()
	if err != nil {
		t.Fatal(err)
	}
	// Suspend a request after signing but before its HTTP transport sends it.
	signed := make(chan struct{})
	send := make(chan struct{})
	defer func() {
		select {
		case <-send:
		default:
			close(send)
		}
	}()
	client := *retained.httpCli
	client.Transport = sessionRegistrationRoundTripper(func(req *http.Request) (*http.Response, error) {
		close(signed)
		<-send
		return http.DefaultTransport.RoundTrip(req)
	})
	retained.httpCli = &client
	requestDone := make(chan error, 1)
	go func() {
		_, err := retained.GetBillingUsage(t.Context(), "test-billing")
		requestDone <- err
	}()
	<-signed

	// Revoke both key use and the already-signed request before allowing its send.
	lockTransportTestSession(t, sess)
	if _, err := first.Sign([]byte("after lock")); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("retained setup key signed after lock: %v", err)
	}
	close(send)
	if err := <-requestDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued signed request survived revocation: %v", err)
	}
	if _, err := retained.GetBillingUsage(t.Context(), "test-billing"); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("retained client sent after lock: %v", err)
	}
	close(releaseSetup)
	if err := <-afterSetup; !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("in-flight setup signed after lock: %v", err)
	}

	// Unlock and require the successor composition to become ready.
	if err := sess.UnlockSession(t.Context(), pin); err != nil {
		t.Fatal(err)
	}
	second := <-setupStarted
	awaitCompositionState(t, &account.transportComposition, TransportCompositionP2PStateNoPeers, 0)
	if first == second {
		t.Fatal("unlock reused the revoked credential handle")
	}

	// Reject signaling authorization and lock while registration is at the server.
	account.transportComposition.findSession(sess.tkr.id).setSnapshot(TransportCompositionSnapshot{
		DirectP2PEnabled: true, P2PState: TransportCompositionP2PStateError, Unauthorized: true,
	})
	select {
	case <-registration:
	case <-t.Context().Done():
		t.Fatal("registration repair did not start")
	}
	lockTransportTestSession(t, sess)
	close(releaseRegistration)
	<-stopped
	if _, err := second.Sign([]byte("late registration")); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("repair generation signed after lock: %v", err)
	}
	if _, err := account.GetSessionClient(); !errors.Is(err, ErrSigningUnavailable) {
		t.Fatalf("registration response restored a locked signer: %v", err)
	}
	if got := signedRequests.Load(); got != 0 {
		t.Fatalf("old generation sent %d signed requests after lock", got)
	}

	// A second unlock must bring transport back with a third live generation.
	if err := sess.UnlockSession(t.Context(), pin); err != nil {
		t.Fatal(err)
	}
	third := <-setupStarted
	awaitCompositionState(t, &account.transportComposition, TransportCompositionP2PStateNoPeers, 0)
	if _, err := third.Sign([]byte("successor")); err != nil {
		t.Fatalf("unlocked transport could not sign: %v", err)
	}
}

// lockTransportTestSession requires lock to finish while network gates remain held.
func lockTransportTestSession(t *testing.T, sess *Session) {
	// Bound the caller rather than releasing setup or registration to make progress.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := sess.LockSession(ctx); err != nil {
		t.Fatalf("lock waited for transport work: %v", err)
	}
}

// sessionRegistrationRoundTripper lets a registration response outlive cancellation.
type sessionRegistrationRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip runs the registration fixture's request policy.
func (f sessionRegistrationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
