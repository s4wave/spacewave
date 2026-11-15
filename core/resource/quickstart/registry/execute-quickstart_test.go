package resource_quickstart_registry

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	space_core "github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

func TestExecuteQuickstartPassesAttachedEngineResourceToPlugin(t *testing.T) {
	// Open a world testbed for the mounted Space engine.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Expose the test Quickstart handler through a plugin resource server.
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

	// Register the plugin loader with the test bus.
	rel, err := tb.Bus.AddController(ctx, &testQuickstartPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()

	// Register the test Quickstart with its required plugins.
	registry := NewQuickstartRegistryResource(le, tb.Bus, nil)
	registry.registrations[1] = &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId:      "notes-blog",
		RegistrationId:    1,
		PluginId:          "test-plugin",
		Name:              "Blog",
		Description:       "Blog workspace",
		Category:          "documents",
		RequiredPluginIds: []string{"required-plugin", "plugin-result"},
	}

	// Expose the mounted Space to the calling resource client.
	const spaceResourceID uint32 = 7
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerID(le, tb.Bus, &testQuickstartSpaceBody{
		engine:   tb.Engine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}, tb.Volume.GetPeerID().String())
	resourceCtx := &testQuickstartResourceClientContext{
		ctx:    ctx,
		values: map[uint32]any{spaceResourceID: spaceResource},
	}

	// Execute the registered Quickstart against the mounted Space.
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

	// Verify the seed result contains its index and merged plugin list.
	if got := resp.GetIndexPath(); got != "/seeded/index" {
		t.Fatalf("IndexPath = %q, want /seeded/index", got)
	}
	wantPlugins := []string{"required-plugin", "plugin-result"}
	if got := resp.GetPluginIds(); len(got) != len(wantPlugins) || got[0] != wantPlugins[0] || got[1] != wantPlugins[1] {
		t.Fatalf("PluginIds = %v, want %v", got, wantPlugins)
	}

	// Open a read transaction to inspect the seeded World objects.
	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Verify the attached engine committed the seeded object.
	{
		objectState, err := world.MustGetObject(ctx, readTx, "quickstart/seeded-object")
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatalf("seeded object was not committed through attached engine: %v", err)
		}
	}

	// Verify the recursive built-in op committed its filesystem and the
	// refused settings op left no settings object.
	{
		objectState, err := world.MustGetObject(ctx, readTx, "quickstart/files")
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatalf("recursive built-in world op did not commit its filesystem: %v", err)
		}
		settings, found, err := readTx.GetObject(ctx, space_world.SpaceSettingsObjectKey)
		world.ReleaseObjectState(settings)
		if err != nil || found {
			t.Fatalf("refused settings op left settings: found=%v err=%v", found, err)
		}
		partial, found, err := readTx.GetObject(ctx, "quickstart/partial")
		world.ReleaseObjectState(partial)
		if err != nil || found {
			t.Fatalf("transaction with a refused op kept its write: found=%v err=%v", found, err)
		}
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
	// Require the test Quickstart and its attached engine resource.
	if req.GetQuickstartId() != "notes-blog" {
		return nil, resource.ErrInvalidResourceID
	}
	attachedEngineID := req.GetAttachedEngineResourceId()
	if attachedEngineID == 0 {
		return nil, resource.ErrInvalidResourceID
	}

	// Resolve the attached engine client from the plugin resource context.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineClient, err := resourceCtx.GetAttachedResource(attachedEngineID)
	if err != nil {
		return nil, err
	}

	// Read the attached engine information.
	engine := s4wave_world.NewSRPCEngineResourceServiceClient(engineClient)
	info, err := engine.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
	if err != nil {
		return nil, err
	}

	// Verify the engine, bucket and session peer match the mounted Space.
	if info.GetEngineInfo().GetEngineId() != h.engineID {
		return nil, resource.ErrInvalidResourceID
	}
	if info.GetEngineInfo().GetBucketId() != h.bucketID {
		return nil, resource.ErrInvalidResourceID
	}
	if info.GetSessionPeerId() != h.sender {
		return nil, resource.ErrInvalidResourceID
	}

	// Open a writable engine transaction and retain its resource.
	txResp, err := engine.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		return nil, err
	}
	txID := txResp.GetResourceId()
	if txID == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	defer resourceCtx.ReleaseResource(txID)

	// Create the seeded World object through the attached transaction.
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

	// applyOp applies a built-in World op to the attached transaction.
	applyOp := func(op world.Operation) (*s4wave_world.ApplyWorldOpResponse, error) {
		data, err := op.MarshalVT()
		if err != nil {
			return nil, err
		}
		return worldState.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{
			OpTypeId: op.TypeID(),
			OpData:   data,
			OpSender: h.sender,
		})
	}

	// The engine writes as this plugin, so the Space settings stay refused.
	settingsResp, err := applyOp(space_world_ops.NewSetSpaceSettingsOp(
		space_world.SpaceSettingsObjectKey,
		&space_world.SpaceSettings{IndexPath: "/quickstart-recursive"},
		true,
		time.Unix(1, 0),
	))
	if err != nil {
		return nil, err
	}
	if settingsResp.GetRejectionCode() != space_world_optypes.PluginWriteRefusedCode {
		return nil, errors.Errorf("settings op response %v, want refusal %s", settingsResp, space_world_optypes.PluginWriteRefusedCode)
	}

	// A built-in op that writes no protected object applies.
	fsResp, err := applyOp(space_world_ops.NewInitUnixFSOp("quickstart/files", time.Unix(1, 0)))
	if err != nil {
		return nil, err
	}
	if fsResp.GetSeqno() == 0 || fsResp.GetSysErr() {
		return nil, errors.Errorf("init-unixfs op response %v", fsResp)
	}

	// Commit the seeded World objects before returning the plugin result.
	tx := s4wave_world.NewSRPCTxResourceServiceClient(txClient)
	if err := s4wave_world.CommitRemote(ctx, tx); err != nil {
		return nil, err
	}

	// An op whose type is allowed but which writes the settings is refused
	// as it writes, and discards the transaction holding an earlier write.
	if err := h.seedPartial(ctx, resourceCtx, engine); err != nil {
		return nil, err
	}
	return &s4wave_quickstart_registry.SeedQuickstartResponse{
		IndexPath: "/seeded/index",
		PluginIds: []string{"plugin-result"},
	}, nil
}

