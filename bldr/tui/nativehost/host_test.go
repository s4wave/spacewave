//go:build !js && !windows

package nativehost

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

const nativeHostHelperEnvironment = "SPACEWAVE_NATIVEHOST_TEST_HELPER"

// nativeHostTestLaunch returns a valid launch record with the fixed inherited descriptor table.
func nativeHostTestLaunch() *native.NativeViewerLaunchRecord {
	return &native.NativeViewerLaunchRecord{
		WireVersion: native.NativeViewerWireVersion, ProtocolVersion: native.NativeViewerProtocolVersion,
		LaunchId: "launch:1", SessionObjectKey: "session:1", SpaceObjectKey: "space:1",
		ManifestObjectKey: "manifest:1", ManifestDigest: "sha256:1",
		ViewerObjectKey: "viewer:1", ViewerProfile: "default",
		ResourceScopeSessionObjectKey: "session:1", SelectedStateKey: "state:1", LaunchNonce: "nonce:1",
		Io: &native.NativeViewerIODescriptor{
			InputFd: 0, OutputFd: 1, DiagnosticFd: 2,
			InputMode:  native.NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ,
			OutputMode: native.NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE,
		},
		Endpoints: []*native.NativeViewerEndpointDescriptor{
			{Kind: native.NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RECORD, Fd: native.RecordFD, Transport: native.NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, ServiceId: "native.viewer.record", ProtocolVersion: 1, Required: true, CloseOnExit: true},
			{Kind: native.NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_READINESS, Fd: native.ReadinessFD, Transport: native.NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, ServiceId: "native.viewer.readiness", ProtocolVersion: 1, Required: true, CloseOnExit: true},
			{Kind: native.NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RESOURCE, Fd: native.ResourceFD, Transport: native.NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: "resource.ResourceService", ProtocolVersion: 1, Required: true, CloseOnExit: true},
			{Kind: native.NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_STATE, Fd: native.StateFD, Transport: native.NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: native.SRPCStateServiceServiceID, ProtocolVersion: 1, Required: true, CloseOnExit: true},
			{Kind: native.NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL, Fd: native.ControlFD, Transport: native.NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: native.SRPCControlServiceServiceID, ProtocolVersion: 1, Required: true, CloseOnExit: true},
		},
	}
}

// nativeHostReady echoes launch identity into a readiness result with status-specific cleanup evidence.
func nativeHostReady(launch *native.NativeViewerLaunchRecord, status native.NativeViewerReadinessStatus, detail string) *native.NativeViewerReadinessRecord {
	r := &native.NativeViewerReadinessRecord{
		WireVersion: native.NativeViewerWireVersion, ProtocolVersion: native.NativeViewerProtocolVersion,
		LaunchId: launch.LaunchId, SessionObjectKey: launch.SessionObjectKey, SpaceObjectKey: launch.SpaceObjectKey,
		ManifestObjectKey: launch.ManifestObjectKey, ManifestDigest: launch.ManifestDigest,
		ViewerObjectKey: launch.ViewerObjectKey, ViewerProfile: launch.ViewerProfile,
		ResourceScopeSessionObjectKey: launch.ResourceScopeSessionObjectKey, SelectedStateKey: launch.SelectedStateKey,
		Status: status, Detail: detail,
	}
	if status == native.NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY {
		r.ResourceRevision, r.ResourceCursor, r.FrameSequence = 3, 4, 1
	} else {
		r.TerminalRestoreAttempted, r.AllWorkersJoined = true, true
	}
	return r
}

// TestNativeHostHelperProcess models child readiness, failure, cancellation, and terminal mutation.
func TestNativeHostHelperProcess(t *testing.T) {
	mode := os.Getenv(nativeHostHelperEnvironment)
	if mode == "" {
		return
	}
	for fd := 3; fd <= 7; fd++ {
		if _, err := os.NewFile(uintptr(fd), "inherited").Stat(); err != nil {
			os.Exit(90 + fd)
		}
	}
	launch, err := native.ReadLaunchRecord(os.NewFile(3, "launch"))
	if err != nil {
		os.Exit(80)
	}
	ready := os.NewFile(4, "readiness")
	if mode == "exit" {
		os.Exit(7)
	}
	if mode == "restart" {
		marker := os.Getenv("SPACEWAVE_NATIVEHOST_RESTART_MARKER")
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("1"), 0o600)
			os.Exit(8)
		}
	}
	if mode == "mismatch" {
		r := nativeHostReady(launch, native.NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY, "")
		r.SessionObjectKey = "session:other"
		frame, _ := r.MarshalVT()
		var prefix [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(prefix[:], uint64(len(frame)))
		_, _ = ready.Write(prefix[:n])
		_, _ = ready.Write(frame)
		os.Exit(0)
	}
	if mode == "failed" {
		_ = native.WriteReadinessRecord(ready, nativeHostReady(launch, native.NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_FAILED, "child failed"), launch)
		os.Exit(9)
	}
	if mode == "raw-crash" {
		_, _ = term.MakeRaw(0)
	}
	if err := native.WriteReadinessRecord(ready, nativeHostReady(launch, native.NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY, ""), launch); err != nil {
		os.Exit(81)
	}
	_ = ready.Close()
	if mode == "ready-wait" {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		os.Exit(0)
	}
	if mode == "raw-crash" {
		os.Exit(10)
	}
	os.Exit(0)
}

// endpointEvents records endpoint shutdown ordering.
type endpointEvents struct {
	// mu guards events.
	mu sync.Mutex
	// events records endpoint lifecycle actions.
	events []string
}

