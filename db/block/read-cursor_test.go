package block_test

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	blockmock "github.com/s4wave/spacewave/db/block/mock"
	transform "github.com/s4wave/spacewave/db/block/transform"
	transformall "github.com/s4wave/spacewave/db/block/transform/all"
	gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// TestReadCursorAuthority keeps transform, scope and staged data boundaries on
// the verified loading path even after an immutable node has become warm.
func TestReadCursorAuthority(t *testing.T) {
	// Use a real shareable Page and the production gzip transformer.
	ctx := t.Context()
	store := blockmock.NewMockStore(0)
	conf, err := transform.NewConfig([]config.Config{&gzip.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	xfrm, err := transform.NewTransformer(controller.ConstructOpts{}, transformall.BuildFactorySet(), conf)
	if err != nil {
		t.Fatal(err)
	}
	first, second := block.NewDecodedBlockCache(), block.NewDecodedBlockCache()
	defer first.Close()
	defer second.Close()
	build := func(target block.StoreOps, xfrm block.Transformer) *block.BlockRef {
		// Stream one inline entry into real encoded Okra pages.
		bt, cs, err := okra.BuildTreeWithEntries(ctx, target, xfrm, nil, func(yield func(okra.BuildEntry) bool) {
			yield(okra.BuildEntry{Key: []byte("key"), ValueIsBlob: true, ValueBlob: blob.NewRawBlob([]byte("body"))})
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := bt.Write(ctx, true); err != nil {
			t.Fatal(err)
		}
		root, err := block.UnmarshalBlock[okra.Root](ctx, cs)
		if err != nil {
			t.Fatal(err)
		}
		return root.GetRootPageRef()
	}
	ref := build(store, xfrm)
	tx, source := block.NewTransaction(store, xfrm, ref, nil)
	tx.SetReadOnly()
	tx.SetDecodedBlockCache(first)
	reader := block.NewReadCursor(ctx, source, ref)
	if reader == nil {
		t.Fatal("missing immutable reader")
	}

	// Warm a verified immutable page and require a subsequent pointer hit.
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *reader); err != nil {
		t.Fatal(err)
	}
	counted, counter := block.WithReadCounter(ctx)
	if _, err := block.UnmarshalReadBlock[okra.Page](counted, *reader); err != nil {
		t.Fatal(err)
	}
	if counter.Snapshot().DecodedBlockUnmarshalCount != 0 {
		t.Fatal("warm immutable node was decoded again")
	}

	// A mutable cursor must not expose or alter the reader's immutable identity.
	materialized := reader.Cursor()
	materialized.GetRef().Hash = nil
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *reader); err != nil {
		t.Fatalf("materialized cursor changed reader identity: %v", err)
	}

	// Changing the borrowed cache scope must bypass the old node's retained data.
	tx.SetDecodedBlockCache(second)
	counted, counter = block.WithReadCounter(ctx)
	if _, err := block.UnmarshalReadBlock[okra.Page](counted, *reader); err != nil {
		t.Fatal(err)
	}
	if counter.Snapshot().DecodedBlockUnmarshalCount != 1 {
		t.Fatal("old cache scope supplied the new scope's read")
	}

	// An identical reference in another owner scope cannot hide absent storage.
	otherTx, otherSource := block.NewTransaction(blockmock.NewMockStore(0), xfrm, ref, nil)
	otherTx.SetReadOnly()
	otherCache := block.NewDecodedBlockCache()
	defer otherCache.Close()
	otherTx.SetDecodedBlockCache(otherCache)
	other := block.NewReadCursor(ctx, otherSource, ref)
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *other); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("other scope supplied absent block: %v", err)
	}

	// Removing the transform cannot reuse a gzip page's decoded pointer.
	wrongTx, wrongSource := block.NewTransaction(store, nil, ref, nil)
	wrongTx.SetReadOnly()
	wrongTx.SetDecodedBlockCache(first)
	wrong := block.NewReadCursor(ctx, wrongSource, ref)
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *wrong); err == nil {
		t.Fatal("gzip page crossed the transform boundary")
	}

	// Stage a different page without publishing it to durable storage.
	stage := block.NewBufferedStore(ctx, store)
	stagedRef := build(stage, nil)
	stagedTx, stagedSource := block.NewTransaction(stage, nil, stagedRef, nil)
	stagedTx.SetReadOnly()
	stagedTx.SetDecodedBlockCache(first)
	stagedReader := block.NewReadCursor(ctx, stagedSource, stagedRef)
	for range 2 {
		if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *stagedReader); err != nil {
			t.Fatal(err)
		}
	}
	stagedSource.SetBlockStore(store)
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *stagedReader); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("staged page survived its authority: %v", err)
	}
}
