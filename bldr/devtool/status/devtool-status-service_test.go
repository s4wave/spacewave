//go:build !js

package status

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

type devtoolStatusTestStream struct {
	ctx  context.Context
	sent chan *WatchDevtoolStatusResponse
}

func newDevtoolStatusTestStream(ctx context.Context) *devtoolStatusTestStream {
	return &devtoolStatusTestStream{
		ctx:  ctx,
		sent: make(chan *WatchDevtoolStatusResponse, 8),
	}
}

func (s *devtoolStatusTestStream) Context() context.Context {
	return s.ctx
}

func (s *devtoolStatusTestStream) MsgSend(msg srpc.Message) error {
	resp, ok := msg.(*WatchDevtoolStatusResponse)
	if !ok {
		return fmt.Errorf("unexpected message type %T", msg)
	}
	return s.Send(resp)
}

func (s *devtoolStatusTestStream) MsgRecv(srpc.Message) error {
	return io.EOF
}

func (s *devtoolStatusTestStream) CloseSend() error {
	return nil
}

func (s *devtoolStatusTestStream) Close() error {
	return nil
}

func (s *devtoolStatusTestStream) Send(resp *WatchDevtoolStatusResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp:
		return nil
	}
}

func (s *devtoolStatusTestStream) SendAndClose(resp *WatchDevtoolStatusResponse) error {
	if resp != nil {
		if err := s.Send(resp); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func recvDevtoolStatusResponse(
	t *testing.T,
	strm *devtoolStatusTestStream,
) *WatchDevtoolStatusResponse {
	t.Helper()
	select {
	case resp := <-strm.sent:
		return resp
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for devtool status response")
		return nil
	}
}

func TestDevtoolStatusWatchServiceEmitsInitialAndChanges(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(
		EmptyBldrDevtoolStatus().WithCommand(BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   BldrDevtoolCommandStateRunning,
			Summary: "starting web",
			LogFile: ".bldr/logs/status.log",
		}),
	)
	service := NewDevtoolStatusWatchService(producer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	strm := newDevtoolStatusTestStream(ctx)
	done := make(chan error, 1)

	go func() {
		done <- service.WatchDevtoolStatus(&WatchDevtoolStatusRequest{}, strm)
	}()

	initial := recvDevtoolStatusResponse(t, strm)
	if initial.GetSnapshot().GetCommand().GetName() != "start web" {
		t.Fatalf("expected initial command snapshot, got %#v", initial.GetSnapshot().GetCommand())
	}

	producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithManifestBuildRows([]BldrDevtoolManifestBuildRow{{
			ID:                "build:web",
			BuildTargets:      "web",
			ManifestID:        "spacewave-web",
			PlatformID:        "web/js/wasm",
			TargetPlatformIDs: "web/js/wasm,js",
			BuildType:         "dev",
			State:             BldrDevtoolManifestStateReady,
			CacheHit:          true,
			Summary:           "cache hit",
		}})
	})

	changed := recvDevtoolStatusResponse(t, strm)
	buildRows := changed.GetSnapshot().GetManifestBuildRows()
	if len(buildRows) != 1 || !buildRows[0].GetCacheHit() {
		t.Fatalf("expected changed build row, got %#v", buildRows)
	}
	if !slices.Equal(buildRows[0].GetTargetPlatformIds(), []string{"web/js/wasm", "js"}) {
		t.Fatalf("expected split target platforms, got %#v", buildRows[0].GetTargetPlatformIds())
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not stop after cancellation")
	}
}

func TestDevtoolStatusWatchServiceClosesAfterTerminalCommand(t *testing.T) {
	producer := NewBldrDevtoolStatusProducer(
		EmptyBldrDevtoolStatus().WithCommand(BldrDevtoolCommandStatus{
			Name:  "build",
			State: BldrDevtoolCommandStateRunning,
		}),
	)
	service := NewDevtoolStatusWatchService(producer)
	ctx := t.Context()
	strm := newDevtoolStatusTestStream(ctx)
	done := make(chan error, 1)

	go func() {
		done <- service.WatchDevtoolStatus(&WatchDevtoolStatusRequest{}, strm)
	}()

	recvDevtoolStatusResponse(t, strm)
	producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithCommand(BldrDevtoolCommandStatus{
			Name:    "build",
			State:   BldrDevtoolCommandStateDone,
			Summary: "done",
		})
	})

	terminal := recvDevtoolStatusResponse(t, strm)
	if terminal.GetSnapshot().GetCommand().GetState() !=
		DevtoolStatusCommandState_DevtoolStatusCommandState_DONE {
		t.Fatalf("expected terminal snapshot, got %#v", terminal.GetSnapshot().GetCommand())
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected terminal watch to close cleanly, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not close after terminal command")
	}
}

