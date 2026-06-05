package resource_quickstart_registry

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	space_core "github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

func TestExecuteQuickstartPassesAttachedEngineResourceToPlugin(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	pluginRoot := srpc.NewMux()
	if err := s4wave_quickstart_registry.SRPCRegisterQuickstartHandlerService(pluginRoot, &testQuickstartHandler{
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
		sender:   tb.Volume.GetPeerID().String(),
	}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))

	rel, err := tb.Bus.AddController(ctx, &testQuickstartPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()

	registry := NewQuickstartRegistryResource(le, tb.Bus)
	registry.registrations[1] = &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId:      "notes-blog",
		RegistrationId:    1,
		PluginId:          "test-plugin",
		Name:              "Blog",
		Description:       "Blog workspace",
		Category:          "documents",
		RequiredPluginIds: []string{"required-plugin", "plugin-result"},
	}

	const spaceResourceID uint32 = 7
	spaceResource := resource_space.NewSpaceResource(le, tb.Bus, &testQuickstartSpaceBody{
		engine:   tb.Engine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	})
	resourceCtx := &testQuickstartResourceClientContext{
		ctx:    ctx,
		values: map[uint32]any{spaceResourceID: spaceResource},
	}

	resp, err := registry.ExecuteQuickstart(
		resource_server.WithResourceClientContext(ctx, resourceCtx),
		&s4wave_quickstart_registry.ExecuteQuickstartRequest{
			QuickstartId:    "notes-blog",
			SpaceResourceId: spaceResourceID,
		},
	)
	if err != nil {
		t.Fatalf("ExecuteQuickstart: %v", err)
	}
	if got := resp.GetIndexPath(); got != "/seeded/index" {
		t.Fatalf("IndexPath = %q, want /seeded/index", got)
	}
	wantPlugins := []string{"required-plugin", "plugin-result"}
	if got := resp.GetPluginIds(); len(got) != len(wantPlugins) || got[0] != wantPlugins[0] || got[1] != wantPlugins[1] {
		t.Fatalf("PluginIds = %v, want %v", got, wantPlugins)
	}

	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, err := world.MustGetObject(ctx, readTx, "quickstart/seeded-object"); err != nil {
		t.Fatalf("seeded object was not committed through attached engine: %v", err)
	}
	settings, _, err := world.LookupObject[*space_world.SpaceSettings](
		ctx,
		readTx,
		"quickstart/settings",
		space_world.NewSpaceSettingsBlock,
	)
	if err != nil {
		t.Fatalf("recursive built-in world op did not commit settings: %v", err)
	}
	if got := settings.GetIndexPath(); got != "/quickstart-recursive" {
		t.Fatalf("settings index path = %q, want /quickstart-recursive", got)
	}
}

type testQuickstartHandler struct {
	engineID string
	bucketID string
	sender   string
}

