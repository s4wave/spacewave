package resource_worldop_registry

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

func TestWorldOpRegistryBridgeControllerAppliesPluginWorldAndObjectOps(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	sender := tb.Volume.GetPeerID()
	pluginRoot := srpc.NewMux()
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(pluginRoot, &testWorldOpHandler{sender: sender}); err != nil {
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

	registry := NewWorldOpRegistryResource()
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

	worldTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	sysErr, err := op.ApplyWorldOp(ctx, le, worldTx, sender)
	if err != nil {
		worldTx.Discard()
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	if sysErr {
		worldTx.Discard()
		t.Fatal("ApplyWorldOp returned system error")
	}
	if err := worldTx.Commit(ctx); err != nil {
		t.Fatalf("Commit world op tx: %v", err)
	}
	assertWorldObjectExists(t, ctx, tb.Engine, "test/world-op-created")

	objectTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := objectTx.CreateObject(ctx, "test/object-op-target", nil)
	if err != nil {
		objectTx.Discard()
		t.Fatalf("CreateObject: %v", err)
	}
	sysErr, err = op.ApplyWorldObjectOp(ctx, le, obj, sender)
	if err != nil {
		objectTx.Discard()
		t.Fatalf("ApplyWorldObjectOp: %v", err)
	}
	if sysErr {
		objectTx.Discard()
		t.Fatal("ApplyWorldObjectOp returned system error")
	}
	if err := objectTx.Commit(ctx); err != nil {
		t.Fatalf("Commit object op tx: %v", err)
	}
	assertObjectRevAtLeast(t, ctx, tb.Engine, "test/object-op-target", 1)
	assertObjectSettingsIndexPath(t, ctx, tb.Engine, "test/object-op-target", "/object-op")
}

func TestWorldOpRegistryBridgeControllerValidatesBeforeMutation(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

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

	rel, err := tb.Bus.AddController(ctx, &testWorldOpPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController plugin load: %v", err)
	}
	defer rel()

	registry := NewWorldOpRegistryResource()
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
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, err := world.MustGetObject(ctx, readTx, key); err != nil {
		t.Fatalf("object %q was not committed through attached world state: %v", key, err)
	}
}

func assertWorldObjectMissing(t *testing.T, ctx context.Context, engine world.Engine, key string) {
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, err := world.MustGetObject(ctx, readTx, key); err == nil {
		t.Fatalf("object %q exists after failed validation", key)
	}
}

func assertObjectRevAtLeast(t *testing.T, ctx context.Context, engine world.Engine, key string, minRev uint64) {
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	obj, err := world.MustGetObject(ctx, readTx, key)
	if err != nil {
		t.Fatalf("object %q was not committed: %v", key, err)
	}
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef(%q): %v", key, err)
	}
	if rev < minRev {
		t.Fatalf("object %q rev = %d, want >= %d", key, rev, minRev)
	}
}

func assertObjectSettingsIndexPath(t *testing.T, ctx context.Context, engine world.Engine, key, want string) {
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	settings, _, err := world.LookupObject[*space_world.SpaceSettings](
		ctx,
		readTx,
		key,
		space_world.NewSpaceSettingsBlock,
	)
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
	if req.GetOperationTypeId() != "test/plugin-op" || string(req.GetOpData()) != "op-data" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedWorldStateResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	worldClient, err := resourceCtx.GetAttachedResource(req.GetAttachedWorldStateResourceId())
	if err != nil {
		return nil, err
	}
	worldState := s4wave_world.NewSRPCWorldStateResourceServiceClient(worldClient)
	objResp, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: "test/world-op-created",
	})
	if err != nil {
		return nil, err
	}
	if objID := objResp.GetResourceId(); objID != 0 {
		defer resourceCtx.ReleaseResource(objID)
	}
	return &s4wave_worldop_registry.ApplyWorldOpResponse{}, nil
}

func (h *testWorldOpHandler) ApplyWorldObjectOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldObjectOpRequest,
) (*s4wave_worldop_registry.ApplyWorldObjectOpResponse, error) {
	if req.GetOperationTypeId() != "test/plugin-op" || string(req.GetOpData()) != "op-data" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetObjectKey() != "test/object-op-target" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedObjectStateResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	objectClient, err := resourceCtx.GetAttachedResource(req.GetAttachedObjectStateResourceId())
	if err != nil {
		return nil, err
	}
	objectState := s4wave_world.NewSRPCObjectStateResourceServiceClient(objectClient)
	keyResp, err := objectState.GetKey(ctx, &s4wave_world.GetKeyRequest{})
	if err != nil {
		return nil, err
	}
	if keyResp.GetObjectKey() != "test/object-op-target" {
		return nil, resource.ErrInvalidResourceID
	}
	revResp, err := objectState.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
	if err != nil {
		return nil, err
	}
	if revResp.GetRev() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
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
	applyResp, err := objectState.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{
		OpTypeId: nestedOp.GetOperationTypeId(),
		OpData:   opData,
		OpSender: h.sender.String(),
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
	client srpc.Client
}

func (c *testWorldOpPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/worldop-plugin-load", controller.MustParseVersion("0.0.1"), "test worldop plugin load")
}

func (c *testWorldOpPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testWorldOpPluginLoadController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(c.client)}), nil)
}

func (c *testWorldOpPluginLoadController) Close() error {
	return nil
}

var _ controller.Controller = (*testWorldOpPluginLoadController)(nil)
var _ s4wave_worldop_registry.SRPCWorldOpHandlerServiceServer = (*testWorldOpHandler)(nil)
