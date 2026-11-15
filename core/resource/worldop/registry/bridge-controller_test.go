package resource_worldop_registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

func TestWorldOpRegistryBridgeControllerAppliesPluginWorldAndObjectOps(t *testing.T) {
	// Start a World testbed for plugin operations on supplied transactions.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Capture the authenticated sender for the typed object factory.
	sender := tb.Volume.GetPeerID()

	// Typed factories must use the same supplied transaction and authenticated sender.
	typed := objecttype.NewObjectType("test/supplied-state", func(
		ctx context.Context, le *logrus.Entry, b bus.Bus, engine world.Engine,
		ws world.WorldState, key string,
	) (srpc.Invoker, func(), error) {
		// Require the typed factory to retain the supplied transaction and sender.
		if engine != nil || ws.GetReadOnly() || objecttype.SessionPeerIDFromContext(ctx) != sender {
			return nil, nil, fmt.Errorf("typed factory escaped the operation context")
		}

		// Create an object through the supplied typed factory transaction.
		obj, err := ws.CreateObject(ctx, "test/typed-op-created", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return nil, nil, err
		}
		return srpc.NewMux(), func() {}, nil
	})

	// Register the typed factory controller for the plugin World operation.
	typedController := objecttype_controller.NewController(func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == "test/supplied-state" {
			return typed, nil
		}
		return nil, nil
	})
	typedRelease, err := tb.Bus.AddController(ctx, typedController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer typedRelease()

	// Expose the test operation handler through the plugin Resource server.
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, &testWorldOpHandler{sender: sender}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))

	// Keep the plugin load controller available throughout the test.
	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()

	// Register the plugin operation and attach the registry bridge controller.
	registry := NewWorldOpRegistryResource(nil)
	registry.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "test/plugin-op",
		RegistrationId:  1,
		PluginId:        "test-plugin",
	}
	ctrl := NewWorldOpRegistryBridgeController(le, tb.Bus, registry)
	rel, err = tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	defer rel()

	// Resolve the registered plugin operation for the test World engine.
	vs, _, ref, err := world.ExLookupWorldOp(
		ctx,
		tb.Bus,
		le,
		"test/plugin-op",
		tb.EngineID,
	)
	if err != nil {
		t.Fatalf("ExLookupWorldOp: %v", err)
	}
	defer ref.Release()
	if len(vs) != 1 {
		t.Fatalf("expected 1 lookup op, got %d", len(vs))
	}

	// Decode a bridge operation and verify its selected World engine.
	op, err := vs[0](ctx, "test/plugin-op")
	if err != nil {
		t.Fatalf("lookup op: %v", err)
	}
	engineID, ok := bridgeOperationEngineID(op)
	if !ok {
		t.Fatalf("expected bridge operation, got %T", op)
	}
	if engineID != tb.EngineID {
		t.Fatalf("expected engineID %q, got %q", tb.EngineID, engineID)
	}
	if err := op.UnmarshalVT([]byte("op-data")); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}

	// Open the supplied World transaction for the plugin operation.
	worldTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Apply the plugin World operation through the supplied transaction.
	sysErr, err := op.ApplyWorldOp(ctx, le, worldTx, sender)
	if err != nil {
		worldTx.Discard()
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	if sysErr {
		worldTx.Discard()
		t.Fatal("ApplyWorldOp returned system error")
	}

	// Commit the plugin World mutation and verify both created objects.
	if err := worldTx.Commit(ctx); err != nil {
		t.Fatalf("Commit world op tx: %v", err)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "test/world-op-created")
	assertWorldObjectExists(t, ctx, tb.Engine, "test/typed-op-created")

	// Open the object operation transaction and create its target object.
	objectTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := objectTx.CreateObject(ctx, "test/object-op-target", nil)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		objectTx.Discard()
		t.Fatalf("CreateObject: %v", err)
	}

	// Apply the plugin operation to the acquired target object.
	_, sysErr, err = obj.ApplyObjectOp(ctx, op, sender)
	if err != nil {
		objectTx.Discard()
		t.Fatalf("ApplyWorldObjectOp: %v", err)
	}
	if sysErr {
		objectTx.Discard()
		t.Fatal("ApplyWorldObjectOp returned system error")
	}

	// Commit the object mutation and verify its revision and file system root.
	if err := objectTx.Commit(ctx); err != nil {
		t.Fatalf("Commit object op tx: %v", err)
	}
	assertObjectRevAtLeast(t, ctx, tb.Engine, "test/object-op-target", 1)
	assertObjectRootSet(t, ctx, tb.Engine, "test/object-op-target")
}

func TestWorldOpRegistryBridgeControllerValidatesBeforeMutation(t *testing.T) {
	// Start a World testbed for plugin validation failure.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Expose a plugin handler that rejects operation validation.
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, &testWorldOpHandler{
		validateError: "plugin validation failed",
	}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))

	// Keep the rejecting plugin load controller available throughout the test.
	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()

	// Register the rejecting plugin operation and attach its registry bridge.
	registry := NewWorldOpRegistryResource(nil)
	registry.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "test/plugin-op",
		RegistrationId:  1,
		PluginId:        "test-plugin",
	}
	ctrl := NewWorldOpRegistryBridgeController(le, tb.Bus, registry)
	rel, err = tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	defer rel()

	// Resolve and decode the plugin operation that will fail validation.
	vs, _, ref, err := world.ExLookupWorldOp(
		ctx,
		tb.Bus,
		le,
		"test/plugin-op",
		tb.EngineID,
	)
	if err != nil {
		t.Fatalf("ExLookupWorldOp: %v", err)
	}
	defer ref.Release()
	op, err := vs[0](ctx, "test/plugin-op")
	if err != nil {
		t.Fatalf("lookup op: %v", err)
	}
	if err := op.UnmarshalVT([]byte("op-data")); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}

	// Apply the rejected operation and verify the World remains unchanged.
	worldTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	sysErr, err := op.ApplyWorldOp(ctx, le, worldTx, tb.Volume.GetPeerID())
	worldTx.Discard()
	if err == nil || err.Error() != "plugin validation failed" {
		t.Fatalf("ApplyWorldOp error = %v, want plugin validation failed", err)
	}
	if sysErr {
		t.Fatal("validation error should not be reported as a system error")
	}
	assertWorldObjectMissing(t, ctx, tb.Engine, "test/world-op-created")
}

