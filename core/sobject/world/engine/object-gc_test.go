package sobject_world_engine_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_file "github.com/s4wave/spacewave/db/block/file"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/testbed"
)

// errAbandon abandons a test transaction.
var errAbandon = errors.New("abandon")

// spaceWorld is a World engine on a local Space SharedObject.
type spaceWorld struct {
	eng    world.Engine
	vol    volume.Volume
	rg     block_gc.RefGraphOps
	bucket string
}

// newSpaceWorld starts a World engine on a new local Space SharedObject.
func newSpaceWorld(ctx context.Context, t *testing.T) *spaceWorld {
	// Start a testbed.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register the provider and World engine factories.
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	// Create a local Space SharedObject.
	providerID, accountID, sobjectID := "local", "test-account", "file-gc-space"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

	// Create the Space through the provider account.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provAccRef.Release)
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err)
	}
	soRef, err := soProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine on the SharedObject.
	engineID := "file-gc-engine"
	ctrl, _, ctrlRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, sobject_world_engine.NewConfig(engineID, soRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrlRef.Release)

	// Register the mock operations and get the engine.
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("file-gc-ops", engineID, world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relOpc)
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up the volume that holds the Space bucket.
	vol, _, volRef, err := volume.ExLookupVolume(ctx, tb.Bus, provider_local.StorageVolumeID(providerID, accountID), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(volRef.Release)
	return &spaceWorld{
		eng:    eng,
		vol:    vol,
		rg:     vol.(kvtx_volume.KvtxVolume).GetRefGraph(),
		bucket: block_gc.BucketIRI(provider_local.BlockStoreBucketID(providerID, accountID, provider_local.SobjectBlockStoreID(sobjectID))),
	}
}

// bucketOnlyBlocks lists the bucket's block children without another parent.
func (w *spaceWorld) bucketOnlyBlocks(ctx context.Context, t *testing.T) []string {
	// List the bucket's children.
	t.Helper()
	children, err := w.rg.GetOutgoingRefs(ctx, w.bucket)
	if err != nil {
		t.Fatal(err)
	}

	// Keep the blocks without an incoming edge from another node.
	var orphans []string
	for _, child := range children {
		if _, ok := block_gc.ParseBlockIRI(child); !ok {
			continue
		}
		owned, err := w.rg.HasIncomingRefsExcluding(ctx, child, w.bucket)
		if err != nil {
			t.Fatal(err)
		}
		if !owned {
			orphans = append(orphans, child)
		}
	}
	return orphans
}

// rawData is the content of raw file object i.
func rawData(i int) []byte {
	return bytes.Repeat([]byte("raw evidence line "+strconv.Itoa(i)+"\n"), 128<<10)
}

// writeRawFile writes raw file object i as a raw evidence writer does.
func writeRawFile(ctx context.Context, ws world.WorldState, i int) error {
	// Create the object as a file node holding the chunked content.
	obj, _, err := world.CreateWorldObject(ctx, ws, "raw/"+strconv.Itoa(i), func(cursor *block.Cursor) error {
		// Build the file node and its chunked content.
		node := unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_FILE, 0o600, nil)
		cursor.SetBlock(node, true)
		var err error
		node.File, err = block_file.BuildFileWithBytes(ctx, cursor.FollowSubBlock(4), rawData(i), nil)
		return err
	})
	world.ReleaseObjectState(obj)
	return err
}

// writeExample sets object key to an example block holding msg.
func writeExample(ctx context.Context, ws world.WorldState, key, msg string) error {
	// Replace the object's root block.
	_, _, err := world.AccessWorldObject(ctx, ws, key, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample(msg), true)
		return nil
	})
	return err
}

// readObject applies cb to the root of object key, which must exist.
func readObject(ctx context.Context, ws world.WorldState, key string, cb world.AccessObjectCb) error {
	// Look up the object.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return errors.New(key + " not found")
	}

	// Read its root.
	_, _, err = world.AccessObjectState(ctx, obj, false, cb)
	return err
}

