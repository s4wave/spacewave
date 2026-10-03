package publish

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/sirupsen/logrus"
)

// TestPostCheckpointFollowsAuthoritativeState covers a destination whose CDN
// pointer has not caught up with its current checkpoint.
func TestPostCheckpointFollowsAuthoritativeState(t *testing.T) {
	// Sign with disposable test material; publication never generates identities.
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Serve the genesis snapshot, whose checkpoint the CDN pointer does not hold yet.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	genesis, _, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.New()), sfs, "test-space", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := (&api.SOStateMessage{Content: &api.SOStateMessage_Snapshot{Snapshot: genesis}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	client := &promoteTestClient{state: state}

	// Write the owner key to a file.
	pem, err := keypem.MarshalPrivKeyPem(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "signer.pem")
	if err := os.WriteFile(keyPath, pem, 0o600); err != nil {
		t.Fatal(err)
	}

	// Post the first checkpoint.
	checkpoint, err := PostCheckpoint(t.Context(), Options{
		Client: client, DstSpaceID: "test-space", OwnerKeyPem: keyPath,
		CdnBaseURL: "https://unavailable.invalid",
	}, testPublishObjectRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.GetHeight() != 1 || client.checkpoints != 1 {
		t.Fatalf("published height=%d checkpoints=%d", checkpoint.GetHeight(), client.checkpoints)
	}
}
