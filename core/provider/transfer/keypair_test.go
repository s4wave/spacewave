package provider_transfer_test

import (
	"context"
	"testing"

	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	volume_store "github.com/s4wave/spacewave/db/volume/store"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
)

// TestKeypairTransfer verifies that TransferKeypair copies the private key
// from one volume to another, resulting in the same peer ID.
func TestKeypairTransfer(t *testing.T) {
	ctx := context.Background()

	srcTb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tgtTb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	srcVol := srcTb.Volume
	tgtVol := tgtTb.Volume

	srcPeerID := srcVol.GetPeerID()
	tgtPeerID := tgtVol.GetPeerID()
	if srcPeerID == tgtPeerID {
		t.Fatal("source and target should have different peer IDs initially")
	}

	if err := provider_transfer.TransferKeypair(ctx, srcVol, tgtVol); err != nil {
		t.Fatal(err)
	}

	// Load the stored key from the target volume to verify the transfer.
	tgtStore, ok := tgtVol.(volume_store.Store)
	if !ok {
		t.Fatal("target volume does not implement Store")
	}
	storedKey, err := tgtStore.LoadPeerPriv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if storedKey == nil {
		t.Fatal("expected non-nil stored key")
	}

	storedPeerID, err := peer.IDFromPrivateKey(storedKey)
	if err != nil {
		t.Fatal(err)
	}
	if storedPeerID != srcPeerID {
		t.Fatalf("expected stored peer ID %s to match source %s", storedPeerID, srcPeerID)
	}
}
