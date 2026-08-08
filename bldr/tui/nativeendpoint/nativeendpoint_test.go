//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
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

type unavailableClient struct{}

func (unavailableClient) ExecCall(context.Context, string, string, srpc.Message, srpc.Message) error {
	return errors.New("unavailable")
}
func (unavailableClient) NewStream(context.Context, string, string, srpc.Message) (srpc.Stream, error) {
	return nil, errors.New("unavailable")
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
	factory, err := NewEndpointFactory(Config{ResourceClient: unavailableClient{}, StateStore: &testStore{}, SelectedStateKey: "state:1"})
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
