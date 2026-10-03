package resource_command

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
)

type fakeAttachedResourceClient struct {
	ctx         context.Context
	srpcClients map[uint32]srpc.Client
}

func newFakeAttachedResourceClient(t *testing.T) *fakeAttachedResourceClient {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	return &fakeAttachedResourceClient{
		ctx:         ctx,
		srpcClients: make(map[uint32]srpc.Client),
	}
}

func (f *fakeAttachedResourceClient) Context() context.Context {
	return f.ctx
}

func (f *fakeAttachedResourceClient) AddResource(srpc.Invoker, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (f *fakeAttachedResourceClient) AddResourceValue(srpc.Invoker, any, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (f *fakeAttachedResourceClient) ReleaseResource(uint32) bool {
	return false
}

func (f *fakeAttachedResourceClient) GetResourceValue(uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

func (f *fakeAttachedResourceClient) GetAttachedResource(id uint32) (srpc.Client, error) {
	client := f.srpcClients[id]
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}

type fakeCommandHandlerClient struct {
	handleCommand func(*s4wave_command_registry.HandleCommandRequest) error
	getSubItems   func(*s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error)
}

func (f *fakeCommandHandlerClient) ExecCall(
	ctx context.Context,
	service string,
	method string,
	in srpc.Message,
	out srpc.Message,
) error {
	switch method {
	case "HandleCommand":
		req, ok := in.(*s4wave_command_registry.HandleCommandRequest)
		if !ok {
			return errors.New("unexpected HandleCommand request type")
		}
		if f.handleCommand != nil {
			if err := f.handleCommand(req); err != nil {
				return err
			}
		}
		resp, ok := out.(*s4wave_command_registry.HandleCommandResponse)
		if !ok {
			return errors.New("unexpected HandleCommand response type")
		}
		*resp = s4wave_command_registry.HandleCommandResponse{}
		return nil
	case "GetSubItems":
		req, ok := in.(*s4wave_command_registry.GetSubItemsRequest)
		if !ok {
			return errors.New("unexpected GetSubItems request type")
		}
		resp, ok := out.(*s4wave_command_registry.GetSubItemsResponse)
		if !ok {
			return errors.New("unexpected GetSubItems response type")
		}
		var items []*s4wave_command_registry.CommandSubItem
		if f.getSubItems != nil {
			var err error
			items, err = f.getSubItems(req)
			if err != nil {
				return err
			}
		}
		*resp = s4wave_command_registry.GetSubItemsResponse{Items: items}
		return nil
	default:
		return srpc.ErrUnimplemented
	}
}

func (f *fakeCommandHandlerClient) NewStream(
	ctx context.Context,
	service string,
	method string,
	firstMsg srpc.Message,
) (srpc.Stream, error) {
	return nil, errors.New("unexpected streaming call")
}

type fakeWatchCommandsStream struct {
	srpc.Stream
	ctx      context.Context
	cancel   context.CancelFunc
	response *s4wave_command_registry.WatchCommandsResponse
}

func newFakeWatchCommandsStream(
	client resource_server.ResourceClientContext,
) *fakeWatchCommandsStream {
	ctx, cancel := context.WithCancel(resource_server.WithResourceClientContext(
		context.Background(),
		client,
	))
	return &fakeWatchCommandsStream{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (f *fakeWatchCommandsStream) Context() context.Context {
	return f.ctx
}

func (f *fakeWatchCommandsStream) Send(resp *s4wave_command_registry.WatchCommandsResponse) error {
	f.response = resp
	f.cancel()
	return nil
}

func (f *fakeWatchCommandsStream) SendAndClose(resp *s4wave_command_registry.WatchCommandsResponse) error {
	return f.Send(resp)
}

func addRegistration(
	t *testing.T,
	m *CommandsManager,
	resourceID uint32,
	commandID string,
	surface s4wave_command.CommandSurface,
	active bool,
	enabled bool,
	handlerClient srpc.Client,
) *fakeAttachedResourceClient {
	client := newFakeAttachedResourceClient(t)
	addRegistrationForClient(m, resourceID, commandID, surface, active, enabled, client, handlerClient)
	return client
}

func addRegistrationForClient(
	m *CommandsManager,
	resourceID uint32,
	commandID string,
	surface s4wave_command.CommandSurface,
	active bool,
	enabled bool,
	client *fakeAttachedResourceClient,
	handlerClient srpc.Client,
) {
	client.srpcClients[resourceID] = handlerClient
	m.registrations[resourceID] = &commandRegistration{
		resourceID:        resourceID,
		command:           &s4wave_command.Command{CommandId: commandID, Label: commandID},
		surface:           surface,
		handlerResourceID: resourceID,
		client:            client,
		clientDone:        resourceClientDone(client),
		active:            active,
		enabled:           enabled,
	}
}

func commandClientContext(client resource_server.ResourceClientContext) context.Context {
	return resource_server.WithResourceClientContext(context.Background(), client)
}

func TestCommandsManagerInvokeCommandUsesActiveRegistration(t *testing.T) {
	// Create a command registry and capture its handler arguments.
	mgr := NewCommandsManager()
	var calledArgs map[string]string

	// Register inactive and active handlers for the same client command.
	client := addRegistration(t, mgr,
		1,
		"spacewave.session.settings",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		false,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				t.Fatalf("inactive handler was invoked")
				return nil
			},
		})
	addRegistrationForClient(
		mgr,
		2,
		"spacewave.session.settings",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		client,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				calledArgs = req.GetArgs()
				return nil
			},
		},
	)

	// Invoke the active command registration with the selected sub-item.
	_, err := mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.session.settings",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		Args: map[string]string{
			"subItemId": "security",
		},
	})
	if err != nil {
		t.Fatalf("InvokeCommand returned error: %v", err)
	}

	// Require the active handler to receive the selected sub-item.
	if got := calledArgs["subItemId"]; got != "security" {
		t.Fatalf("expected active handler args, got %q", got)
	}
}

