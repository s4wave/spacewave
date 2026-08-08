//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

type testStore struct {
	mu      sync.Mutex
	json    string
	setJSON []string
}

func (s *testStore) Get(context.Context) (string, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.json, uint64(len(s.setJSON)), nil
}
func (s *testStore) Set(_ context.Context, value string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.json = value
	s.setJSON = append(s.setJSON, value)
	return uint64(len(s.setJSON)), nil
}
func (s *testStore) WaitSeqno(context.Context, uint64) (uint64, error) { return 0, nil }
func (s *testStore) GetStoreID() string                                { return "test" }

var _ resource_state.StateAtomStore = (*testStore)(nil)

type unavailableClient struct {
	mu       sync.Mutex
	services []string
}

func (c *unavailableClient) ExecCall(context.Context, string, string, srpc.Message, srpc.Message) error {
	return errors.New("unavailable")
}
func (c *unavailableClient) NewStream(_ context.Context, service, _ string, _ srpc.Message) (srpc.Stream, error) {
	c.mu.Lock()
	c.services = append(c.services, service)
	c.mu.Unlock()
	return nil, errors.New("upstream marker")
}
func (c *unavailableClient) called(service string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Contains(c.services, service)
}

func TestStateServiceValidationAndDetachedLoad(t *testing.T) {
	store := &testStore{}
	service := newStateService(store, "state:1")
	if _, err := service.Load(context.Background(), &native.NativeViewerStateLoadRequest{StateKey: "other"}); err == nil {
		t.Fatal("accepted wrong state key")
	}
	loaded, err := service.Load(context.Background(), &native.NativeViewerStateLoadRequest{StateKey: "state:1"})
	if err != nil || loaded.State == nil {
		t.Fatalf("Load: %v", err)
	}
	loaded.State.Tabs = []string{"tab:1"}
	store.json = `{"tabs":["tab:2"]}`
	again, err := service.Load(context.Background(), &native.NativeViewerStateLoadRequest{StateKey: "state:1"})
	if err != nil || len(again.State.Tabs) != 1 || again.State.Tabs[0] != "tab:2" {
		t.Fatalf("detached Load: %#v, %v", again, err)
	}
	if _, err := service.Save(context.Background(), &native.NativeViewerStateSaveRequest{StateKey: "state:1", RequestId: "", State: &native.NativeViewerSelectedState{}}); err == nil {
		t.Fatal("accepted empty request ID")
	}
	if len(store.setJSON) != 0 {
		t.Fatal("invalid Save changed storage")
	}
	state := &native.NativeViewerSelectedState{Tabs: []string{"tab:1"}, Focused: "tab:1"}
	resp, err := service.Save(context.Background(), &native.NativeViewerStateSaveRequest{StateKey: "state:1", RequestId: "request:1", State: state})
	if err != nil || !resp.Accepted || resp.StateKey != "state:1" || resp.RequestId != "request:1" {
		t.Fatalf("Save: %#v, %v", resp, err)
	}
	if len(store.setJSON) != 1 {
		t.Fatalf("Set calls=%d", len(store.setJSON))
	}
}

func TestOpenEndpointsCloseAndWait(t *testing.T) {
	factory, err := NewEndpointFactory(Config{ResourceClient: &unavailableClient{}, StateStore: &testStore{}, SelectedStateKey: "state:1"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := factory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stateConn, err := net.FileConn(set.State)
	if err != nil {
		t.Fatal(err)
	}
	stateClientMux, err := srpc.NewMuxedConn(stateConn, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	stateClient := srpc.NewClientWithMuxedConn(stateClientMux)
	state := native.NewSRPCStateServiceClient(stateClient)
	loaded, err := state.Load(context.Background(), &native.NativeViewerStateLoadRequest{StateKey: "state:1"})
	if err != nil || loaded.State == nil {
		t.Fatalf("Load: %#v, %v", loaded, err)
	}
	if err := set.CloseFunc(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- set.WaitFunc() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked")
	}
	_ = stateClientMux.Close()
	_ = stateConn.Close()
	_ = set.CloseFunc()
	_ = set.WaitFunc()
}

func TestEndpointServiceIsolation(t *testing.T) {
	upstream := new(unavailableClient)
	factory, err := NewEndpointFactory(Config{
		ResourceClient:   upstream,
		StateStore:       &testStore{},
		SelectedStateKey: "state:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := factory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = set.CloseFunc()
		_ = set.WaitFunc()
	}()
	openClient := func(file *os.File) (srpc.Client, srpc.MuxedConn) {
		conn, err := net.FileConn(file)
		if err != nil {
			t.Fatal(err)
		}
		mux, err := srpc.NewMuxedConn(conn, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		return srpc.NewClientWithMuxedConn(mux), mux
	}
	resourceClient, resourceMux := openClient(set.Resource)
	defer resourceMux.Close()
	controlClient, controlMux := openClient(set.Control)
	defer controlMux.Close()

	resourceStream, err := resourceClient.NewStream(t.Context(), resource.SRPCResourceServiceServiceID, "ResourceClient", &resource.ResourceClientRequest{})
	if err == nil {
		err = resourceStream.MsgRecv(&resource.ResourceClientResponse{})
		_ = resourceStream.Close()
	}
	if err == nil {
		t.Fatal("resource probe unexpectedly succeeded")
	}
	if !upstream.called(resource.SRPCResourceServiceServiceID) {
		t.Fatal("resource endpoint did not forward ResourceService")
	}
	blockedControl, err := resourceClient.NewStream(t.Context(), native.SRPCControlServiceServiceID, "ListCommands", &native.NativeViewerListCommandsRequest{})
	if err == nil {
		err = blockedControl.MsgRecv(&native.NativeViewerListCommandsResponse{})
		_ = blockedControl.Close()
	}
	if err == nil {
		t.Fatal("blocked control probe unexpectedly succeeded")
	}
	if upstream.called(native.SRPCControlServiceServiceID) {
		t.Fatal("resource endpoint forwarded ControlService")
	}

	controlStream, err := controlClient.NewStream(t.Context(), native.SRPCControlServiceServiceID, "ListCommands", &native.NativeViewerListCommandsRequest{})
	if err == nil {
		err = controlStream.MsgRecv(&native.NativeViewerListCommandsResponse{})
		_ = controlStream.Close()
	}
	if err == nil {
		t.Fatal("control probe unexpectedly succeeded")
	}
	if !upstream.called(native.SRPCControlServiceServiceID) {
		t.Fatal("control endpoint did not forward ControlService")
	}
	blockedResource, err := controlClient.NewStream(t.Context(), resource.SRPCResourceServiceServiceID, "ResourceClient", &resource.ResourceClientRequest{})
	if err == nil {
		err = blockedResource.MsgRecv(&resource.ResourceClientResponse{})
		_ = blockedResource.Close()
	}
	if err == nil {
		t.Fatal("blocked resource probe unexpectedly succeeded")
	}
	upstream.mu.Lock()
	resourceCalls := 0
	for _, service := range upstream.services {
		if service == resource.SRPCResourceServiceServiceID {
			resourceCalls++
		}
	}
	upstream.mu.Unlock()
	if resourceCalls != 1 {
		t.Fatalf("resource forwarding calls = %d, want 1", resourceCalls)
	}
}

func TestFactoryRejectsNilContext(t *testing.T) {
	factory, err := New(Config{ResourceClient: new(unavailableClient), StateStore: &testStore{}, SelectedStateKey: "state:1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(nil); err == nil {
		t.Fatal("accepted nil lifecycle context")
	}
}
