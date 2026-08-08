//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	command "github.com/s4wave/spacewave/sdk/command"
	command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

// commandRegistryTestServer records watched and invoked commands while serving a configured snapshot.
type commandRegistryTestServer struct {
	// mu guards request, invoked, and snapshot.
	mu sync.Mutex
	// request records the watched command surface.
	request command.CommandSurface
	// invoked records the invoked command.
	invoked *command_registry.InvokeCommandRequest
	// snapshot is returned by WatchCommands.
	snapshot *command_registry.WatchCommandsResponse
}

// RegisterCommand is unused by the command projection fixture.
func (s *commandRegistryTestServer) RegisterCommand(context.Context, *command_registry.RegisterCommandRequest) (*command_registry.RegisterCommandResponse, error) {
	return nil, errors.New("not used")
}

// SetActive is unused by the command projection fixture.
func (s *commandRegistryTestServer) SetActive(context.Context, *command_registry.SetActiveRequest) (*command_registry.SetActiveResponse, error) {
	return nil, errors.New("not used")
}

// SetEnabled is unused by the command projection fixture.
func (s *commandRegistryTestServer) SetEnabled(context.Context, *command_registry.SetEnabledRequest) (*command_registry.SetEnabledResponse, error) {
	return nil, errors.New("not used")
}

