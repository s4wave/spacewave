package resource_viewer_registry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
)

func setupViewerRegistryClient(t *testing.T) (context.Context, *resource_client.Client, *ViewerRegistryResource) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	r := NewViewerRegistryResource()
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

func TestRegisterViewerReleaseRemovesRegistration(t *testing.T) {
	ctx, client, r := setupViewerRegistryClient(t)

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	resp, err := svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{
			TypeId:      "spacewave/test",
			ViewerName:  "Test",
			ScriptPath:  "/viewer.js",
			ComponentId: "spacewave.test.viewer",
			Surface:     s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

	list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(list.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(list.GetRegistrations()))
	}
	if list.GetRegistrations()[0].GetComponentId() != "spacewave.test.viewer" {
		t.Fatalf("expected component id to round trip, got %q", list.GetRegistrations()[0].GetComponentId())
	}

	waitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB)

	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()

	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for registration release")
	}

	list, err = svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(list.GetRegistrations()) != 0 {
		t.Fatalf("expected registration release to remove viewer, got %d", len(list.GetRegistrations()))
	}
}

func TestViewerRegistryFiltersRegistrationsBySurface(t *testing.T) {
	ctx, client, _ := setupViewerRegistryClient(t)

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	surfaces := []s4wave_viewer_registry.ViewerSurface{
		s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
		s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TERMINAL,
	}
	for _, surface := range surfaces {
		resp, err := svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
			Registration: &s4wave_viewer_registry.ViewerRegistration{
				TypeId:      "spacewave/test",
				ViewerName:  "Test",
				ScriptPath:  "/viewer.js",
				ComponentId: "spacewave.test.viewer",
				Surface:     surface,
			},
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		ref := client.CreateResourceReference(resp.GetResourceId())
		t.Cleanup(ref.Release)
	}

	for _, surface := range surfaces {
		list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
			Surface: surface,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		assertViewerSurface(t, list.GetRegistrations(), surface)

		watch, err := svc.WatchViewers(ctx, &s4wave_viewer_registry.WatchViewersRequest{
			Surface: surface,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		snapshot, err := watch.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		assertViewerSurface(t, snapshot.GetRegistrations(), surface)
	}
}

func TestViewerRegistryNotifiesOnlyChangedSurface(t *testing.T) {
	r := NewViewerRegistryResource()
	webWaitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB)
	terminalWaitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TERMINAL)

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_viewer_registry.ViewerRegistration{
			Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TERMINAL,
		}
		r.broadcastSurfaceLocked(s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TERMINAL)
	})

	select {
	case <-webWaitCh:
		t.Fatal("terminal registration woke web watchers")
	default:
	}
	select {
	case <-terminalWaitCh:
	default:
		t.Fatal("terminal registration did not wake terminal watchers")
	}
}

func surfaceWaitCh(
	t *testing.T,
	r *ViewerRegistryResource,
	surface s4wave_viewer_registry.ViewerSurface,
) <-chan struct{} {
	t.Helper()
	var waitCh <-chan struct{}
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.getSurfaceBroadcastLocked(surface).HoldLock(func(
			_ func(),
			getWaitCh func() <-chan struct{},
		) {
			waitCh = getWaitCh()
		})
	})
	return waitCh
}

func TestViewerRegistryRejectsUnspecifiedSurface(t *testing.T) {
	ctx, client, _ := setupViewerRegistryClient(t)

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	_, err = svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{
			TypeId:      "spacewave/test",
			ViewerName:  "Test",
			ScriptPath:  "/viewer.js",
			ComponentId: "spacewave.test.viewer",
		},
	})
	if err == nil {
		t.Fatal("expected registration surface validation error")
	}

	_, err = svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{})
	if err == nil {
		t.Fatal("expected list surface validation error")
	}

	watch, err := svc.WatchViewers(ctx, &s4wave_viewer_registry.WatchViewersRequest{})
	if err == nil {
		_, err = watch.Recv()
	}
	if err == nil {
		t.Fatal("expected watch surface validation error")
	}
}

func assertViewerSurface(
	t *testing.T,
	regs []*s4wave_viewer_registry.ViewerRegistration,
	surface s4wave_viewer_registry.ViewerSurface,
) {
	t.Helper()
	if len(regs) != 1 {
		t.Fatalf("expected 1 %s registration, got %d", surface.String(), len(regs))
	}
	if regs[0].GetSurface() != surface {
		t.Fatalf("expected %s registration, got %s", surface.String(), regs[0].GetSurface().String())
	}
}

func TestRegisterViewerRequiresComponentID(t *testing.T) {
	ctx, client, _ := setupViewerRegistryClient(t)

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	_, err = svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{
			TypeId:     "spacewave/test",
			ViewerName: "Test",
			ScriptPath: "/viewer.js",
			Surface:    s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
		},
	})
	if err == nil {
		t.Fatal("expected component id validation error")
	}
}

func TestRegisterViewerClonesRegistrationState(t *testing.T) {
	ctx, client, _ := setupViewerRegistryClient(t)

	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	reg := &s4wave_viewer_registry.ViewerRegistration{
		TypeId:      "spacewave/test",
		ViewerName:  "Test",
		ScriptPath:  "/viewer.js",
		ComponentId: "spacewave.test.viewer",
		Surface:     s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	}
	resp, err := svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: reg,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := client.CreateResourceReference(resp.GetResourceId())
	t.Cleanup(ref.Release)

	reg.ComponentId = "mutated.viewer"
	list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if list.GetRegistrations()[0].GetComponentId() != "spacewave.test.viewer" {
		t.Fatalf("expected cloned component id, got %q", list.GetRegistrations()[0].GetComponentId())
	}
}
