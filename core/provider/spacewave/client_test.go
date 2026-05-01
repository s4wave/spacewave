package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// generateTestKeypair generates an Ed25519 keypair for testing.
func generateTestKeypair(t *testing.T) (crypto.PrivKey, peer.ID) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("deriving peer ID: %v", err)
	}
	return priv, pid
}

// newTestSignedClient builds a SignedHTTPClient pointed at a test server.
func newTestSignedClient(t *testing.T, baseURL string) (*SignedHTTPClient, peer.ID) {
	t.Helper()
	priv, pid := generateTestKeypair(t)
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: baseURL,
		envPfx:  "spacewave",
		priv:    priv,
		peerID:  pid,
	}
	return cli, pid
}

func TestNormalizeSigningEnvPrefix(t *testing.T) {
	cases := []struct {
		name          string
		signingEnvPfx string
		want          string
	}{
		{
			name:          "empty uses default",
			signingEnvPfx: "",
			want:          "spacewave",
		},
		{
			name:          "explicit staging",
			signingEnvPfx: "spacewave-staging",
			want:          "spacewave-staging",
		},
		{
			name:          "explicit custom",
			signingEnvPfx: "spacewave-dev",
			want:          "spacewave-dev",
		},
	}

	for _, tc := range cases {
		if got := normalizeSigningEnvPrefix(tc.signingEnvPfx); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClientConstructorsUseExplicitSigningEnvPrefix(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	entityCli := NewEntityClientDirect(
		http.DefaultClient,
		"https://staging.spacewave.app",
		"spacewave-staging",
		priv,
		pid,
	)
	if got := entityCli.envPfx; got != "spacewave-staging" {
		t.Fatalf("entity client env prefix: got %q, want %q", got, "spacewave-staging")
	}

	sessCli := NewSessionClient(
		http.DefaultClient,
		"https://account-staging.spacewave.app",
		"spacewave-staging",
		priv,
		pid.String(),
	)
	if got := sessCli.envPfx; got != "spacewave-staging" {
		t.Fatalf("session client env prefix: got %q, want %q", got, "spacewave-staging")
	}
}

// TestSignRequest_Headers verifies signRequest sets all expected auth headers.
func TestSignRequest_Headers(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: "http://localhost",
		envPfx:  "spacewave",
		priv:    priv,
		peerID:  pid,
	}

	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	body := []byte("hello")
	if err := cli.signRequest(req, body); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	// Verify required headers are set.
	if req.Header.Get("X-Peer-ID") == "" {
		t.Fatal("missing X-Peer-ID header")
	}
	if req.Header.Get("X-Peer-ID") != pid.String() {
		t.Fatalf("X-Peer-ID mismatch: got %q, want %q", req.Header.Get("X-Peer-ID"), pid.String())
	}

	ts := req.Header.Get("X-Timestamp")
	if ts == "" {
		t.Fatal("missing X-Timestamp header")
	}
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil {
		t.Fatalf("X-Timestamp not a valid int: %v", err)
	}

	bodyHash := req.Header.Get("X-Sw-Hash")
	if bodyHash == "" {
		t.Fatal("missing X-Sw-Hash header")
	}
	expectedHash := sha256.Sum256(body)
	if bodyHash != hex.EncodeToString(expectedHash[:]) {
		t.Fatalf("X-Sw-Hash mismatch: got %q, want %q", bodyHash, hex.EncodeToString(expectedHash[:]))
	}

	sig := req.Header.Get("X-Signature")
	if sig == "" {
		t.Fatal("missing X-Signature header")
	}
	if _, err := base64.StdEncoding.DecodeString(sig); err != nil {
		t.Fatalf("X-Signature not valid base64: %v", err)
	}
}

// TestSignRequest_SignedHeaders verifies X-Signed-Headers is set when signable headers are present.
func TestSignRequest_SignedHeaders(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: "http://localhost",
		envPfx:  "spacewave",
		priv:    priv,
		peerID:  pid,
	}

	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pack-ID", "pack123")

	if err := cli.signRequest(req, []byte("{}")); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	sh := req.Header.Get("X-Signed-Headers")
	if sh == "" {
		t.Fatal("expected X-Signed-Headers to be set")
	}
	// The signed headers should be sorted and include content-type and x-pack-id.
	parts := strings.Split(sh, ",")
	found := map[string]bool{}
	for _, p := range parts {
		found[p] = true
	}
	if !found["content-type"] {
		t.Fatal("X-Signed-Headers missing content-type")
	}
	if !found["x-pack-id"] {
		t.Fatal("X-Signed-Headers missing x-pack-id")
	}
}

// TestSignRequest_NoSignedHeadersWhenAbsent verifies X-Signed-Headers is not set
// when no signable headers are present on the request.
func TestSignRequest_NoSignedHeadersWhenAbsent(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: "http://localhost",
		envPfx:  "spacewave",
		priv:    priv,
		peerID:  pid,
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := cli.signRequest(req, nil); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	if req.Header.Get("X-Signed-Headers") != "" {
		t.Fatal("X-Signed-Headers should not be set for request without signable headers")
	}
}

