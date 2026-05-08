package statusprojector

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

type recordingTrayActionHandler struct {
	requests []*desktop_tray.HandleDesktopTrayActionRequest
}

func (h *recordingTrayActionHandler) HandleDesktopTrayAction(
	ctx context.Context,
	req *desktop_tray.HandleDesktopTrayActionRequest,
) (*desktop_tray.HandleDesktopTrayActionResponse, error) {
	h.requests = append(h.requests, req.CloneVT())
	return &desktop_tray.HandleDesktopTrayActionResponse{}, nil
}

func TestDesktopTrayPublisherPublishesUpdatesAndReleasesProjectedEntries(t *testing.T) {
	ctx := t.Context()
	publisher, tray := newTestDesktopTrayPublisher(t)
	defer func() {
		if err := publisher.Release(context.Background()); err != nil {
			t.Fatalf("release publisher: %v", err)
		}
	}()

	handler := &recordingTrayActionHandler{}
	publisher.actionHandlers["apply-update"] = handler

	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath:       "/run/spacewave.sock",
		Listening:        true,
		ConnectedClients: 1,
	})
	state.Update = &desktop_runtime.DesktopRuntimeUpdateStatus{
		Ready:   true,
		Version: "1.2.3",
		Label:   "Update ready",
	}

	changed, err := publisher.Publish(ctx, state)
	if err != nil {
		t.Fatalf("publish initial state: %v", err)
	}
	if !changed {
		t.Fatal("expected initial publish to change tray entries")
	}

	if _, err := tray.InvokeDesktopTrayEntry(ctx, &desktop_tray.InvokeDesktopTrayEntryRequest{
		EntryId: "apply-update",
	}); err != nil {
		t.Fatalf("invoke published update action: %v", err)
	}
	if len(handler.requests) != 1 {
		t.Fatalf("update action requests = %d, want 1", len(handler.requests))
	}
	if handler.requests[0].GetAction().GetValue() != "1.2.3" {
		t.Fatalf("update action value = %q, want version", handler.requests[0].GetAction().GetValue())
	}

	changed, err = publisher.Publish(ctx, state.CloneVT())
	if err != nil {
		t.Fatalf("publish unchanged state: %v", err)
	}
	if changed {
		t.Fatal("expected unchanged publish to be suppressed")
	}

	state.Update = nil
	changed, err = publisher.Publish(ctx, state)
	if err != nil {
		t.Fatalf("publish state without update: %v", err)
	}
	if !changed {
		t.Fatal("expected removing update action to change tray entries")
	}
	if _, err := tray.InvokeDesktopTrayEntry(ctx, &desktop_tray.InvokeDesktopTrayEntryRequest{
		EntryId: "apply-update",
	}); err == nil {
		t.Fatal("expected removed update action to be unavailable")
	}
}

func newTestDesktopTrayPublisher(
	t *testing.T,
) (*desktopTrayPublisher, desktop_tray.SRPCDesktopTrayResourceServiceClient) {
	t.Helper()

	tray := desktop_tray.NewDesktopTray()
	server := resource_server.NewResourceServer(tray.GetMux())
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	resourceService := bldr_resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))),
	)
	resources, err := resource_client.NewClient(t.Context(), resourceService)
	if err != nil {
		t.Fatalf("new resource client: %v", err)
	}
	rootRef := resources.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		t.Fatalf("root resource client: %v", err)
	}
	service := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(rootClient)
	return &desktopTrayPublisher{
		resources: resources,
		trayRef:   rootRef,
		tray:      service,
		entries:   make(map[string]*desktopTrayEntryRegistration),
		actionHandlers: map[string]desktop_tray.SRPCDesktopTrayActionHandlerServiceServer{
			"apply-update": &recordingTrayActionHandler{},
		},
	}, service
}

var _ desktop_tray.SRPCDesktopTrayActionHandlerServiceServer = ((*recordingTrayActionHandler)(nil))
