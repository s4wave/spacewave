package resource_command

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
)

type fakeAttachedResourceClient struct {
	srpcClient srpc.Client
	err        error
}

func (f *fakeAttachedResourceClient) GetAttachedResource(id uint32) (srpc.Client, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.srpcClient, nil
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

func addRegistration(
	m *CommandsManager,
	resourceID uint32,
	commandID string,
	active bool,
	enabled bool,
	handlerClient srpc.Client,
) {
	m.registrations[resourceID] = &commandRegistration{
		resourceID:        resourceID,
		command:           &s4wave_command.Command{CommandId: commandID, Label: commandID},
		handlerResourceID: 1,
		client: &fakeAttachedResourceClient{
			srpcClient: handlerClient,
		},
		active:  active,
		enabled: enabled,
	}
}

func TestCommandsManagerInvokeCommandUsesActiveRegistration(t *testing.T) {
	mgr := NewCommandsManager()
	var calledArgs map[string]string

	addRegistration(
		mgr,
		1,
		"spacewave.session.settings",
		false,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				t.Fatalf("inactive handler was invoked")
				return nil
			},
		},
	)
	addRegistration(
		mgr,
		2,
		"spacewave.session.settings",
		true,
		true,
		&fakeCommandHandlerClient{
			handleCommand: func(req *s4wave_command_registry.HandleCommandRequest) error {
				calledArgs = req.GetArgs()
				return nil
			},
		},
	)

	_, err := mgr.InvokeCommand(context.Background(), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.session.settings",
		Args: map[string]string{
			"subItemId": "security",
		},
	})
	if err != nil {
		t.Fatalf("InvokeCommand returned error: %v", err)
	}
	if got := calledArgs["subItemId"]; got != "security" {
		t.Fatalf("expected active handler args, got %q", got)
	}
}

func TestCommandsManagerInvokeCommandRejectsMultipleActiveRegistrations(t *testing.T) {
	mgr := NewCommandsManager()

	addRegistration(mgr, 1, "spacewave.session.settings", true, true, &fakeCommandHandlerClient{})
	addRegistration(mgr, 2, "spacewave.session.settings", true, true, &fakeCommandHandlerClient{})

	_, err := mgr.InvokeCommand(context.Background(), &s4wave_command_registry.InvokeCommandRequest{
		CommandId: "spacewave.session.settings",
	})
	if !errors.Is(err, ErrMultipleActiveRegistrations) {
		t.Fatalf("expected ErrMultipleActiveRegistrations, got %v", err)
	}
}

func TestCommandsManagerGetSubItemsUsesActiveRegistration(t *testing.T) {
	mgr := NewCommandsManager()

	addRegistration(
		mgr,
		1,
		"spacewave.nav.go-to-space",
		false,
		true,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				t.Fatalf("inactive sub-item provider was queried")
				return nil, nil
			},
		},
	)
	addRegistration(
		mgr,
		2,
		"spacewave.nav.go-to-space",
		true,
		true,
		&fakeCommandHandlerClient{
			getSubItems: func(req *s4wave_command_registry.GetSubItemsRequest) ([]*s4wave_command_registry.CommandSubItem, error) {
				return []*s4wave_command_registry.CommandSubItem{{
					Id:    "docs",
					Label: "Docs",
				}}, nil
			},
		},
	)

	resp, err := mgr.GetSubItems(context.Background(), &s4wave_command_registry.GetSubItemsRequest{
		CommandId: "spacewave.nav.go-to-space",
		Query:     "do",
	})
	if err != nil {
		t.Fatalf("GetSubItems returned error: %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetId() != "docs" {
		t.Fatalf("unexpected sub-items: %#v", resp.GetItems())
	}
}

func TestCommandsManagerSetActiveAndEnabledByResourceID(t *testing.T) {
	mgr := NewCommandsManager()
	addRegistration(mgr, 7, "spacewave.session.settings", false, true, &fakeCommandHandlerClient{})

	if _, err := mgr.SetActive(context.Background(), &s4wave_command_registry.SetActiveRequest{
		ResourceId: 7,
		Active:     true,
	}); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}
	if !mgr.registrations[7].active {
		t.Fatalf("expected registration to be active")
	}

	if _, err := mgr.SetEnabled(context.Background(), &s4wave_command_registry.SetEnabledRequest{
		ResourceId: 7,
		Enabled:    false,
	}); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}
	if mgr.registrations[7].enabled {
		t.Fatalf("expected registration to be disabled")
	}
}

func TestCommandsManagerGetCommandStatesLocked(t *testing.T) {
	mgr := NewCommandsManager()
	addRegistration(mgr, 9, "spacewave.zeta", true, false, &fakeCommandHandlerClient{})
	addRegistration(mgr, 3, "spacewave.alpha", false, true, &fakeCommandHandlerClient{})

	var states []*s4wave_command_registry.CommandState
	mgr.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		states = mgr.getCommandStatesLocked()
	})

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