// TestSignRequest_NoPrivateKey verifies signRequest returns an error without a key.
func TestSignRequest_NoPrivateKey(t *testing.T) {
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: "http://localhost",
		envPfx:  "spacewave",
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = cli.signRequest(req, nil)
	if err == nil {
		t.Fatal("expected error when no private key configured")
	}
}

// TestSignRequest_SignatureVerifies verifies the signature can be verified with the public key.
func TestSignRequest_SignatureVerifies(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := &SignedHTTPClient{
		httpCli: http.DefaultClient,
		baseURL: "http://localhost",
		envPfx:  "spacewave",
		priv:    priv,
		peerID:  pid,
	}

	body := []byte("test-body")
	req, err := http.NewRequest(http.MethodPost, "http://localhost/api/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	if err := cli.signRequest(req, body); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	// Reconstruct the payload that was signed using proto binary serialization.
	ts := req.Header.Get("X-Timestamp")
	timestampMs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Fatalf("parsing timestamp: %v", err)
	}
	bodyHashHex := req.Header.Get("X-Sw-Hash")
	signedHdrs := req.Header.Get("X-Signed-Headers")
	var hdrs strings.Builder
	if signedHdrs != "" {
		keys := strings.Split(signedHdrs, ",")
		for i, k := range keys {
			if i > 0 {
				hdrs.WriteByte(',')
			}
			hdrs.WriteString(k)
			hdrs.WriteByte('=')
			hdrs.WriteString(req.Header.Get(k))
		}
	}

	payload := &api.SigningPayload{
		EnvPrefix:     "spacewave",
		Method:        "POST",
		Path:          "/api/test",
		TimestampMs:   timestampMs,
		ContentLength: int64(len(body)),
		BodyHashHex:   bodyHashHex,
		SignedHeaders: hdrs.String(),
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}

	sigB64 := req.Header.Get("X-Signature")
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}

	pub := priv.GetPublic()
	ok, err := pub.Verify(payloadBytes, sigBytes)
	if err != nil {
		t.Fatalf("verifying signature: %v", err)
	}
	if !ok {
		t.Fatal("signature verification failed")
	}
}

// TestMarshalWriteTicketProofPayload verifies the proof payload bytes are the
// deterministic proto binary serialization of the canonical field set.
func TestMarshalWriteTicketProofPayload(t *testing.T) {
	payloadBytes, err := marshalWriteTicketProofPayload(WriteTicketProofPayloadFields{
		Ticket:        "ticket-123",
		Method:        "POST",
		Path:          "/api/sobject/01/op",
		TimestampMs:   123456789,
		ContentLength: 42,
		BodyHashHex:   "abcd",
		SignedHeaders: map[string]string{
			"x-pack-id":     "pack-1",
			"content-type":  "application/octet-stream",
			"x-block-count": "12",
		},
	})
	if err != nil {
		t.Fatalf("marshalWriteTicketProofPayload: %v", err)
	}

	want, err := (&api.WriteTicketProofPayload{
		Ticket:        "ticket-123",
		Method:        "POST",
		Path:          "/api/sobject/01/op",
		TimestampMs:   123456789,
		ContentLength: 42,
		BodyHashHex:   "abcd",
		SignedHeaders: "content-type=application/octet-stream,x-block-count=12,x-pack-id=pack-1",
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal want payload: %v", err)
	}
	if !bytes.Equal(payloadBytes, want) {
		t.Fatal("write ticket proof payload bytes mismatch")
	}
}

// TestBuildWriteTicketProof verifies the proof envelope signs the serialized
// payload bytes with the provided session private key.
func TestBuildWriteTicketProof(t *testing.T) {
	priv, _ := generateTestKeypair(t)
	payload := []byte("proof-payload")

	proof, err := buildWriteTicketProof(payload, priv)
	if err != nil {
		t.Fatalf("buildWriteTicketProof: %v", err)
	}
	if !bytes.Equal(proof.GetPayload(), payload) {
		t.Fatal("payload bytes not preserved")
	}

	pub := priv.GetPublic()
	ok, err := pub.Verify(proof.GetPayload(), proof.GetSignature())
	if err != nil {
		t.Fatalf("verify proof signature: %v", err)
	}
	if !ok {
		t.Fatal("proof signature verification failed")
	}
}

// TestMarshalSObjectWriteTicketProofPayload verifies shared-object proof
// payloads bind the body hash and content-type directly.
func TestMarshalSObjectWriteTicketProofPayload(t *testing.T) {
	body := []byte("root-or-op-body")
	payloadBytes, err := marshalSObjectWriteTicketProofPayload(
		"ticket-123",
		http.MethodPost,
		"/api/sobject/01/op",
		"application/octet-stream",
		body,
		123456789,
	)
	if err != nil {
		t.Fatalf("marshalSObjectWriteTicketProofPayload: %v", err)
	}

	var payload api.WriteTicketProofPayload
	if err := payload.UnmarshalVT(payloadBytes); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.GetTicket() != "ticket-123" {
		t.Fatalf("unexpected ticket: %q", payload.GetTicket())
	}
	if payload.GetMethod() != http.MethodPost {
		t.Fatalf("unexpected method: %q", payload.GetMethod())
	}
	if payload.GetPath() != "/api/sobject/01/op" {
		t.Fatalf("unexpected path: %q", payload.GetPath())
	}
	if payload.GetSignedHeaders() != "content-type=application/octet-stream" {
		t.Fatalf("unexpected signed headers: %q", payload.GetSignedHeaders())
	}
	wantHash := sha256.Sum256(body)
	if payload.GetBodyHashHex() != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("unexpected body hash: %q", payload.GetBodyHashHex())
	}
	if payload.GetContentLength() != int64(len(body)) {
		t.Fatalf("unexpected content length: %d", payload.GetContentLength())
	}
}

