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
	// Attribute viewer registry setup failures to the calling test.
	t.Helper()

	// Create a standalone viewer registry and connected RPC pipes.
	ctx, cancel := context.WithCancel(context.Background())
	r := NewViewerRegistryResource(nil)
	clientPipe, serverPipe := net.Pipe()

	// Connect the RPC client to the viewer registry transport.
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Register the viewer registry root with the resource server.
	resourceSrv := resource_server.NewResourceServer(r.GetMux())
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err.Error())
	}

	// Serve the viewer registry RPC connection until the test context ends.
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

	// Create the resource client used to access the viewer registry.
	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	client, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Release the resource client and close both RPC pipes after the test.
	t.Cleanup(func() {
		client.Release()
		cancel()
		clientPipe.Close()
		serverPipe.Close()
	})

	return ctx, client, r
}

func TestRegisterViewerReleaseRemovesRegistration(t *testing.T) {
	// Start the viewer registry client for the registration release test.
	ctx, client, r := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Register a web viewer through the resource service.
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

	// Require a resource ID that controls the viewer registration lifetime.
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

	// List the web viewers while the registration resource is retained.
	list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the registry preserves the viewer component identity.
	if len(list.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(list.GetRegistrations()))
	}
	if list.GetRegistrations()[0].GetComponentId() != "spacewave.test.viewer" {
		t.Fatalf("expected component id to round trip, got %q", list.GetRegistrations()[0].GetComponentId())
	}

	// Capture the web surface notification before releasing the registration.
	waitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB)

	// Release the resource that retains the web viewer registration.
	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()

	// Require a web surface notification when the registration is removed.
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for registration release")
	}

	// List the web viewers after the registration resource is released.
	list, err = svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that releasing the resource removes the web viewer.
	if len(list.GetRegistrations()) != 0 {
		t.Fatalf("expected registration release to remove viewer, got %d", len(list.GetRegistrations()))
	}
}

func TestViewerRegistryFiltersRegistrationsBySurface(t *testing.T) {
	// Start the viewer registry client for surface filtering.
	ctx, client, _ := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Register a viewer on both the web and terminal surfaces.
	surfaces := []s4wave_viewer_registry.ViewerSurface{
		s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
		s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TUI,
	}
	for _, surface := range surfaces {
		// Register the viewer for the current surface.
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

		// Retain the viewer registration until the test completes.
		ref := client.CreateResourceReference(resp.GetResourceId())
		t.Cleanup(ref.Release)
	}

	// Check list and watch snapshots for each registered surface.
	for _, surface := range surfaces {
		// List the viewers for the current surface.
		list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
			Surface: surface,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify that the list contains only the requested surface.
		assertViewerSurface(t, list.GetRegistrations(), surface)

		// Open a viewer watch for the current surface.
		watch, err := svc.WatchViewers(ctx, &s4wave_viewer_registry.WatchViewersRequest{
			Surface: surface,
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Read the initial viewer snapshot from the surface watch.
		snapshot, err := watch.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Verify that the watch contains only the requested surface.
		assertViewerSurface(t, snapshot.GetRegistrations(), surface)
	}
}

func TestViewerRegistryNotifiesOnlyChangedSurface(t *testing.T) {
	// Create viewer notification channels for the web and terminal surfaces.
	r := NewViewerRegistryResource(nil)
	webWaitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB)
	terminalWaitCh := surfaceWaitCh(t, r, s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TUI)

	// Publish a terminal viewer and notify its surface under the registry lock.
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_viewer_registry.ViewerRegistration{
			Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TUI,
		}
		r.broadcastSurfaceLocked(s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_TUI)
	})

	// Verify that the terminal registration leaves web watchers asleep.
	select {
	case <-webWaitCh:
		t.Fatal("terminal registration woke web watchers")
	default:
	}

	// Verify that the terminal registration wakes terminal watchers.
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
	// Start the viewer registry client for surface validation.
	ctx, client, _ := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Attempt to register a viewer without a concrete surface.
	_, err = svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{TypeId: "spacewave/test", ViewerName: "Test"},
	})

	// Require rejection of the unspecified viewer surface.
	if err == nil {
		t.Fatal("expected unspecified viewer surface rejection")
	}
}

func TestViewerRegistryRejectsUnknownSurface(t *testing.T) {
	// Start the viewer registry client for unknown surface validation.
	ctx, client, _ := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Choose a viewer surface outside the supported enum values.
	unknown := s4wave_viewer_registry.ViewerSurface(99)

	// Attempt to register a viewer on the unknown surface.
	_, err = svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{
			TypeId:      "spacewave/test",
			ViewerName:  "Test",
			ScriptPath:  "/viewer.js",
			ComponentId: "spacewave.test.viewer",
			Surface:     unknown,
		},
	})

	// Require rejection of the unknown registration surface.
	if err == nil {
		t.Fatal("expected registration surface validation error")
	}

	// Attempt to list viewers on the unknown surface.
	_, err = svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: unknown,
	})

	// Require rejection of the unknown list surface.
	if err == nil {
		t.Fatal("expected list surface validation error")
	}

	// Attempt to watch viewers on the unknown surface and read any initial response.
	watch, err := svc.WatchViewers(ctx, &s4wave_viewer_registry.WatchViewersRequest{
		Surface: unknown,
	})
	if err == nil {
		_, err = watch.Recv()
	}

	// Require rejection of the unknown watch surface.
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
	// Start the viewer registry client for component validation.
	ctx, client, _ := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Attempt to register a web viewer without a component ID.
	_, err = svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: &s4wave_viewer_registry.ViewerRegistration{
			TypeId:     "spacewave/test",
			ViewerName: "Test",
			ScriptPath: "/viewer.js",
			Surface:    s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
		},
	})

	// Require rejection of the viewer without a component ID.
	if err == nil {
		t.Fatal("expected component id validation error")
	}
}

func TestRegisterViewerClonesRegistrationState(t *testing.T) {
	// Start the viewer registry client for registration isolation.
	ctx, client, _ := setupViewerRegistryClient(t)

	// Access the viewer registry root and retain it for the test.
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}
	svc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)

	// Prepare the web viewer record whose stored copy must remain isolated.
	reg := &s4wave_viewer_registry.ViewerRegistration{
		TypeId:      "spacewave/test",
		ViewerName:  "Test",
		ScriptPath:  "/viewer.js",
		ComponentId: "spacewave.test.viewer",
		Surface:     s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	}

	// Register the viewer record through the resource service.
	resp, err := svc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{
		Registration: reg,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Retain the viewer registration until the test completes.
	ref := client.CreateResourceReference(resp.GetResourceId())
	t.Cleanup(ref.Release)

	// Mutate the original viewer record and list the stored registration.
	reg.ComponentId = "mutated.viewer"
	list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{
		Surface: s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the stored viewer retains its original component ID.
	if list.GetRegistrations()[0].GetComponentId() != "spacewave.test.viewer" {
		t.Fatalf("expected cloned component id, got %q", list.GetRegistrations()[0].GetComponentId())
	}
}