func TestWorldOpRegistryBridgeControllerDeadlineReleasesWriter(t *testing.T) {
	// Start a World testbed whose plugin never answers ApplyWorldOp.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	handler := &stallWorldOpHandler{entered: make(chan struct{}, 1), stall: make(chan struct{})}
	t.Cleanup(func() { close(handler.stall) })

	// Expose the stalling handler through the plugin Resource server.
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, handler); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))
	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()

	// Apply the operation through the World resource over the engine, which
	// holds the writer for it.
	ws := world.NewEngineWorldState(tb.Engine, true)
	res := resource_world.NewWorldStateResource(le, tb.Bus, ws, lookupPluginOp(ctx, t, tb, le), resource_world.WithSessionPeerID(tb.Volume.GetPeerID()))
	applied := make(chan *s4wave_world.ApplyWorldOpResponse, 1)
	go func() {
		resp, err := res.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{OpTypeId: "test/plugin-op", OpData: []byte("op-data")})
		if err != nil {
			t.Errorf("ApplyWorldOp: %v", err)
		}
		applied <- resp
	}()
	<-handler.entered

	// A second writer proceeds once the deadline releases the writer.
	writeCtx, cancel := context.WithTimeout(ctx, 3*pluginOpDeadline)
	defer cancel()
	obj, err := ws.CreateObject(writeCtx, "test/second-writer", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("second writer: %v", err)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "test/second-writer")

	// The stalled operation ends with the timeout code: nothing written.
	if code := (<-applied).GetErrorCode(); code != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT {
		t.Fatalf("ApplyWorldOp error code = %v, want WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT", code)
	}
}

