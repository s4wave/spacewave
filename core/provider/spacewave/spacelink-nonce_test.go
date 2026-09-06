package provider_spacewave

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

// TestCachedSpaceLinkNoncePreservesConsumption verifies the persisted marker
// format written before cloud replay protection, without creating new markers.
func TestCachedSpaceLinkNoncePreservesConsumption(t *testing.T) {
	ctx := context.Background()
	store := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	account := &ProviderAccount{objStore: store}
	key := []byte("spacelink-nonce/agent=6167656e742d70656572/nonce=6e6f6e63652d31/payload=2e6709af8dbfe7cd5abb2f716924848e527b4486c30c4509b0e4aa8171987335")
	marker := make([]byte, 8)
	binary.BigEndian.PutUint64(marker, uint64(time.Now().Add(time.Minute).Unix()))
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, key, marker); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := account.checkCachedSpaceLinkNonce(ctx, []byte("agent-peer"), []byte("nonce-1"), []byte("payload-1")); !errors.Is(err, ErrSpaceLinkNonceConsumed) {
		t.Fatalf("persisted consumption = %v", err)
	}
}
