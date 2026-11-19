package space_world_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestPayloadHoldCoversUnappliedOps gives a Space more than a cursor save's
// worth of another device's operations whose payloads only a stage holds, as
// a migration's import does at a target with no peer to fetch them from.
// Replay stops at an operation whose plugin has not registered, after a
// cursor save, with the operations after it unapplied. The stage releases,
// and a collection that frees runs: every payload must still read, and the
// replay must finish once the plugin registers.
func TestPayloadHoldCoversUnappliedOps(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer engRef.Release()
	listTestOpPlugin(ctx, t, eng)
	so := f.space(t)
	store := so.GetBlockStore()
	dev := addDevice(t, so)

	// Store the payloads under a stage root, as an import copies them.
	const n = 70
	var payloads []*block.BlockRef
	var entries []*block.PutBatchEntry
	for i := range 2 * n {
		data := []byte("payload " + strconv.Itoa(i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, ref)
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
	}
	stageData := []byte("stage")
	for _, ref := range payloads {
		stageData = append(stageData, ref.MarshalString()...)
	}
	stageRef, err := block.BuildBlockRef(stageData, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries = append(entries, &block.PutBatchEntry{Ref: stageRef, Data: stageData, Refs: payloads, RefsKnown: true})
	if err := store.SetRoots(ctx, entries, []block.NamedRoot{{Name: "test-stage", Ref: stageRef}}); err != nil {
		t.Fatal(err)
	}
	cursor, err := store.GetRoot(ctx, space_world.RootReplayCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Add the device's operations in one state change: n creates naming a
	// payload each, an operation only the plugin resolves, and n more.
	keys := []string{"plugin-object"}
	ops := [][]byte{createOp(t, "plugin-object")}
	for i := range 2 * n {
		if i == n {
			ops = append(ops, txOp(t, &world_block_tx.Tx{
				TxType: world_block_tx.TxType_TxType_APPLY_WORLD_OP,
				TxApplyWorldOp: &world_block_tx.TxApplyWorldOp{
					OperationTypeId: world_mock.MockWorldOpTypeID,
					OperationBody:   marshalOp(t, world_mock.NewMockWorldOp("plugin-object", "hello")),
				},
			}))
		}
		key := "payload-" + strconv.Itoa(i)
		keys = append(keys, key)
		ops = append(ops, txOp(t, &world_block_tx.Tx{
			TxType:         world_block_tx.TxType_TxType_CREATE_OBJECT,
			TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: key, RootRef: &bucket.ObjectRef{RootRef: payloads[i]}},
		}))
	}
	loads := make(chan []string, 1)
	start := make(chan struct{})
	releaseLoader, err := f.tb.Bus.AddController(ctx, &gatedOpPluginLoader{b: f.tb.Bus, engineID: f.engineID, loads: loads, start: start}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	if err := so.(sobject.InviteHost).GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		for _, op := range ops {
			if err := dev.add(t, so, state, op, false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for replay to stop at the plugin's operation, after a cursor save.
	select {
	case <-loads:
	case <-ctx.Done():
		t.Fatal("replay never stopped at the plugin's operation")
	}
	for {
		saved, err := store.GetRoot(ctx, space_world.RootReplayCursor)
		if err != nil {
			t.Fatal(err)
		}
		if !saved.EqualsRef(cursor) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("replay stopped without saving the cursor")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Release the stages and collect: every payload still reads.
	if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: "test-stage", Previous: stageRef}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	requirePayloads := func(step string) {
		t.Helper()
		for i, ref := range payloads {
			if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("%s: payload %d freed: %v, %v", step, i, found, err)
			}
		}
	}
	requirePayloads("after the stage released")

	// Register the plugin: replay applies every operation. The World then
	// holds each transaction's payload.
	close(start)
	waitObjects(ctx, t, eng, keys...)
	f.collect(t)
	requirePayloads("after replay")
}

// txOp encodes the World operation that applies tx.
func txOp(t *testing.T, tx *world_block_tx.Tx) []byte {
	t.Helper()
	op, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: tx}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// marshalOp encodes a World operation's body.
func marshalOp(t *testing.T, op *world_mock.MockWorldOp) []byte {
	t.Helper()
	data, err := op.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// gatedOpPluginLoader stands in for a Space resource's operation plugin
// loader, as testOpPluginLoader does, but starts the plugin only once start
// closes.
type gatedOpPluginLoader struct {
	// b is the bus the plugin registers on.
	b bus.Bus
	// engineID is the Space World engine.
	engineID string
	// loads receives the plugins of the first request.
	loads chan []string
	// start closes to start the plugin.
	start chan struct{}
}

// HandleDirective reports the engine's first request, and starts the test
// plugin once start closes.
func (l *gatedOpPluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(space_world.LoadOpPlugins)
	if !ok || dir.LoadOpPluginsEngineID() != l.engineID {
		return nil, nil
	}
	select {
	case l.loads <- dir.LoadOpPluginsPluginIDs():
		go func() {
			<-l.start
			_, _ = l.b.AddController(context.WithoutCancel(ctx), world.NewLookupOpController(testOpPlugin, "", world_mock.LookupMockOp), nil)
		}()
	default:
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (l *gatedOpPluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/gated-op-plugin-loader", controller.MustParseVersion("0.0.1"), "gated test operation plugin loader")
}

// Execute returns immediately.
func (l *gatedOpPluginLoader) Execute(context.Context) error {
	return nil
}

// Close releases nothing.
func (l *gatedOpPluginLoader) Close() error {
	return nil
}

// TestForeignManifestBucketReplays stores a manifest whose ManifestRef names
// a bucket no bucket on the bus has, once as the Space's own write and once
// as another device's operation the Space replays, in the plain and the
// authorized form. Each replayed write must reach the same stored root as
// the local one, and neither may look the foreign bucket up: the World
// cursor reads every ref from the Space's own store.
//
// Both paths run on one engine, so both share one World bucket ID. Two
// replicas whose own World bucket IDs differ are not covered.
func TestForeignManifestBucketReplays(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer engRef.Release()
	so := f.space(t)
	store := so.GetBlockStore()
	dev := addDevice(t, so)

	// Record every bucket lookup or bucket API directive naming the foreign
	// bucket; resolve none.
	const foreign = "foreign-bucket"
	var mtx sync.Mutex
	var lookups int
	removeHandler, err := f.tb.Bus.AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			var id string
			switch dir := di.GetDirective().(type) {
			case bucket_lookup.BuildBucketLookup:
				id = dir.BuildBucketLookupBucketID()
			case bucket.BuildBucketAPI:
				id = dir.BuildBucketAPIBucketID()
			}
			if id == foreign {
				mtx.Lock()
				lookups++
				mtx.Unlock()
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	// Store the manifest block in the Space store, as a download would.
	data, err := (&bldr_manifest.Manifest{Meta: &bldr_manifest.ManifestMeta{ManifestId: "foreign"}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	root, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: root, Data: data, RefsKnown: true}}
	if err := store.SetRoots(ctx, entries, []block.NamedRoot{{Name: "test-manifest", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	storeOp := func(key string, authorized bool) *manifest_world.StoreManifestOp {
		ref := &bldr_manifest.ManifestRef{
			Meta:        &bldr_manifest.ManifestMeta{ManifestId: "foreign"},
			ManifestRef: &bucket.ObjectRef{BucketId: foreign, RootRef: root},
		}
		if authorized {
			if err := ref.SignReleaseAuthorization(dev.priv); err != nil {
				t.Fatal(err)
			}
		}
		return manifest_world.NewStoreManifestOp(key, nil, ref)
	}

	for _, authorized := range []bool{false, true} {
		form := "plain"
		if authorized {
			form = "authorized"
		}
		local, replayed := "local-"+form, "replayed-"+form

		// The Space's own write applies at once.
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			_, _, err := ws.ApplyWorldOp(ctx, storeOp(local, authorized), so.GetPeerID())
			return err
		}); err != nil {
			t.Fatalf("%s: local write: %v", form, err)
		}

		// The device's operation replays.
		body, err := storeOp(replayed, authorized).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		dev.write(t, so, txOp(t, &world_block_tx.Tx{
			TxType: world_block_tx.TxType_TxType_APPLY_WORLD_OP,
			TxApplyWorldOp: &world_block_tx.TxApplyWorldOp{
				OperationTypeId: manifest_world.StoreManifestOpTypeID,
				OperationBody:   body,
			},
		}), false, false)
		waitObjects(ctx, t, eng, replayed)

		// Both objects hold the same root.
		refs := make([]*bucket.ObjectRef, 2)
		if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
			for i, key := range []string{local, replayed} {
				obj, err := world.MustGetObject(ctx, ws, key)
				if err != nil {
					return err
				}
				refs[i], _, err = obj.GetRootRef(ctx)
				world.ReleaseObjectState(obj)
				if err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !refs[0].EqualVT(refs[1]) {
			t.Fatalf("%s: replayed root %v differs from the local root %v", form, refs[1], refs[0])
		}
	}

	mtx.Lock()
	defer mtx.Unlock()
	if lookups != 0 {
		t.Fatalf("%d directives looked up the foreign bucket", lookups)
	}
}

// TestMissingTreePageWaits removes pages of a World tree from the Space's
// store, then replays another device's operation that reads one: replay must
// wait for the block as a gap, not refuse the operation, since another
// replica with the page applies it. It covers the object tree, an IAVL tree,
// read by a World operation, and the graph tree, an Okra tree, read by a
// graph quad write. Both trees are those every Space World has.
func TestMissingTreePageWaits(t *testing.T) {
	cases := []struct {
		name string
		// pages returns the refs of pages the operation reads, and the
		// operation.
		pages func(t *testing.T, w *world_block.World, keys []string) ([]*block.BlockRef, []byte)
		// applied reports whether ws holds the operation's write.
		applied func(ctx context.Context, ws world.WorldState, keys []string) (bool, error)
	}{
		{"iavl", func(t *testing.T, w *world_block.World, keys []string) ([]*block.BlockRef, []byte) {
			// Take both children of the root, and an object other than the
			// root's, which one of them holds.
			tree := w.GetObjectKeyValue()
			root := tree.GetIavlRoot()
			impl := tree.GetImplType()
			if impl != kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL && impl != kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH {
				t.Fatalf("object tree is %v, not IAVL", impl)
			}
			if root.GetLeftChildRef().GetEmpty() || root.GetRightChildRef().GetEmpty() {
				t.Fatal("the object tree's root has no two child pages")
			}
			key := keys[0]
			if string(root.GetKey()) == key {
				key = keys[1]
			}
			return []*block.BlockRef{root.GetLeftChildRef(), root.GetRightChildRef()}, txOp(t, &world_block_tx.Tx{
				TxType: world_block_tx.TxType_TxType_APPLY_WORLD_OP,
				TxApplyWorldOp: &world_block_tx.TxApplyWorldOp{
					OperationTypeId: world_mock.MockWorldOpTypeID,
					OperationBody:   marshalOp(t, world_mock.NewMockWorldOp(key, "hello")),
				},
			})
		}, func(ctx context.Context, ws world.WorldState, keys []string) (bool, error) {
			// The operation wrote its message to one of the first two objects.
			for _, key := range keys[:2] {
				obj, err := world.MustGetObject(ctx, ws, key)
				if err != nil {
					return false, err
				}
				var msg string
				_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
					ex, err := block_mock.UnmarshalExample(ctx, bcs)
					msg = ex.GetMsg()
					return err
				})
				world.ReleaseObjectState(obj)
				if err != nil || msg == "hello" {
					return err == nil, err
				}
			}
			return false, nil
		}},
		{"okra", func(t *testing.T, w *world_block.World, keys []string) ([]*block.BlockRef, []byte) {
			// Take the root page, which every graph read starts from.
			tree := w.GetGraphKeyValue()
			switch impl := tree.GetImplType(); impl {
			case kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA,
				kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE,
				kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE_NODE_GRAPH:
			default:
				t.Fatalf("graph tree is %v, not Okra", impl)
			}
			ref := tree.GetOkraRoot().GetRootPageRef()
			if ref.GetEmpty() {
				t.Fatal("the graph tree has no root page")
			}
			quad := world.NewGraphQuadWithKeys(keys[0], "<other>", keys[2], "")
			return []*block.BlockRef{ref}, txOp(t, &world_block_tx.Tx{
				TxType:         world_block_tx.TxType_TxType_SET_GRAPH_QUAD,
				TxSetGraphQuad: &world_block_tx.TxSetGraphQuad{Quad: world.GraphQuadToQuad(quad)},
			})
		}, func(ctx context.Context, ws world.WorldState, keys []string) (bool, error) {
			// The operation wrote its quad.
			quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(keys[0], "<other>", keys[2], ""), 1)
			return len(quads) == 1, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runMissingTreePage(t, tc.pages, tc.applied)
		})
	}
}

// runMissingTreePage fills both World trees past the Space's page bound, so
// the replay cursor save stores the head's pages, removes the pages pages
// names from the Space's store, and remounts, so the World reads them from
// the store and not from the pages the adapter kept in memory, as after a
// reload. It then replays the operation pages returns from another device,
// checks that replay waits on the gap, and puts the pages back: replay must
// then catch up with the operation's write, as applied reports, which shows
// the wait was for those pages and that it ends once they return.
func runMissingTreePage(
	t *testing.T,
	pages func(t *testing.T, w *world_block.World, keys []string) ([]*block.BlockRef, []byte),
	applied func(ctx context.Context, ws world.WorldState, keys []string) (bool, error),
) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{maxPending: 4096})
	eng, engRef := f.start(t)
	defer func() { engRef.Release() }()
	so := f.space(t)
	store := so.GetBlockStore()
	dev := addDevice(t, so)
	accessor, ok := so.(sobject.SharedObjectHealthAccessor)
	if !ok {
		t.Fatal("the Space's shared object exposes no health")
	}
	health, releaseHealth, err := accessor.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHealth()
	cursor, err := store.GetRoot(ctx, space_world.RootReplayCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Fill both trees past one page: objects, each linked to the next.
	var keys []string
	for i := range 64 {
		keys = append(keys, "obj-"+strconv.Itoa(100+i))
	}
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		for i, key := range keys {
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if i != 0 {
				if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(keys[i-1], "<next>", key, "")); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	waitHealth(ctx, t, health, "caught up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})

	// Find the pages in the installed World.
	var refs []*block.BlockRef
	var op []byte
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		return ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			_, bcs := bls.BuildTransaction(nil)
			w, err := world_block.UnmarshalWorld(ctx, bcs)
			if err != nil {
				return err
			}
			refs, op = pages(t, w, keys)
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}

	// The fill passed the page bound, so the cursor save stores the World:
	// wait for its root to move, then for each page in the store.
	for {
		moved, err := store.GetRoot(ctx, space_world.RootReplayCursor)
		if err != nil {
			t.Fatal(err)
		}
		if !moved.GetEmpty() && !moved.EqualVT(cursor) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("the replay cursor was never saved")
		case <-time.After(10 * time.Millisecond):
		}
	}
	saved := make([]*block.StoredBlock, len(refs))
	for i, ref := range refs {
		stored, err := store.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil {
			t.Fatalf("the saved cursor's World lacks a page: %v", err)
		}
		saved[i] = stored
	}

	// Evict the pages from the Volume, which leaves holes as a writeback
	// cache's eviction does while its backend is out of reach, so a read
	// misses; the Space store's RmBlock only releases a hold. Then remount,
	// as a reload does.
	for _, ref := range refs {
		if err := f.volume.RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
		if exists, err := store.GetBlockExists(ctx, ref); err != nil || exists {
			t.Fatalf("page still stored after removal: %v, %v", exists, err)
		}
	}
	eng, engRef = f.remount(ctx, t, eng, engRef)

	// The device's operation reads a page: replay waits on the gap.
	dev.write(t, so, op, false, false)
	waitHealth(ctx, t, health, "the gap wait", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetWaiting().GetGap() != nil
	})

	// With the pages back, as the read-through refill puts them once the
	// backend answers, replay applies the operation.
	for i, ref := range refs {
		if _, _, err := store.PutBlock(ctx, saved[i].Data, saved[i].PutOpts(ref)); err != nil {
			t.Fatal(err)
		}
	}
	waitHealth(ctx, t, health, "caught up after the pages returned", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})
	var wrote bool
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		var err error
		wrote, err = applied(ctx, ws, keys)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("replay caught up without the operation's write, so it refused the operation")
	}
}