func TestWorldOpRegistryBridgeControllerRetriesShareWriteBudget(t *testing.T) {
	// Start a World testbed whose plugin applies a nested plugin operation
	// that sleeps for most of the write's budget.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	const budgetLength = time.Second
	handler := &nestedSleepWorldOpHandler{sleep: 800 * time.Millisecond, attached: make(chan attachedWorldState, 4)}

	// Expose the handler through the plugin Resource server.
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, handler); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))
	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()
	lookupOp := lookupPluginOp(ctx, t, tb, le)
	op, err := lookupOp(ctx, "test/plugin-op")
	if err != nil {
		t.Fatal(err)
	}
	if err := op.UnmarshalVT([]byte("outer")); err != nil {
		t.Fatal(err)
	}

	// Resolve the nested operation to the same plugin, recording the host's
	// result of each nested apply.
	nested := make(chan nestedResult, 4)
	relNested, err := tb.Bus.AddController(ctx, world.NewLookupOpController("test/nested-op-lookup", "", func(ctx context.Context, opTypeID string) (world.Operation, error) {
		if opTypeID != nestedOpTypeID {
			return nil, nil
		}
		op, err := lookupOp(ctx, "test/plugin-op")
		if err != nil {
			return nil, err
		}
		return &recordedOp{bridgeOperation: op.(*bridgeOperation), results: nested}, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relNested()

	// The first attempt spends most of the budget, then another writer
	// supersedes it. The replay spends the rest and is cut at the deadline.
	var attempts int
	start := time.Now()
	budgetCtx := world.WithWriteBudget(ctx, world.NewWriteBudget(budgetLength))
	err = world.ExecTransaction(budgetCtx, tb.Engine, true, func(ctx context.Context, wtx world.WorldState) error {
		attempts++
		if _, _, err := wtx.ApplyWorldOp(ctx, op, tb.Volume.GetPeerID()); err != nil {
			return err
		}
		return world.ErrSuperseded
	})
	held := time.Since(start)
	var timeout *world.PluginOpTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("ExecTransaction error = %v, want PluginOpTimeoutError", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if held > budgetLength+500*time.Millisecond {
		t.Fatalf("writer held %v, want at most %v plus margin", held, budgetLength)
	}

	// The first nested operation finishes within the budget. The host cuts
	// the second with the write's budget at its deadline. Given a fresh
	// budget, it would instead sleep through and succeed, or end when the
	// outer call's end canceled it, and neither is a PluginOpTimeoutError.
	for i := range 2 {
		var r nestedResult
		select {
		case r = <-nested:
		case <-time.After(5 * time.Second):
			t.Fatalf("nested operation %d did not return", i)
		}
		ended := r.at.Sub(start)
		if i == 0 {
			if r.err != nil {
				t.Fatalf("first nested operation: %v, want OK", r.err)
			}
			continue
		}
		var nestedTimeout *world.PluginOpTimeoutError
		if !errors.As(r.err, &nestedTimeout) || nestedTimeout.Deadline != budgetLength {
			t.Fatalf("second nested operation ended after %v with %v, want a PluginOpTimeoutError at the %v budget", ended, r.err, budgetLength)
		}
		// The type and deadline above are the proof; this window only rules
		// out the 1.6s a fresh budget's full sleep would take.
		if ended < budgetLength || ended > budgetLength+500*time.Millisecond {
			t.Fatalf("second nested operation ended after %v, want at the %v budget", ended, budgetLength)
		}
	}

	// Each attempt's World state reaches the plugin detached, including the
	// one attached when the budget expired.
	close(handler.attached)
	var n int
	for a := range handler.attached {
		n++
		waitDetached(t, a)
	}
	if n != 2 {
		t.Fatalf("attached World states = %d, want 2", n)
	}
}

func TestWriteBudgetGoWriteReplaysPastBudget(t *testing.T) {
	// Start a World testbed and a write whose budget every attempt outlasts.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	const budgetLength = 20 * time.Millisecond
	budgetCtx := world.WithWriteBudget(ctx, world.NewWriteBudget(budgetLength))

	// No plugin call claims the budget, so the write replays past it and
	// commits on its third attempt.
	var attempts int
	err = world.ExecTransaction(budgetCtx, tb.Engine, true, func(ctx context.Context, wtx world.WorldState) error {
		attempts++
		time.Sleep(2 * budgetLength)
		obj, err := wtx.CreateObject(ctx, "go-write", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if attempts < 3 {
			return world.ErrSuperseded
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecTransaction: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "go-write")
}

func TestWriteBudgetPerformOpInstallsBudget(t *testing.T) {
	// Start a World testbed and the engine's operation-scoped World state.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	probe := &budgetProbeOp{Operation: world_mock.NewMockWorldOp("probe", "probe")}

	// Without a caller's budget, the write installs one of the default length
	// and starts it when it takes the writer.
	if _, _, err := ws.ApplyWorldOp(ctx, probe, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	if probe.budget == nil || probe.budget.GetLength() != world.WriteBudgetLength {
		t.Fatalf("operation budget = %v, want one of %v", probe.budget, world.WriteBudgetLength)
	}
	if _, started := probe.budget.Deadline(); !started {
		t.Fatal("operation budget was not started")
	}

	// A caller's budget governs the write instead.
	own := world.NewWriteBudget(time.Second)
	if _, _, err := ws.ApplyWorldOp(world.WithWriteBudget(ctx, own), probe, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	if probe.budget != own {
		t.Fatal("operation did not carry the caller's budget")
	}
}

func TestWorldOpRegistryBridgeControllerColdLoadHoldsNoWriter(t *testing.T) {
	// Start a World testbed whose plugin never finishes loading.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	loading := make(chan struct{}, 1)
	rel, err := tb.Bus.AddController(ctx, &hangPluginLoadController{loading: loading}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()

	// Apply the operation through the World resource over the engine while
	// its plugin loads, under a short load deadline.
	const loadDeadline = 2 * time.Second
	ws := world.NewEngineWorldState(tb.Engine, true)
	lookup := lookupPluginOp(ctx, t, tb, le, WithPluginLoadDeadline(loadDeadline))
	res := resource_world.NewWorldStateResource(le, tb.Bus, ws, lookup, resource_world.WithSessionPeerID(tb.Volume.GetPeerID()))
	applied := make(chan *s4wave_world.ApplyWorldOpResponse, 1)
	start := time.Now()
	go func() {
		resp, err := res.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{OpTypeId: "test/plugin-op", OpData: []byte("op-data")})
		if err != nil {
			t.Errorf("ApplyWorldOp: %v", err)
		}
		applied <- resp
	}()
	<-loading

	// A second writer proceeds while the load still hangs: the load holds no
	// writer.
	writeCtx, cancel := context.WithTimeout(ctx, loadDeadline/2)
	defer cancel()
	obj, err := ws.CreateObject(writeCtx, "test/second-writer", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("second writer: %v", err)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "test/second-writer")
	select {
	case <-applied:
		t.Fatal("ApplyWorldOp returned before its plugin load deadline")
	default:
	}

	// The load deadline ends the operation with the unavailable code: it was
	// never sent, so nothing was written.
	resp := <-applied
	if code := resp.GetErrorCode(); code != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE {
		t.Fatalf("ApplyWorldOp error code = %v, want WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE", code)
	}
	if elapsed := time.Since(start); elapsed > 2*loadDeadline {
		t.Fatalf("ApplyWorldOp returned after %v, want about %v", elapsed, loadDeadline)
	}
}

func TestWorldOpRegistryBridgeControllerPluginWriteGuard(t *testing.T) {
	// Seed the Space settings object, a manifest store and a manifest pinned by
	// a manifest edge through the Space's own handle, which carries no guard.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	sender := tb.Volume.GetPeerID()
	ws := world.NewEngineWorldState(tb.Engine, true)
	if world.GuardWorldState(ws, nil) != ws {
		t.Fatal("a session handle must make its writes with no guard lookup")
	}
	const settingsKey = space_world.SpaceSettingsObjectKey
	const storeKey, manifestKey, plainKey = "test/manifest-store", "test/manifest", "test/plain"
	for key, typeID := range map[string]string{
		settingsKey: "",
		storeKey:    bldr_manifest_world.ManifestStoreTypeID,
		manifestKey: bldr_manifest_world.ManifestTypeID,
	} {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if typeID != "" {
			if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
				t.Fatal(err)
			}
		}
	}
	manifestEdge := world.NewGraphQuadWithKeys(storeKey, bldr_manifest_world.PredManifest.String(), manifestKey, "")
	if err := ws.SetGraphQuad(ctx, manifestEdge); err != nil {
		t.Fatal(err)
	}

	// Give the plugin a guarded handle that counts the guard's lookups, and
	// create a plain object through it to aim the raw writes from.
	guard := &lookupCountingGuard{guard: space_world_optypes.NewPluginWriteGuard("test-plugin")}
	pws := world.GuardWorldState(ws, guard)
	obj, err := pws.CreateObject(ctx, plainKey, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("plugin CreateObject(%q): %v", plainKey, err)
	}
	before := snapshotProtectedObjects(ctx, t, ws, settingsKey, storeKey, manifestKey, plainKey)

	// objectWrite applies write to the object at key through the plugin handle.
	objectWrite := func(key string, write func(world.ObjectState) error) error {
		obj, err := world.MustGetObject(ctx, pws, key)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		return write(obj)
	}
	applyOp := func(op world.Operation) error {
		_, _, err := pws.ApplyWorldOp(ctx, op, sender)
		return err
	}
	fsPath := func(name string) *unixfs_block.FSPath { return unixfs_block.NewFSPath([]string{name}, false) }
	ts := time.Unix(1, 0)
	settingsOp := space_world_ops.NewSetSpaceSettingsOp(settingsKey, &space_world.SpaceSettings{IndexPath: "/evil"}, true, ts)

	// tombstone removes a block through a cursor; removal is an unscoped write.
	tombstoneRef, err := block.BuildBlockRef([]byte("plugin tombstone target"), nil)
	if err != nil {
		t.Fatal(err)
	}
	tombstone := func(cursor *bucket_lookup.Cursor) error {
		_, err := cursor.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: tombstoneRef, Tombstone: true}})
		return err
	}
	cursorTombstone := func(build func(context.Context) (*bucket_lookup.Cursor, error)) error {
		cursor, err := build(ctx)
		if err != nil {
			return err
		}
		defer cursor.Release()
		return tombstone(cursor)
	}

	// guardedTxCursor calls cb with a writable cursor from a guarded write
	// transaction, as a plugin op handler's World holds, then discards it.
	guardedTxCursor := func(cb func(*bucket_lookup.Cursor) error) error {
		wtx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()
		cursor, err := world.GuardWorldState(wtx, guard).BuildStorageCursor(ctx)
		if err != nil {
			return err
		}
		defer cursor.Release()
		return cb(cursor)
	}

	// Every refused write fails with the final plugin rejection code.
	refused := []struct {
		name  string
		write func() error
	}{
		{"op set-settings", func() error { return applyOp(settingsOp) }},
		{"op add-plugins", func() error { return applyOp(&space_world_ops.AddSpacePluginsOp{PluginIds: []string{"evil"}}) }},
		{"op set-index-path", func() error { return applyOp(&space_world_ops.SetSpaceIndexPathOp{IndexPath: "/evil"}) }},
		{"op store manifest", func() error { return applyOp(&bldr_manifest_world.StoreManifestOp{ObjectKey: storeKey}) }},
		{"op extract manifest bundle", func() error {
			return applyOp(&bldr_manifest_world.ExtractManifestBundleOp{ObjectKey: storeKey})
		}},
		{"fs-init overwrite of settings", func() error {
			return applyOp(unixfs_world.NewFsInitOp(settingsKey, unixfs_world.FSType_FSType_FS_NODE, nil, true, ts))
		}},
		{"fs-rename inside settings", func() error {
			return applyOp(unixfs_world.NewFsRenameOp(settingsKey, unixfs_world.FSType_FSType_FS_NODE, fsPath("a"), fsPath("b"), ts))
		}},
		{"fs-copy inside manifest store", func() error {
			return applyOp(unixfs_world.NewFsCopyOp(storeKey, unixfs_world.FSType_FSType_FS_NODE, fsPath("a"), fsPath("b"), ts))
		}},
		{"kv set-root on the manifest store", func() error {
			return applyOp(&s4wave_kv_world.KvSetRootOp{ObjectKey: storeKey})
		}},
		{"create settings", func() error {
			obj, err := pws.CreateObject(ctx, settingsKey, nil)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"delete settings", func() error { _, err := pws.DeleteObject(ctx, settingsKey); return err }},
		{"delete manifest store", func() error { _, err := pws.DeleteObject(ctx, storeKey); return err }},
		{"rename settings away", func() error {
			obj, err := pws.RenameObject(ctx, settingsKey, "test/moved", false)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"rename onto settings", func() error {
			obj, err := pws.RenameObject(ctx, plainKey, settingsKey, false)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"rename with descendants", func() error {
			obj, err := pws.RenameObject(ctx, plainKey, "test/moved", true)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"set root of settings", func() error {
			return objectWrite(settingsKey, func(obj world.ObjectState) error {
				_, err := obj.SetRootRef(ctx, &bucket.ObjectRef{})
				return err
			})
		}},
		{"set root of manifest store", func() error {
			return objectWrite(storeKey, func(obj world.ObjectState) error {
				_, err := obj.SetRootRef(ctx, &bucket.ObjectRef{})
				return err
			})
		}},
		{"repin manifest by setting its root", func() error {
			return objectWrite(manifestKey, func(obj world.ObjectState) error {
				_, err := obj.SetRootRef(ctx, &bucket.ObjectRef{})
				return err
			})
		}},
		{"increment rev of manifest store", func() error {
			return objectWrite(storeKey, func(obj world.ObjectState) error {
				_, err := obj.IncrementRev(ctx)
				return err
			})
		}},
		{"object op set-settings on settings", func() error {
			return objectWrite(settingsKey, func(obj world.ObjectState) error {
				_, _, err := obj.ApplyObjectOp(ctx, settingsOp, sender)
				return err
			})
		}},
		{"type a plain object as a manifest", func() error {
			return world_types.SetObjectType(ctx, pws, plainKey, bldr_manifest_world.ManifestTypeID)
		}},
		{"pin a plain object into the manifest store", func() error {
			return pws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(storeKey, bldr_manifest_world.PredManifest.String(), plainKey, ""))
		}},
		{"manifest edge between plain objects", func() error {
			return pws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(plainKey, bldr_manifest_world.PredManifest.String(), plainKey, ""))
		}},
		{"edge to settings", func() error {
			return pws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(plainKey, "<test/link>", settingsKey, ""))
		}},
		{"delete manifest edge", func() error { return pws.DeleteGraphQuad(ctx, manifestEdge) }},
		{"delete manifest store graph", func() error { return pws.DeleteGraphObject(ctx, storeKey) }},
		{"writable graph handle", func() error {
			return pws.AccessCayleyGraph(ctx, true, func(context.Context, world.CayleyHandle) error {
				t.Error("writable graph handle opened for a plugin")
				return nil
			})
		}},
		{"tombstone through a storage cursor", func() error { return cursorTombstone(pws.BuildStorageCursor) }},
		{"tombstone through a transaction's cursor", func() error { return guardedTxCursor(tombstone) }},
		{"tombstone through world access", func() error { return pws.AccessWorldState(ctx, nil, tombstone) }},
		{"tombstone through object access", func() error {
			return objectWrite(plainKey, func(obj world.ObjectState) error {
				return obj.AccessWorldState(ctx, nil, tombstone)
			})
		}},
		{"tombstone through a stage", func() error {
			stage, err := pws.StageWorldState(ctx)
			if err != nil {
				return err
			}
			defer stage.Release()
			return cursorTombstone(stage.BuildStorageCursor)
		}},
	}
	for _, c := range refused {
		guard.reset()
		err := c.write()
		var rejection *world.OperationRejection
		if !errors.As(err, &rejection) || rejection.Code != space_world_optypes.PluginWriteRefusedCode {
			t.Errorf("%s: error = %v, want rejection %q", c.name, err, space_world_optypes.PluginWriteRefusedCode)
		}
		t.Logf("refused %s: %d guard checks, %d guard lookups", c.name, guard.checks, guard.lookups)
	}

	// A batch through a guarded engine handle refuses each raw write kind at the
	// protected entry and keeps none of its earlier entries.
	lookupSettingsOp := func(_ context.Context, typeID string) (world.Operation, error) {
		if typeID == settingsOp.TypeID() {
			return &space_world_ops.SetSpaceSettingsOp{}, nil
		}
		return nil, nil
	}
	settingsOpData, err := settingsOp.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	engineRes := resource_world.NewEngineResource(le, tb.Bus, tb.Engine, lookupSettingsOp, nil, resource_world.WithWriteGuard(guard))
	batchRefused := []struct {
		name  string
		entry *s4wave_world.WorldMutation
	}{
		{"rename settings away", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_RenameObject{
			RenameObject: &s4wave_world.RenameObjectRequest{OldObjectKey: settingsKey, NewObjectKey: "test/moved"},
		}}},
		{"rename onto settings", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_RenameObject{
			RenameObject: &s4wave_world.RenameObjectRequest{OldObjectKey: plainKey, NewObjectKey: settingsKey},
		}}},
		{"delete manifest edge", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_DeleteGraphQuad{
			DeleteGraphQuad: &s4wave_world.DeleteGraphQuadRequest{Quad: world.GraphQuadToQuad(manifestEdge)},
		}}},
		{"delete manifest store graph", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_DeleteGraphObject{
			DeleteGraphObject: &s4wave_world.DeleteGraphObjectRequest{ObjectKey: storeKey},
		}}},
		{"set root of settings", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_SetRootRef{
			SetRootRef: &s4wave_world.SetRootRefMutation{ObjectKey: settingsKey, RootRef: &bucket.ObjectRef{}},
		}}},
		{"object op set-settings on settings", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_ApplyObjectOp{
			ApplyObjectOp: &s4wave_world.ApplyObjectOpMutation{ObjectKey: settingsKey, OpTypeId: settingsOp.TypeID(), OpData: settingsOpData},
		}}},
		{"increment rev of manifest store", &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_IncrementRev{
			IncrementRev: &s4wave_world.IncrementRevMutation{ObjectKey: storeKey},
		}}},
	}
	for _, c := range batchRefused {
		guard.reset()
		batch, err := engineRes.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{Mutations: []*s4wave_world.WorldMutation{
			{Mutation: &s4wave_world.WorldMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "test/batch-created"}}},
			c.entry,
		}})
		if err != nil {
			t.Fatalf("batch %s: ExecuteWorldBatch: %v", c.name, err)
		}
		if batch.GetFailedIndex() != 1 || batch.GetRejectionCode() != space_world_optypes.PluginWriteRefusedCode {
			t.Errorf("batch %s: refusal = index %d code %q error code %v, want index 1 code %q",
				c.name, batch.GetFailedIndex(), batch.GetRejectionCode(), batch.GetErrorCode(), space_world_optypes.PluginWriteRefusedCode)
		}
		assertWorldObjectMissing(t, ctx, tb.Engine, "test/batch-created")
		t.Logf("batch refused %s: %d guard checks, %d guard lookups", c.name, guard.checks, guard.lookups)
	}

	// The engine a guarded mount hands onward, as to a typed object factory,
	// opens only guarded transactions.
	etx, err := engineRes.GetEngine().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	settingsObj, err := world.MustGetObject(ctx, etx, settingsKey)
	if err == nil {
		_, err = settingsObj.SetRootRef(ctx, &bucket.ObjectRef{})
		world.ReleaseObjectState(settingsObj)
	}
	etx.Discard()
	var engineRejection *world.OperationRejection
	if !errors.As(err, &engineRejection) || engineRejection.Code != space_world_optypes.PluginWriteRefusedCode {
		t.Errorf("engine transaction set root of settings: error = %v, want rejection %q", err, space_world_optypes.PluginWriteRefusedCode)
	}
	err = cursorTombstone(engineRes.GetEngine().BuildStorageCursor)
	if !errors.As(err, &engineRejection) || engineRejection.Code != space_world_optypes.PluginWriteRefusedCode {
		t.Errorf("engine storage cursor tombstone: error = %v, want rejection %q", err, space_world_optypes.PluginWriteRefusedCode)
	}

	// A plugin op handler refused mid-apply, which swallows the refusal and
	// reports success, still fails its op and leaves none of its writes.
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, &testWorldOpHandler{
		sender:         sender,
		refuseMidApply: true,
		objectKey:      settingsKey,
	}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))
	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()
	lookup := lookupPluginOp(ctx, t, tb, le)
	res := resource_world.NewWorldStateResource(le, tb.Bus, ws, lookup, resource_world.WithSessionPeerID(sender))
	resp, err := res.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{OpTypeId: "test/plugin-op", OpData: []byte("op-data")})
	if err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	if resp.GetRejectionCode() != space_world_optypes.PluginWriteRefusedCode || resp.GetSysErr() {
		t.Fatalf("mid-apply refusal = code %q sysErr %v, want code %q", resp.GetRejectionCode(), resp.GetSysErr(), space_world_optypes.PluginWriteRefusedCode)
	}
	assertWorldObjectMissing(t, ctx, tb.Engine, "test/world-op-created")

	// Applied by a plugin through its engine handle, as a Forge exec holds
	// one, the op is refused too: no handle a plugin holds reaches record mode.
	pluginOp, err := lookup(ctx, "test/plugin-op")
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginOp.UnmarshalVT([]byte("op-data")); err != nil {
		t.Fatal(err)
	}
	etx, err = engineRes.GetEngine().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = etx.ApplyWorldOp(ctx, pluginOp, sender)
	etx.Discard()
	if !errors.As(err, &engineRejection) || engineRejection.Code != space_world_optypes.PluginWriteRefusedCode {
		t.Errorf("engine transaction mid-apply refusal: error = %v, want rejection %q", err, space_world_optypes.PluginWriteRefusedCode)
	}
	assertWorldObjectMissing(t, ctx, tb.Engine, "test/world-op-created")

	// Applied by the session onto the settings object, the plugin's object op
	// handler writes as its plugin too: its revision bump, swallowed by the
	// handler, is refused and fails the op.
	var objSysErr bool
	err = func() error {
		obj, err := world.MustGetObject(ctx, ws, settingsKey)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		_, objSysErr, err = obj.ApplyObjectOp(ctx, pluginOp, sender)
		return err
	}()
	if !errors.As(err, &engineRejection) || engineRejection.Code != space_world_optypes.PluginWriteRefusedCode || objSysErr {
		t.Errorf("session object op on settings: sysErr %v error = %v, want rejection %q", objSysErr, err, space_world_optypes.PluginWriteRefusedCode)
	}

	// Replayed from the Space's log, the op changes the World exactly as an
	// unguarded handler does: record mode lets the refused edge through and
	// counts it. Both runs discard their transaction, so neither sees the other.
	bridgeOp, ok := pluginOp.(*bridgeOperation)
	if !ok {
		t.Fatalf("plugin op is %T, want *bridgeOperation", pluginOp)
	}
	replayRun := func(guard world.WriteGuard, record bool) (string, int) {
		hguard := newHandlerWriteGuard(le, guard, "test/plugin-op", record)
		rtx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer rtx.Discard()
		if _, err := bridgeOp.applyWorldOp(ctx, rtx, sender, hguard); err != nil {
			t.Fatalf("apply record=%v: %v", record, err)
		}
		return snapshotProtectedObjects(ctx, t, rtx, "test/world-op-created", settingsKey), hguard.getRecorded()
	}
	unguarded, _ := replayRun(allowAllGuard{}, false)
	replayed, recorded := replayRun(space_world_optypes.NewPluginWriteGuard("test-plugin"), true)
	if replayed != unguarded {
		t.Fatalf("replayed op changed the World differently:\nreplayed  %s\nunguarded %s", replayed, unguarded)
	}
	if !strings.Contains(replayed, "<test/link>") || recorded != 1 {
		t.Fatalf("replay recorded %d refused writes, World %s; want the edge onto settings, recorded once", recorded, replayed)
	}

	// Replayed, the object op on settings records its revision bump instead.
	func() {
		hguard := newHandlerWriteGuard(le, space_world_optypes.NewPluginWriteGuard("test-plugin"), "test/plugin-op", true)
		rtx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer rtx.Discard()
		obj, err := world.MustGetObject(ctx, rtx, settingsKey)
		if err != nil {
			t.Fatal(err)
		}
		defer world.ReleaseObjectState(obj)
		if _, err := bridgeOp.applyWorldObjectOp(ctx, world.GuardObjectState(obj, rtx, hguard), sender, hguard); err != nil {
			t.Fatalf("replay object op on settings: %v", err)
		}
		if got := hguard.getRecorded(); got != 1 {
			t.Fatalf("replay object op on settings recorded %d refused writes, want 1", got)
		}
	}()

	// A guard that fails to decide fails the op as a system error at
	// admission, never as a final rejection; on replay it is swallowed.
	func() {
		rtx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer rtx.Discard()
		var rejection *world.OperationRejection
		sysErr, err := bridgeOp.applyWorldOp(ctx, rtx, sender, newHandlerWriteGuard(le, failingGuard{}, "test/plugin-op", false))
		if err == nil || !sysErr || errors.As(err, &rejection) {
			t.Fatalf("guard read error at admission: sysErr %v error = %v, want a system error", sysErr, err)
		}
		_, sysErr, err = world.GuardWorldState(rtx, failingGuard{}).ApplyWorldOp(ctx, settingsOp, sender)
		if err == nil || !sysErr || errors.As(err, &rejection) {
			t.Fatalf("guard read error on a guarded op: sysErr %v error = %v, want a system error", sysErr, err)
		}
	}()
	replayRun(failingGuard{}, true)

	// The record log names a quad's object keys and never a literal value.
	logged := recordedWriteFields(&world.GuardedWrite{
		Quad: world.NewGraphQuad("<test/world-op-created>", "<test/link>", "\"secret value\"", ""),
	})["object-keys"]
	if keys, _ := logged.([]string); !slices.Equal(keys, []string{"test/world-op-created"}) {
		t.Fatalf("recorded quad logs keys %#v, want only the subject key", logged)
	}

	// The settings object, the manifest store, the manifest and their edges are
	// unchanged.
	after := snapshotProtectedObjects(ctx, t, ws, settingsKey, storeKey, manifestKey, plainKey)
	if before != after {
		t.Fatalf("protected objects changed:\nbefore %s\nafter  %s", before, after)
	}

	// Ordinary plugin writes still succeed, at one guard lookup per object they
	// name and per object a graph edge touches.
	allowed := []struct {
		name    string
		lookups int
		write   func() error
	}{
		{"create object", 1, func() error {
			obj, err := pws.CreateObject(ctx, "test/plugin-ok", nil)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"set root", 1, func() error {
			return objectWrite("test/plugin-ok", func(obj world.ObjectState) error {
				_, err := obj.SetRootRef(ctx, &bucket.ObjectRef{})
				return err
			})
		}},
		{"graph edge", 2, func() error {
			return pws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(plainKey, "<test/link>", "test/plugin-ok", ""))
		}},
		{"op init-unixfs", 2, func() error { return applyOp(space_world_ops.NewInitUnixFSOp("test/plugin-fs", ts)) }},
		{"op fs-rename", 1, func() error {
			// The Space's own handle makes the source file the plugin renames.
			w := unixfs_world.NewFSWriter(ws, "test/plugin-fs", unixfs_world.FSType_FSType_FS_NODE, sender)
			if err := w.Mknod(ctx, [][]string{{"a"}}, unixfs.NewFSCursorNodeType_File(), 0o644, ts); err != nil {
				return err
			}
			return applyOp(unixfs_world.NewFsRenameOp("test/plugin-fs", unixfs_world.FSType_FSType_FS_NODE, fsPath("a"), fsPath("b"), ts))
		}},
		{"put a block through a transaction's cursor", 0, func() error {
			return guardedTxCursor(func(cursor *bucket_lookup.Cursor) error {
				_, _, err := cursor.PutBlock(ctx, []byte("plugin block"), nil)
				return err
			})
		}},
		{"op fs-copy", 1, func() error {
			return applyOp(unixfs_world.NewFsCopyOp("test/plugin-fs", unixfs_world.FSType_FSType_FS_NODE, fsPath("b"), fsPath("c"), ts))
		}},
	}
	for _, c := range allowed {
		guard.reset()
		if err := c.write(); err != nil {
			t.Fatalf("allowed %s: %v", c.name, err)
		}
		if guard.lookups != c.lookups {
			t.Errorf("allowed %s: %d guard lookups, want %d", c.name, guard.lookups, c.lookups)
		}
		t.Logf("allowed %s: %d guard checks, %d guard lookups", c.name, guard.checks, guard.lookups)
	}
}