func TestCommandsManagerInvokeCommandRejectsMultipleActiveRegistrationsOnSameSurface(t *testing.T) {
	// Create a command registry for conflicting active registrations.
	mgr := NewCommandsManager()

	// Register two active handlers for the same client and surface.
	client := addRegistration(t, mgr, 1, "spacewave.session.settings", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, &fakeCommandHandlerClient{})
	addRegistrationForClient(mgr, 2, "spacewave.session.settings", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, client, &fakeCommandHandlerClient{})

	// Invoke the command and require a multiple-registration error.
	_, err := mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.session.settings",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
	})
	if !errors.Is(err, ErrMultipleActiveRegistrations) {
		t.Fatalf("expected ErrMultipleActiveRegistrations, got %v", err)
	}
}

func TestCommandsManagerInvokeCommandScopesRegistrationsByClient(t *testing.T) {
	// Create a command registry and capture the invoked client handler.
	mgr := NewCommandsManager()
	var called string

	// Register separate handlers for clients A and B.
	clientA := addRegistration(t, mgr,
		1,
		"spacewave.file.close-space",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(*s4wave_command_registry.HandleCommandRequest) error {
				called = "a"
				return nil
			},
		})
	clientB := addRegistration(t, mgr,
		2,
		"spacewave.file.close-space",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(*s4wave_command_registry.HandleCommandRequest) error {
				called = "b"
				return nil
			},
		})

	// Invoke the shared command identifier through client A.
	req := &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.file.close-space",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
	}
	if _, err := mgr.InvokeCommand(commandClientContext(clientA), req); err != nil {
		t.Fatalf("client A InvokeCommand returned error: %v", err)
	}

	// Require client A's handler to receive the command.
	if called != "a" {
		t.Fatalf("expected client A handler, got %q", called)
	}

	// Invoke the shared command identifier through client B.
	called = ""
	if _, err := mgr.InvokeCommand(commandClientContext(clientB), req); err != nil {
		t.Fatalf("client B InvokeCommand returned error: %v", err)
	}

	// Require client B's handler to receive the command.
	if called != "b" {
		t.Fatalf("expected client B handler, got %q", called)
	}
}

