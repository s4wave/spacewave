package sobject_world_engine_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_file "github.com/s4wave/spacewave/db/block/file"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// errAbandon abandons a test transaction.
var errAbandon = errors.New("abandon")

// spaceWorld is a World engine on a local Space SharedObject.
type spaceWorld struct {
	bus      bus.Bus
	soRef    *sobject.SharedObjectRef
	engineID string
	sender   peer.ID
	eng      world.Engine
	vol      volume.Volume
	rg       block_gc.RefGraphOps
	bucket   string
}

// freshMember is a Space SharedObject seen by a member that has decoded none
// of its blocks, so its replay reads every block from the store.
type freshMember struct {
	sobject.SharedObject
}

// GetBlockStore returns the block store without its decoded block cache.
func (m freshMember) GetBlockStore() bstore.BlockStore {
	return uncachedStore{m.SharedObject.GetBlockStore()}
}

// uncachedStore is a block store with no decoded block cache.
type uncachedStore struct {
	bstore.BlockStore
}

// GetDecodedBlockCache returns nil, so each reader decodes from the store.
func (uncachedStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return nil
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

	// Start the World engine on the SharedObject without a changelog, as a
	// Space does, so replaced roots are unreachable from the head.
	engineID := "file-gc-engine"
	conf := sobject_world_engine.NewConfig(engineID, soRef)
	conf.InitWorldOp = &sobject_world_engine.InitWorldOp{LastChangeDisable: true}
	ctrl, _, ctrlRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrlRef.Release)

	// Register the mock and file operations and get the engine.
	lookupOp := world.NewLookupOpFromSlice(world.LookupOpSlice{world_mock.LookupMockOp, unixfs_world.LookupFsOp})
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("file-gc-ops", engineID, lookupOp), nil)
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
		bus:      tb.Bus,
		soRef:    soRef,
		engineID: engineID,
		sender:   tb.Volume.GetPeerID(),
		eng:      eng,
		vol:      vol,
		rg:       vol.(kvtx_volume.KvtxVolume).GetRefGraph(),
		bucket:   block_gc.BucketIRI(provider_local.BlockStoreBucketID(providerID, accountID, provider_local.SobjectBlockStoreID(sobjectID))),
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

// fileData is the content appended to the file of the payload test.
var fileData = []byte("appended file content")

// createFile creates FS object "fs" holding the empty file "file".
func (w *spaceWorld) createFile(ctx context.Context, ws world.WorldState) error {
	// Create the FS object.
	_, _, err := unixfs_world.FsInit(ctx, ws, w.sender, "fs", unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now())
	if err != nil {
		return err
	}

	// Create the file in it.
	obj, err := world.MustGetObject(ctx, ws, "fs")
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	_, _, err = unixfs_world.FsMknod(ctx, obj, w.sender, unixfs_world.FSType_FSType_FS_NODE, [][]string{{"file"}}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now())
	return err
}

// appendFile appends fileData to the empty file "file" of FS object "fs".
func (w *spaceWorld) appendFile(ctx context.Context, ws world.WorldState) error {
	// Look up the FS object and write at the file's end.
	obj, err := world.MustGetObject(ctx, ws, "fs")
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	_, _, err = unixfs_world.FsWriteAt(ctx, ws, obj, w.sender, unixfs_world.FSType_FSType_FS_NODE, []string{"file"}, 0, fileData, time.Now())
	return err
}

// checkFile checks that the file "file" of FS object "fs" holds fileData.
func checkFile(ctx context.Context, ws world.WorldState) error {
	return readObject(ctx, ws, "fs", func(cursor *block.Cursor) error {
		// Find the file in the FS tree.
		root, err := unixfs_block.NewFSTree(ctx, cursor, unixfs_block.NodeType_NodeType_UNKNOWN)
		if err != nil {
			return err
		}
		node, _, err := unixfs_block.LookupFSTreePath(root, []string{"file"})
		if err != nil {
			return err
		}

		// Read its content and compare it with what was written.
		fh, err := node.BuildFileHandle(ctx)
		if err != nil {
			return err
		}
		defer fh.Close()
		data, err := io.ReadAll(fh)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, fileData) {
			return errors.New("file read " + strconv.Quote(string(data)))
		}
		return nil
	})
}