// snapshotProtectedObjects describes the root, revision and outgoing edges of
// each object at keys.
func snapshotProtectedObjects(ctx context.Context, t *testing.T, ws world.WorldState, keys ...string) string {
	t.Helper()
	var out string
	for _, key := range keys {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			t.Fatalf("object %q: %v", key, err)
		}
		ref, rev, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatalf("GetRootRef(%q): %v", key, err)
		}
		refData, err := ref.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, "", "", ""), 100)
		if err != nil {
			t.Fatalf("LookupGraphQuads(%q): %v", key, err)
		}
		out += fmt.Sprintf("%s rev=%d root=%x edges=", key, rev, refData)
		for _, q := range quads {
			out += q.GetPredicate() + q.GetObj() + ","
		}
		out += "; "
	}
	return out
}

// lookupCountingGuard counts the checks of guard and the graph lookups it
// makes for them.
type lookupCountingGuard struct {
	guard   world.WriteGuard
	checks  int
	lookups int
}

// CheckWrite counts one check and the lookups guard makes for it.
func (g *lookupCountingGuard) CheckWrite(ctx context.Context, ws world.WorldState, w *world.GuardedWrite) error {
	g.checks++
	return g.guard.CheckWrite(ctx, &lookupCountingWorldState{WorldState: ws, lookups: &g.lookups}, w)
}