func TestBuildProjectStatusNormalizesTargets(t *testing.T) {
	project := BuildProjectStatus(&bldr_project.ProjectConfig{
		Id: "spacewave-devtool",
		Start: &bldr_project.StartConfig{
			Plugins:        []string{"notes", "web"},
			LoadWebStartup: "bldr/web/devtool-status/startup.tsx",
		},
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-app": {},
			"spacewave-web": {},
		},
		Build: map[string]*bldr_project.BuildConfig{
			"web": {
				Manifests:   []string{"spacewave-web"},
				Targets:     []string{"browser"},
				PlatformIds: []string{"desktop/darwin/arm64"},
			},
		},
	})

	if project.ProjectID != "spacewave-devtool" {
		t.Fatalf("expected project id, got %q", project.ProjectID)
	}
	if !slices.Equal(project.ManifestIDs, []string{"spacewave-app", "spacewave-web"}) {
		t.Fatalf("expected sorted manifest ids, got %#v", project.ManifestIDs)
	}
	if len(project.BuildTargets) != 1 {
		t.Fatalf("expected one build target, got %#v", project.BuildTargets)
	}
	target := project.BuildTargets[0]
	if target.ID != "web" || !slices.Equal(target.ConfiguredTargetIDs, []string{"browser"}) {
		t.Fatalf("expected web browser target, got %#v", target)
	}
	if !slices.Equal(
		target.ResolvedPlatformIDs,
		[]string{"web/js/wasm", "js", "desktop/darwin/arm64"},
	) {
		t.Fatalf("expected resolved platform ids, got %#v", target.ResolvedPlatformIDs)
	}
	if !slices.Equal(target.BuildTypes, []string{"dev", "release"}) {
		t.Fatalf("expected dev and release build types, got %#v", target.BuildTypes)
	}
}

