package space_exec

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestV86RegistrationAndDefaultRegistryWithBus(t *testing.T) {
	b := bus_inmem.NewBus(nil)

	r := NewRegistry()
	RegisterV86(r, b)
	if V86ConfigID != "space-exec/v86" {
		t.Fatalf("V86ConfigID = %q", V86ConfigID)
	}
	if r.Lookup(V86ConfigID) == nil {
		t.Fatal("v86 factory not found in registry")
	}

	defaults := NewDefaultRegistryWithBus(b)
	if defaults.Lookup(V86ConfigID) == nil {
		t.Fatal("v86 not in default registry with bus")
	}
}

func TestV86ConfigValidation(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterV86(r, bus_inmem.NewBus(nil))

	for _, tc := range []struct {
		name      string
		config    []byte
		wantError string
	}{
		{name: "empty", config: nil, wantError: "empty config"},
		{name: "invalid json", config: []byte("not json"), wantError: "parse config json"},
		{name: "missing object key", config: []byte(`{"name":"vm"}`), wantError: "object_key is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.CreateHandler(ctx, nil, nil, nil, nil, V86ConfigID, tc.config)
			if err == nil {
				t.Fatal("expected config validation error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantError)
			}
		})
	}

	handler, err := r.CreateHandler(ctx, nil, nil, nil, nil, V86ConfigID, []byte(`{"object_key":"vm/test"}`))
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestV86ExecuteSetsStartingAndStreamsLogs(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	vmKey := "vm/v86-exec-test"
	createTestV86VM(t, ctx, tb.WorldState, vmKey)

	b := bus_inmem.NewBus(nil)
	execFake := &v86PersistentExecutionServiceFake{
		ws:        tb.WorldState,
		objectKey: vmKey,
		states: []s4wave_process.ExecutionState{
			s4wave_process.ExecutionState_ExecutionState_STARTING,
			s4wave_process.ExecutionState_ExecutionState_RUNNING,
		},
		done: make(chan struct{}),
	}
	loadCalled := false
	factory := newV86Handler(b, func(
		ctx context.Context,
		le *logrus.Entry,
		gotBus bus.Bus,
		gotWS world.WorldState,
		objectKey string,
	) (srpc.Invoker, func(), error) {
		loadCalled = true
		if gotBus != b {
			t.Fatalf("invoker factory got unexpected bus")
		}
		if gotWS != tb.WorldState {
			t.Fatalf("invoker factory got unexpected world state")
		}
		if objectKey != vmKey {
			t.Fatalf("invoker factory object key = %q", objectKey)
		}
		mux := srpc.NewMux()
		if err := s4wave_process.SRPCRegisterPersistentExecutionService(mux, execFake); err != nil {
			return nil, nil, err
		}
		return mux, nil, nil
	})

	handle := &pluginExecHandleStub{}
	handler, err := factory(
		ctx,
		logrus.NewEntry(logrus.StandardLogger()),
		tb.WorldState,
		handle,
		forge_target.InputMap{},
		[]byte(`{"object_key":"`+vmKey+`"}`),
	)
	if err != nil {
		t.Fatalf("CreateHandler: %v", err)
	}
	if err := handler.Execute(ctx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	<-execFake.done

	if !loadCalled {
		t.Fatal("invoker factory was not called")
	}
	if !execFake.called {
		t.Fatal("persistent execution service was not called")
	}
	if execFake.stateAtExecute != s4wave_vm.VmState_VmState_STARTING {
		t.Fatalf("state at service Execute = %s, want STARTING", execFake.stateAtExecute.String())
	}
	if got := readTestV86State(t, ctx, tb.WorldState, vmKey); got != s4wave_vm.VmState_VmState_STARTING {
		t.Fatalf("stored state after Execute = %s, want STARTING", got.String())
	}
	wantStates := []s4wave_process.ExecutionState{
		s4wave_process.ExecutionState_ExecutionState_STARTING,
		s4wave_process.ExecutionState_ExecutionState_RUNNING,
	}
	if !slices.Equal(execFake.sentStates, wantStates) {
		t.Fatalf("sent states = %v, want %v", execFake.sentStates, wantStates)
	}

	wantLogs := []PluginExecLog{
		{Level: "info", Message: "v86 starting: " + vmKey},
		{Level: "info", Message: "v86 running: " + vmKey},
	}
	if len(handle.logs) != len(wantLogs) {
		t.Fatalf("logs = %#v, want %#v", handle.logs, wantLogs)
	}
	for i, want := range wantLogs {
		if got := handle.logs[i]; got.GetLevel() != want.GetLevel() || got.GetMessage() != want.GetMessage() {
			t.Fatalf("log %d = {%q, %q}, want {%q, %q}", i, got.GetLevel(), got.GetMessage(), want.GetLevel(), want.GetMessage())
		}
	}
}

type v86PersistentExecutionServiceFake struct {
	ws             world.WorldState
	objectKey      string
	states         []s4wave_process.ExecutionState
	done           chan struct{}
	called         bool
	stateAtExecute s4wave_vm.VmState
	sentStates     []s4wave_process.ExecutionState
}

func (f *v86PersistentExecutionServiceFake) Execute(
	req *s4wave_process.ExecuteRequest,
	stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream,
) error {
	if f.done != nil {
		defer close(f.done)
	}
	f.called = true
	state, err := readV86State(stream.Context(), f.ws, f.objectKey)
	if err != nil {
		return err
	}
	f.stateAtExecute = state
	for _, state := range f.states {
		f.sentStates = append(f.sentStates, state)
		if err := stream.Send(&s4wave_process.ExecuteStatus{State: state}); err != nil {
			return err
		}
	}
	return nil
}

func createTestV86VM(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	imageKey := objectKey + "/image"
	_, _, err := ws.ApplyWorldOp(
		ctx,
		s4wave_vm.NewCreateV86ImageOp(imageKey, &s4wave_vm.V86Image{Name: "test image", Platform: "v86"}, time.Unix(1, 0)),
		"",
	)
	if err != nil {
		t.Fatalf("CreateV86Image: %v", err)
	}
	_, _, err = ws.ApplyWorldOp(
		ctx,
		s4wave_vm.NewCreateVmV86Op(objectKey, "test VM", imageKey, time.Unix(1, 0)),
		"",
	)
	if err != nil {
		t.Fatalf("CreateVmV86: %v", err)
	}
}

func readTestV86State(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) s4wave_vm.VmState {
	t.Helper()
	state, err := readV86State(ctx, ws, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func readV86State(ctx context.Context, ws world.WorldState, objectKey string) (s4wave_vm.VmState, error) {
	obj, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, errors.Errorf("VM %q not found", objectKey)
	}

	var state s4wave_vm.VmState
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		vm, err := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if err != nil {
			return err
		}
		if vm == nil {
			return errors.Errorf("VM %q block missing", objectKey)
		}
		state = vm.GetState()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return state, nil
}
