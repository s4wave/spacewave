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
			TypeId:     "spacewave/test",
			ViewerName: "Test",
			ScriptPath: "/viewer.js",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected registration resource id")
	}

	list, err := svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(list.GetRegistrations()) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(list.GetRegistrations()))
	}

	var waitCh <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()

	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for registration release")
	}

	list, err = svc.ListViewers(ctx, &s4wave_viewer_registry.ListViewersRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(list.GetRegistrations()) != 0 {
		t.Fatalf("expected registration release to remove viewer, got %d", len(list.GetRegistrations()))
	}
}
