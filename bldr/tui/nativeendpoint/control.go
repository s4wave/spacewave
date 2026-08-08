//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	command "github.com/s4wave/spacewave/sdk/command"
	command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

const (
	maxNativeCommands = 256
	maxCommandText    = native.MaxIdentityBytes
)

var errInvalidCommands = errors.New("native endpoint: invalid command snapshot")

// controlBridge keeps the selected viewer controls on the selected resource
// client while translating the command registry into the native control API.
type controlBridge struct {
	selected native.SRPCControlServiceClient
	registry command_registry.SRPCCommandRegistryResourceServiceClient
}

func newControlBridge(
	selected native.SRPCControlServiceClient,
	registry command_registry.SRPCCommandRegistryResourceServiceClient,
) *controlBridge {
	return &controlBridge{selected: selected, registry: registry}
}

func (b *controlBridge) AvailableSessions(ctx context.Context, req *native.NativeViewerAvailableSessionsRequest) (*native.NativeViewerAvailableSessionsResponse, error) {
	return b.selected.AvailableSessions(ctx, req)
}

func (b *controlBridge) SelectSession(ctx context.Context, req *native.NativeViewerSelectSessionRequest) (*native.NativeViewerSelectSessionResponse, error) {
	return b.selected.SelectSession(ctx, req)
}

func (b *controlBridge) SendInput(ctx context.Context, req *native.NativeViewerSendInputRequest) (*native.NativeViewerControlResponse, error) {
	return b.selected.SendInput(ctx, req)
}

func (b *controlBridge) Interrupt(ctx context.Context, req *native.NativeViewerInterruptRequest) (*native.NativeViewerControlResponse, error) {
	return b.selected.Interrupt(ctx, req)
}

func (b *controlBridge) FollowUp(ctx context.Context, req *native.NativeViewerFollowUpRequest) (*native.NativeViewerFollowUpResponse, error) {
	return b.selected.FollowUp(ctx, req)
}

func (b *controlBridge) ListCommands(ctx context.Context, _ *native.NativeViewerListCommandsRequest) (*native.NativeViewerListCommandsResponse, error) {
	watch, err := b.registry.WatchCommands(ctx, &command_registry.WatchCommandsRequest{
		Surface: command.CommandSurface_COMMAND_SURFACE_TUI,
	})
	if err != nil {
		return nil, err
	}
	defer watch.Close()
	snapshot, err := watch.Recv()
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, errInvalidCommands
	}
	commands, err := projectCommands(snapshot.GetCommands())
	if err != nil {
		return nil, err
	}
	return &native.NativeViewerListCommandsResponse{Commands: commands}, nil
}

func (b *controlBridge) ExecuteCommand(ctx context.Context, req *native.NativeViewerExecuteCommandRequest) (*native.NativeViewerExecuteCommandResponse, error) {
	if req == nil || !validIdentity(req.GetCommandId()) || !validRequestID(req.GetRequestId()) {
		return rejectedCommand(req, "invalid command request"), nil
	}
	_, err := b.registry.InvokeCommand(ctx, &command_registry.InvokeCommandRequest{
		CommandId: req.GetCommandId(),
		Surface:   command.CommandSurface_COMMAND_SURFACE_TUI,
	})
	if err != nil {
		return rejectedCommand(req, err.Error()), nil
	}
	return &native.NativeViewerExecuteCommandResponse{
		Status:    native.NativeViewerControlStatus_NATIVE_VIEWER_CONTROL_STATUS_ACCEPTED,
		CommandId: req.GetCommandId(),
		RequestId: req.GetRequestId(),
	}, nil
}

func rejectedCommand(req *native.NativeViewerExecuteCommandRequest, detail string) *native.NativeViewerExecuteCommandResponse {
	if req == nil {
		return &native.NativeViewerExecuteCommandResponse{
			Status: native.NativeViewerControlStatus_NATIVE_VIEWER_CONTROL_STATUS_REJECTED,
			Detail: detail,
		}
	}
	return &native.NativeViewerExecuteCommandResponse{
		Status:    native.NativeViewerControlStatus_NATIVE_VIEWER_CONTROL_STATUS_REJECTED,
		CommandId: req.GetCommandId(),
		RequestId: req.GetRequestId(),
		Detail:    detail,
	}
}

func projectCommands(states []*command_registry.CommandState) ([]*native.NativeViewerCommand, error) {
	if len(states) > maxNativeCommands {
		return nil, errInvalidCommands
	}
	seen := make(map[string]struct{}, len(states))
	commands := make([]*native.NativeViewerCommand, 0, len(states))
	for _, state := range states {
		if state == nil || state.GetSurface() != command.CommandSurface_COMMAND_SURFACE_TUI || state.GetCommand() == nil {
			return nil, errInvalidCommands
		}
		cmd := state.GetCommand()
		id := cmd.GetCommandId()
		if !validIdentity(id) || !validCommandText(cmd.GetLabel()) {
			return nil, errInvalidCommands
		}
		if _, ok := seen[id]; ok {
			return nil, errInvalidCommands
		}
		seen[id] = struct{}{}
		binding, err := commandDisplayBinding(cmd.GetDefaultBindings())
		if err != nil {
			return nil, err
		}
		if !state.GetActive() || !state.GetEnabled() {
			continue
		}
		commands = append(commands, &native.NativeViewerCommand{
			Id: id, Label: cmd.GetLabel(), Binding: binding,
			Surface: uint32(command.CommandSurface_COMMAND_SURFACE_TUI),
		})
	}
	slices.SortFunc(commands, func(a, b *native.NativeViewerCommand) int {
		if cmp := strings.Compare(a.GetId(), b.GetId()); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.GetBinding(), b.GetBinding())
	})
	return commands, nil
}

func commandDisplayBinding(bindings []*command.CommandBinding) (string, error) {
	if len(bindings) == 0 {
		return "", nil
	}
	displays := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil || binding.GetSurface() != command.CommandSurface_COMMAND_SURFACE_TUI {
			return "", errInvalidCommands
		}
		var display string
		switch {
		case binding.GetCombo() != nil:
			display = binding.GetCombo().GetCombo()
		case binding.GetSequence() != nil:
			steps := binding.GetSequence().GetSteps()
			if len(steps) == 0 {
				return "", errInvalidCommands
			}
			display = strings.Join(steps, " ")
		default:
			return "", errInvalidCommands
		}
		if !validCommandText(display) {
			return "", errInvalidCommands
		}
		displays = append(displays, display)
	}
	slices.Sort(displays)
	return displays[0], nil
}

func validCommandText(value string) bool {
	if value == "" || len(value) > maxCommandText || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

var _ native.SRPCControlServiceServer = (*controlBridge)(nil)
