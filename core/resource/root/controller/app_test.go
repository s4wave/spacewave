package resource_root_controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_core "github.com/s4wave/spacewave/bldr/core"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/testbed"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	s4wave_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestNestedAppRuntime creates real local accounts through independent Resource
// clients, then checks shared attachments and a persistent World-backed reopen.
func TestNestedAppRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	parent, factories, err := bldr_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	factories.AddFactory(NewFactory(parent))
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*Controller](ctx, parent,
		resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()

	connect := func(invoker srpc.Invoker) (*resource_client.Client, s4wave_root.SRPCRootResourceServiceClient) {
		t.Helper()
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(
			srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Release)
		ref := client.AccessRootResource()
		t.Cleanup(ref.Release)
		rpcClient, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return client, s4wave_root.NewSRPCRootResourceServiceClient(rpcClient)
	}
	outerClient, outer := connect(ctrl)
	mount := func() (*resource_client.Client, s4wave_root.SRPCRootResourceServiceClient, string) {
		t.Helper()
		resp, err := outer.MountApp(ctx, &s4wave_root.MountAppRequest{Ephemeral: true})
		if err != nil {
			t.Fatal(err)
		}
		ref := outerClient.CreateResourceReference(resp.ResourceId)
		t.Cleanup(ref.Release)
		rpcClient, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(rpcClient))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Release)
		rootRef := client.AccessRootResource()
		t.Cleanup(rootRef.Release)
		rootClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return client, s4wave_root.NewSRPCRootResourceServiceClient(rootClient), resp.GetHttpPathPrefix()
	}
	create := func(client *resource_client.Client, root s4wave_root.SRPCRootResourceServiceClient) {
		t.Helper()
		provider, err := root.LookupProvider(ctx, &s4wave_root.LookupProviderRequest{ProviderId: "local"})
		if err != nil {
			t.Fatal(err)
		}
		ref := client.CreateResourceReference(provider.ResourceId)
		defer ref.Release()
		rpcClient, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s4wave_local.NewSRPCLocalProviderResourceServiceClient(rpcClient).CreateAccount(ctx, &s4wave_local.CreateAccountRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	assertSessions := func(root s4wave_root.SRPCRootResourceServiceClient, count int) {
		t.Helper()
		resp, err := root.ListSessions(ctx, &s4wave_root.ListSessionsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Sessions) != count {
			t.Fatalf("sessions = %d, want %d", len(resp.Sessions), count)
		}
	}
	left, leftRoot, leftHTTP := mount()
	_, rightRoot, rightHTTP := mount()

	// Plugins register with the host root; nested apps serve the same entries.
	if _, err := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(outer.SRPCClient()).RegisterQuickstart(ctx,
		&s4wave_quickstart_registry.RegisterQuickstartRequest{Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId: "notebook", PluginId: "spacewave-notes", Name: "Notebook", Description: "Notes", Category: "notes",
		}}); err != nil {
		t.Fatal(err)
	}
	listed, err := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(leftRoot.SRPCClient()).ListQuickstarts(ctx,
		&s4wave_quickstart_registry.ListQuickstartsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if regs := listed.GetRegistrations(); len(regs) != 1 || regs[0].GetQuickstartId() != "notebook" {
		t.Fatalf("nested Quickstarts = %v, want the host registration", regs)
	}

	create(left, leftRoot)
	assertSessions(leftRoot, 1)
	assertSessions(rightRoot, 0)
	rpcFor := func(id uint32) srpc.Client {
		t.Helper()
		ref := left.CreateResourceReference(id)
		t.Cleanup(ref.Release)
		rpc, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return rpc
	}
	session, err := leftRoot.MountSessionByIdx(ctx, &s4wave_root.MountSessionByIdxRequest{SessionIdx: 1})
	if err != nil {
		t.Fatal(err)
	}
	space, err := s4wave_session.NewSRPCSessionResourceServiceClient(rpcFor(session.ResourceId)).CreateSpace(ctx,
		&s4wave_session.CreateSpaceRequest{SpaceName: "Nested Canvas"})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := s4wave_space.NewSRPCSpaceResourceServiceClient(rpcFor(space.SharedObjectBodyResourceId)).MountSpaceContents(ctx,
		&s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	rpcFor(contents.ResourceId)
	engine := s4wave_world.NewSRPCEngineResourceServiceClient(rpcFor(space.SpaceWorldResourceId))
	apply := func(op world.Operation, key string) {
		t.Helper()
		wtx, err := engine.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
		if err != nil {
			t.Fatal(err)
		}
		txRPC := rpcFor(wtx.ResourceId)
		txClient := s4wave_world.NewSRPCTxResourceServiceClient(txRPC)
		defer func() {
			if _, err := txClient.Discard(ctx, &s4wave_world.DiscardRequest{}); err != nil {
				t.Error(err)
			}
		}()
		data, err := op.MarshalBlock()
		if err != nil {
			t.Fatal(err)
		}
		worldRPC := s4wave_world.NewSRPCWorldStateResourceServiceClient(txRPC)
		applied, err := worldRPC.ApplyWorldOp(ctx,
			&s4wave_world.ApplyWorldOpRequest{OpTypeId: op.GetOperationTypeId(), OpData: data})
		if err != nil {
			t.Fatal(err)
		}
		if applied.GetErrorCode() != 0 || applied.GetSysErr() {
			t.Fatalf("initialize %s: %v", key, applied)
		}
		published, err := worldRPC.GetObject(ctx,
			&s4wave_world.GetObjectRequest{ObjectKey: key})
		if err != nil {
			t.Fatal(err)
		}
		if published.GetResourceId() == 0 {
			t.Fatalf("operation did not create %s", key)
		}
		rpcFor(published.ResourceId)
		if _, err := txClient.Commit(ctx, &s4wave_world.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	apply(space_world_ops.NewInitUnixFSOp("files", time.Now()), "files")
	apply(space_world_ops.NewInitCanvasDemoOp("canvas-1", time.Now()), "canvas-1")
	apply(space_world_ops.NewSetSpaceSettingsOp("settings", &space_world.SpaceSettings{IndexPath: "canvas-1"}, true, time.Now()), "settings")

	// Projected file requests resolve Sessions on the selected child bus.
	filePath := "/fs/u/1/so/" + space.GetSharedObjectRef().GetProviderResourceRef().GetId() + "/-/files/-/"
	for prefix, expected := range map[string]int{leftHTTP: http.StatusMovedPermanently, rightHTTP: http.StatusServiceUnavailable} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(ctx, http.MethodGet, prefix+filePath, nil)
		bifrost_http.NewBusHandler(parent, "", true).ServeHTTP(recorder, request)
		if recorder.Code != expected {
			t.Fatalf("projected files through %s: status %d, body %s", prefix, recorder.Code, recorder.Body.String())
		}
	}

	// Two attachments share one installation; closing both preserves its World.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	binding := resource_root.AppStorage{Engine: tb.Engine, Prefix: "apps/persistent"}
	first, releaseFirst, err := ctrl.mountApp(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	second, releaseSecond, err := ctrl.mountApp(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("shared storage binding created separate runtimes")
	}
	persistentClient, persistentRoot := connect(first)
	create(persistentClient, persistentRoot)
	assertSessions(persistentRoot, 1)
	persistentClient.Release()
	<-persistentClient.Done()
	releaseFirst()
	releaseSecond()
	reopened, releaseReopened, err := ctrl.mountApp(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReopened()
	_, reopenedRoot := connect(reopened)
	assertSessions(reopenedRoot, 1)
}
