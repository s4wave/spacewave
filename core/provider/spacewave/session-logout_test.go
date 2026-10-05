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
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
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
	_, loadRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
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
	if account.GetSessionClient() != nil {
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
	client := account.GetSessionClient()
	if client == nil {
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