func TestCommandsManagerInvokeCommandSelectsRegistrationBySurface(t *testing.T) {
	// Create a command registry and capture the invoked surface.
	mgr := NewCommandsManager()
	var invokedSurface s4wave_command.CommandSurface

	// Register web and terminal handlers for the same client command.
	client := addRegistration(t, mgr,
		1,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				invokedSurface = s4wave_command.CommandSurface_COMMAND_SURFACE_WEB
				return nil
			},
		})
	addRegistrationForClient(
		mgr,
		2,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
		true,
		true,
		client,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				invokedSurface = s4wave_command.CommandSurface_COMMAND_SURFACE_TUI
				return nil
			},
		},
	)

	// Invoke the command on the web surface.
	_, err := mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.object.open",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
	})
	if err != nil {
		t.Fatalf("web InvokeCommand returned error: %v", err)
	}

	// Require the web handler to receive the command.
	if invokedSurface != s4wave_command.CommandSurface_COMMAND_SURFACE_WEB {
		t.Fatalf("expected web handler, got %v", invokedSurface)
	}

	// Invoke the command on the terminal surface.
	invokedSurface = s4wave_command.CommandSurface_COMMAND_SURFACE_UNKNOWN
	_, err = mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.object.open",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
	})
	if err != nil {
		t.Fatalf("terminal InvokeCommand returned error: %v", err)
	}

	// Require the terminal handler to receive the command.
	if invokedSurface != s4wave_command.CommandSurface_COMMAND_SURFACE_TUI {
		t.Fatalf("expected terminal handler, got %v", invokedSurface)
	}
}

func TestCommandsManagerInvokeCommandSelectsWebSurface(t *testing.T) {
	// Create a command registry and track web handler invocation.
	mgr := NewCommandsManager()
	var webInvoked bool

	// Register web and terminal handlers for the same client command.
	client := addRegistration(t, mgr,
		1,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				webInvoked = true
				return nil
			},
		})
	addRegistrationForClient(
		mgr,
		2,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
		true,
		true,
		client,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				t.Fatal("terminal handler was invoked for unspecified surface")
				return nil
			},
		},
	)

	// Invoke the command using its web registration.
	_, err := mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.object.open",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
	})
	if err != nil {
		t.Fatalf("InvokeCommand returned error: %v", err)
	}

	// Require the web handler to receive the command.
	if !webInvoked {
		t.Fatal("expected unspecified surface to invoke web handler")
	}
}

func TestCommandsManagerRejectsUnknownSurface(t *testing.T) {
	mgr := NewCommandsManager()

	_, err := mgr.InvokeCommand(context.Background(), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.object.open",
		Surface:   s4wave_command.CommandSurface(99),
	})
	if !errors.Is(err, ErrInvalidCommandSurface) {
		t.Fatalf("expected ErrInvalidCommandSurface, got %v", err)
	}
}

func TestCommandsManagerWatchCommandsFiltersSurface(t *testing.T) {
	// Create command registrations across surfaces and client sessions.
	mgr := NewCommandsManager()
	client := addRegistration(t, mgr, 1, "spacewave.object.open", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, &fakeCommandHandlerClient{})
	addRegistrationForClient(mgr, 2, "spacewave.object.open", s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, true, true, client, &fakeCommandHandlerClient{})
	addRegistration(t, mgr, 3, "spacewave.object.open", s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, true, true, &fakeCommandHandlerClient{})
	strm := newFakeWatchCommandsStream(client)

	// Watch the calling client's terminal command registrations.
	err := mgr.WatchCommands(
		&s4wave_command_registry.WatchCommandsRequest{
			Surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
		},
		strm,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchCommands returned error: %v", err)
	}

	// Require a snapshot containing only the calling client's terminal registration.
	if strm.response == nil {
		t.Fatal("WatchCommands returned no response")
	}
	states := strm.response.GetCommands()
	if len(states) != 1 {
		t.Fatalf("expected one terminal state, got %d", len(states))
	}
	if states[0].GetResourceId() != 2 {
		t.Fatalf("expected terminal registration, got resource %d", states[0].GetResourceId())
	}
	if states[0].GetSurface() != s4wave_command.CommandSurface_COMMAND_SURFACE_TUI {
		t.Fatalf("expected terminal state surface, got %v", states[0].GetSurface())
	}
}

