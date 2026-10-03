package resource_worldop_registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
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
	if err := op.UnmarshalBlock([]byte("op-data")); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
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
	sysErr, err = op.ApplyWorldObjectOp(ctx, le, obj, sender)
	if err != nil {
		objectTx.Discard()
		t.Fatalf("ApplyWorldObjectOp: %v", err)
	}
	if sysErr {
		objectTx.Discard()
		t.Fatal("ApplyWorldObjectOp returned system error")
	}

	// Commit the object mutation and verify its revision and settings.
	if err := objectTx.Commit(ctx); err != nil {
		t.Fatalf("Commit object op tx: %v", err)
	}
	assertObjectRevAtLeast(t, ctx, tb.Engine, "test/object-op-target", 1)
	assertObjectSettingsIndexPath(t, ctx, tb.Engine, "test/object-op-target", "/object-op")
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
	if err := op.UnmarshalBlock([]byte("op-data")); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
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

func assertObjectSettingsIndexPath(t *testing.T, ctx context.Context, engine world.Engine, key, want string) {
	// Open a World read transaction for the object settings assertion.
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Read the committed settings and compare their index path.
	settings, objectState, err := world.LookupObject[*space_world.SpaceSettings](
		ctx,
		readTx,
		key,
		space_world.NewSpaceSettingsBlock,
	)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatalf("LookupObject(%q): %v", key, err)
	}
	if got := settings.GetIndexPath(); got != want {
		t.Fatalf("object %q settings index path = %q, want %q", key, got, want)
	}
}

type testWorldOpHandler struct {
	sender        peer.ID
	validateError string
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
	if req.GetObjectKey() != "test/object-op-target" {
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
	if keyResp.GetObjectKey() != "test/object-op-target" {
		return nil, resource.ErrInvalidResourceID
	}

	// Increment the attached object revision and verify it changed.
	revResp, err := objectState.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
	if err != nil {
		return nil, err
	}
	if revResp.GetRev() == 0 {
		return nil, resource.ErrInvalidResourceID
	}

	// Encode a nested settings operation for the attached object.
	nestedOp := space_world_ops.NewSetSpaceSettingsOp(
		"test/object-op-target",
		&space_world.SpaceSettings{IndexPath: "/object-op"},
		true,
		time.Unix(1, 0),
	)
	opData, err := nestedOp.MarshalBlock()
	if err != nil {
		return nil, err
	}

	// Apply the nested settings operation and verify its returned revision.
	applyResp, err := objectState.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{
		OpTypeId: nestedOp.GetOperationTypeId(),
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

var (
	_ controller.Controller                                   = (*testWorldOpPluginLoadController)(nil)
	_ s4wave_worldop_registry.SRPCWorldOpHandlerServiceServer = (*testWorldOpHandler)(nil)
)
