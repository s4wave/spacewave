package relay_cloudflare_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	webrtc_relay "github.com/s4wave/spacewave/net/transport/webrtc/relay"
	relay_cloudflare "github.com/s4wave/spacewave/net/transport/webrtc/relay/cloudflare"
)

// TestIssueRecordedResponse issues against a recorded generate-ice-servers
// response and checks the request, the servers and the expiry.
func TestIssueRecordedResponse(t *testing.T) {
	recorded, err := os.ReadFile("testdata/generate-ice-servers.json")
	if err != nil {
		t.Fatal(err)
	}

	// Answer as Cloudflare does, recording what the issuer sent.
	var path, auth, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		path, auth, body = r.Method+" "+r.URL.Path, r.Header.Get("Authorization"), string(data)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(recorded)
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	iss := &relay_cloudflare.Issuer{
		KeyID:    "key-1",
		APIToken: "server-only-token",
		Client:   srv.Client(),
		Endpoint: srv.URL + "/v1/turn/keys/",
		Now:      func() time.Time { return now },
	}
	creds, err := iss.Issue(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}

	// One request for a six-hour credential, with the token as bearer.
	if path != "POST /v1/turn/keys/key-1/credentials/generate-ice-servers" || auth != "Bearer server-only-token" || body != `{"ttl":21600}` {
		t.Fatalf("request %q %q %q", path, auth, body)
	}

	// The relays and their expiry come back; the client keeps the six relay URLs.
	if !creds.GetExpiresAt().AsTime().Equal(now.Add(6 * time.Hour)) {
		t.Fatalf("expires %v", creds.GetExpiresAt().AsTime())
	}
	relays := creds.RelayServers()
	if len(relays) != 1 || len(relays[0].GetUrls()) != 6 || !strings.HasPrefix(relays[0].GetUsername(), "bc91b63e") || relays[0].GetPassword() == "" {
		t.Fatalf("relays %v", relays)
	}
}

// TestIssueUnavailable checks an unconfigured issuer sends nothing and an
// upstream failure reports unavailable without the upstream reply.
func TestIssueUnavailable(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "upstream-secret", http.StatusInternalServerError)
	}))
	defer srv.Close()

	// No key, no request.
	var session peer.ID
	for _, iss := range []*relay_cloudflare.Issuer{nil, {APIToken: "t"}, {KeyID: "k"}} {
		if _, err := iss.Issue(t.Context(), session); !errors.Is(err, webrtc_relay.ErrUnavailable) {
			t.Fatalf("unconfigured: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("an unconfigured issuer called Cloudflare")
	}

	// A failed request names nothing from the reply.
	iss := &relay_cloudflare.Issuer{KeyID: "k", APIToken: "t", Client: srv.Client(), Endpoint: srv.URL + "/"}
	_, err := iss.Issue(t.Context(), session)
	if !errors.Is(err, webrtc_relay.ErrUnavailable) || strings.Contains(err.Error(), "upstream-secret") || calls != 1 {
		t.Fatalf("upstream failure: %v after %d calls", err, calls)
	}
}
