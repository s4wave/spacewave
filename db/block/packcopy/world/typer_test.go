package packcopy_world

import (
	"bytes"
	"context"
	"crypto/rand"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// hiddenRefs serves a store's blocks with their refs unknown, as a store that
// never recorded them does.
type hiddenRefs struct {
	store block.StoreOps
}

// GetStoredBlock returns the block's bytes alone.
func (h hiddenRefs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, found, err := h.store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

// holes collects the copy's holes.
type holes struct {
	mtx     sync.Mutex
	reasons map[string]string
}

// add records one hole.
func (h *holes) add(ref *block.BlockRef, reason string) error {
	h.mtx.Lock()
	h.reasons[ref.MarshalString()] = reason
	h.mtx.Unlock()
	return nil
}

// TestTyperRecoversWorldRefs checks that a copy of an encrypted World whose
// store knows no block's refs reaches every block of the typed objects'
// roots, and leaves only the untyped objects' roots as holes.
func TestTyperRecoversWorldRefs(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Encrypt every block, so only a typed decode reads one.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	transform, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      key,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, tb.Bus, le, tb.StepFactorySet, tb.BucketId, tb.Volume.GetID(), transform, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	engine, err := world_block.NewEngine(ctx, le, cursor, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// Write objects with chunked blob roots, typed and untyped, and link them.
	const typed, untyped = 24, 4
	written := make(map[string][]byte, typed)
	ws := world.NewEngineWorldState(engine, true)
	for i := range typed + untyped {
		objKey := "obj-" + strconv.Itoa(i)
		data := make([]byte, 1<<20)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		obj, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
			_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
			return err
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		typeID := "test/blob"
		if i < typed {
			written[objKey] = data
		}
		if i >= typed {
			typeID = "test/other"
		}
		if err := world_types.SetObjectType(ctx, ws, objKey, typeID); err != nil {
			t.Fatal(err)
		}
		if i != 0 {
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("obj-0", "<link>", objKey, "")); err != nil {
				t.Fatal(err)
			}
		}
	}
	head := engine.GetRootRef()
	head.TransformConf = transform
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Copy with the store's refs hidden, typed by the World.
	typer, err := New(ctx, le, tb.Volume, []*bucket.ObjectRef{head}, RootTypes{"test/blob": blob.NewBlobBlock})
	if err != nil {
		t.Fatal(err)
	}
	_, client := s3test.New(t)
	target := block_store_s3.NewPackStore(le, client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true, Index: store_kvtx_inmem.NewStore()})
	defer target.Close()
	h := &holes{reasons: make(map[string]string)}
	result, err := packcopy.Copy(ctx, &packcopy.Options{
		Roots:   []*block.BlockRef{head.GetRootRef()},
		Target:  target,
		Sources: []packcopy.Source{hiddenRefs{store: tb.Volume}},
		Proofs:  store_kvtx_inmem.NewStore(),
		Holes:   h.add,
		Typer:   typer,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only the untyped objects' roots are holes; every other block is copied.
	t.Logf("result %+v untyped %v", *result, typer.Untyped())
	if len(h.reasons) != untyped {
		t.Fatalf("holes %v, want the %d untyped roots", h.reasons, untyped)
	}
	for k, reason := range h.reasons {
		if reason != packcopy.ReasonRefsUnknown {
			t.Fatalf("hole %s is %q, want %q", k, reason, packcopy.ReasonRefsUnknown)
		}
	}
	if typer.Untyped()["object without a listed type"] < untyped {
		t.Fatalf("untyped %v, want at least %d unlisted roots", typer.Untyped(), untyped)
	}

	// The copy alone opens the World and reads back every typed object's blob.
	snap, err := world_block.OpenSnapshot(ctx, le, packcopy.NewSourceStore([]packcopy.Source{target}), head)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	tx, err := snap.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for objKey, data := range written {
		obj, found, err := tx.GetObject(ctx, objKey)
		if err != nil || !found {
			t.Fatalf("object %s: found=%t %v", objKey, found, err)
		}
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			got, err := blob.FetchToBytes(ctx, bcs)
			if err != nil {
				return err
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("object %s read back %d bytes that differ", objKey, len(got))
			}
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatalf("read object %s from the copy: %v", objKey, err)
		}
	}
}
