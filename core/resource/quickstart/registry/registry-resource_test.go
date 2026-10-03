package resource_quickstart_registry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
)

func setupQuickstartRegistryClient(t *testing.T) (context.Context, *resource_client.Client, *QuickstartRegistryResource) {
	// Create the quickstart registry and its in-memory RPC connection.
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := NewQuickstartRegistryResource(nil, nil, nil)
	clientPipe, serverPipe := net.Pipe()

	// Connect the quickstart RPC client to the client pipe.
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Register the quickstart resource service on the server mux.
	resourceSrv := resource_server.NewResourceServer(r.GetMux())
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err.Error())
	}

	// Serve the quickstart registry over the server pipe.
	server := srpc.NewServer(serverMux)
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	go func() {
		if err := server.AcceptMuxedConn(ctx, serverMp); err != nil && ctx.Err() == nil {
			panic(err)
		}
	}()

	// Create the resource client for quickstart RPC calls.
	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	client, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Release the quickstart client and pipes when the test ends.
	t.Cleanup(func() {
		client.Release()
		cancel()
		clientPipe.Close()
		serverPipe.Close()
	})

	return ctx, client, r
}

type testQuickstartResourceClient struct {
	ctx        context.Context
	nextID     uint32
	releaseFns map[uint32]func()
	values     map[uint32]any
}

func newTestQuickstartResourceClient(ctx context.Context) *testQuickstartResourceClient {
	return &testQuickstartResourceClient{
		ctx:        ctx,
		nextID:     1,
		releaseFns: make(map[uint32]func()),
		values:     make(map[uint32]any),
	}
}

func (c *testQuickstartResourceClient) Context() context.Context {
	return c.ctx
}

func (c *testQuickstartResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *testQuickstartResourceClient) AddResourceValue(_ srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	// Retain the resource value and cleanup under a fresh resource ID.
	id := c.nextID
	c.nextID++
	c.releaseFns[id] = releaseFn
	c.values[id] = value
	return id, nil
}

func (c *testQuickstartResourceClient) ReleaseResource(resourceID uint32) bool {
	// Find the resource cleanup before releasing its retained value.
	releaseFn, ok := c.releaseFns[resourceID]
	if !ok {
		return false
	}

	// Remove the resource records and invoke their cleanup.
	delete(c.releaseFns, resourceID)
	delete(c.values, resourceID)
	if releaseFn != nil {
		releaseFn()
	}
	return true
}