func (h *testQuickstartHandler) SeedQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.SeedQuickstartRequest,
) (*s4wave_quickstart_registry.SeedQuickstartResponse, error) {
	if req.GetQuickstartId() != "notes-blog" {
		return nil, resource.ErrInvalidResourceID
	}
	attachedEngineID := req.GetAttachedEngineResourceId()
	if attachedEngineID == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineClient, err := resourceCtx.GetAttachedResource(attachedEngineID)
	if err != nil {
		return nil, err
	}
	engine := s4wave_world.NewSRPCEngineResourceServiceClient(engineClient)
	info, err := engine.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
	if err != nil {
		return nil, err
	}
	if info.GetEngineInfo().GetEngineId() != h.engineID {
		return nil, resource.ErrInvalidResourceID
	}
	if info.GetEngineInfo().GetBucketId() != h.bucketID {
		return nil, resource.ErrInvalidResourceID
	}
	txResp, err := engine.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		return nil, err
	}
	txID := txResp.GetResourceId()
	if txID == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	defer resourceCtx.ReleaseResource(txID)

	txClient, err := resourceCtx.GetAttachedResource(txID)
	if err != nil {
		return nil, err
	}
	worldState := s4wave_world.NewSRPCWorldStateResourceServiceClient(txClient)
	objResp, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: "quickstart/seeded-object",
	})
	if err != nil {
		return nil, err
	}
	if objID := objResp.GetResourceId(); objID != 0 {
		defer resourceCtx.ReleaseResource(objID)
	}
	settingsOp := space_world_ops.NewSetSpaceSettingsOp(
		"quickstart/settings",
		&space_world.SpaceSettings{IndexPath: "/quickstart-recursive"},
		true,
		time.Unix(1, 0),
	)
	settingsData, err := settingsOp.MarshalBlock()
	if err != nil {
		return nil, err
	}
	applyResp, err := worldState.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{
		OpTypeId: settingsOp.GetOperationTypeId(),
		OpData:   settingsData,
		OpSender: h.sender,
	})
	if err != nil {
		return nil, err
	}
	if applyResp.GetSeqno() == 0 || applyResp.GetSysErr() {
		return nil, resource.ErrInvalidResourceID
	}
	tx := s4wave_world.NewSRPCTxResourceServiceClient(txClient)
	if _, err := tx.Commit(ctx, &s4wave_world.CommitRequest{}); err != nil {
		return nil, err
	}
	return &s4wave_quickstart_registry.SeedQuickstartResponse{
		IndexPath: "/seeded/index",
		PluginIds: []string{"plugin-result"},
	}, nil
}

type testQuickstartPluginLoadController struct {
	client srpc.Client
}

func (c *testQuickstartPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/quickstart-plugin-load", controller.MustParseVersion("0.0.1"), "test quickstart plugin load")
}

func (c *testQuickstartPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testQuickstartPluginLoadController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(c.client)}), nil)
}

func (c *testQuickstartPluginLoadController) Close() error {
	return nil
}

type testQuickstartSpaceBody struct {
	engine   world.Engine
	engineID string
	bucketID string
}

func (b *testQuickstartSpaceBody) GetWorldEngine() world.Engine {
	return b.engine
}

func (b *testQuickstartSpaceBody) GetWorldEngineID() string {
	return b.engineID
}

func (b *testQuickstartSpaceBody) GetWorldEngineBucketID() string {
	return b.bucketID
}

func (b *testQuickstartSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return nil
}

func (b *testQuickstartSpaceBody) GetSharedObject() sobject.SharedObject {
	return nil
}

type testQuickstartResourceClientContext struct {
	ctx      context.Context
	nextID   uint32
	values   map[uint32]any
	releases map[uint32]func()
}

func (c *testQuickstartResourceClientContext) Context() context.Context {
	return c.ctx
}

func (c *testQuickstartResourceClientContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *testQuickstartResourceClientContext) AddResourceValue(_ srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.nextID++
	if c.values == nil {
		c.values = make(map[uint32]any)
	}
	if c.releases == nil {
		c.releases = make(map[uint32]func())
	}
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *testQuickstartResourceClientContext) ReleaseResource(resourceID uint32) bool {
	releaseFn, ok := c.releases[resourceID]
	if !ok {
		return false
	}
	delete(c.releases, resourceID)
	delete(c.values, resourceID)
	if releaseFn != nil {
		releaseFn()
	}
	return true
}

func (c *testQuickstartResourceClientContext) GetResourceValue(resourceID uint32) (any, error) {
	value, ok := c.values[resourceID]
	if !ok {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *testQuickstartResourceClientContext) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

var _ controller.Controller = (*testQuickstartPluginLoadController)(nil)
var _ space_core.SpaceSharedObjectBody = (*testQuickstartSpaceBody)(nil)
var _ s4wave_quickstart_registry.SRPCQuickstartHandlerServiceServer = (*testQuickstartHandler)(nil)
var _ resource_server.ResourceClientContext = (*testQuickstartResourceClientContext)(nil)