// seedPartial writes an object in a new transaction, applies an op that writes
// the Space settings, and requires the refusal to leave nothing to commit.
func (h *testQuickstartHandler) seedPartial(
	ctx context.Context,
	resourceCtx resource_server.ResourceClientContext,
	engine s4wave_world.SRPCEngineResourceServiceClient,
) error {
	txResp, err := engine.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		return err
	}
	txID := txResp.GetResourceId()
	defer resourceCtx.ReleaseResource(txID)
	txClient, err := resourceCtx.GetAttachedResource(txID)
	if err != nil {
		return err
	}
	worldState := s4wave_world.NewSRPCWorldStateResourceServiceClient(txClient)
	objResp, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{ObjectKey: "quickstart/partial"})
	if err != nil {
		return err
	}
	if objID := objResp.GetResourceId(); objID != 0 {
		defer resourceCtx.ReleaseResource(objID)
	}
	op := unixfs_world.NewFsInitOp(space_world.SpaceSettingsObjectKey, unixfs_world.FSType_FSType_FS_NODE, nil, true, time.Unix(1, 0))
	data, err := op.MarshalVT()
	if err != nil {
		return err
	}
	resp, err := worldState.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{OpTypeId: op.TypeID(), OpData: data, OpSender: h.sender})
	if err != nil {
		return err
	}
	if resp.GetRejectionCode() != space_world_optypes.PluginWriteRefusedCode {
		return errors.Errorf("fs-init of settings response %v, want refusal %s", resp, space_world_optypes.PluginWriteRefusedCode)
	}
	tx := s4wave_world.NewSRPCTxResourceServiceClient(txClient)
	if err := s4wave_world.CommitRemote(ctx, tx); err == nil {
		return errors.New("transaction committed after a refused op")
	}
	return nil
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
	// Allocate a test resource identity and initialize its retained maps.
	c.nextID++
	if c.values == nil {
		c.values = make(map[uint32]any)
	}
	if c.releases == nil {
		c.releases = make(map[uint32]func())
	}

	// Retain the test resource value and release callback.
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *testQuickstartResourceClientContext) ReleaseResource(resourceID uint32) bool {
	// Resolve the release callback for the retained test resource.
	releaseFn, ok := c.releases[resourceID]
	if !ok {
		return false
	}

	// Remove the test resource and invoke its release callback.
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

var (
	_ controller.Controller                                         = (*testQuickstartPluginLoadController)(nil)
	_ space_core.SpaceSharedObjectBody                              = (*testQuickstartSpaceBody)(nil)
	_ s4wave_quickstart_registry.SRPCQuickstartHandlerServiceServer = (*testQuickstartHandler)(nil)
	_ resource_server.ResourceClientContext                         = (*testQuickstartResourceClientContext)(nil)
)
