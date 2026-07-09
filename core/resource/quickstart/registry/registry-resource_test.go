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
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	r := NewQuickstartRegistryResource(nil, nil)
	clientPipe, serverPipe := net.Pipe()

	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	resourceSrv := resource_server.NewResourceServer(r.GetMux())
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err.Error())
	}

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

	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	client, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		t.Fatal(err.Error())
	}

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
	id := c.nextID
	c.nextID++
	c.releaseFns[id] = releaseFn
	c.values[id] = value
	return id, nil
}

func (c *testQuickstartResourceClient) ReleaseResource(resourceID uint32) bool {
	releaseFn, ok := c.releaseFns[resourceID]
	if !ok {
		return false
	}
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
	r := NewQuickstartRegistryResource(nil, nil)
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
	r := NewQuickstartRegistryResource(nil, nil)

	_, err := r.RegisterQuickstart(context.Background(), &s4wave_quickstart_registry.RegisterQuickstartRequest{})
	if err != ErrRegistrationRequired {
		t.Fatalf("expected ErrRegistrationRequired, got %v", err)
	}

	base := &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId: "glados-workspace",
		PluginId:     "glados-web",
		Name:         "Glados Workspace",
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.RegisterQuickstart(context.Background(), &s4wave_quickstart_registry.RegisterQuickstartRequest{Registration: tc.reg})
			if err != tc.err {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestRegisterQuickstartListWatchAndRelease(t *testing.T) {
	ctx, client, _ := setupQuickstartRegistryClient(t)
	watchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(rootClient)

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

	resp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "glados-workspace",
			PluginId:          "glados-web",
			Name:              "Glados Workspace",
			Description:       "Operator workspace",
			Category:          "tools",
			IconName:          "bot",
			SpaceName:         "Glados Workspace",
			RequiredPluginIds: []string{"glados-core", "glados-web"},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

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
	if reg.GetQuickstartId() != "glados-workspace" {
		t.Fatalf("expected glados-workspace, got %s", reg.GetQuickstartId())
	}
	if got := reg.GetRequiredPluginIds(); len(got) != 2 || got[0] != "glados-core" || got[1] != "glados-web" {
		t.Fatalf("unexpected required plugin ids: %v", got)
	}

	second, err := watch.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(second.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 watched registration, got %d", len(second.GetRegistrations()))
	}

	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()

	third, err := watch.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(third.GetRegistrations()) != 0 {
		t.Fatalf("expected release to remove quickstart, got %d", len(third.GetRegistrations()))
	}
}

func TestRegisterQuickstartRejectsDuplicateIdFromDifferentPlugin(t *testing.T) {
	r := NewQuickstartRegistryResource(nil, nil)
	clientCtx := newTestQuickstartResourceClient(context.Background())
	ctx := resource_server.WithResourceClientContext(context.Background(), clientCtx)

	resp, err := r.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId: "glados-workspace",
			PluginId:     "glados-web",
			Name:         "Glados Workspace",
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

	_, err = r.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId: "glados-workspace",
			PluginId:     "spacewave-v86",
			Name:         "V86 Workspace",
			Description:  "VM workspace",
			Category:     "compute",
		},
	})
	if err != ErrQuickstartIdAlreadyRegistered {
		t.Fatalf("expected ErrQuickstartIdAlreadyRegistered, got %v", err)
	}

	list, err := r.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	regs := list.GetRegistrations()
	if len(regs) != 1 {
		t.Fatalf("expected duplicate rejection to leave exactly 1 registration, got %d", len(regs))
	}
	if regs[0].GetPluginId() != "glados-web" {
		t.Fatalf("duplicate rejection replaced original plugin: %s", regs[0].GetPluginId())
	}
}