func (c *testQuickstartResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	value, ok := c.values[resourceID]
	if !ok {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *testQuickstartResourceClient) GetAttachedResource(id uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

var _ resource_server.ResourceClientContext = (*testQuickstartResourceClient)(nil)

func TestNewQuickstartRegistryResource(t *testing.T) {
	// Create an empty quickstart registry for initialization checks.
	r := NewQuickstartRegistryResource(nil, nil, nil)

	// Verify the registry initializes its mux, registration map, and ID sequence.
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if r.GetMux() == nil {
		t.Fatal("expected non-nil mux")
	}
	if r.registrations == nil {
		t.Fatal("expected non-nil registrations map")
	}
	if r.nextID != 1 {
		t.Fatalf("expected nextID=1, got %d", r.nextID)
	}
}

func TestRegisterQuickstartValidation(t *testing.T) {
	// Create the registry for quickstart validation checks.
	r := NewQuickstartRegistryResource(nil, nil, nil)

	// Verify the registry requires a registration record.
	_, err := r.RegisterQuickstart(context.Background(), &s4wave_quickstart_registry.RegisterQuickstartRequest{})
	if err != ErrRegistrationRequired {
		t.Fatalf("expected ErrRegistrationRequired, got %v", err)
	}

	// Prepare registrations missing each required quickstart field.
	base := &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId: "sample-workspace",
		PluginId:     "sample-web",
		Name:         "Gizmo Workspace",
		Description:  "Operator workspace",
		Category:     "tools",
	}
	cases := []struct {
		name string
		reg  *s4wave_quickstart_registry.QuickstartRegistration
		err  error
	}{
		{
			name: "quickstart id",
			reg:  &s4wave_quickstart_registry.QuickstartRegistration{PluginId: base.PluginId, Name: base.Name, Description: base.Description, Category: base.Category},
			err:  ErrQuickstartIdRequired,
		},
		{
			name: "plugin id",
			reg:  &s4wave_quickstart_registry.QuickstartRegistration{QuickstartId: base.QuickstartId, Name: base.Name, Description: base.Description, Category: base.Category},
			err:  ErrPluginIdRequired,
		},
		{
			name: "name",
			reg:  &s4wave_quickstart_registry.QuickstartRegistration{QuickstartId: base.QuickstartId, PluginId: base.PluginId, Description: base.Description, Category: base.Category},
			err:  ErrNameRequired,
		},
		{
			name: "description",
			reg:  &s4wave_quickstart_registry.QuickstartRegistration{QuickstartId: base.QuickstartId, PluginId: base.PluginId, Name: base.Name, Category: base.Category},
			err:  ErrDescriptionRequired,
		},
		{
			name: "category",
			reg:  &s4wave_quickstart_registry.QuickstartRegistration{QuickstartId: base.QuickstartId, PluginId: base.PluginId, Name: base.Name, Description: base.Description},
			err:  ErrCategoryRequired,
		},
	}

	// Verify each incomplete quickstart registration is rejected.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Register the incomplete quickstart record.
			_, err := r.RegisterQuickstart(context.Background(), &s4wave_quickstart_registry.RegisterQuickstartRequest{Registration: tc.reg})

			// Verify the missing field produces its expected error.
			if err != tc.err {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestRegisterQuickstartListWatchAndRelease(t *testing.T) {
	// Connect the quickstart client with a bounded watch lifetime.
	ctx, client, _ := setupQuickstartRegistryClient(t)
	watchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Open the root quickstart registry service.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(rootClient)

	// Watch the registry and verify its initial snapshot is empty.
	watch, err := svc.WatchQuickstarts(watchCtx, &s4wave_quickstart_registry.WatchQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	first, err := watch.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(first.GetRegistrations()) != 0 {
		t.Fatalf("expected empty initial watch, got %d", len(first.GetRegistrations()))
	}

	// Register a quickstart with its workspace and required plugins.
	resp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "sample-workspace",
			PluginId:          "sample-web",
			Name:              "Gizmo Workspace",
			Description:       "Operator workspace",
			Category:          "tools",
			IconName:          "bot",
			SpaceName:         "Gizmo Workspace",
			RequiredPluginIds: []string{"sample-core", "sample-web"},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

	// Verify the quickstart list retains its assigned ID and required plugins.
	list, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(list.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(list.GetRegistrations()))
	}
	reg := list.GetRegistrations()[0]
	if reg.GetRegistrationId() == 0 {
		t.Fatal("expected assigned registration id")
	}
	if reg.GetQuickstartId() != "sample-workspace" {
		t.Fatalf("expected sample-workspace, got %s", reg.GetQuickstartId())
	}
	if got := reg.GetRequiredPluginIds(); len(got) != 2 || got[0] != "sample-core" || got[1] != "sample-web" {
		t.Fatalf("unexpected required plugin ids: %v", got)
	}

	// Verify the watch publishes the quickstart registration.
	second, err := watch.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(second.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 watched registration, got %d", len(second.GetRegistrations()))
	}

	// Release the quickstart registration resource.
	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()

	// Verify the watch removes the released quickstart.
	third, err := watch.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(third.GetRegistrations()) != 0 {
		t.Fatalf("expected release to remove quickstart, got %d", len(third.GetRegistrations()))
	}
}

func TestRegisterQuickstartRejectsDuplicateIdFromDifferentPlugin(t *testing.T) {
	// Prepare a quickstart registry with a resource client context.
	r := NewQuickstartRegistryResource(nil, nil, nil)
	clientCtx := newTestQuickstartResourceClient(context.Background())
	ctx := resource_server.WithResourceClientContext(context.Background(), clientCtx)

	// Register the original plugin quickstart.
	resp, err := r.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId: "sample-workspace",
			PluginId:     "sample-web",
			Name:         "Gizmo Workspace",
			Description:  "Operator workspace",
			Category:     "tools",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

	// Verify another plugin cannot replace the same quickstart ID.
	_, err = r.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId: "sample-workspace",
			PluginId:     "spacewave-v86",
			Name:         "V86 Workspace",
			Description:  "VM workspace",
			Category:     "compute",
		},
	})
	if err != ErrQuickstartIdAlreadyRegistered {
		t.Fatalf("expected ErrQuickstartIdAlreadyRegistered, got %v", err)
	}

	// Verify duplicate rejection preserves the original registration.
	list, err := r.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	regs := list.GetRegistrations()
	if len(regs) != 1 {
		t.Fatalf("expected duplicate rejection to leave exactly 1 registration, got %d", len(regs))
	}
	if regs[0].GetPluginId() != "sample-web" {
		t.Fatalf("duplicate rejection replaced original plugin: %s", regs[0].GetPluginId())
	}
}