// checkExample checks that object key holds an example block with msg.
func checkExample(ctx context.Context, ws world.WorldState, key, msg string) error {
	return readObject(ctx, ws, key, func(cursor *block.Cursor) error {
		// Read the example and compare its message.
		ex, err := block.UnmarshalBlock[*block_mock.Example](ctx, cursor, block_mock.NewExampleBlock)
		if err != nil {
			return err
		}
		if got := ex.GetMsg(); got != msg {
			return errors.New(key + ": read " + got + ", want " + msg)
		}
		return nil
	})
}

// checkRawFile checks the content of raw file object i.
func checkRawFile(ctx context.Context, ws world.WorldState, i int) error {
	return readObject(ctx, ws, "raw/"+strconv.Itoa(i), func(cursor *block.Cursor) error {
		// Read the file node and its content.
		node, err := unixfs_block.UnmarshalFSNode(ctx, cursor)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(block_file.NewHandle(ctx, cursor.FollowSubBlock(4), node.GetFile()))
		if err != nil {
			return err
		}

		// Compare it with what was written.
		if !bytes.Equal(data, rawData(i)) {
			return errors.New("raw/" + strconv.Itoa(i) + " content differs")
		}
		return nil
	})
}

// TestWorldEngineObjectBlocksHaveParents checks that every block a
// SharedObject World transaction writes is owned by its parent block or
// released, never left owned directly by the Space bucket, and that every
// committed object survives a sweep afterwards. It covers chunked file
// objects, abandoned transactions, including one that drained part of its
// write buffer, and an object updated several times within one transaction.
func TestWorldEngineObjectBlocksHaveParents(t *testing.T) {
	// Start the Space World.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	w := newSpaceWorld(ctx, t)

	// Commit three file objects and abandon a fourth.
	for i := range 4 {
		err := world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
			if err := writeRawFile(ctx, ws, i); err != nil || i != 3 {
				return err
			}
			return errAbandon
		})
		if err != nil && !errors.Is(err, errAbandon) {
			t.Fatal(err)
		}
	}

	// Abandon a transaction that wrote more blocks than its write buffer holds,
	// so part of it drained to the bucket before the abandon.
	err := world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		for i := range 5000 {
			if err := writeExample(ctx, ws, "drained/"+strconv.Itoa(i), "drained "+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return errAbandon
	})
	if !errors.Is(err, errAbandon) {
		t.Fatal(err)
	}

	// Update one object several times within one transaction, as a batched
	// writer does. Only the final root stays referenced.
	err = world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		for i := range 4 {
			if err := writeExample(ctx, ws, "record", "record version "+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Batch many sequential writers into one commit, as a write-behind
	// queue does, across several commits.
	batch := world.NewBatchEngine(w.eng)
	for round := range 4 {
		err := batch.Run(ctx, func(ctx context.Context) error {
			for i := range 64 {
				key := "batch/" + strconv.Itoa(i%24)
				err := world.ExecTransaction(ctx, batch, true, func(ctx context.Context, ws world.WorldState) error {
					if err := writeExample(ctx, ws, key, key+" round "+strconv.Itoa(round)+" write "+strconv.Itoa(i)); err != nil {
						return err
					}
					return world_types.SetObjectType(ctx, ws, key, "test/type-"+strconv.Itoa(i%3))
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// No block may be owned by the bucket alone.
	if orphans := w.bucketOnlyBlocks(ctx, t); len(orphans) != 0 {
		t.Fatalf("%d blocks are owned only by the bucket: %v", len(orphans), orphans)
	}

	// Sweep everything unreferenced.
	if _, err := block_gc.NewCollector(w.rg, w.vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}

	// Read back every committed object.
	err = world.ExecTransaction(ctx, w.eng, false, func(ctx context.Context, ws world.WorldState) error {
		for i := range 3 {
			if err := checkRawFile(ctx, ws, i); err != nil {
				return err
			}
		}
		if err := checkExample(ctx, ws, "record", "record version 3"); err != nil {
			return err
		}
		for i := 40; i < 64; i++ {
			key := "batch/" + strconv.Itoa(i%24)
			if err := checkExample(ctx, ws, key, key+" round 3 write "+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