func TestBuildDevtoolStatusSnapshotMapsRows(t *testing.T) {
	wire := BuildDevtoolStatusSnapshot(
		EmptyBldrDevtoolStatus().
			WithCommand(BldrDevtoolCommandStatus{
				Name:    "start web",
				State:   BldrDevtoolCommandStateError,
				Summary: "failed",
				Error:   "compile failed",
				LogFile: ".bldr/logs/status.log",
			}).
			WithProject(BldrDevtoolProjectStatus{
				ProjectID:      "spacewave-devtool",
				StartupPlugins: []string{"web"},
				WebStartupPath: "bldr/web/devtool-status/startup.tsx",
				ManifestIDs:    []string{"spacewave-web"},
				BuildTargets: []BldrDevtoolBuildTargetRow{{
					ID:                  "web",
					ManifestIDs:         []string{"spacewave-web"},
					ConfiguredTargetIDs: []string{"browser"},
					ResolvedPlatformIDs: []string{"web/js/wasm", "js"},
					BuildTypes:          []string{"dev", "release"},
				}},
			}).
			WithManifestFetchRows([]BldrDevtoolManifestFetchRow{{
				ID:                  "fetch:web",
				ManifestID:          "spacewave-web",
				PlatformID:          "web/js/wasm, js",
				BuildType:           "dev, release",
				RemoteID:            "devtool",
				State:               BldrDevtoolManifestStateQueued,
				ReadyRefCount:       2,
				ReadyRefs:           "refs ready",
				LocalBuildIDs:       "build:web",
				BlockedOnLocalBuild: true,
			}}).
			WithManifestBuildRows([]BldrDevtoolManifestBuildRow{{
				ID:                "build:web",
				BuildTargets:      "web",
				ManifestID:        "spacewave-web",
				PlatformID:        "web/js/wasm",
				TargetPlatformIDs: "web/js/wasm,js",
				BuildType:         "dev",
				State:             BldrDevtoolManifestStateReady,
				CacheHit:          true,
			}}).
			WithControllerRows([]BldrDevtoolControllerRow{{
				ID:           "controller:web",
				ControllerID: "web",
				Kind:         "plugin",
				State:        BldrDevtoolControllerStateRunning,
			}}).
			WithPluginRows([]BldrDevtoolPluginRow{{
				ID:          "plugin:web",
				PluginID:    "web",
				InstanceKey: "main",
				State:       BldrDevtoolPluginStateErrored,
				Error:       "plugin failed",
			}}).
			WithAttentionRows([]BldrDevtoolAttentionRow{{
				ID:       "attention:plugin",
				Source:   "plugin",
				Message:  "plugin failed",
				Severity: BldrDevtoolAttentionSeverityError,
			}}),
	)

	if wire.GetCommand().GetState() != DevtoolStatusCommandState_DevtoolStatusCommandState_ERROR {
		t.Fatalf("expected command error state, got %#v", wire.GetCommand())
	}
	if wire.GetProject().GetProjectId() != "spacewave-devtool" {
		t.Fatalf("expected project snapshot, got %#v", wire.GetProject())
	}
	if got := wire.GetManifestFetchRows()[0].GetPlatformIds(); !slices.Equal(got, []string{"web/js/wasm", "js"}) {
		t.Fatalf("expected split fetch platforms, got %#v", got)
	}
	if wire.GetManifestBuildRows()[0].GetState() !=
		DevtoolStatusManifestState_DevtoolStatusManifestState_READY {
		t.Fatalf("expected ready build state, got %#v", wire.GetManifestBuildRows()[0])
	}
	if wire.GetControllerRows()[0].GetState() !=
		DevtoolStatusControllerState_DevtoolStatusControllerState_RUNNING {
		t.Fatalf("expected running controller state, got %#v", wire.GetControllerRows()[0])
	}
	if wire.GetPluginRows()[0].GetState() != DevtoolStatusPluginState_DevtoolStatusPluginState_ERRORED {
		t.Fatalf("expected errored plugin state, got %#v", wire.GetPluginRows()[0])
	}
	if wire.GetAttentionRows()[0].GetSeverity() !=
		DevtoolStatusAttentionSeverity_DevtoolStatusAttentionSeverity_ERROR {
		t.Fatalf("expected error attention severity, got %#v", wire.GetAttentionRows()[0])
	}
}

func TestDevtoolStatusSourceAvoidsPollingLogParsingAndControls(t *testing.T) {
	files := []string{
		"status-snapshot-proto.go",
		"project-status-adapter.go",
		"../../web/devtool-status/BldrDeveloperStatusApp.tsx",
		"../../web/devtool-status/startup.tsx",
	}
	forbidden := []string{
		"setInterval(",
		"setTimeout(",
		"requestAnimationFrame(",
		"fetch(",
		"WebSocket(",
		"ReadFile(",
		"readFile(",
		"createReadStream(",
		"parseLog",
		"tail -f",
		"BuildTargets(",
		"BuildManifests(",
		"UpdateProjectConfig(",
		"LoadPlugin(",
		"RestartRoutine(",
		"window.location.reload",
	}

	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		for _, needle := range forbidden {
			if strings.Contains(src, needle) {
				t.Fatalf("%s contains forbidden read-only status token %q", path, needle)
			}
		}
	}
}

func TestDevtoolStatusWebRuntimeRegistrationKeepsTinygoGate(t *testing.T) {
	body, err := os.ReadFile("../start-web-wasm.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	required := []string{
		"devtool_status.RegisterDevtoolStatusService(mux, d.GetStatusProducer())",
		"tinygoCompatible := false",
		"useTinygo := entryBuildType.IsRelease() && minifyEntrypoint && tinygoCompatible",
	}
	for _, needle := range required {
		if !strings.Contains(src, needle) {
			t.Fatalf("start-web-wasm.go missing required non-interference proof %q", needle)
		}
	}
	if strings.Contains(src, "tinygoCompatible := true") {
		t.Fatal("devtool status registration must not enable TinyGo")
	}
}