func TestRegisterQuickstartReconnectsSamePluginAndReleasesLatestOnly(t *testing.T) {
	ctx, client, _ := setupQuickstartRegistryClient(t)
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(rootClient)

	firstResp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "glados-workspace",
			PluginId:          "glados-web",
			Name:              "Glados Workspace",
			Description:       "Operator workspace",
			Category:          "tools",
			IconName:          "bot",
			SpaceName:         "Glados Workspace",
			RequiredPluginIds: []string{"glados-core"},
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if firstResp.GetResourceId() == 0 {
		t.Fatal("expected first registration resource id")
	}

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

	latestResp, err := svc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      "glados-workspace",
			PluginId:          "glados-web",
			Name:              "Glados Workspace Reconnected",
			Description:       "Operator workspace after browser restart",
			Category:          "browser-tools",
			IconName:          "sparkles",
			SpaceName:         "Reconnected Workspace",
			RequiredPluginIds: []string{"glados-core", "glados-web", "spacewave-sql"},
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
	if latest.GetQuickstartId() != "glados-workspace" ||
		latest.GetPluginId() != "glados-web" ||
		latest.GetName() != "Glados Workspace Reconnected" ||
		latest.GetDescription() != "Operator workspace after browser restart" ||
		latest.GetCategory() != "browser-tools" ||
		latest.GetIconName() != "sparkles" ||
		latest.GetSpaceName() != "Reconnected Workspace" {
		t.Fatalf("latest registration metadata did not win: %#v", latest)
	}
	if got := latest.GetRequiredPluginIds(); len(got) != 3 ||
		got[0] != "glados-core" ||
		got[1] != "glados-web" ||
		got[2] != "spacewave-sql" {
		t.Fatalf("latest required plugin ids did not win: %v", got)
	}
	latestRegistrationID := latest.GetRegistrationId()

	oldRef := client.CreateResourceReference(firstResp.GetResourceId())
	oldRef.Release()

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
	if regs[0].GetName() != "Glados Workspace Reconnected" {
		t.Fatalf("old release changed latest registration metadata: %s", regs[0].GetName())
	}

	latestRef := client.CreateResourceReference(latestResp.GetResourceId())
	latestRef.Release()

	afterLatestRelease, err := svc.ListQuickstarts(ctx, &s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(afterLatestRelease.GetRegistrations()) != 0 {
		t.Fatalf("expected latest release to remove registration, got %d", len(afterLatestRelease.GetRegistrations()))
	}
}

func TestExecuteQuickstartValidation(t *testing.T) {
	r := NewQuickstartRegistryResource(nil, nil)

	_, err := r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{})
	if err != ErrQuickstartIdRequired {
		t.Fatalf("expected ErrQuickstartIdRequired, got %v", err)
	}

	_, err = r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{
		QuickstartId: "glados-workspace",
	})
	if err != ErrSpaceResourceIdRequired {
		t.Fatalf("expected ErrSpaceResourceIdRequired, got %v", err)
	}

	_, err = r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{
		QuickstartId:    "glados-workspace",
		SpaceResourceId: 1,
	})
	if err != ErrQuickstartNotRegistered {
		t.Fatalf("expected ErrQuickstartNotRegistered, got %v", err)
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:   "glados-workspace",
			RegistrationId: 1,
			PluginId:       "glados-web",
			Name:           "Glados Workspace",
			Description:    "Operator workspace",
			Category:       "tools",
		}
		broadcast()
	})
	_, err = r.ExecuteQuickstart(context.Background(), &s4wave_quickstart_registry.ExecuteQuickstartRequest{
		QuickstartId:    "glados-workspace",
		SpaceResourceId: 1,
	})
	if err != ErrQuickstartExecutionUnavailable {
		t.Fatalf("expected ErrQuickstartExecutionUnavailable, got %v", err)
	}
}

func TestMergePluginIDsDedupesInOrder(t *testing.T) {
	ids := mergePluginIDs(
		[]string{"glados-core", "glados-web"},
		[]string{"glados-web", "spacewave-v86", ""},
	)
	if len(ids) != 3 {
		t.Fatalf("expected 3 plugin ids, got %d: %v", len(ids), ids)
	}
	if ids[0] != "glados-core" || ids[1] != "glados-web" || ids[2] != "spacewave-v86" {
		t.Fatalf("unexpected plugin ids: %v", ids)
	}
}

func TestListQuickstartsSortsById(t *testing.T) {
	r := NewQuickstartRegistryResource(nil, nil)
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
	r := NewQuickstartRegistryResource(nil, nil)
	orig := &s4wave_quickstart_registry.QuickstartRegistration{
		QuickstartId:   "glados-workspace",
		RegistrationId: 1,
		PluginId:       "glados-web",
		Name:           "Glados Workspace",
		Description:    "Operator workspace",
		Category:       "tools",
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = orig
		broadcast()
	})

	reg := r.LookupRegistration("glados-workspace")
	if reg == nil {
		t.Fatal("expected registration")
	}
	reg.QuickstartId = "mutated"
	reg = r.LookupRegistration("glados-workspace")
	if reg == nil {
		t.Fatal("expected registration after mutating clone")
	}
	if reg.GetQuickstartId() != "glados-workspace" {
		t.Fatalf("stored registration was mutated: got %s", reg.GetQuickstartId())
	}
}
