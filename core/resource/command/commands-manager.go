package resource_command

import (
	"context"
	"sort"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
)

// attachedResourceClient resolves attached command handler resources.
type attachedResourceClient interface {
	GetAttachedResource(id uint32) (srpc.Client, error)
}

// commandRegistration holds a registered command and its associated handler.
type commandRegistration struct {
	resourceID        uint32
	command           *s4wave_command.Command
	handlerResourceID uint32
	client            attachedResourceClient
	active            bool
	enabled           bool
}

// CommandsManager provides an in-memory command registry.
// Plugins register commands via RegisterCommand and watch for changes via WatchCommands.
type CommandsManager struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	registrations map[uint32]*commandRegistration
}

// NewCommandsManager creates a new CommandsManager.
func NewCommandsManager() *CommandsManager {
	r := &CommandsManager{
		registrations: make(map[uint32]*commandRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_command_registry.SRPCRegisterCommandRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *CommandsManager) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterCommand registers a command with an optional handler.
func (r *CommandsManager) RegisterCommand(
	ctx context.Context,
	req *s4wave_command_registry.RegisterCommandRequest,
) (*s4wave_command_registry.RegisterCommandResponse, error) {
	cmd := req.GetCommand()
	if cmd == nil {
		return nil, ErrCommandRequired
	}
	cmdID := cmd.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	reg := &commandRegistration{
		command:           cmd,
		handlerResourceID: req.GetHandlerResourceId(),
		client:            client,
		enabled:           true,
	}

	emptyMux := srpc.NewMux()
	var released bool
	var resourceID uint32
	resourceID, err = client.AddResource(emptyMux, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			released = true
			if _, ok := r.registrations[resourceID]; !ok {
				return
			}
			delete(r.registrations, resourceID)
			broadcast()
		})
	})
	if err != nil {
		return nil, err
	}

	reg.resourceID = resourceID
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if released {
			return
		}
		r.registrations[resourceID] = reg
		broadcast()
	})
	if released {
		return nil, resource.ErrClientReleased
	}

	return &s4wave_command_registry.RegisterCommandResponse{
		ResourceId: resourceID,
	}, nil
}

// SetActive sets the active state of a registration.
func (r *CommandsManager) SetActive(
	ctx context.Context,
	req *s4wave_command_registry.SetActiveRequest,
) (*s4wave_command_registry.SetActiveResponse, error) {
	resourceID := req.GetResourceId()
	if resourceID == 0 {
		return nil, ErrResourceIdRequired
	}

	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil {
			return
		}
		if reg.active == req.GetActive() {
			found = true
			return
		}
		reg.active = req.GetActive()
		found = true
		broadcast()
	})
	if !found {
		return nil, ErrRegistrationNotFound
	}

	return &s4wave_command_registry.SetActiveResponse{}, nil
}

// SetEnabled sets the enabled state of a registration.
func (r *CommandsManager) SetEnabled(
	ctx context.Context,
	req *s4wave_command_registry.SetEnabledRequest,
) (*s4wave_command_registry.SetEnabledResponse, error) {
	resourceID := req.GetResourceId()
	if resourceID == 0 {
		return nil, ErrResourceIdRequired
	}

	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil {
			return
		}
		if reg.enabled == req.GetEnabled() {
			found = true
			return
		}
		reg.enabled = req.GetEnabled()
		found = true
		broadcast()
	})
	if !found {
		return nil, ErrRegistrationNotFound
	}

	return &s4wave_command_registry.SetEnabledResponse{}, nil
}

// GetSubItems returns sub-items for the active registration of a command.
func (r *CommandsManager) GetSubItems(
	ctx context.Context,
	req *s4wave_command_registry.GetSubItemsRequest,
) (*s4wave_command_registry.GetSubItemsResponse, error) {
	cmdID := req.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	reg, err := r.getActiveRegistration(cmdID)
	if err != nil {
		return nil, err
	}
	if reg.handlerResourceID == 0 {
		return nil, ErrNoHandler
	}

	attachedClient, err := reg.client.GetAttachedResource(reg.handlerResourceID)
	if err != nil {
		return nil, err
	}

	handler := s4wave_command_registry.NewSRPCCommandHandlerServiceClient(attachedClient)
	return handler.GetSubItems(ctx, &s4wave_command_registry.GetSubItemsRequest{
		CommandId: cmdID,
		Query:     req.GetQuery(),
	})
}

// WatchCommands streams the full command registry with active state.
func (r *CommandsManager) WatchCommands(
	req *s4wave_command_registry.WatchCommandsRequest,
	strm s4wave_command_registry.SRPCCommandRegistryResourceService_WatchCommandsStream,
) error {
	ctx := strm.Context()

	for {
		var states []*s4wave_command_registry.CommandState
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			states = r.getCommandStatesLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_command_registry.WatchCommandsResponse{
			Commands: states,
		}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// InvokeCommand invokes a registered command.
func (r *CommandsManager) InvokeCommand(
	ctx context.Context,
	req *s4wave_command_registry.InvokeCommandRequest,
) (*s4wave_command_registry.InvokeCommandResponse, error) {
	cmdID := req.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	reg, err := r.getActiveRegistration(cmdID)
	if err != nil {
		return nil, err
	}
	if reg.handlerResourceID == 0 {
		return nil, ErrNoHandler
	}

	attachedClient, err := reg.client.GetAttachedResource(reg.handlerResourceID)
	if err != nil {
		return nil, err
	}

	handler := s4wave_command_registry.NewSRPCCommandHandlerServiceClient(attachedClient)
	_, err = handler.HandleCommand(ctx, &s4wave_command_registry.HandleCommandRequest{
		CommandId: cmdID,
		Args:      req.GetArgs(),
	})
	if err != nil {
		return nil, err
	}

	return &s4wave_command_registry.InvokeCommandResponse{}, nil
}

// getCommandStatesLocked builds CommandState entries from registrations.
// Must be called with bcast lock held.
func (r *CommandsManager) getCommandStatesLocked() []*s4wave_command_registry.CommandState {
	regs := make([]*commandRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		if reg == nil || reg.command == nil {
			continue
		}
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(i, j int) bool {
		leftID := regs[i].command.GetCommandId()
		rightID := regs[j].command.GetCommandId()
		if leftID != rightID {
			return leftID < rightID
		}
		return regs[i].resourceID < regs[j].resourceID
	})

	states := make([]*s4wave_command_registry.CommandState, 0, len(regs))
	for _, reg := range regs {
		states = append(states, &s4wave_command_registry.CommandState{
			ResourceId: reg.resourceID,
			Command:    reg.command,
			Active:     reg.active,
			Enabled:    reg.enabled,
		})
	}
	return states
}

// getActiveRegistration returns the single active registration for a command.
func (r *CommandsManager) getActiveRegistration(
	cmdID string,
) (*commandRegistration, error) {
	var reg *commandRegistration
	var err error

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, candidate := range r.registrations {
			if candidate == nil || candidate.command == nil {
				continue
			}
			if candidate.command.GetCommandId() != cmdID || !candidate.active {
				continue
			}
			if reg != nil {
				err = ErrMultipleActiveRegistrations
				return
			}
			reg = candidate
		}
	})
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, ErrCommandNotFound
	}
	return reg, nil
}

// _ is a type assertion
var _ s4wave_command_registry.SRPCCommandRegistryResourceServiceServer = (*CommandsManager)(nil)
