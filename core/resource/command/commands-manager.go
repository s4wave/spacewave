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
	// Create the command registry and expose its RPC service.
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
	// Require a command and its identifier before registering it.
	cmd := req.GetCommand()
	if cmd == nil {
		return nil, ErrCommandRequired
	}
	cmdID := cmd.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	// Validate the command surface and its default bindings.
	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}
	if err := validateCommandDefaultBindingSurfaces(cmd, surface); err != nil {
		return nil, err
	}

	// Resolve the client session that will hold the registration.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Prepare an enabled registration bound to the client session.
	reg := &commandRegistration{
		command:           cmd,
		surface:           surface,
		handlerResourceID: req.GetHandlerResourceId(),
		client:            client,
		clientDone:        resourceClientDone(client),
		enabled:           true,
	}

	// Allocate a client resource whose release removes the registration.
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

	// Publish the registration unless its client resource was already released.
	reg.resourceID = resourceID
	var wasReleased bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		wasReleased = released
		if wasReleased {
			return
		}
		r.registrations[resourceID] = reg
		broadcast()
	})
	if wasReleased {
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
	// Require the resource identifier of the registration to activate.
	resourceID := req.GetResourceId()
	if resourceID == 0 {
		return nil, ErrResourceIdRequired
	}

	// Resolve the client session requesting the active-state change.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Update the client registration under the registry lock.
	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Find the registration belonging to the requesting client.
		reg := r.registrations[resourceID]
		if reg == nil || !registrationMatchesClient(reg, client) {
			return
		}

		// Deactivate matching registrations before applying the requested active state.
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
	// Require the resource identifier of the registration to enable.
	resourceID := req.GetResourceId()
	if resourceID == 0 {
		return nil, ErrResourceIdRequired
	}

	// Resolve the client session requesting the enabled-state change.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Update the client registration under the registry lock.
	var found bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Find the client registration and publish any enabled-state change.
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
	// Require the command identifier for the sub-item query.
	cmdID := req.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	// Validate the requested command surface.
	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}

	// Resolve the client session requesting the sub-items.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Find the active registration with a sub-item handler.
	reg, err := r.getActiveRegistration(cmdID, surface, client)
	if err != nil {
		return nil, err
	}
	if reg.handlerResourceID == 0 {
		return nil, ErrNoHandler
	}

	// Attach to the registration's handler resource.
	attachedClient, err := reg.client.GetAttachedResource(reg.handlerResourceID)
	if err != nil {
		return nil, err
	}

	// Forward the sub-item query to the command handler.
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
	// Validate the command surface to watch.
	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return err
	}

	// Resolve the client session associated with the command stream.
	ctx := strm.Context()
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return err
	}

	// Stream command snapshots as the registry changes.
	for {
		// Read the command snapshot and its change notification under the registry lock.
		var states []*s4wave_command_registry.CommandState
		var waitCh <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			states = r.getCommandStatesLocked(surface, client)
			waitCh = getWaitCh()
		})

		// Send the current command states to the watching client.
		if err := strm.Send(&s4wave_command_registry.WatchCommandsResponse{
			Commands: states,
		}); err != nil {
			return err
		}

		// Wait for a registry change or stream cancellation.
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
	// Require the identifier of the command to invoke.
	cmdID := req.GetCommandId()
	if cmdID == "" {
		return nil, ErrCommandIdRequired
	}

	// Validate the requested command surface.
	surface, err := normalizeCommandSurface(req.GetSurface())
	if err != nil {
		return nil, err
	}

	// Resolve the client session invoking the command.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Find the active registration with a command handler.
	reg, err := r.getActiveRegistration(cmdID, surface, client)
	if err != nil {
		return nil, err
	}
	if reg.handlerResourceID == 0 {
		return nil, ErrNoHandler
	}

	// Attach to the registration's handler resource.
	attachedClient, err := reg.client.GetAttachedResource(reg.handlerResourceID)
	if err != nil {
		return nil, err
	}

	// Forward the command arguments to the registered handler.
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
	// Collect registrations for the requested client and surface.
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

	// Order registrations by command identifier and resource identifier.
	slices.SortFunc(regs, func(a, b *commandRegistration) int {
		if c := strings.Compare(a.command.GetCommandId(), b.command.GetCommandId()); c != 0 {
			return c
		}
		return cmp.Compare(a.resourceID, b.resourceID)
	})

	// Build the command-state snapshot from the ordered registrations.
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
	// Prepare the result of the active-registration lookup.
	var reg *commandRegistration
	var err error

	// Find the client's active registration under the registry lock.
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