// WatchCommands records the requested surface and sends the configured snapshot.
func (s *commandRegistryTestServer) WatchCommands(_ *command_registry.WatchCommandsRequest, stream command_registry.SRPCCommandRegistryResourceService_WatchCommandsStream) error {
	s.mu.Lock()
	snapshot := s.snapshot.CloneVT()
	s.request = command.CommandSurface_COMMAND_SURFACE_TUI
	s.mu.Unlock()
	if err := stream.Send(snapshot); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

// GetSubItems is unused by the command projection fixture.
func (s *commandRegistryTestServer) GetSubItems(context.Context, *command_registry.GetSubItemsRequest) (*command_registry.GetSubItemsResponse, error) {
	return nil, errors.New("not used")
}

// InvokeCommand records the request accepted by the bridge.
func (s *commandRegistryTestServer) InvokeCommand(_ context.Context, req *command_registry.InvokeCommandRequest) (*command_registry.InvokeCommandResponse, error) {
	s.mu.Lock()
	s.invoked = req.CloneVT()
	s.request = req.GetSurface()
	s.mu.Unlock()
	return &command_registry.InvokeCommandResponse{}, nil
}

// TestControlBridgeSocketpairCommands proves command projection and invocation across the SRPC transport.
func TestControlBridgeSocketpairCommands(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	serverMux, err := srpc.NewMuxedConn(serverConn, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientMux, err := srpc.NewMuxedConn(clientConn, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := &commandRegistryTestServer{snapshot: &command_registry.WatchCommandsResponse{Commands: []*command_registry.CommandState{
		{Command: &command.Command{CommandId: "z.command", Label: "Z", DefaultBindings: []*command.CommandBinding{{Surface: command.CommandSurface_COMMAND_SURFACE_TUI, Binding: &command.CommandBinding_Combo{Combo: &command.KeyCombo{Combo: "z"}}}}}, Active: true, Enabled: true, Surface: command.CommandSurface_COMMAND_SURFACE_TUI},
		{Command: &command.Command{CommandId: "a.command", Label: "A", DefaultBindings: []*command.CommandBinding{{Surface: command.CommandSurface_COMMAND_SURFACE_TUI, Binding: &command.CommandBinding_Combo{Combo: &command.KeyCombo{Combo: "a"}}}}}, Active: true, Enabled: true, Surface: command.CommandSurface_COMMAND_SURFACE_TUI},
	}}}
	registryMux := srpc.NewMux()
	if err := command_registry.SRPCRegisterCommandRegistryResourceService(registryMux, backend); err != nil {
		t.Fatal(err)
	}
	serveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srpc.NewServer(registryMux).AcceptMuxedConn(serveCtx, serverMux) }()
	registryClient := command_registry.NewSRPCCommandRegistryResourceServiceClient(srpc.NewClientWithMuxedConn(clientMux))
	factory, err := NewEndpointFactory(Config{ResourceClient: &unavailableClient{}, StateStore: &testStore{}, SelectedStateKey: "state:1", CommandRegistryClient: registryClient})
	if err != nil {
		t.Fatal(err)
	}
	set, err := factory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	controlConn, err := net.FileConn(set.Control)
	if err != nil {
		t.Fatal(err)
	}
	controlMux, err := srpc.NewMuxedConn(controlConn, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	control := native.NewSRPCControlServiceClient(srpc.NewClientWithMuxedConn(controlMux))
	list, err := control.ListCommands(t.Context(), &native.NativeViewerListCommandsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetCommands()) != 2 || list.GetCommands()[0].GetId() != "a.command" || list.GetCommands()[1].GetId() != "z.command" {
		t.Fatalf("commands=%v", list.GetCommands())
	}
	response, err := control.ExecuteCommand(t.Context(), &native.NativeViewerExecuteCommandRequest{CommandId: "a.command", RequestId: "req:1"})
	if err != nil || response.GetStatus() != native.NativeViewerControlStatus_NATIVE_VIEWER_CONTROL_STATUS_ACCEPTED || response.GetCommandId() != "a.command" || response.GetRequestId() != "req:1" {
		t.Fatalf("execute=%v err=%v", response, err)
	}
	backend.mu.Lock()
	invoked := backend.invoked.CloneVT()
	surface := backend.request
	backend.mu.Unlock()
	if invoked == nil || invoked.GetCommandId() != "a.command" || surface != command.CommandSurface_COMMAND_SURFACE_TUI {
		t.Fatalf("invoke=%v surface=%v", invoked, surface)
	}
	bad, err := control.ExecuteCommand(t.Context(), &native.NativeViewerExecuteCommandRequest{CommandId: "", RequestId: "req:2"})
	if err != nil || bad.GetStatus() != native.NativeViewerControlStatus_NATIVE_VIEWER_CONTROL_STATUS_REJECTED || bad.GetRequestId() != "req:2" {
		t.Fatalf("bad=%v err=%v", bad, err)
	}
	_ = set.CloseFunc()
	_ = set.WaitFunc()
	_ = controlMux.Close()
	_ = controlConn.Close()
	_ = clientMux.Close()
	_ = serverMux.Close()
	cancel()
	<-serveDone
}

// TestProjectCommandsRejectsMalformedSnapshots proves invalid or duplicate registry entries never cross the viewer boundary.
func TestProjectCommandsRejectsMalformedSnapshots(t *testing.T) {
	valid := &command_registry.CommandState{Command: &command.Command{CommandId: "a", Label: "A"}, Active: true, Enabled: true, Surface: command.CommandSurface_COMMAND_SURFACE_TUI}
	for name, states := range map[string][]*command_registry.CommandState{
		"nil": {nil}, "duplicate": {valid, valid}, "web": {{Command: valid.Command, Active: true, Enabled: true, Surface: command.CommandSurface_COMMAND_SURFACE_WEB}},
		"empty label": {{Command: &command.Command{CommandId: "a"}, Active: true, Enabled: true, Surface: command.CommandSurface_COMMAND_SURFACE_TUI}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := projectCommands(states); err == nil {
				t.Fatal("accepted malformed snapshot")
			}
		})
	}
	tooMany := make([]*command_registry.CommandState, maxNativeCommands+1)
	for i := range tooMany {
		tooMany[i] = valid.CloneVT()
		tooMany[i].Command = valid.Command.CloneVT()
		tooMany[i].Command.CommandId = string(rune('a' + i))
	}
	if _, err := projectCommands(tooMany); err == nil {
		t.Fatal("accepted over-capacity snapshot")
	}
}
