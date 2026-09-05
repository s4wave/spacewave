package publish

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// TestPostRootUsesAuthoritativeSequence covers a destination whose CDN pointer
// has not caught up with its current initialized or previously published root.
func TestPostRootUsesAuthoritativeSequence(t *testing.T) {
	// Use the existing publication client fixture with a current snapshot.
	state := &api.SOStateMessage{Content: &api.SOStateMessage_Snapshot{
		Snapshot: &sobject.SOState{Root: &sobject.SORoot{InnerSeqno: 42}},
	}}
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	client := &promoteTestClient{state: data}

	// Sign with disposable test material; publication never generates identities.
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPrivKeyPem(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "signer.pem")
	if err := os.WriteFile(keyPath, pem, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := PostRoot(t.Context(), Options{
		Client: client, DstSpaceID: "test-space", ValidatorKeyPem: keyPath,
		CdnBaseURL: "https://unavailable.invalid",
	}, testPublishObjectRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if root.GetInnerSeqno() != 43 || client.roots != 1 {
		t.Fatalf("published sequence=%d roots=%d", root.GetInnerSeqno(), client.roots)
	}
}
