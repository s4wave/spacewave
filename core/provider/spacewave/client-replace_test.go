package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestSyncReplaceDataBindsRetirement verifies the public replacement API sends
// the complete body and cryptographically binds the superseded catalog entries.
func TestSyncReplaceDataBindsRetirement(t *testing.T) {
	data := []byte("replacement pack")
	digest := sha256.Sum256(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(syncPushReplacesHeader); got != "old-a,old-b" {
			t.Errorf("replacement IDs = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != string(data) {
			t.Errorf("replacement body = %q, error = %v", body, err)
		}
		encoded, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Write-Proof"))
		if err != nil {
			t.Error(err)
			return
		}
		proof := new(api.WriteTicketProof)
		if err := proof.UnmarshalVT(encoded); err != nil {
			t.Error(err)
			return
		}
		payload := new(api.WriteTicketProofPayload)
		if err := payload.UnmarshalVT(proof.GetPayload()); err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(payload.GetSignedHeaders(), "x-replaces-pack-ids=old-a%2Cold-b") {
			t.Errorf("replacement IDs omitted from signed headers: %s", payload.GetSignedHeaders())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	priv, id := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, priv, id.String())
	client.executeWriteTicketAudience = func(ctx context.Context, resourceID string, audience writeTicketAudience, fn func(string) error) error {
		if resourceID != "resource" || audience != writeTicketAudienceBstoreSyncPush {
			t.Fatalf("unexpected write authority: %s %s", resourceID, audience)
		}
		return fn("replacement-ticket")
	}
	if err := client.SyncReplaceData(t.Context(), "resource", "replacement", 1, data, digest[:], []byte("bloom"), packfile.BloomFormatVersionV1, []string{"old-a", "old-b"}); err != nil {
		t.Fatal(err)
	}
}