// reset zeroes the counts.
func (g *lookupCountingGuard) reset() {
	g.checks, g.lookups = 0, 0
}

// lookupCountingWorldState counts graph lookups and hides the metadata batch
// reader, so each object type read is one counted lookup.
type lookupCountingWorldState struct {
	world.WorldState
	lookups *int
}

// LookupGraphQuads counts the lookup and forwards it.
func (s *lookupCountingWorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	*s.lookups++
	return s.WorldState.LookupGraphQuads(ctx, filter, limit)
}

// lookupPluginOp registers test/plugin-op for test-plugin, attaches the
// registry bridge to the bus of tb, and returns the lookup it resolves.
func lookupPluginOp(ctx context.Context, t *testing.T, tb *world_testbed.Testbed, le *logrus.Entry, opts ...BridgeOption) world.LookupOp {
	t.Helper()
	registry := NewWorldOpRegistryResource(nil)
	registry.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "test/plugin-op",
		RegistrationId:  1,
		PluginId:        "test-plugin",
	}
	rel, err := tb.Bus.AddController(ctx, NewWorldOpRegistryBridgeController(le, tb.Bus, registry, opts...), nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	t.Cleanup(rel)
	vs, _, ref, err := world.ExLookupWorldOp(ctx, tb.Bus, le, "test/plugin-op", tb.EngineID)
	if err != nil {
		t.Fatalf("ExLookupWorldOp: %v", err)
	}
	t.Cleanup(ref.Release)
	return vs[0]
}

