package provider_spacewave

import (
	"bytes"
	"crypto/sha256"
	"net/http"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/packfile"
)

// TestSyncReplaceDataBindsRetirement verifies the public replacement API
// uploads the complete body and names the superseded catalog entries in the
// session-signed push request.
func TestSyncReplaceDataBindsRetirement(t *testing.T) {
	// Serve the push protocol.
	data := []byte("replacement pack")
	digest := sha256.Sum256(data)
	push := startTestPushServer(t)

	// Replace two packs with one.
	priv, id := generateTestKeypair(t)
	client := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, id.String())
	if err := client.SyncReplaceData(t.Context(), "resource", "replacement", 1, data, digest[:], []byte("bloom"), packfile.BloomFormatVersionV1, []string{"old-a", "old-b"}); err != nil {
		t.Fatal(err)
	}

	// The cloud committed the body and the replaced IDs.
	packs := push.committed()
	if len(packs) != 1 {
		t.Fatalf("committed %d packs, want 1", len(packs))
	}
	if got := packs[0].req.GetReplacesPackIds(); !slices.Equal(got, []string{"old-a", "old-b"}) {
		t.Errorf("replacement IDs = %v", got)
	}
	if !bytes.Equal(packs[0].data, data) {
		t.Errorf("replacement body = %q", packs[0].data)
	}
}
