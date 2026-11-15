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
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
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

	// Release the stage and collect: every payload still reads.
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

	// Register the plugin: replay applies every operation.
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
