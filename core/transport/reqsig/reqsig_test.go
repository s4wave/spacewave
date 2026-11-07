package reqsig_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/transport/reqsig"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// testEnv is the signing environment of the tests.
const testEnv = "spacewave-test"

// TestVerifiesSessionClientRequests proves Verify accepts a request the
// session client signed, signed headers included, and refuses one whose body,
// signed header, environment or time differ.
func TestVerifiesSessionClientRequests(t *testing.T) {
	// Capture requests the session client signs.
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	var got *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		got = r
	}))
	defer srv.Close()
	cli := provider_spacewave.NewSessionClient(http.DefaultClient, srv.URL, testEnv, priv, pid.String())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/bstore/space/sync/push", bytes.NewReader([]byte("push body")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Pack-ID", "abc")
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got.Header.Get("X-Signed-Headers") == "" {
		t.Fatal("session client signed no headers")
	}

	// The signed request verifies as the signing peer.
	now := time.Now()
	if signer, err := reqsig.Verify(got, gotBody, testEnv, now); err != nil || signer != pid {
		t.Fatalf("verify: %v %v", signer, err)
	}

	// A changed body, signed header, environment or time is refused.
	if _, err := reqsig.Verify(got, []byte("other body"), testEnv, now); err == nil {
		t.Fatal("changed body verified")
	}
	if _, err := reqsig.Verify(got, gotBody, "spacewave", now); err == nil {
		t.Fatal("other environment verified")
	}
	if _, err := reqsig.Verify(got, gotBody, testEnv, now.Add(2*reqsig.ClockSkew)); err == nil {
		t.Fatal("late request verified")
	}
	got.Header.Del("X-Sw-Hash")
	got.Header.Set("X-Pack-ID", "abd")
	if _, err := reqsig.Verify(got, gotBody, testEnv, now); err == nil {
		t.Fatal("changed signed header verified")
	}
}