// TestMarshalSyncPushWriteTicketProofPayload verifies sync/push proof payloads
// bind the precomputed body hash and critical upload headers.
func TestMarshalSyncPushWriteTicketProofPayload(t *testing.T) {
	bodyHash := []byte{0xaa, 0xbb, 0xcc}
	bloom := []byte{0x01, 0x02, 0x03}
	payloadBytes, err := marshalSyncPushWriteTicketProofPayload(
		"ticket-456",
		http.MethodPost,
		"/api/bstore/01/sync/push",
		"application/octet-stream",
		42,
		bodyHash,
		"pack-1",
		12,
		bloom,
		123456790,
	)
	if err != nil {
		t.Fatalf("marshalSyncPushWriteTicketProofPayload: %v", err)
	}

	var payload api.WriteTicketProofPayload
	if err := payload.UnmarshalVT(payloadBytes); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.GetTicket() != "ticket-456" {
		t.Fatalf("unexpected ticket: %q", payload.GetTicket())
	}
	if payload.GetSignedHeaders() != "content-type=application/octet-stream,x-block-count=12,x-bloom-filter=AQID,x-pack-id=pack-1" {
		t.Fatalf("unexpected signed headers: %q", payload.GetSignedHeaders())
	}
	if payload.GetBodyHashHex() != hex.EncodeToString(bodyHash) {
		t.Fatalf("unexpected body hash: %q", payload.GetBodyHashHex())
	}
	if payload.GetContentLength() != 42 {
		t.Fatalf("unexpected content length: %d", payload.GetContentLength())
	}
}

// TestDoPost_Success verifies doPost sends a signed POST and returns the body.
func TestDoPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-Peer-ID") == "" {
			t.Error("missing X-Peer-ID")
		}
		if r.Header.Get("X-Signature") == "" {
			t.Error("missing X-Signature")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "request-body" {
			t.Errorf("unexpected body: %q", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response-ok"))
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	resp, err := cli.doPost(context.Background(), "/test", "text/plain", []byte("request-body"), nil, SeedReasonMutation)
	if err != nil {
		t.Fatalf("doPost: %v", err)
	}
	if string(resp) != "response-ok" {
		t.Fatalf("unexpected response: %q", resp)
	}
}

// TestDoPost_ExtraHeaders verifies doPost sends extra headers on the request.
func TestDoPost_ExtraHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "val" {
			t.Errorf("missing custom header, got %q", r.Header.Get("X-Custom"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	_, err := cli.doPost(context.Background(), "/test", "text/plain", nil, map[string]string{
		"X-Custom": "val",
	}, SeedReasonMutation)
	if err != nil {
		t.Fatalf("doPost: %v", err)
	}
}

// TestDoPost_ErrorStatus verifies doPost returns an error for non-2xx status codes.
func TestDoPost_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	_, err := cli.doPost(context.Background(), "/test", "text/plain", nil, nil, SeedReasonMutation)
	if err == nil {
		t.Fatal("expected error for 403 status")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error should contain status code 403: %v", err)
	}
}

// TestDoGet_Success verifies doGet sends a signed GET and returns the body.
func TestDoGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Peer-ID") == "" {
			t.Error("missing X-Peer-ID")
		}
		if r.Header.Get("X-Signature") == "" {
			t.Error("missing X-Signature")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("get-response"))
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	resp, err := cli.doGet(context.Background(), "/data", SeedReasonColdSeed)
	if err != nil {
		t.Fatalf("doGet: %v", err)
	}
	if string(resp) != "get-response" {
		t.Fatalf("unexpected response: %q", resp)
	}
}

// TestDoGet_ErrorStatus verifies doGet returns an error for non-200 status.
func TestDoGet_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	_, err := cli.doGet(context.Background(), "/missing", SeedReasonColdSeed)
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error should contain status code 404: %v", err)
	}
}

