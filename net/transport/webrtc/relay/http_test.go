package webrtc_relay_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	webrtc_relay "github.com/s4wave/spacewave/net/transport/webrtc/relay"
)

// issuerFunc adapts a function to Issuer.
type issuerFunc func(ctx context.Context, session peer.ID) (*webrtc_relay.Credentials, error)

// Issue calls f.
func (f issuerFunc) Issue(ctx context.Context, session peer.ID) (*webrtc_relay.Credentials, error) {
	return f(ctx, session)
}

// TestHandlerServesSession checks the route issues only to a signed-in
// session, returns uncached credentials the fetch reads back, and hides issuer
// failures.
func TestHandlerServesSession(t *testing.T) {
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	session := p.GetPeerID()

	// The session cookie names the peer; the issuer fails on request.
	var issuedFor peer.ID
	var fail bool
	issuer := issuerFunc(func(_ context.Context, s peer.ID) (*webrtc_relay.Credentials, error) {
		if fail {
			return nil, errors.New("upstream-secret")
		}
		issuedFor = s
		return &webrtc_relay.Credentials{
			IceServers: []*webrtc.IceServerConfig{{
				Urls:       []string{"turns:relay.test:443?transport=tcp"},
				Username:   "u",
				Credential: &webrtc.IceServerConfig_Password{Password: "p"},
			}},
			ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		}, nil
	})
	auth := func(r *http.Request) (peer.ID, bool) {
		c, err := r.Cookie("session")
		return session, err == nil && c.Value == "ok"
	}
	srv := httptest.NewServer(webrtc_relay.NewHandler(issuer, auth))
	defer srv.Close()
	signedIn := func(cookie string) *http.Client {
		c := srv.Client()
		c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			r.AddCookie(&http.Cookie{Name: "session", Value: cookie})
			return http.DefaultTransport.RoundTrip(r)
		})
		return c
	}

	// A signed-out client gets nothing.
	if _, err := webrtc_relay.NewHTTPFetch(signedIn("no"), srv.URL)(t.Context()); !errors.Is(err, webrtc_relay.ErrStatus) || issuedFor != "" {
		t.Fatalf("signed out: %v, issued for %q", err, issuedFor)
	}

	// The signed-in session reads its own credentials back.
	creds, err := webrtc_relay.NewHTTPFetch(signedIn("ok"), srv.URL)(t.Context())
	if err != nil || issuedFor != session || creds.GetIceServers()[0].GetPassword() != "p" {
		t.Fatalf("signed in: %v, issued for %q, %v", err, issuedFor, creds)
	}
	resp, err := signedIn("ok").Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control %q", resp.Header.Get("Cache-Control"))
	}

	// A write is refused.
	resp, err = signedIn("ok").Post(srv.URL, "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("post status %d", resp.StatusCode)
	}

	// An issuer failure answers 503 without its reason.
	fail = true
	resp, err = signedIn("ok").Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || strings.Contains(string(body), "upstream-secret") {
		t.Fatalf("failure %d %q", resp.StatusCode, body)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls f.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
