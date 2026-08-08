package resource_command

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
)

// commandRegistration holds a registered command and its associated handler.
type commandRegistration struct {
	resourceID        uint32
	command           *s4wave_command.Command
	surface           s4wave_command.CommandSurface
	handlerResourceID uint32
	client            resource_server.ResourceClientContext
	clientDone        <-chan struct{}
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

	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}
	if err := validateCommandDefaultBindingSurfaces(cmd, surface); err != nil {
		return nil, err
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	reg := &commandRegistration{
		command:           cmd,
		surface:           surface,
		handlerResourceID: req.GetHandlerResourceId(),
		client:            client,
		clientDone:        resourceClientDone(client),
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

// SetActive sets a registration active and deactivates active registrations
// from the same client with the same command ID and surface.
func (r *CommandsManager) SetActive(
	ctx context.Context,
	req *s4wave_command_registry.SetActiveRequest,
) (*s4wave_command_registry.SetActiveResponse, error) {
	resourceID := req.GetResourceId()
	if resourceID == 0 {
		return nil, ErrResourceIdRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil || !registrationMatchesClient(reg, client) {
			return
		}

		active := req.GetActive()
		changed := reg.active != active
		if active {
			for _, candidate := range r.registrations {
				if candidate == nil ||
					candidate == reg ||
					candidate.command == nil ||
					candidate.command.GetCommandId() != reg.command.GetCommandId() ||
					!sameRegisteredClient(candidate, reg) ||
					candidate.surface != reg.surface ||
					!candidate.active {
					continue
				}
				candidate.active = false
				changed = true
			}
		}
		reg.active = active
		found = true
		if changed {
			broadcast()
		}
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

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		reg := r.registrations[resourceID]
		if reg == nil || !registrationMatchesClient(reg, client) {
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

	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	reg, err := r.getActiveRegistration(cmdID, surface, client)
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
		Surface:   surface,
	})
}

// WatchCommands streams the calling client's command registry with active state.
func (r *CommandsManager) WatchCommands(
	req *s4wave_command_registry.WatchCommandsRequest,
	strm s4wave_command_registry.SRPCCommandRegistryResourceService_WatchCommandsStream,
) error {
	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return err
	}

	ctx := strm.Context()
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return err
	}

	for {
		var states []*s4wave_command_registry.CommandState
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			states = r.getCommandStatesLocked(surface, client)
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

	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	reg, err := r.getActiveRegistration(cmdID, surface, client)
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

// getCommandStatesLocked builds CommandState entries for one client.
// Must be called with bcast lock held.
func (r *CommandsManager) getCommandStatesLocked(
	surface s4wave_command.CommandSurface,
	client resource_server.ResourceClientContext,
) []*s4wave_command_registry.CommandState {
	regs := make([]*commandRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		if reg == nil || reg.command == nil {
			continue
		}
		if reg.surface != surface {
			continue
		}
		if !registrationMatchesClient(reg, client) {
			continue
		}
		regs = append(regs, reg)
	}
	slices.SortFunc(regs, func(a, b *commandRegistration) int {
		if c := strings.Compare(a.command.GetCommandId(), b.command.GetCommandId()); c != 0 {
			return c
		}
		return cmp.Compare(a.resourceID, b.resourceID)
	})

	states := make([]*s4wave_command_registry.CommandState, 0, len(regs))
	for _, reg := range regs {
		states = append(states, &s4wave_command_registry.CommandState{
			ResourceId: reg.resourceID,
			Command:    reg.command,
			Active:     reg.active,
			Enabled:    reg.enabled,
			Surface:    reg.surface,
		})
	}
	return states
}

// getActiveRegistration returns one client's active registration for a command surface.
func (r *CommandsManager) getActiveRegistration(
	cmdID string,
	surface s4wave_command.CommandSurface,
	client resource_server.ResourceClientContext,
) (*commandRegistration, error) {
	var reg *commandRegistration
	var err error

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, candidate := range r.registrations {
			if candidate == nil || candidate.command == nil {
				continue
			}
			if candidate.command.GetCommandId() != cmdID ||
				candidate.surface != surface ||
				!registrationMatchesClient(candidate, client) ||
				!candidate.active {
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

// resourceClientDone returns the stable client-session cancellation channel.
func resourceClientDone(client resource_server.ResourceClientContext) <-chan struct{} {
	if client == nil {
		return nil
	}
	ctx := client.Context()
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

func sameRegisteredClient(a, b *commandRegistration) bool {
	return a.clientDone != nil && a.clientDone == b.clientDone
}

func registrationMatchesClient(
	reg *commandRegistration,
	client resource_server.ResourceClientContext,
) bool {
	return reg.clientDone != nil && reg.clientDone == resourceClientDone(client)
}

func validateCommandDefaultBindingSurfaces(
	command *s4wave_command.Command,
	surface s4wave_command.CommandSurface,
) error {
	for _, binding := range command.GetDefaultBindings() {
		if binding.GetSurface() != surface {
			return ErrInvalidDefaultBindingSurface
		}
	}
	return nil
}

func normalizeCommandSurface(
	surface s4wave_command.CommandSurface,
) (s4wave_command.CommandSurface, error) {
	switch surface {
	case s4wave_command.CommandSurface_COMMAND_SURFACE_WEB:
		return s4wave_command.CommandSurface_COMMAND_SURFACE_WEB, nil
	case s4wave_command.CommandSurface_COMMAND_SURFACE_TUI:
		return s4wave_command.CommandSurface_COMMAND_SURFACE_TUI, nil
	default:
		return s4wave_command.CommandSurface_COMMAND_SURFACE_UNKNOWN, ErrInvalidCommandSurface
	}
}

// _ is a type assertion
var _ s4wave_command_registry.SRPCCommandRegistryResourceServiceServer = (*CommandsManager)(nil)
