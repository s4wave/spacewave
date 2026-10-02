//go:build !goscript

package provider_spacewave

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestJoinAccountSession verifies a repeat sign-in joins the account's Session.
func TestJoinAccountSession(t *testing.T) {
	for _, tc := range []struct {
		name        string
		enrolled    bool
		wantRevoke  bool
		wantRekeyed bool
	}{
		{name: "enrolled Session keeps its key", enrolled: true, wantRevoke: true},
		{name: "revoked Session takes the handoff key", wantRekeyed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Generate the Session's current key and the new handoff key.
			ctx := t.Context()
			existingKey, existingPeer := generateTestKeypair(t)
			handoffKey, handoffPeer := generateTestKeypair(t)

			// Serve the account's enrolled keys and record self-revocation.
			var revoked atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/account/sessions":
					resp := &api.ListAccountSessionsResponse{
						Sessions: []*api.AccountSessionInfo{{PeerId: handoffPeer.String()}},
					}
					if tc.enrolled {
						resp.Sessions = append(resp.Sessions, &api.AccountSessionInfo{PeerId: existingPeer.String()})
					}
					data, _ := resp.MarshalVT()
					_, _ = w.Write(data)
				case r.Method == http.MethodDelete && r.URL.Path == "/api/session/revoke":
					if r.Header.Get("X-Peer-ID") != handoffPeer.String() {
						t.Errorf("revoked %s, want the handoff key", r.Header.Get("X-Peer-ID"))
					}
					revoked.Store(true)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			// Build an account on a real volume.
			tb, err := testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tb.Release()

			// Construct the provider account that owns the Session store.
			le := logrus.New().WithField("test", t.Name())
			prov := NewProvider(le, tb.Bus, &Config{Endpoint: srv.URL}, NewProviderInfo("spacewave"), nil, nil)
			acc := &ProviderAccount{le: le, p: prov, accountID: "test-account", vol: tb.Volume}
			acc.sessions = keyed.NewKeyedRefCount(acc.buildSessionTracker)

			// Register one local Session under its current key.
			entry := &session.SessionListEntry{
				SessionIndex: 1,
				SessionRef: &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
					Id:                "existing",
					ProviderId:        "spacewave",
					ProviderAccountId: "test-account",
				}},
			}
			if err := prov.seedHandoffSession(ctx, acc, entry.GetSessionRef(), existingKey); err != nil {
				t.Fatal(err)
			}

			// Join through the handoff key's client.
			sessions, err := acc.readAccountSessions(ctx, []*session.SessionListEntry{entry})
			if err != nil {
				t.Fatal(err)
			}
			client := NewSessionClient(srv.Client(), srv.URL, DefaultSigningEnvPrefix, handoffKey, handoffPeer.String())
			joined, err := acc.joinAccountSession(ctx, client, sessions, handoffKey)
			if err != nil {
				t.Fatal(err)
			}
			if joined != entry {
				t.Fatal("join returned another Session")
			}
			if revoked.Load() != tc.wantRevoke {
				t.Fatalf("handoff key revoked = %v, want %v", revoked.Load(), tc.wantRevoke)
			}

			// The registration marker names the key the Session now holds.
			sessions, err = acc.readAccountSessions(ctx, []*session.SessionListEntry{entry})
			if err != nil {
				t.Fatal(err)
			}
			wantPeer := existingPeer
			if tc.wantRekeyed {
				wantPeer = handoffPeer
			}
			if sessions[0].peerID != wantPeer.String() {
				t.Fatalf("registered peer = %s, want %s", sessions[0].peerID, wantPeer)
			}
		})
	}
}
