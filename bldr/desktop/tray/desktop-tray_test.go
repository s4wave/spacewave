package desktop_tray

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

type testResourceClientContext struct {
	ctx context.Context

	nextID   uint32
	values   map[uint32]any
	releases map[uint32]func()
	attached map[uint32]srpc.Client
}

func newTestResourceClientContext(ctx context.Context) *testResourceClientContext {
	return &testResourceClientContext{
		ctx:      ctx,
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
		attached: make(map[uint32]srpc.Client),
	}
}

func (c *testResourceClientContext) Context() context.Context {
	return c.ctx
}

func (c *testResourceClientContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *testResourceClientContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.nextID++
	resourceID := c.nextID
	c.values[resourceID] = value
	c.releases[resourceID] = releaseFn
	return resourceID, nil
}

func (c *testResourceClientContext) ReleaseResource(resourceID uint32) bool {
	releaseFn := c.releases[resourceID]
	if releaseFn == nil {
		return false
	}
	delete(c.releases, resourceID)
	delete(c.values, resourceID)
	releaseFn()
	return true
}

func (c *testResourceClientContext) GetResourceValue(resourceID uint32) (any, error) {
	value, ok := c.values[resourceID]
	if !ok {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *testResourceClientContext) GetAttachedResource(id uint32) (srpc.Client, error) {
	client := c.attached[id]
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}

type testActionClient struct {
	requests []*HandleDesktopTrayActionRequest
}

func (c *testActionClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	req := in.(*HandleDesktopTrayActionRequest)
	c.requests = append(c.requests, req.CloneVT())
	return nil
}

func (c *testActionClient) NewStream(
	ctx context.Context,
	service string,
	method string,
	firstMsg srpc.Message,
) (srpc.Stream, error) {
	return nil, resource.ErrResourceNotFound
}

type testActionHandler struct {
	requests []*HandleDesktopTrayActionRequest
}

func (h *testActionHandler) HandleDesktopTrayAction(
	ctx context.Context,
	req *HandleDesktopTrayActionRequest,
) (*HandleDesktopTrayActionResponse, error) {
	h.requests = append(h.requests, req.CloneVT())
	return &HandleDesktopTrayActionResponse{}, nil
}

type testWatchStream struct {
	ctx context.Context

	updates chan *WatchDesktopTrayResponse
}

func newTestWatchStream(ctx context.Context) *testWatchStream {
	return &testWatchStream{
		ctx:     ctx,
		updates: make(chan *WatchDesktopTrayResponse, 8),
	}
}

func (s *testWatchStream) Context() context.Context {
	return s.ctx
}

func (s *testWatchStream) MsgSend(msg srpc.Message) error {
	return nil
}

func (s *testWatchStream) MsgRecv(msg srpc.Message) error {
	return io.EOF
}

func (s *testWatchStream) CloseSend() error {
	return nil
}

func (s *testWatchStream) Close() error {
	return nil
}

func (s *testWatchStream) Send(resp *WatchDesktopTrayResponse) error {
	s.updates <- resp.CloneVT()
	return nil
}

func (s *testWatchStream) SendAndClose(resp *WatchDesktopTrayResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	return nil
}

func TestDesktopTrayRegistryOrdersEntriesAndUpdatesState(t *testing.T) {
	ctx := context.Background()
	client := newTestResourceClientContext(ctx)
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()

	statusResp, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:        "status",
			Kind:      DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Path:      []string{"Status"},
			Group:     "runtime",
			Order:     20,
			Label:     "Runtime",
			Enabled:   true,
			IconState: DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	actionResp, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:        "open",
			Kind:      DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
			Path:      []string{"Actions"},
			Group:     "primary",
			Order:     10,
			Label:     "Open Spacewave",
			Enabled:   true,
			IconState: DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	state := tray.snapshot()
	entries := state.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].GetId() != "open" || entries[1].GetId() != "status" {
		t.Fatalf("unexpected order: %s, %s", entries[0].GetId(), entries[1].GetId())
	}
	if state.GetIconState() != DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE {
		t.Fatalf("expected active icon state, got %s", state.GetIconState())
	}

	value, err := client.GetResourceValue(actionResp.GetResourceId())
	if err != nil {
		t.Fatal(err)
	}
	entryResource := value.(*DesktopTrayEntryResource)
	if _, err := entryResource.SetDesktopTrayEntryActive(ctx, &SetDesktopTrayEntryActiveRequest{Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := entryResource.SetDesktopTrayEntryEnabled(ctx, &SetDesktopTrayEntryEnabledRequest{Enabled: false}); err != nil {
		t.Fatal(err)
	}

	state = tray.snapshot()
	entries = state.GetEntries()
	if !entries[0].GetActive() {
		t.Fatal("expected active entry")
	}
	if entries[0].GetEnabled() {
		t.Fatal("expected disabled entry")
	}

	if !client.ReleaseResource(statusResp.GetResourceId()) {
		t.Fatal("expected release to unregister entry")
	}
	state = tray.snapshot()
	entries = state.GetEntries()
	if len(entries) != 1 || entries[0].GetId() != "open" {
		t.Fatalf("unexpected entries after release: %#v", entries)
	}
}

func TestDesktopTrayRegistryWatchStreamsSnapshots(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newTestResourceClientContext(ctx)
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()
	strm := newTestWatchStream(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- tray.WatchDesktopTray(&WatchDesktopTrayRequest{}, strm)
	}()

	resp := <-strm.updates
	if len(resp.GetState().GetEntries()) != 0 {
		t.Fatalf("expected empty initial snapshot, got %d entries", len(resp.GetState().GetEntries()))
	}

	_, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:      "runtime",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label:   "Runtime",
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp = <-strm.updates
	entries := resp.GetState().GetEntries()
	if len(entries) != 1 || entries[0].GetId() != "runtime" {
		t.Fatalf("unexpected watch snapshot: %#v", entries)
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected canceled watch, got %v", err)
	}
}

func TestDesktopTrayRegistryInvokesAttachedActionHandlers(t *testing.T) {
	ctx := context.Background()
	client := newTestResourceClientContext(ctx)
	actionClient := &testActionClient{}
	client.attached[7] = actionClient
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()

	_, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		AttachedActionResourceId: 7,
		Entry: &DesktopTrayEntry{
			Id:      "open",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
			Label:   "Open",
			Enabled: true,
			Action: &DesktopTrayAction{
				Kind:  DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER,
				Route: "/spaces",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tray.InvokeDesktopTrayEntry(ctx, &InvokeDesktopTrayEntryRequest{EntryId: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if len(actionClient.requests) != 1 {
		t.Fatalf("expected 1 action request, got %d", len(actionClient.requests))
	}
	if actionClient.requests[0].GetEntryId() != "open" {
		t.Fatalf("unexpected entry id: %s", actionClient.requests[0].GetEntryId())
	}
	if actionClient.requests[0].GetAction().GetRoute() != "/spaces" {
		t.Fatalf("unexpected route: %s", actionClient.requests[0].GetAction().GetRoute())
	}
}

func TestDesktopTrayRegistryRejectsNonInvokableEntries(t *testing.T) {
	ctx := context.Background()
	client := newTestResourceClientContext(ctx)
	actionClient := &testActionClient{}
	client.attached[7] = actionClient
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()

	_, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		AttachedActionResourceId: 7,
		Entry: &DesktopTrayEntry{
			Id:      "disabled",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
			Label:   "Disabled",
			Enabled: false,
			Action: &DesktopTrayAction{
				Kind: DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:    "status",
			Kind:  DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label: "Status",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tray.InvokeDesktopTrayEntry(ctx, &InvokeDesktopTrayEntryRequest{EntryId: "disabled"}); err != ErrDesktopTrayEntryNotInvokable {
		t.Fatalf("expected disabled entry to be non-invokable, got %v", err)
	}
	if _, err := tray.InvokeDesktopTrayEntry(ctx, &InvokeDesktopTrayEntryRequest{EntryId: "status"}); err != ErrDesktopTrayEntryNotInvokable {
		t.Fatalf("expected status entry to be non-invokable, got %v", err)
	}
	if len(actionClient.requests) != 0 {
		t.Fatalf("expected no action requests, got %d", len(actionClient.requests))
	}
}

func TestDesktopTrayRegistryRejectsDuplicateEntryIDs(t *testing.T) {
	ctx := context.Background()
	client := newTestResourceClientContext(ctx)
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()

	first, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:    "same",
			Kind:  DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label: "First",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:    "same",
			Kind:  DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label: "Second",
		},
	})
	if err != ErrDesktopTrayEntryDuplicate {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}

	value, err := client.GetResourceValue(first.GetResourceId())
	if err != nil {
		t.Fatal(err)
	}
	entryResource := value.(*DesktopTrayEntryResource)
	_, err = tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:    "other",
			Kind:  DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label: "Other",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = entryResource.SetDesktopTrayEntry(ctx, &SetDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:    "other",
			Kind:  DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label: "Renamed",
		},
	})
	if err != ErrDesktopTrayEntryDuplicate {
		t.Fatalf("expected duplicate update error, got %v", err)
	}
}

func TestDesktopTrayRegistrySortsTiesByEntryIDThenResourceID(t *testing.T) {
	ctx := context.Background()
	client := newTestResourceClientContext(ctx)
	reqCtx := resource_server.WithResourceClientContext(ctx, client)
	tray := NewDesktopTray()

	for _, id := range []string{"zeta", "alpha", "middle"} {
		_, err := tray.RegisterDesktopTrayEntry(reqCtx, &RegisterDesktopTrayEntryRequest{
			Entry: &DesktopTrayEntry{
				Id:      id,
				Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
				Path:    []string{"Actions"},
				Group:   "primary",
				Order:   10,
				Label:   id,
				Enabled: true,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	entries := tray.snapshot().GetEntries()
	got := []string{entries[0].GetId(), entries[1].GetId(), entries[2].GetId()}
	want := []string{"alpha", "middle", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestReconcileDesktopTrayMirrorsEntriesToTargetResource(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	sourceClient, source, sourceRelease := newTestDesktopTrayResourceClient(t)
	defer sourceRelease()
	targetClient, target, targetRelease := newTestDesktopTrayResourceClient(t)
	defer targetRelease()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReconcileDesktopTray(ctx, source, target, targetClient)
	}()

	targetStream, err := target.WatchDesktopTray(ctx, &WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatalf("watch target tray: %v", err)
	}
	if _, err := targetStream.Recv(); err != nil {
		t.Fatalf("recv target initial snapshot: %v", err)
	}

	first, err := source.RegisterDesktopTrayEntry(ctx, &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:      "status",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label:   "Runtime - Running",
			Enabled: false,
		},
	})
	if err != nil {
		t.Fatalf("register source entry: %v", err)
	}

	state := recvTrayState(t, targetStream)
	if len(state.GetEntries()) != 1 {
		t.Fatalf("target entries = %d, want 1", len(state.GetEntries()))
	}
	if state.GetEntries()[0].GetLabel() != "Runtime - Running" {
		t.Fatalf("target label = %q, want mirrored source label", state.GetEntries()[0].GetLabel())
	}

	firstRef := sourceClient.CreateResourceReference(first.GetResourceId())
	defer firstRef.Release()
	firstClient, err := firstRef.GetClient()
	if err != nil {
		t.Fatalf("source entry client: %v", err)
	}
	firstEntry := NewSRPCDesktopTrayEntryResourceServiceClient(firstClient)
	_, err = firstEntry.SetDesktopTrayEntry(ctx, &SetDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:      "status",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label:   "Runtime - Busy",
			Enabled: false,
		},
	})
	if err != nil {
		t.Fatalf("update source entry: %v", err)
	}

	state = recvTrayState(t, targetStream)
	if len(state.GetEntries()) != 1 {
		t.Fatalf("target entries after update = %d, want 1", len(state.GetEntries()))
	}
	if state.GetEntries()[0].GetLabel() != "Runtime - Busy" {
		t.Fatalf("target label after update = %q, want updated source label", state.GetEntries()[0].GetLabel())
	}

	firstRef.Release()
	state = recvTrayState(t, targetStream)
	if len(state.GetEntries()) != 0 {
		t.Fatalf("target entries after source release = %d, want 0", len(state.GetEntries()))
	}

	cancel()
	if err := <-errCh; err == nil {
		t.Fatalf("expected reconciler to stop on context cancel")
	}
}

func TestReconcileDesktopTrayForwardsAttachedActionHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	sourceClient, source, sourceRelease := newTestDesktopTrayResourceClient(t)
	defer sourceRelease()
	targetClient, target, targetRelease := newTestDesktopTrayResourceClient(t)
	defer targetRelease()

	handler := &testActionHandler{}
	handlerMux := srpc.NewMux()
	if err := SRPCRegisterDesktopTrayActionHandlerService(handlerMux, handler); err != nil {
		t.Fatalf("register action handler: %v", err)
	}
	attachedActionResourceID, err := sourceClient.AttachRawInvoker(ctx, "test-tray-action", handlerMux)
	if err != nil {
		t.Fatalf("attach source action handler: %v", err)
	}
	defer func() {
		_ = sourceClient.DetachResource(context.Background(), attachedActionResourceID)
	}()

	_, err = source.RegisterDesktopTrayEntry(ctx, &RegisterDesktopTrayEntryRequest{
		AttachedActionResourceId: attachedActionResourceID,
		Entry: &DesktopTrayEntry{
			Id:      "copy-diagnostics",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
			Label:   "Copy Diagnostics",
			Enabled: true,
			Action: &DesktopTrayAction{
				Kind:  DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER,
				Value: "diagnostics",
			},
		},
	})
	if err != nil {
		t.Fatalf("register source entry: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReconcileDesktopTray(ctx, source, target, targetClient)
	}()

	targetStream, err := target.WatchDesktopTray(ctx, &WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatalf("watch target tray: %v", err)
	}
	state := recvTrayState(t, targetStream)
	if len(state.GetEntries()) == 0 {
		state = recvTrayState(t, targetStream)
	}
	if len(state.GetEntries()) != 1 || state.GetEntries()[0].GetId() != "copy-diagnostics" {
		t.Fatalf("target entries = %#v", state.GetEntries())
	}

	_, err = target.InvokeDesktopTrayEntry(ctx, &InvokeDesktopTrayEntryRequest{
		EntryId: "copy-diagnostics",
	})
	if err != nil {
		t.Fatalf("invoke target attached entry: %v", err)
	}
	if len(handler.requests) != 1 {
		t.Fatalf("source handler requests = %d, want 1", len(handler.requests))
	}
	if handler.requests[0].GetEntryId() != "copy-diagnostics" {
		t.Fatalf("source handler entry id = %q", handler.requests[0].GetEntryId())
	}

	cancel()
	if err := <-errCh; err == nil {
		t.Fatalf("expected reconciler to stop on context cancel")
	}
}

func TestReconcileDesktopTrayMirrorsExistingEntriesAcrossTargetReconnect(t *testing.T) {
	_, source, sourceRelease := newTestDesktopTrayResourceClient(t)
	defer sourceRelease()

	_, err := source.RegisterDesktopTrayEntry(t.Context(), &RegisterDesktopTrayEntryRequest{
		Entry: &DesktopTrayEntry{
			Id:      "space",
			Kind:    DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
			Label:   "My Drive",
			Enabled: true,
			Action: &DesktopTrayAction{
				Kind:  DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE,
				Route: "/u/1/so/space",
			},
		},
	})
	if err != nil {
		t.Fatalf("register source entry: %v", err)
	}

	firstTargetClient, firstTarget, firstTargetRelease := newTestDesktopTrayResourceClient(t)
	defer firstTargetRelease()
	firstStream, err := firstTarget.WatchDesktopTray(t.Context(), &WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatalf("watch first target tray: %v", err)
	}
	if state := recvTrayState(t, firstStream); len(state.GetEntries()) != 0 {
		t.Fatalf("first target initial entries = %d, want 0", len(state.GetEntries()))
	}

	firstCtx, firstCancel := context.WithCancel(t.Context())
	firstErrCh := make(chan error, 1)
	go func() {
		firstErrCh <- ReconcileDesktopTray(firstCtx, source, firstTarget, firstTargetClient)
	}()

	state := recvTrayState(t, firstStream)
	if len(state.GetEntries()) != 1 || state.GetEntries()[0].GetLabel() != "My Drive" {
		t.Fatalf("first target mirrored entries = %#v", state.GetEntries())
	}

	firstCancel()
	if err := <-firstErrCh; err == nil {
		t.Fatalf("expected first reconciler to stop on context cancel")
	}
	state = recvTrayState(t, firstStream)
	if len(state.GetEntries()) != 0 {
		t.Fatalf("first target entries after reconciler stop = %d, want 0", len(state.GetEntries()))
	}

	secondTargetClient, secondTarget, secondTargetRelease := newTestDesktopTrayResourceClient(t)
	defer secondTargetRelease()
	secondStream, err := secondTarget.WatchDesktopTray(t.Context(), &WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatalf("watch second target tray: %v", err)
	}
	if state := recvTrayState(t, secondStream); len(state.GetEntries()) != 0 {
		t.Fatalf("second target initial entries = %d, want 0", len(state.GetEntries()))
	}

	secondCtx, secondCancel := context.WithCancel(t.Context())
	secondErrCh := make(chan error, 1)
	go func() {
		secondErrCh <- ReconcileDesktopTray(secondCtx, source, secondTarget, secondTargetClient)
	}()

	state = recvTrayState(t, secondStream)
	if len(state.GetEntries()) != 1 || state.GetEntries()[0].GetLabel() != "My Drive" {
		t.Fatalf("second target mirrored entries = %#v", state.GetEntries())
	}

	secondCancel()
	if err := <-secondErrCh; err == nil {
		t.Fatalf("expected second reconciler to stop on context cancel")
	}
}

func newTestDesktopTrayResourceClient(
	t *testing.T,
) (*resource_client.Client, SRPCDesktopTrayResourceServiceClient, func()) {
	t.Helper()

	tray := NewDesktopTray()
	server := resource_server.NewResourceServer(tray.GetMux())
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	resourceService := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))),
	)
	client, err := resource_client.NewClient(t.Context(), resourceService)
	if err != nil {
		t.Fatalf("new resource client: %v", err)
	}
	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		t.Fatalf("root resource client: %v", err)
	}
	service := NewSRPCDesktopTrayResourceServiceClient(rootClient)
	return client, service, func() {
		rootRef.Release()
		client.Release()
	}
}

func recvTrayState(
	t *testing.T,
	strm SRPCDesktopTrayResourceService_WatchDesktopTrayClient,
) *DesktopTrayState {
	t.Helper()
	resp, err := strm.Recv()
	if err != nil {
		t.Fatalf("recv tray state: %v", err)
	}
	return resp.GetState()
}

// _ is a type assertion
var (
	_ resource_server.ResourceClientContext                 = ((*testResourceClientContext)(nil))
	_ SRPCDesktopTrayResourceService_WatchDesktopTrayStream = ((*testWatchStream)(nil))
	_ srpc.Client                                           = ((*testActionClient)(nil))
)
