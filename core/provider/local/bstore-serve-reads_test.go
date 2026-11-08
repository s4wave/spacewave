package provider_local

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// TestServeReadsBelowLocalVolume proves a peer's request reads a block held
// only on the backend or only in the read fallback, and fills neither into the
// local store.
func TestServeReadsBelowLocalVolume(t *testing.T) {
	ctx := t.Context()
	local, remote := newLeafStore(), newLeafStore()
	fallback := &countingStore{StoreOps: newLeafStore()}
	opts := &block.PutOpts{HashType: hash.HashType_HashType_SHA256}
	onRemote, _, err := remote.PutBlock(ctx, []byte("held-on-the-backend"), opts)
	if err != nil {
		t.Fatal(err)
	}
	onFallback, _, err := fallback.PutBlock(ctx, []byte("held-in-the-fallback"), opts)
	if err != nil {
		t.Fatal(err)
	}

	p := &placementState{}
	serve := p.serveReads(local, func() block.StoreOps { return remote }, fallback)
	for ref, want := range map[*block.BlockRef]string{onRemote: "held-on-the-backend", onFallback: "held-in-the-fallback"} {
		stored, err := serve.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !bytes.Equal(stored.Data, []byte(want)) {
			t.Fatalf("serve %s = %v, %v", want, stored, err)
		}
		if has, err := local.GetBlockExists(ctx, ref); err != nil || has {
			t.Fatalf("local holds %s after serving: %v, %v", want, has, err)
		}
	}
	if _, _, err := serve.PutBlock(ctx, []byte("write"), opts); err == nil {
		t.Fatal("serve reads took a write")
	}
}

// TestServeStoreRemount proves a store that exits after its remount leaves
// the remount's serve reads registered.
func TestServeStoreRemount(t *testing.T) {
	a := &ProviderAccount{}
	prev, next := &serveStore{StoreOps: newLeafStore()}, &serveStore{StoreOps: newLeafStore()}
	a.serveStores.Store("bucket", prev)
	a.serveStores.Store("bucket", next)
	a.serveStores.CompareAndDelete("bucket", prev)
	if got := a.getServeStore("bucket"); got != next.StoreOps {
		t.Fatal("the exit of the previous mount dropped the remount's serve reads")
	}
	a.serveStores.CompareAndDelete("bucket", next)
	if a.getServeStore("bucket") != nil {
		t.Fatal("serve reads outlived their mount")
	}
}