func TestRegisterQuickstartReconnectsSamePluginAndReleasesLatestOnly(t *testing.T) {
	// Connect to the root quickstart registry service.
	ctx, client, _ := setupQuickstartRegistryClient(t)
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(rootClient)

	// Register the first quickstart connection.
	firstResp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "sample-workspace",
			PluginId:          "sample-web",
			Name:              "Gizmo Workspace",
			Description:       "Operator workspace",
			Category:          "tools",
			IconName:          "bot",
			SpaceName:         "Gizmo Workspace",
			RequiredPluginIds: []string{"sample-core"},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if firstResp.GetResourceId() == 0 {
		t.Fatal("expected first registration resource id")
	}

	// Verify the first connection receives a visible registration ID.
	firstList, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(firstList.GetRegistrations()) != 1 {
		t.Fatalf("expected first registration to be visible, got %d", len(firstList.GetRegistrations()))
	}
	firstRegistrationID := firstList.GetRegistrations()[0].GetRegistrationId()
	if firstRegistrationID == 0 {
		t.Fatal("expected first assigned registration id")
	}

	// Reconnect the plugin with revised quickstart metadata.
	latestResp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "sample-workspace",
			PluginId:          "sample-web",
			Name:              "Gizmo Workspace Reconnected",
			Description:       "Operator workspace after browser restart",
			Category:          "browser-tools",
			IconName:          "sparkles",
			SpaceName:         "Reconnected Workspace",
			RequiredPluginIds: []string{"sample-core", "sample-web", "spacewave-sql"},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if latestResp.GetResourceId() == 0 {
		t.Fatal("expected latest registration resource id")
	}
	if latestResp.GetResourceId() == firstResp.GetResourceId() {
		t.Fatalf("expected reconnect to return a new resource id, got %d", latestResp.GetResourceId())
	}

	// Verify reconnection replaces the visible registration identity.
	list, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	regs := list.GetRegistrations()
	if len(regs) != 1 {
		t.Fatalf("expected reconnect to leave exactly 1 registration, got %d", len(regs))
	}
	latest := regs[0]
	if latest.GetRegistrationId() == 0 {
		t.Fatal("expected latest assigned registration id")
	}
	if latest.GetRegistrationId() == firstRegistrationID {
		t.Fatalf("expected reconnect to assign a new registration id, got %d", latest.GetRegistrationId())
	}

	// Verify the latest quickstart metadata and required plugins win.
	if latest.GetQuickstartId() != "sample-workspace" ||
		latest.GetPluginId() != "sample-web" ||
		latest.GetName() != "Gizmo Workspace Reconnected" ||
		latest.GetDescription() != "Operator workspace after browser restart" ||
		latest.GetCategory() != "browser-tools" ||
		latest.GetIconName() != "sparkles" ||
		latest.GetSpaceName() != "Reconnected Workspace" {
		t.Fatalf("latest registration metadata did not win: %#v", latest)
	}
	if got := latest.GetRequiredPluginIds(); len(got) != 3 ||
		got[0] != "sample-core" ||
		got[1] != "sample-web" ||
		got[2] != "spacewave-sql" {
		t.Fatalf("latest required plugin ids did not win: %v", got)
	}
	latestRegistrationID := latest.GetRegistrationId()

	// Release the superseded quickstart connection.
	oldRef := client.CreateResourceReference(firstResp.GetResourceId())
	oldRef.Release()

	// Verify the old release preserves the latest quickstart registration.
	afterOldRelease, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	regs = afterOldRelease.GetRegistrations()
	if len(regs) != 1 {
		t.Fatalf("expected old release to keep latest registration, got %d", len(regs))
	}
	if regs[0].GetRegistrationId() != latestRegistrationID {
		t.Fatalf("old release removed or changed latest registration: got id %d, want %d", regs[0].GetRegistrationId(), latestRegistrationID)
	}
	if regs[0].GetName() != "Gizmo Workspace Reconnected" {
		t.Fatalf("old release changed latest registration metadata: %s", regs[0].GetName())
	}

	// Release the latest quickstart connection.
	latestRef := client.CreateResourceReference(latestResp.GetResourceId())
	latestRef.Release()

	// Verify the latest release removes the quickstart registration.
	afterLatestRelease, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(afterLatestRelease.GetRegistrations()) != 0 {
		t.Fatalf("expected latest release to remove registration, got %d", len(afterLatestRelease.GetRegistrations()))
	}
}