// add records one endpoint lifecycle event.
func (e *endpointEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

// nativeHostEndpointFactory returns fresh pipe endpoints and records their close-before-wait lifecycle.
func nativeHostEndpointFactory(events *endpointEvents) EndpointFactory {
	return func(context.Context) (*EndpointSet, error) {
		children := make([]*os.File, 0, 3)
		parents := make([]*os.File, 0, 3)
		for range 3 {
			r, w, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			children = append(children, r)
			parents = append(parents, w)
		}
		return &EndpointSet{
			Resource: children[0], State: children[1], Control: children[2],
			CloseFunc: func() error {
				events.add("close")
				for _, f := range parents {
					_ = f.Close()
				}
				return nil
			},
			WaitFunc: func() error { events.add("wait"); return nil },
		}, nil
	}
}

// nativeHostTestExecutable resolves the current test binary as the supervised child executable.
func nativeHostTestExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nativehost-helper")
	script := "#!/bin/sh\nexec \"$SPACEWAVE_NATIVEHOST_TEST_BINARY\" -test.run '^TestNativeHostHelperProcess$'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPACEWAVE_NATIVEHOST_TEST_BINARY", os.Args[0])
	return path
}

// nativeHostFiles opens one PTY and returns duplicated input, output, and diagnostic handles.
func nativeHostFiles(t *testing.T) (*os.File, *os.File, *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = master.Close()
		_ = slave.Close()
	})
	return slave, slave, slave
}

// TestHostLifecycleMatrix proves readiness, retry, failure, and cancellation preserve endpoint and terminal custody.
func TestHostLifecycleMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, mode, contains string
		cancel               bool
		restarts             uint
	}{
		{name: "ready", mode: "ready"},
		{name: "failed", mode: "failed", contains: "child failed"},
		{name: "exit-before-ready", mode: "exit", contains: "readiness"},
		{name: "mismatch", mode: "mismatch", contains: "identity echo"},
		{name: "cancel", mode: "ready-wait", contains: "context canceled", cancel: true},
		{name: "restart", mode: "restart", restarts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, out, errFile := nativeHostFiles(t)
			events := &endpointEvents{}
			t.Setenv(nativeHostHelperEnvironment, tc.mode)
			if tc.mode == "restart" {
				t.Setenv("SPACEWAVE_NATIVEHOST_RESTART_MARKER", filepath.Join(t.TempDir(), "attempt"))
			}
			h, err := NewHost(Config{Executable: nativeHostTestExecutable(t), LaunchRecord: nativeHostTestLaunch(), Stdin: in, Stdout: out, Stderr: errFile, RestartLimit: tc.restarts, EndpointFactory: nativeHostEndpointFactory(events)})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := make(chan struct{}, 2)
			done := make(chan error, 1)
			go func() { done <- h.Run(ctx, func() { ready <- struct{}{} }) }()
			if tc.cancel {
				select {
				case <-ready:
					cancel()
				case <-time.After(5 * time.Second):
					t.Fatal("readiness timeout")
				}
			}
			var runErr error
			select {
			case runErr = <-done:
			case <-time.After(8 * time.Second):
				t.Fatal("host timeout")
			}
			if tc.contains == "" && runErr != nil {
				t.Fatalf("Run: %v", runErr)
			}
			if tc.contains != "" && (runErr == nil || !strings.Contains(runErr.Error(), tc.contains)) {
				t.Fatalf("Run err=%v", runErr)
			}
			if len(ready) != 1 && tc.mode == "restart" {
				t.Fatalf("ready callbacks=%d", len(ready))
			}
			events.mu.Lock()
			got := append([]string(nil), events.events...)
			events.mu.Unlock()
			wantAttempts := 1
			if tc.mode == "restart" {
				wantAttempts = 2
			}
			if len(got) != wantAttempts*2 {
				t.Fatalf("events=%v", got)
			}
			for i := 0; i < len(got); i += 2 {
				if !reflect.DeepEqual(got[i:i+2], []string{"close", "wait"}) {
					t.Fatalf("events=%v", got)
				}
			}
		})
	}
}

// TestHostRejectsInvalidEndpoints proves nil and aliased child descriptors are closed without launch.
func TestHostRejectsInvalidEndpoints(t *testing.T) {
	in, out, errFile := nativeHostFiles(t)
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h, err := NewHost(Config{Executable: nativeHostTestExecutable(t), LaunchRecord: nativeHostTestLaunch(), Stdin: in, Stdout: out, Stderr: errFile, EndpointFactory: func(context.Context) (*EndpointSet, error) {
		return &EndpointSet{Resource: f, State: f, Control: f}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Run(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "aliased") {
		t.Fatalf("Run err=%v", err)
	}
}

// TestHostRestoresRawTermiosAfterCrash proves a crashing child cannot leave the shared PTY in raw mode.
func TestHostRestoresRawTermiosAfterCrash(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(nativeHostHelperEnvironment, "raw-crash")
	events := &endpointEvents{}
	h, err := NewHost(Config{Executable: nativeHostTestExecutable(t), LaunchRecord: nativeHostTestLaunch(), Stdin: slave, Stdout: slave, Stderr: slave, EndpointFactory: nativeHostEndpointFactory(events)})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Run(context.Background(), nil); err == nil {
		t.Fatal("expected crash")
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("termios not restored")
	}
}

// TestNewHostRejectsNonTerminal proves host construction rejects streams without terminal custody.
func TestNewHostRejectsNonTerminal(t *testing.T) {
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	_, err = NewHost(Config{
		Executable: nativeHostTestExecutable(t), LaunchRecord: nativeHostTestLaunch(),
		Stdin: input, Stdout: output, Stderr: output, EndpointFactory: nativeHostEndpointFactory(&endpointEvents{}),
	})
	if err == nil || !strings.Contains(err.Error(), "must be terminals") {
		t.Fatalf("NewHost err=%v", err)
	}
}
