package provider_spacewave_handoff

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session_handoff "github.com/s4wave/spacewave/core/session/handoff"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func TestStartHandoffKeepsReceivingKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	requests := make(chan *session_handoff.HandoffRequest, 1)
	originalOpener := browserOpener
	t.Cleanup(func() { browserOpener = originalOpener })
	browserOpener = func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		encoded, _, _ := strings.Cut(strings.TrimPrefix(u.Fragment, "/auth/link/"), "?")
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return err
		}
		request := &session_handoff.HandoffRequest{}
		if err := request.UnmarshalVT(data); err != nil {
			return err
		}
		requests <- request
		return nil
	}
	var offeredPeer peer.ID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/session/create" {
			data, _ := (&api.AuthSessionCreateResponse{WsTicket: "test-ticket"}).MarshalVT()
			_, _ = w.Write(data)
			return
		}
		if r.URL.Path != "/api/auth/session/ws" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		request := <-requests
		if request.GetProtocolVersion() != 1 {
			t.Error("handoff did not request independent Session enrollment")
		}
		key, err := crypto.UnmarshalEd25519PublicKey(request.GetDevicePublicKey())
		if err != nil {
			t.Error(err)
			return
		}
		offeredPeer, err = peer.IDFromPublicKey(key)
		if err != nil {
			t.Error(err)
			return
		}
		data, _ := (&api.WsAuthSessionServerFrame{Body: &api.WsAuthSessionServerFrame_Completion{Completion: &session_handoff.HandoffCompletion{AccountId: "account", EntityId: "user", SessionPeerId: offeredPeer.String()}}}).MarshalVT()
		if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
			t.Error(err)
			return
		}
		if _, _, err := conn.Read(ctx); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	key, accountID, _, err := StartHandoff(ctx, server.Client(), server.URL, server.URL, "desktop", "login", "")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if actual != offeredPeer || accountID != "account" {
		t.Fatal("handoff replaced the receiving client's own Session identity")
	}
}

func TestBuildHandoffURLs(t *testing.T) {
	apiEndpoint := "https://api.spacewave.test/"
	publicBaseURL := "https://spacewave.test/"
	payload := "payload-123"
	wsTicket := "ticket-123"

	createURL := buildCreateSessionURL(apiEndpoint)
	if createURL != "https://api.spacewave.test/api/auth/session/create" {
		t.Fatalf("unexpected create URL: %q", createURL)
	}

	authURL := buildHandoffBrowserURL(publicBaseURL, payload, "signup", "spacewave")
	if authURL != "https://spacewave.test/#/auth/link/payload-123?intent=signup&username=spacewave" {
		t.Fatalf("unexpected auth URL: %q", authURL)
	}

	wsURL := buildHandoffWSURL(apiEndpoint, wsTicket)
	if wsURL != "wss://api.spacewave.test/api/auth/session/ws?tk=ticket-123" {
		t.Fatalf("unexpected ws URL: %q", wsURL)
	}
}

func TestValidateOpenURL(t *testing.T) {
	hosts := []string{"accounts.google.com", "spacewave.test"}
	ok := []string{
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=x",
		"https://spacewave.test/#/auth/link/payload",
		"https://SPACEWAVE.TEST/whatever",
	}
	for _, u := range ok {
		if err := validateOpenURL(u, hosts); err != nil {
			t.Errorf("expected %q to validate, got %v", u, err)
		}
	}

	bad := []string{
		"",
		"http://spacewave.test/insecure",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ms-help://hostile",
		"https://evil.example.com/steal",
		"https:///no-host",
		`https://spacewave.test" && calc.exe && "`,
	}
	for _, u := range bad {
		if err := validateOpenURL(u, hosts); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}
}

func TestHostsFromURLs(t *testing.T) {
	got := hostsFromURLs("https://a.example/", "", "not a url", "https://b.example:8443/x")
	want := []string{"a.example", "b.example:8443"}
	if len(got) != len(want) {
		t.Fatalf("unexpected hosts: %v", got)
	}
	for i, h := range want {
		if got[i] != h {
			t.Errorf("host %d: got %q want %q", i, got[i], h)
		}
	}
}

func TestReadHandoffResultRedialsDroppedSocket(t *testing.T) {
	// Serve a socket that drops the first connection without a close frame.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var dials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept the socket and drop the first connection.
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		if dials.Add(1) == 1 {
			_ = conn.CloseNow()
			return
		}

		// Answer the redial with the result frame.
		defer conn.Close(websocket.StatusNormalClosure, "")
		_ = conn.Write(r.Context(), websocket.MessageBinary, []byte("result"))
	}))
	defer server.Close()

	// Expect the result from the second connection.
	conn, msg, err := readHandoffResult(ctx, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if string(msg) != "result" || dials.Load() != 2 {
		t.Fatalf("got %q after %d dials", msg, dials.Load())
	}
}

func TestReadHandoffResultStopsOnExpiry(t *testing.T) {
	// Serve a socket that reports an expired auth session.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept the socket and close it as the expired session does.
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close(4008, "auth session expired")
	}))
	defer server.Close()

	// Expect the wait to end with the expiry explanation.
	_, _, err := readHandoffResult(ctx, server.URL)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiry error, got %v", err)
	}
}