func assertWorldObjectExists(t *testing.T, ctx context.Context, engine world.Engine, key string) {
	// Read the committed World and require the expected object to exist.
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	{
		objectState, err := world.MustGetObject(ctx, readTx, key)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatalf("object %q was not committed through attached world state: %v", key, err)
		}
	}
}

func assertWorldObjectMissing(t *testing.T, ctx context.Context, engine world.Engine, key string) {
	// Read the committed World and require the rejected object to remain absent.
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	{
		objectState, err := world.MustGetObject(ctx, readTx, key)
		world.ReleaseObjectState(objectState)
		if err == nil {
			t.Fatalf("object %q exists after failed validation", key)
		}
	}
}

func assertObjectRevAtLeast(t *testing.T, ctx context.Context, engine world.Engine, key string, minRev uint64) {
	// Open a World read transaction for the object revision assertion.
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Acquire the committed object state for the revision assertion.
	obj, err := world.MustGetObject(ctx, readTx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("object %q was not committed: %v", key, err)
	}

	// Read the committed object revision and compare it with the minimum.
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef(%q): %v", key, err)
	}
	if rev < minRev {
		t.Fatalf("object %q rev = %d, want >= %d", key, rev, minRev)
	}
}

func assertObjectRootSet(t *testing.T, ctx context.Context, engine world.Engine, key string) {
	// Read the committed object's root and require one.
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	obj, err := world.MustGetObject(ctx, readTx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("object %q was not committed: %v", key, err)
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef(%q): %v", key, err)
	}
	if ref.GetEmpty() {
		t.Fatalf("object %q has no root", key)
	}
}