func TestCommandsManagerGetSubItemsFiltersSurface(t *testing.T) {
	// Create sub-item providers across surfaces and client sessions.
	mgr := NewCommandsManager()
	client := addRegistration(t, mgr,
		1,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				t.Fatal("web sub-item provider was queried for terminal surface")
				return nil, nil
			},
		})
	addRegistrationForClient(
		mgr,
		2,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
		true,
		true,
		client,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				if req.GetSurface() != s4wave_command.CommandSurface_COMMAND_SURFACE_TUI {
					t.Fatalf("expected terminal sub-item request, got %v", req.GetSurface())
				}
				return []*s4wave_command_registry.CommandSubItem{{
					Id:    "terminal-object",
					Label: "Terminal Object",
				}}, nil
			},
		},
	)
	addRegistration(t, mgr,
		3,
		"spacewave.object.open",
		s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
		true,
		true,
		&fakeCommandHandlerClient{
			getSubItems: func(*s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				t.Fatal("another client's sub-item provider was queried")
				return nil, nil
			},
		})

	// Query sub-items for the calling client's terminal registration.
	resp, err := mgr.GetSubItems(commandClientContext(client), &s4wave_command_registry.GetSubItemsRequest{
		CommandId: "spacewave.object.open",
		Query:     "terminal",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_TUI,
	})
	if err != nil {
		t.Fatalf("GetSubItems returned error: %v", err)
	}

	// Require the terminal provider's sole sub-item.
	items := resp.GetItems()
	if len(items) != 1 || items[0].GetId() != "terminal-object" {
		t.Fatalf("unexpected sub-items: %#v", items)
	}
}

func TestCommandsManagerGetSubItemsUsesActiveRegistration(t *testing.T) {
	// Create a command registry for active sub-item selection.
	mgr := NewCommandsManager()

	// Register inactive and active sub-item providers for the same client.
	client := addRegistration(t, mgr,
		1,
		"spacewave.nav.go-to-space",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		false,
		true,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				t.Fatalf("inactive sub-item provider was queried")
				return nil, nil
			},
		})
	addRegistrationForClient(
		mgr,
		2,
		"spacewave.nav.go-to-space",
		s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		true,
		true,
		client,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				return []*s4wave_command_registry.CommandSubItem{{
					Id:    "docs",
					Label: "Docs",
				}}, nil
			},
		},
	)

	// Query the active provider for matching command sub-items.
	resp, err := mgr.GetSubItems(commandClientContext(client), &s4wave_command_registry.GetSubItemsRequest{
		CommandId: "spacewave.nav.go-to-space",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
		Query:     "do",
	})
	if err != nil {
		t.Fatalf("GetSubItems returned error: %v", err)
	}

	// Require the active provider's matching sub-item.
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetId() != "docs" {
		t.Fatalf("unexpected sub-items: %#v", resp.GetItems())
	}
}

func TestCommandsManagerSetActiveAndEnabledByResourceID(t *testing.T) {
	// Create registrations belonging to two separate client sessions.
	mgr := NewCommandsManager()
	client := addRegistration(t, mgr, 7, "spacewave.session.settings", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, false, true, &fakeCommandHandlerClient{})
	otherClient := addRegistration(t, mgr, 8, "spacewave.session.settings", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, &fakeCommandHandlerClient{})

	// Activate the calling client's registration.
	if _, err := mgr.SetActive(commandClientContext(client), &s4wave_command_registry.SetActiveRequest{
		ResourceId: 7,
		Active:     true,
	}); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}

	// Require the client registration to remain active.
	if !mgr.registrations[7].active {
		t.Fatalf("expected registration to be active")
	}

	// Disable the calling client's registration.
	if _, err := mgr.SetEnabled(commandClientContext(client), &s4wave_command_registry.SetEnabledRequest{
		ResourceId: 7,
		Enabled:    false,
	}); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}

	// Require the client registration to remain disabled.
	if mgr.registrations[7].enabled {
		t.Fatalf("expected registration to be disabled")
	}

	// Attempt to deactivate the registration through another client.
	_, err := mgr.SetActive(commandClientContext(otherClient), &s4wave_command_registry.SetActiveRequest{
		ResourceId: 7,
		Active:     false,
	})
	if !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("expected ErrRegistrationNotFound from another client, got %v", err)
	}

	// Require the client registration to remain active.
	if !mgr.registrations[7].active {
		t.Fatal("expected registration to remain active")
	}

	// Attempt to enable the registration through another client.
	_, err = mgr.SetEnabled(commandClientContext(otherClient), &s4wave_command_registry.SetEnabledRequest{
		ResourceId: 7,
		Enabled:    true,
	})
	if !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("expected ErrRegistrationNotFound from another client, got %v", err)
	}

	// Require the client registration to remain disabled.
	if mgr.registrations[7].enabled {
		t.Fatal("expected registration to remain disabled")
	}
}