// TestDoGet_ServerError verifies doGet returns an error for 500 status.
func TestDoGet_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	cli, _ := newTestSignedClient(t, srv.URL)
	_, err := cli.doGet(context.Background(), "/fail", SeedReasonColdSeed)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error should contain status code 500: %v", err)
	}
}

// TestUpdateSOMetadata_Success verifies UpdateSOMetadata posts the expected payload.
func TestUpdateSOMetadata_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.URL.Path; got != "/api/sobject/so-123/update" {
			t.Errorf("unexpected path: %s", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var req api.SpaceMetadataResponse
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if req.GetDisplayName() != "Renamed Space" {
			t.Fatalf("unexpected display name: %q", req.GetDisplayName())
		}

		resp := &api.SpaceMetadataResponse{
			DisplayName: "Renamed Space",
			ObjectType:  "space",
		}
		data, err := resp.MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	signedCli, _ := newTestSignedClient(t, srv.URL)
	cli := &SessionClient{SignedHTTPClient: signedCli}
	data, err := cli.UpdateSOMetadata(context.Background(), "so-123", &api.SpaceMetadataResponse{DisplayName: "Renamed Space"})
	if err != nil {
		t.Fatalf("UpdateSOMetadata: %v", err)
	}

	var resp api.SpaceMetadataResponse
	if err := resp.UnmarshalVT(data); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.GetDisplayName() != "Renamed Space" {
		t.Fatalf("unexpected response display name: %q", resp.GetDisplayName())
	}
	if resp.GetObjectType() != "space" {
		t.Fatalf("unexpected response object type: %q", resp.GetObjectType())
	}
}

// TestRegisterAccount_Success verifies RegisterAccount sends the correct proto binary and parses the response.
func TestRegisterAccount_Success(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/account/register") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		// Decode the request body.
		body, _ := io.ReadAll(r.Body)
		reqMsg := &api.RegisterAccountRequest{}
		if err := reqMsg.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if reqMsg.GetEntityId() != "test-entity" {
			t.Errorf("unexpected entity_id: %s", reqMsg.GetEntityId())
		}
		if len(reqMsg.GetKeypairs()) != 1 {
			t.Fatalf("expected 1 keypair, got %d", len(reqMsg.GetKeypairs()))
		}
		kp := reqMsg.GetKeypairs()[0]
		if kp.GetPeerId() != pid.String() {
			t.Errorf("keypair peer_id mismatch: %s", kp.GetPeerId())
		}
		if kp.GetAuthMethod() != "ed25519" {
			t.Errorf("unexpected auth_method: %s", kp.GetAuthMethod())
		}

		// Return a response.
		resp := &api.RegisterAccountResponse{AccountId: "acct-123"}
		data, _ := resp.MarshalVT()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	accountID, err := cli.RegisterAccount(context.Background(), "test-entity", "ed25519", nil, "")
	if err != nil {
		t.Fatalf("RegisterAccount: %v", err)
	}
	if accountID != "acct-123" {
		t.Fatalf("unexpected account ID: %q", accountID)
	}
}

// TestRegisterAccount_MissingAccountID verifies RegisterAccount returns error when response has no account_id.
func TestRegisterAccount_MissingAccountID(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &api.RegisterAccountResponse{}
		data, _ := resp.MarshalVT()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	_, err := cli.RegisterAccount(context.Background(), "test-entity", "ed25519", nil, "")
	if err == nil {
		t.Fatal("expected error for missing account_id")
	}
	if !strings.Contains(err.Error(), "missing account_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRegisterAccount_NoPrivateKey verifies RegisterAccount returns error without a key.
func TestRegisterAccount_NoPrivateKey(t *testing.T) {
	_, pid := generateTestKeypair(t)
	cli := &EntityClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: http.DefaultClient,
			baseURL: "http://localhost",
			envPfx:  "spacewave",
			peerID:  pid,
		},
	}

	_, err := cli.RegisterAccount(context.Background(), "test", "ed25519", nil, "")
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

// TestRegisterAccount_ServerError verifies RegisterAccount returns error for server failures.
func TestRegisterAccount_ServerError(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	_, err := cli.RegisterAccount(context.Background(), "test-entity", "ed25519", nil, "")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

// TestRegisterSessionDirect_Success verifies RegisterSessionDirect sends the correct request.
func TestRegisterSessionDirect_Success(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	_, sessionPID := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/account/session/register") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		reqMsg := &api.RegisterSessionRequest{}
		if err := reqMsg.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if reqMsg.GetSessionPeerId() != sessionPID.String() {
			t.Errorf("unexpected session_peer_id: %s", reqMsg.GetSessionPeerId())
		}
		if reqMsg.GetDeviceInfo() != "test-device" {
			t.Errorf("unexpected device_info: %s", reqMsg.GetDeviceInfo())
		}

		resp := &api.RegisterSessionResponse{
			PeerId:    sessionPID.String(),
			AccountId: "acct-789",
		}
		data, _ := resp.MarshalVT()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	err := cli.RegisterSessionDirect(context.Background(), sessionPID.String(), "test-device")
	if err != nil {
		t.Fatalf("RegisterSessionDirect: %v", err)
	}
}

// TestIsBlockedCloudError verifies isBlockedCloudError returns true for dmca_blocked.
func TestIsBlockedCloudError(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"dmca_blocked", "dmca_blocked", true},
		{"unknown_session", "unknown_session", false},
		{"account_not_found", "account_not_found", false},
		{"empty", "", false},
		{"random", "some_other_error", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &cloudError{StatusCode: 451, Code: tt.code, Message: "test"}
			got := isBlockedCloudError(err)
			if got != tt.want {
				t.Errorf("isBlockedCloudError(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// TestIsRefreshableWriteTicketCloudError verifies write-ticket-specific error
// codes are classified separately from session unauthentication and permanent
// failures.
func TestIsRefreshableWriteTicketCloudError(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"invalid_write_ticket", "invalid_write_ticket", true},
		{"expired_write_ticket", "expired_write_ticket", true},
		{"stale_write_ticket", "stale_write_ticket", true},
		{"stale_session_account_write_ticket", "stale_session_account_write_ticket", true},
		{"stale_resource_write_ticket", "stale_resource_write_ticket", true},
		{"unknown_session", "unknown_session", false},
		{"account_not_found", "account_not_found", false},
		{"dmca_blocked", "dmca_blocked", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &cloudError{StatusCode: 401, Code: tt.code, Message: "test"}
			got := isRefreshableWriteTicketCloudError(err)
			if got != tt.want {
				t.Errorf("isRefreshableWriteTicketCloudError(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// TestIsRefreshableWriteTicketCloudError_NonCloudError verifies
// isRefreshableWriteTicketCloudError returns false for non-cloud errors.
func TestIsRefreshableWriteTicketCloudError_NonCloudError(t *testing.T) {
	err := errors.New("generic error")
	if isRefreshableWriteTicketCloudError(err) {
		t.Error("isRefreshableWriteTicketCloudError should return false for non-cloud errors")
	}
}

// TestIsBlockedCloudError_NonCloudError verifies isBlockedCloudError returns false for non-cloud errors.
func TestIsBlockedCloudError_NonCloudError(t *testing.T) {
	err := errors.New("generic error")
	if isBlockedCloudError(err) {
		t.Error("isBlockedCloudError should return false for non-cloud errors")
	}
}

// TestIsAccountDeletedCloudError verifies that only deletedCodes trigger the
// account-deletion cascade, not arbitrary non-retryable cloud responses.
func TestIsAccountDeletedCloudError(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		retryable bool
		want      bool
	}{
		{"account_not_found", "account_not_found", false, true},
		{"invalid_peer_id", "invalid_peer_id", false, true},
		{"unknown_entity", "unknown_entity", false, true},
		{"unknown_session", "unknown_session", false, false},
		{"dmca_blocked", "dmca_blocked", false, false},
		{"duplicate_connection_non_retryable", "duplicate_connection", false, false},
		{"duplicate_connection_retryable", "duplicate_connection", true, false},
		{"empty", "", false, false},
		{"random", "some_other_error", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &cloudError{StatusCode: 409, Code: tt.code, Message: "test", Retryable: tt.retryable}
			got := isAccountDeletedCloudError(err)
			if got != tt.want {
				t.Errorf("isAccountDeletedCloudError(%q, retryable=%v) = %v, want %v", tt.code, tt.retryable, got, tt.want)
			}
		})
	}
}

// TestIsAccountDeletedCloudError_NonCloudError verifies that
// isAccountDeletedCloudError returns false for non-cloud errors.
func TestIsAccountDeletedCloudError_NonCloudError(t *testing.T) {
	err := errors.New("generic error")
	if isAccountDeletedCloudError(err) {
		t.Error("isAccountDeletedCloudError should return false for non-cloud errors")
	}
}

// TestSignMultiSig_Success verifies signMultiSig produces valid signatures.
func TestSignMultiSig_Success(t *testing.T) {
	priv1, pid1 := generateTestKeypair(t)
	priv2, pid2 := generateTestKeypair(t)

	cli := NewEntityClientDirect(http.DefaultClient, "http://localhost", DefaultSigningEnvPrefix, priv1, pid1)

	envelope := []byte("test-envelope-bytes")
	keys := []crypto.PrivKey{priv1, priv2}
	peerIDs := []string{pid1.String(), pid2.String()}

	sigs, err := cli.signMultiSig(envelope, keys, peerIDs)
	if err != nil {
		t.Fatalf("signMultiSig: %v", err)
	}

	if len(sigs) != 2 {
		t.Fatalf("expected 2 signatures, got %d", len(sigs))
	}

	// All signatures share the same timestamp.
	signedAt := sigs[0].GetSignedAt()
	payload := BuildMultiSigPayload(signedAt, envelope)

	// Verify each signature against the full signing payload.
	for i, sig := range sigs {
		if sig.GetPeerId() != peerIDs[i] {
			t.Errorf("sig[%d] peer_id: got %q, want %q", i, sig.GetPeerId(), peerIDs[i])
		}
		if sig.GetSignedAt() == nil {
			t.Fatalf("sig[%d] signed_at is nil", i)
		}
		pub := keys[i].GetPublic()
		ok, err := pub.Verify(payload, sig.GetSignature())
		if err != nil {
			t.Fatalf("verify sig[%d]: %v", i, err)
		}
		if !ok {
			t.Fatalf("sig[%d] verification failed", i)
		}
	}
}

// TestSignMultiSig_LengthMismatch verifies signMultiSig rejects mismatched key/peerID lengths.
func TestSignMultiSig_LengthMismatch(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := NewEntityClientDirect(http.DefaultClient, "http://localhost", DefaultSigningEnvPrefix, priv, pid)

	_, err := cli.signMultiSig([]byte("envelope"), []crypto.PrivKey{priv}, []string{})
	if err == nil {
		t.Fatal("expected error for length mismatch")
	}
}

// TestAddKeypair_Success verifies AddKeypair sends a multi-sig request.
func TestAddKeypair_Success(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	_, newPID := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/account/acct-123/keypair/add"
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		req := &api.MultiSigRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal multi-sig request: %v", err)
		}
		if len(req.GetEnvelope()) == 0 {
			t.Fatal("envelope is empty")
		}
		if len(req.GetSignatures()) != 1 {
			t.Fatalf("expected 1 signature, got %d", len(req.GetSignatures()))
		}
		if req.GetSignatures()[0].GetPeerId() != pid.String() {
			t.Errorf("unexpected signer peer_id: %s", req.GetSignatures()[0].GetPeerId())
		}

		// Decode the typed envelope and verify its bindings.
		env := &api.MultiSigActionEnvelope{}
		if err := env.UnmarshalVT(req.GetEnvelope()); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetAccountId() != "acct-123" {
			t.Errorf("unexpected envelope account_id: %s", env.GetAccountId())
		}
		if env.GetKind() != api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR {
			t.Errorf("unexpected envelope kind: %v", env.GetKind())
		}
		if env.GetMethod() != http.MethodPost {
			t.Errorf("unexpected envelope method: %s", env.GetMethod())
		}
		if env.GetPath() != expectedPath {
			t.Errorf("unexpected envelope path: %s", env.GetPath())
		}

		// Verify the payload is a valid AddKeypairAction.
		action := &api.AddKeypairAction{}
		if err := action.UnmarshalVT(env.GetPayload()); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if action.GetKeypair().GetPeerId() != newPID.String() {
			t.Errorf("unexpected keypair peer_id: %s", action.GetKeypair().GetPeerId())
		}

		// Verify signature over the envelope bytes.
		pub := priv.GetPublic()
		payload := BuildMultiSigPayload(req.GetSignatures()[0].GetSignedAt(), req.GetEnvelope())
		ok, err := pub.Verify(payload, req.GetSignatures()[0].GetSignature())
		if err != nil {
			t.Fatalf("verify signature: %v", err)
		}
		if !ok {
			t.Fatal("signature verification failed")
		}

		respBody, err := (&api.MultiSigActionResponse{
			Result: &api.MultiSigActionResponse_KeypairAdd{
				KeypairAdd: &api.KeypairAddResult{KeypairId: newPID.String()},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)

	keypair := &session.EntityKeypair{
		PeerId:     newPID.String(),
		AuthMethod: "pem",
	}

	result, err := cli.AddKeypair(context.Background(), "acct-123", keypair, []crypto.PrivKey{priv}, []string{pid.String()})
	if err != nil {
		t.Fatalf("AddKeypair: %v", err)
	}
	if result.GetKeypairId() != newPID.String() {
		t.Fatalf("unexpected keypair_id: got %q want %q", result.GetKeypairId(), newPID.String())
	}
}

// TestRemoveKeypair_Success verifies RemoveKeypair sends a multi-sig request.
func TestRemoveKeypair_Success(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	_, removePID := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/account/acct-456/keypair/remove"
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
		}

		body, _ := io.ReadAll(r.Body)
		req := &api.MultiSigRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal multi-sig request: %v", err)
		}

		env := &api.MultiSigActionEnvelope{}
		if err := env.UnmarshalVT(req.GetEnvelope()); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetKind() != api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REMOVE_KEYPAIR {
			t.Errorf("unexpected envelope kind: %v", env.GetKind())
		}
		if env.GetPath() != expectedPath {
			t.Errorf("unexpected envelope path: %s", env.GetPath())
		}

		action := &api.RemoveKeypairAction{}
		if err := action.UnmarshalVT(env.GetPayload()); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if action.GetPeerId() != removePID.String() {
			t.Errorf("unexpected peer_id to remove: %s", action.GetPeerId())
		}

		respBody, err := (&api.MultiSigActionResponse{
			Result: &api.MultiSigActionResponse_KeypairRemove{
				KeypairRemove: &api.KeypairRemoveResult{Removed: true},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	result, err := cli.RemoveKeypair(context.Background(), "acct-456", removePID.String(), []crypto.PrivKey{priv}, []string{pid.String()})
	if err != nil {
		t.Fatalf("RemoveKeypair: %v", err)
	}
	if !result.GetRemoved() {
		t.Fatalf("expected removed=true, got %v", result.GetRemoved())
	}
}

func TestEnsureAccountSObjectBinding_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/account/sobject-binding/ensure" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		req := &api.EnsureAccountSObjectBindingRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal ensure binding request: %v", err)
		}
		if req.GetPurpose() != "account-settings" {
			t.Fatalf("unexpected binding purpose: %q", req.GetPurpose())
		}

		respBody, err := (&api.EnsureAccountSObjectBindingResponse{
			Binding: &api.AccountSObjectBinding{
				Purpose: "account-settings",
				SoId:    "so-123",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED,
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal ensure binding response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	binding, err := cli.EnsureAccountSObjectBinding(context.Background(), "account-settings")
	if err != nil {
		t.Fatalf("EnsureAccountSObjectBinding: %v", err)
	}
	if binding.GetPurpose() != "account-settings" {
		t.Fatalf("unexpected binding purpose: %q", binding.GetPurpose())
	}
	if binding.GetSoId() != "so-123" {
		t.Fatalf("unexpected binding so id: %q", binding.GetSoId())
	}
	if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED {
		t.Fatalf("unexpected binding state: %v", binding.GetState())
	}
}

func TestFinalizeAccountSObjectBinding_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/account/sobject-binding/finalize" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		req := &api.FinalizeAccountSObjectBindingRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal finalize binding request: %v", err)
		}
		if req.GetPurpose() != "account-settings" {
			t.Fatalf("unexpected binding purpose: %q", req.GetPurpose())
		}
		if req.GetSoId() != "so-123" {
			t.Fatalf("unexpected binding so id: %q", req.GetSoId())
		}

		respBody, err := (&api.FinalizeAccountSObjectBindingResponse{
			Binding: &api.AccountSObjectBinding{
				Purpose: "account-settings",
				SoId:    "so-123",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY,
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal finalize binding response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	binding, err := cli.FinalizeAccountSObjectBinding(
		context.Background(),
		"account-settings",
		"so-123",
	)
	if err != nil {
		t.Fatalf("FinalizeAccountSObjectBinding: %v", err)
	}
	if binding.GetPurpose() != "account-settings" {
		t.Fatalf("unexpected binding purpose: %q", binding.GetPurpose())
	}
	if binding.GetSoId() != "so-123" {
		t.Fatalf("unexpected binding so id: %q", binding.GetSoId())
	}
	if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY {
		t.Fatalf("unexpected binding state: %v", binding.GetState())
	}
}

func TestSessionClientGetWriteTicketBundle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/session/write-tickets/res-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get(SeedReasonHeader); got != string(SeedReasonMutation) {
			t.Fatalf("unexpected %s: got %q", SeedReasonHeader, got)
		}

		body, err := (&api.WriteTicketBundleResponse{
			SoOpTicket:           "so-op-ticket",
			SoRootTicket:         "so-root-ticket",
			BstoreSyncPushTicket: "sync-push-ticket",
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	resp, err := cli.GetWriteTicketBundle(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("GetWriteTicketBundle: %v", err)
	}
	if resp.GetSoOpTicket() != "so-op-ticket" {
		t.Fatalf("unexpected so op ticket: %q", resp.GetSoOpTicket())
	}
	if resp.GetSoRootTicket() != "so-root-ticket" {
		t.Fatalf("unexpected so root ticket: %q", resp.GetSoRootTicket())
	}
	if resp.GetBstoreSyncPushTicket() != "sync-push-ticket" {
		t.Fatalf("unexpected sync push ticket: %q", resp.GetBstoreSyncPushTicket())
	}
}

func TestSessionClientGetWriteTicket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/session/write-ticket/res-1/so-op" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get(SeedReasonHeader); got != string(SeedReasonMutation) {
			t.Fatalf("unexpected %s: got %q", SeedReasonHeader, got)
		}

		body, err := (&api.TicketResponse{Ticket: "fresh-so-op-ticket"}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	ticket, err := cli.GetWriteTicket(context.Background(), "res-1", "so-op")
	if err != nil {
		t.Fatalf("GetWriteTicket: %v", err)
	}
	if ticket != "fresh-so-op-ticket" {
		t.Fatalf("unexpected ticket: %q", ticket)
	}
}

// TestSessionClientSeedReason verifies every tagged SessionClient request
// method attaches the expected X-Alpha-Seed-Reason header and that the full
// SeedReason taxonomy is referenced by at least one call site.
func TestSessionClientSeedReason(t *testing.T) {
	cases := []struct {
		name   string
		reason SeedReason
		call   func(t *testing.T, cli *SessionClient) error
	}{
		{
			name:   "GetSessionTicket",
			reason: SeedReasonReconnect,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetSessionTicket(context.Background())
				// Accept any outcome; we only care about the header tagging.
				_ = err
				return nil
			},
		},
		{
			name:   "SyncPull",
			reason: SeedReasonColdSeed,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.SyncPull(context.Background(), "res-1", "")
				return err
			},
		},
		{
			name:   "PostOp",
			reason: SeedReasonMutation,
			call: func(t *testing.T, cli *SessionClient) error {
				cli.executeWriteTicketAudience = func(
					ctx context.Context,
					resourceID string,
					audience writeTicketAudience,
					fn func(ticket string) error,
				) error {
					return fn("ticket-seed-reason")
				}
				return cli.PostOp(context.Background(), "so-1", []byte("op"))
			},
		},
		{
			name:   "ListSharedObjects",
			reason: SeedReasonListBootstrap,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.ListSharedObjects(context.Background())
				return err
			},
		},
		{
			name:   "GetSOState_cold",
			reason: SeedReasonColdSeed,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetSOState(context.Background(), "so-1", 0, SeedReasonColdSeed)
				return err
			},
		},
		{
			name:   "GetSOState_gap",
			reason: SeedReasonGapRecovery,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetSOState(context.Background(), "so-1", 5, SeedReasonGapRecovery)
				return err
			},
		},
		{
			name:   "GetSOState_reconnect",
			reason: SeedReasonReconnect,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetSOState(context.Background(), "so-1", 5, SeedReasonReconnect)
				return err
			},
		},
		{
			name:   "GetConfigChain",
			reason: SeedReasonConfigChainVerify,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetConfigChain(context.Background(), "so-1")
				return err
			},
		},
		{
			name:   "GetSORecoveryEnvelope",
			reason: SeedReasonRejoin,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.GetSORecoveryEnvelope(context.Background(), "so-1")
				// 500 without a valid body surfaces as unmarshal-ish errors; tolerate any error.
				_ = err
				return nil
			},
		},
		{
			name:   "ListSORecoveryEntityKeypairs",
			reason: SeedReasonRejoin,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.ListSORecoveryEntityKeypairs(context.Background(), "so-1")
				_ = err
				return nil
			},
		},
		{
			name:   "ListOrganizations",
			reason: SeedReasonListBootstrap,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.ListOrganizations(context.Background())
				return err
			},
		},
		{
			name:   "ListEmails",
			reason: SeedReasonColdSeed,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.ListEmails(context.Background())
				// Stubbed body is not a valid proto; tolerate any error.
				_ = err
				return nil
			},
		},
		{
			name:   "DeleteOrganization",
			reason: SeedReasonMutation,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.DeleteOrganization(context.Background(), "org-1")
				return err
			},
		},
		{
			name:   "SelfRevoke",
			reason: SeedReasonMutation,
			call: func(t *testing.T, cli *SessionClient) error {
				return cli.SelfRevoke(context.Background())
			},
		},
		{
			name:   "RevokeOrgInvite",
			reason: SeedReasonMutation,
			call: func(t *testing.T, cli *SessionClient) error {
				_, err := cli.RevokeOrgInvite(context.Background(), "org-1", "inv-1")
				return err
			},
		},
	}

	seen := make(map[SeedReason]bool)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotReason string
			var hit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotReason = r.Header.Get(SeedReasonHeader)
				hit = true
				if strings.HasSuffix(r.URL.Path, "/op") {
					body, _ := (&api.SubmitOpResponse{Seqno: 1}).MarshalVT()
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/delete") {
					body, _ := (&api.OrgDeleteResponse{Id: "org-1"}).MarshalVT()
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body)
					return
				}
				if strings.Contains(r.URL.Path, "/invite/") {
					body, _ := (&api.CancelOrgInviteResponse{InviteId: "inv-1"}).MarshalVT()
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{}"))
			}))
			defer srv.Close()

			priv, pid := generateTestKeypair(t)
			cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

			if err := tc.call(t, cli); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !hit {
				t.Fatalf("%s: server not reached", tc.name)
			}
			if gotReason != string(tc.reason) {
				t.Fatalf("%s: X-Alpha-Seed-Reason = %q, want %q", tc.name, gotReason, tc.reason)
			}
			seen[tc.reason] = true
		})
	}

	for _, r := range SeedReasons {
		if !seen[r] {
			t.Errorf("SeedReason %q not referenced by any tested SessionClient call site", r)
		}
	}
}