type testWorldOpHandler struct {
	sender        peer.ID
	validateError string
	// refuseMidApply makes ApplyWorldOp create its object, attempt a write
	// the plugin guard refuses, ignore the refusal and report success.
	refuseMidApply bool
	// objectKey, when set, makes ApplyWorldObjectOp expect that object, bump
	// its revision, ignore any refusal and report success.
	objectKey string
}

func (h *testWorldOpHandler) ApplyWorldOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldOpRequest,
) (*s4wave_worldop_registry.ApplyWorldOpResponse, error) {
	// Require the expected World operation payload, sender, and attached state.
	if req.GetOperationTypeId() != "test/plugin-op" || string(req.GetOpData()) != "op-data" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetSender() != h.sender.String() {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedWorldStateResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}

	// Access the attached World state through the plugin Resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	worldClient, err := resourceCtx.GetAttachedResource(req.GetAttachedWorldStateResourceId())
	if err != nil {
		return nil, err
	}
	worldState := s4wave_world.NewSRPCWorldStateResourceServiceClient(worldClient)

	// Create the test object through the attached World state.
	objResp, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: "test/world-op-created",
	})
	if err != nil {
		return nil, err
	}
	if objID := objResp.GetResourceId(); objID != 0 {
		defer resourceCtx.ReleaseResource(objID)
	}

	// Attempt a refused write, an edge onto the settings object, ignore its
	// refusal and report success.
	if h.refuseMidApply {
		_, _ = worldState.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{
			Quad: &quad.Quad{
				Subject: "<test/world-op-created>", Predicate: "<test/link>", Obj: "<" + space_world.SpaceSettingsObjectKey + ">",
			},
		})
		return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
	}

	// The bridge exposes typed access alongside the operation-scoped WorldState.
	if _, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: "types/test/supplied-state",
	}); err != nil {
		return nil, err
	}
	if _, err := worldState.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{
		Quad: &quad.Quad{
			Subject: "<test/world-op-created>", Predicate: "<type>", Obj: "<types/test/supplied-state>",
		},
	}); err != nil {
		return nil, err
	}

	// Access the created object through its typed Resource service.
	typed := s4wave_world.NewSRPCTypedObjectResourceServiceClient(worldClient)
	if _, err := typed.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: "test/world-op-created",
	}); err != nil {
		return nil, err
	}
	return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
}

func (h *testWorldOpHandler) ApplyWorldObjectOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldObjectOpRequest,
) (*s4wave_worldop_registry.ApplyWorldObjectOpResponse, error) {
	// Require the expected object operation payload, target, and sender.
	if req.GetOperationTypeId() != "test/plugin-op" || string(req.GetOpData()) != "op-data" {
		return nil, resource.ErrInvalidResourceID
	}
	objectKey := h.objectKey
	if objectKey == "" {
		objectKey = "test/object-op-target"
	}
	if req.GetObjectKey() != objectKey {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetSender() != h.sender.String() {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedObjectStateResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}

	// Access the attached object state through the plugin Resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	objectClient, err := resourceCtx.GetAttachedResource(req.GetAttachedObjectStateResourceId())
	if err != nil {
		return nil, err
	}
	objectState := s4wave_world.NewSRPCObjectStateResourceServiceClient(objectClient)

	// Verify the attached object key before mutating its revision.
	keyResp, err := objectState.GetKey(ctx, &s4wave_world.GetKeyRequest{})
	if err != nil {
		return nil, err
	}
	if keyResp.GetObjectKey() != objectKey {
		return nil, resource.ErrInvalidResourceID
	}

	// Bump the named object's revision, ignore a refusal and report success.
	if h.objectKey != "" {
		_, _ = objectState.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
		return &s4wave_worldop_registry.ApplyWorldObjectOpResponse{}, nil
	}

	// Increment the attached object revision and verify it changed.
	revResp, err := objectState.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
	if err != nil {
		return nil, err
	}
	if revResp.GetRev() == 0 {
		return nil, resource.ErrInvalidResourceID
	}

	// Encode a nested file system init for the attached object.
	nestedOp := unixfs_world.NewFsInitOp(
		"test/object-op-target",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		false,
		time.Unix(1, 0),
	)
	opData, err := nestedOp.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Apply the nested operation and verify its returned revision.
	applyResp, err := objectState.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{
		OpTypeId: nestedOp.TypeID(),
		OpData:   opData,
		OpSender: "caller-chosen-invalid-peer",
	})
	if err != nil {
		return nil, err
	}
	if applyResp.GetRev() == 0 || applyResp.GetSysErr() {
		return nil, resource.ErrInvalidResourceID
	}
	return &s4wave_worldop_registry.ApplyWorldObjectOpResponse{}, nil
}

func (h *testWorldOpHandler) ValidateOp(
	_ context.Context,
	req *s4wave_worldop_registry.ValidateOpRequest,
) (*s4wave_worldop_registry.ValidateOpResponse, error) {
	if req.GetOperationTypeId() != "test/plugin-op" || string(req.GetOpData()) != "op-data" {
		return nil, resource.ErrInvalidResourceID
	}
	return &s4wave_worldop_registry.ValidateOpResponse{Error: h.validateError}, nil
}