func TestCommandsManagerSetActiveDeactivatesMatchingRegistrationsForClient(t *testing.T) {
	// Create matching registrations across clients and command surfaces.
	mgr := NewCommandsManager()
	client := addRegistration(t, mgr, 7, "spacewave.file.close-space", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, &fakeCommandHandlerClient{})
	addRegistrationForClient(mgr, 8, "spacewave.file.close-space", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, false, true, client, &fakeCommandHandlerClient{})
	addRegistrationForClient(mgr, 9, "spacewave.file.close-space", s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, true, true, client, &fakeCommandHandlerClient{})
	addRegistration(t, mgr, 10, "spacewave.file.close-space", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, true, &fakeCommandHandlerClient{})

	// Activate the second web registration for the calling client.
	if _, err := mgr.SetActive(commandClientContext(client), &s4wave_command_registry.SetActiveRequest{
		ResourceId: 8,
		Active:     true,
	}); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}

	// Require only the selected matching web registration to replace the prior one.
	if mgr.registrations[7].active {
		t.Fatal("expected prior web registration to be inactive")
	}
	if !mgr.registrations[8].active {
		t.Fatal("expected selected web registration to be active")
	}
	if !mgr.registrations[9].active {
		t.Fatal("expected terminal registration to remain active")
	}
	if !mgr.registrations[10].active {
		t.Fatal("expected other client registration to remain active")
	}

	// Invoke the command through the selected web registration.
	if _, err := mgr.InvokeCommand(commandClientContext(client), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.file.close-space",
		Surface:   s4wave_command.CommandSurface_COMMAND_SURFACE_WEB,
	}); err != nil {
		t.Fatalf("InvokeCommand returned error: %v", err)
	}
}

func TestCommandsManagerGetCommandStatesLocked(t *testing.T) {
	// Create registrations with distinct command identifiers and states.
	mgr := NewCommandsManager()
	client := addRegistration(t, mgr, 9, "spacewave.zeta", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, true, false, &fakeCommandHandlerClient{})
	addRegistrationForClient(mgr, 3, "spacewave.alpha", s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, false, true, client, &fakeCommandHandlerClient{})

	// Read the command-state snapshot under the registry lock.
	var states []*s4wave_command_registry.CommandState
	mgr.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		states = mgr.getCommandStatesLocked(s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, client)
	})

	// Require the command states in identifier order with their registered flags.
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if states[0].GetCommand().GetCommandId() != "spacewave.alpha" || states[0].GetResourceId() != 3 {
		t.Fatalf("unexpected first state: %#v", states[0])
	}
	if states[0].GetActive() {
		t.Fatalf("expected first state to be inactive")
	}
	if !states[0].GetEnabled() {
		t.Fatalf("expected first state to be enabled")
	}
	if states[1].GetCommand().GetCommandId() != "spacewave.zeta" || states[1].GetResourceId() != 9 {
		t.Fatalf("unexpected second state: %#v", states[1])
	}
	if !states[1].GetActive() {
		t.Fatalf("expected second state to be active")
	}
	if states[1].GetEnabled() {
		t.Fatalf("expected second state to be disabled")
	}
}

func TestValidateCommandDefaultBindingSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name         string
		registration s4wave_command.CommandSurface
		binding      s4wave_command.CommandSurface
		wantErr      bool
	}{
		{name: "web", registration: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, binding: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB},
		{name: "tui", registration: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, binding: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI},
		{name: "missing", registration: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, binding: s4wave_command.CommandSurface_COMMAND_SURFACE_UNKNOWN, wantErr: true},
		{name: "cross", registration: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, binding: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, wantErr: true},
		{name: "invalid", registration: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, binding: s4wave_command.CommandSurface(99), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := &s4wave_command.Command{DefaultBindings: []*s4wave_command.CommandBinding{{Surface: tc.binding}}}
			err := validateCommandDefaultBindingSurfaces(command, tc.registration)
			if got := errors.Is(err, ErrInvalidDefaultBindingSurface); got != tc.wantErr {
				t.Fatalf("error = %v, want invalid surface = %v", err, tc.wantErr)
			}
		})
	}
}