// TestWorldEngineKeepsOperationPayloads checks that the payload of a file
// write survives a sweep while its operation is above the checkpoint, so a
// replay from the checkpoint rebuilds the file. An append copies the payload
// into the file, so the World after the write does not reference the payload.
func TestWorldEngineKeepsOperationPayloads(t *testing.T) {
	// Start the Space World.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	w := newSpaceWorld(ctx, t)

	// Create an empty file, append to it, then sweep everything unreferenced.
	if err := world.ExecTransaction(ctx, w.eng, true, w.createFile); err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, w.eng, true, w.appendFile); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(w.rg, w.vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	w.checkReplayedFile(ctx, t)
}

// checkReplayedFile replays the Space's operation set from the checkpoint, as
// another member does, and checks that the replayed World holds the file.
func (w *spaceWorld) checkReplayedFile(ctx context.Context, t *testing.T) {
	// Read the Space's operation set.
	t.Helper()
	so, soRef, err := sobject.ExMountSharedObject(ctx, w.bus, w.soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soRef.Release()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Replay it from the checkpoint.
	le := logrus.NewEntry(logrus.New())
	replayed, release, err := sobject_world_engine.OpenReadCheckpoint(ctx, le, w.bus, freshMember{so}, w.engineID, unixfs_world.LookupFsOp, snap)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The replayed World holds the appended content.
	if err := world.ExecTransaction(ctx, replayed, false, checkFile); err != nil {
		t.Fatal(err)
	}
}

// editRecord applies a mock object op to object "record", which reads the
// object's root when it applies.
func (w *spaceWorld) editRecord(ctx context.Context, ws world.WorldState) error {
	// Apply the op to the existing object.
	obj, err := world.MustGetObject(ctx, ws, "record")
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	_, _, err = obj.ApplyObjectOp(ctx, world_mock.NewMockObjectOp("edited"), w.sender)
	return err
}

// TestWorldEngineKeepsReplacedObjectRoots checks that an object root a later
// write replaced survives a sweep while the operations that read it are above
// the checkpoint, so a replay from the checkpoint reaches the head. The head
// reaches only the last root of the object.
func TestWorldEngineKeepsReplacedObjectRoots(t *testing.T) {
	// Start the Space World.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	w := newSpaceWorld(ctx, t)

	// Create the object, edit it with an operation that reads its root,
	// replace its root, then sweep everything unreferenced.
	writes := []func(context.Context, world.WorldState) error{
		func(ctx context.Context, ws world.WorldState) error {
			return writeExample(ctx, ws, "record", "created")
		},
		w.editRecord,
		func(ctx context.Context, ws world.WorldState) error {
			return writeExample(ctx, ws, "record", "replaced")
		},
	}
	for _, write := range writes {
		if err := world.ExecTransaction(ctx, w.eng, true, write); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := block_gc.NewCollector(w.rg, w.vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}

	// Read the Space's operation set.
	so, soRef, err := sobject.ExMountSharedObject(ctx, w.bus, w.soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soRef.Release()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Replay it from the checkpoint, as another member does.
	le := logrus.NewEntry(logrus.New())
	replayed, release, err := sobject_world_engine.OpenReadCheckpoint(ctx, le, w.bus, freshMember{so}, w.engineID, world_mock.LookupMockObjectOp, snap)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The replayed World holds the last root.
	err = world.ExecTransaction(ctx, replayed, false, func(ctx context.Context, ws world.WorldState) error {
		return checkExample(ctx, ws, "record", "replaced")
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorldEngineObjectBlocksHaveParents checks that every block a
// SharedObject World transaction writes is owned by its parent block or
// released, never left owned directly by the Space bucket, and that every
// committed object survives a sweep afterwards. It covers chunked file
// objects, abandoned transactions and an object updated several times within
// one transaction.
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

	// Update one object several times within one transaction, as a batched
	// writer does. Only the final root stays referenced.
	err := world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
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