type testWorldOpPluginLoadController struct {
	client    srpc.Client
	manifests map[string]srpc.Client
	// missing lists pinned roots the host proved have no build for it.
	missing map[string]bool
}

func (c *testWorldOpPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/worldop-plugin-load", controller.MustParseVersion("0.0.1"), "test worldop plugin load")
}

func (c *testWorldOpPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testWorldOpPluginLoadController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	// Match the requested test plugin before selecting its executable client.
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}

	// Select the pinned manifest client or the default test plugin client.
	client := c.client
	if root := dir.LoadPluginManifestRoot(); root != "" {
		if c.missing[root] {
			return directive.R(&testFailResolver{err: &bldr_plugin.PinnedManifestMissingError{PluginID: dir.LoadPluginID(), ManifestRoot: root}}, nil)
		}
		client = c.manifests[root]
	}
	if client == nil {
		return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{}), nil)
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(client)}), nil)
}

func (c *testWorldOpPluginLoadController) Close() error {
	return nil
}

// testFailResolver fails its directive with a fixed error, as the plugin host
// does when a load reaches a final state.
type testFailResolver struct {
	err error
}

// Resolve returns the fixed error.
func (r *testFailResolver) Resolve(context.Context, directive.ResolverHandler) error {
	return r.err
}

var (
	_ controller.Controller                                   = (*testWorldOpPluginLoadController)(nil)
	_ s4wave_worldop_registry.SRPCWorldOpHandlerServiceServer = (*testWorldOpHandler)(nil)
)

// stallWorldOpHandler validates every operation and never answers
// ApplyWorldOp until stall closes, ignoring its context like a stalled worker.
type stallWorldOpHandler struct {
	testWorldOpHandler
	entered chan struct{}
	stall   chan struct{}
}

func (h *stallWorldOpHandler) ApplyWorldOp(
	context.Context,
	*s4wave_worldop_registry.ApplyWorldOpRequest,
) (*s4wave_worldop_registry.ApplyWorldOpResponse, error) {
	h.entered <- struct{}{}
	<-h.stall
	return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
}

// attachedWorldState is a World state resource the bridge attached to the
// plugin for an outer operation.
type attachedWorldState struct {
	rctx resource_server.ResourceClientContext
	id   uint32
}

// budgetProbeOp records the WriteBudget its apply's context carries and
// applies nothing.
type budgetProbeOp struct {
	world.Operation
	budget *world.WriteBudget
}

// ApplyWorldOp records the context's budget.
func (o *budgetProbeOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, _ world.WorldState, _ peer.ID) (bool, error) {
	o.budget = world.GetWriteBudget(ctx)
	return false, nil
}

// nestedOpTypeID is the operation type the nested operation is applied as.
const nestedOpTypeID = "test/nested-op"

// nestedResult is the host's result of a nested operation and when it
// returned.
type nestedResult struct {
	err error
	at  time.Time
}

// recordedOp records the result of each apply of the bridge operation it
// wraps, keeping the operation's other methods, such as its write keys.
type recordedOp struct {
	*bridgeOperation
	results chan<- nestedResult
}

// ApplyWorldOp applies the operation and records its result.
func (o *recordedOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	sysErr, err := o.bridgeOperation.ApplyWorldOp(ctx, le, ws, sender)
	o.results <- nestedResult{err: err, at: time.Now()}
	return sysErr, err
}

// nestedSleepWorldOpHandler applies a nested plugin operation for the "outer"
// payload through its attached World state, and sleeps for the "nested" one
// without watching its context, like a stalled worker. It applies the nested
// operation on a context the outer call's end does not cancel, so only the
// host can end it.
type nestedSleepWorldOpHandler struct {
	testWorldOpHandler
	sleep    time.Duration
	attached chan attachedWorldState
}

func (h *nestedSleepWorldOpHandler) ValidateOp(
	context.Context,
	*s4wave_worldop_registry.ValidateOpRequest,
) (*s4wave_worldop_registry.ValidateOpResponse, error) {
	return &s4wave_worldop_registry.ValidateOpResponse{}, nil
}

func (h *nestedSleepWorldOpHandler) ApplyWorldOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldOpRequest,
) (*s4wave_worldop_registry.ApplyWorldOpResponse, error) {
	// Sleep through the nested operation.
	if string(req.GetOpData()) == "nested" {
		time.Sleep(h.sleep)
		return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
	}

	// Record the attached World state, then apply the nested operation.
	rctx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	id := req.GetAttachedWorldStateResourceId()
	h.attached <- attachedWorldState{rctx: rctx, id: id}
	worldClient, err := rctx.GetAttachedResource(id)
	if err != nil {
		return nil, err
	}
	resp, err := s4wave_world.NewSRPCWorldStateResourceServiceClient(worldClient).ApplyWorldOp(
		context.WithoutCancel(ctx),
		&s4wave_world.ApplyWorldOpRequest{OpTypeId: nestedOpTypeID, OpData: []byte("nested")},
	)
	if err != nil {
		return nil, err
	}
	if code := resp.GetErrorCode(); code != 0 {
		return nil, errors.New("nested operation failed: " + code.String())
	}
	return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
}

// waitDetached waits for the bridge's detach of a to reach the plugin. The
// detach is sent without an acknowledgment, so it checks the plugin's view.
func waitDetached(t *testing.T, a attachedWorldState) {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	for {
		if _, err := a.rctx.GetAttachedResource(a.id); errors.Is(err, resource.ErrResourceNotFound) {
			return
		}
		select {
		case <-tick.C:
		case <-timeout:
			t.Fatalf("attached World state %d was not detached", a.id)
		}
	}
}

// hangPluginLoadController resolves the load of test-plugin with a resolver
// that never yields the running plugin, like a cold load that hangs. It
// signals loading when the load starts.
type hangPluginLoadController struct {
	loading chan struct{}
}

func (c *hangPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/worldop-plugin-hang", controller.MustParseVersion("0.0.1"), "test worldop plugin load that never completes")
}

func (c *hangPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *hangPluginLoadController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		select {
		case c.loading <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return context.Cause(ctx)
	}), nil)
}

func (c *hangPluginLoadController) Close() error {
	return nil
}

// failingGuard fails every check without deciding, as a guard whose read
// failed does.
type failingGuard struct{}

// CheckWrite fails to decide w.
func (failingGuard) CheckWrite(context.Context, world.WorldState, *world.GuardedWrite) error {
	return errors.New("guard read failed")
}

// allowAllGuard allows every write, as a handle with no guard does.
type allowAllGuard struct{}

// CheckWrite allows w.
func (allowAllGuard) CheckWrite(context.Context, world.WorldState, *world.GuardedWrite) error {
	return nil
}