func TestExecuteQuickstartValidation(t *testing.T) {
	// Create the registry for quickstart execution validation.
	r := NewQuickstartRegistryResource(nil, nil, nil)

	// Verify quickstart execution requires a quickstart ID.
	_, err := r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{})
	if err != ErrQuickstartIdRequired {
		t.Fatalf("expected ErrQuickstartIdRequired, got %v", err)
	}

	// Verify quickstart execution requires a Space resource ID.
	_, err = r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{
		QuickstartId: "sample-workspace",
	})
	if err != ErrSpaceResourceIdRequired {
		t.Fatalf("expected ErrSpaceResourceIdRequired, got %v", err)
	}

	// Register a quickstart without an execution service.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:   "sample-workspace",
			RegistrationId: 1,
			PluginId:       "sample-web",
			Name:           "Gizmo Workspace",
			Description:    "Operator workspace",
			Category:       "tools",
		}
		broadcast()
	})

	// Verify the registered quickstart reports execution as unavailable.
	_, err = r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{
		QuickstartId:    "sample-workspace",
		SpaceResourceId: 1,
	})
	if err != ErrQuickstartExecutionUnavailable {
		t.Fatalf("expected ErrQuickstartExecutionUnavailable, got %v", err)
	}
}

func TestMergePluginIDsDedupesInOrder(t *testing.T) {
	ids := mergePluginIDs(
		[]string{"sample-core", "sample-web"},
		[]string{"sample-web", "spacewave-v86", ""},
	)
	if len(ids) != 3 {
		t.Fatalf("expected 3 plugin ids, got %d: %v", len(ids), ids)
	}
	if ids[0] != "sample-core" || ids[1] != "sample-web" || ids[2] != "spacewave-v86" {
		t.Fatalf("unexpected plugin ids: %v", ids)
	}
}

func TestListQuickstartsSortsById(t *testing.T) {
	// Populate the registry with quickstart IDs in reverse order.
	r := NewQuickstartRegistryResource(nil, nil, nil)
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[2] = &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:   "zeta",
			RegistrationId: 2,
			PluginId:       "plugin",
			Name:           "Zeta",
			Description:    "Zeta workspace",
			Category:       "tools",
		}
		r.registrations[1] = &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:   "alpha",
			RegistrationId: 1,
			PluginId:       "plugin",
			Name:           "Alpha",
			Description:    "Alpha workspace",
			Category:       "tools",
		}
		broadcast()
	})

	// Verify the quickstart list sorts registrations by ID.
	resp, err := r.ListQuickstarts(context.Background(), &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	regs := resp.GetRegistrations()
	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}
	if regs[0].GetQuickstartId() != "alpha" || regs[1].GetQuickstartId() != "zeta" {
		t.Fatalf("unexpected registration order: %s, %s", regs[0].GetQuickstartId(), regs[1].GetQuickstartId())
	}
}

func TestLookupQuickstartRegistrationReturnsClone(t *testing.T) {
	// Populate the registry with the original quickstart record.
	r := NewQuickstartRegistryResource(nil, nil, nil)
	orig := &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId:   "sample-workspace",
		RegistrationId: 1,
		PluginId:       "sample-web",
		Name:           "Gizmo Workspace",
		Description:    "Operator workspace",
		Category:       "tools",
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = orig
		broadcast()
	})

	// Look up an independent quickstart registration record.
	reg := r.LookupRegistration("sample-workspace", "")
	if reg == nil {
		t.Fatal("expected registration")
	}

	// Verify mutating the returned record preserves the stored quickstart.
	reg.QuickstartId = "mutated"
	reg = r.LookupRegistration("sample-workspace", "")
	if reg == nil {
		t.Fatal("expected registration after mutating clone")
	}
	if reg.GetQuickstartId() != "sample-workspace" {
		t.Fatalf("stored registration was mutated: got %s", reg.GetQuickstartId())
	}
}
