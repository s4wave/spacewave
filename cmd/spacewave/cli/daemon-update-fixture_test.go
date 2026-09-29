//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// fixtureLauncherController resolves the plugin-qualified launcher lookup to
// the bare service invoker that the production launcher client calls.
type fixtureLauncherController struct {
	// mux serves the launcher's bare RPC service identifier.
	mux srpc.Mux
}

// fixtureLauncherInvoker forwards the bare launcher service without exposing
// a queryable mux for the plugin-qualified lookup filter.
type fixtureLauncherInvoker struct {
	// inner serves the launcher's bare service identifier.
	inner srpc.Invoker
}

// InvokeMethod forwards one launcher RPC method.
func (f *fixtureLauncherInvoker) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	return f.inner.InvokeMethod(serviceID, methodID, stream)
}

// GetControllerInfo identifies this fixture-only launcher route.
func (f *fixtureLauncherController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/launcher", controller.MustParseVersion("0.0.1"), "")
}

// Execute keeps this static fixture route registered for its bus lifetime.
func (f *fixtureLauncherController) Execute(context.Context) error { return nil }

// HandleDirective publishes the bare invoker for the qualified launcher route.
func (f *fixtureLauncherController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	lookup, ok := inst.GetDirective().(bifrost_rpc.LookupRpcService)
	if !ok || lookup.LookupRpcServiceID() != spacewave_launcher.PluginLauncherServiceID {
		return nil, nil
	}
	return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(&fixtureLauncherInvoker{inner: f.mux}), nil)
}

// Close releases no resources beyond the bus-owned mux.
func (f *fixtureLauncherController) Close() error { return nil }

// fixtureLauncher publishes the selected CLI artifact to the production
// daemon update watch when the isolated Resource fixture accepts it.
type fixtureLauncher struct {
	// state holds the selected and accepted launcher snapshots.
	state *ccontainer.CContainer[*spacewave_launcher.LauncherInfo]
	// path and statePath contain only controlled fixture files.
	path, statePath string
	// corruptOnce changes the selected bytes after the first acceptance.
	corruptOnce  bool
	corruptReady chan struct{}
	mu           sync.Mutex
}

// newFixtureLauncher constructs the fixture's staged CLI selection.
func newFixtureLauncher(path, digest, statePath string, corruptOnce bool) *fixtureLauncher {
	f := &fixtureLauncher{path: path, statePath: statePath, corruptOnce: corruptOnce}
	if corruptOnce {
		f.corruptReady = make(chan struct{})
	}
	f.state = ccontainer.NewCContainer(&spacewave_launcher.LauncherInfo{
		DaemonUpdateState: &spacewave_launcher.UpdateState{
			Phase:              spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
			Version:            "fixture-new",
			StagedPath:         path,
			StagedSha256:       digest,
			Target:             desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
			ArtifactManifestId: "spacewave-cli",
		},
	})
	return f
}

// WatchLauncherInfo streams the current selection and its accepted transition.
func (f *fixtureLauncher) WatchLauncherInfo(
	_ *spacewave_launcher.WatchLauncherInfoRequest,
	stream spacewave_launcher.SRPCLauncher_WatchLauncherInfoStream,
) error {
	return ccontainer.WatchChanges(stream.Context(), nil, f.state, func(info *spacewave_launcher.LauncherInfo) error {
		if info.GetDaemonUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING && f.corruptReady != nil {
			select {
			case <-f.corruptReady:
			case <-stream.Context().Done():
				return stream.Context().Err()
			}
		}
		return stream.Send(info)
	}, nil)
}

// PushDistConfigMsg is outside the fixture's update contract.
func (f *fixtureLauncher) PushDistConfigMsg(context.Context, *spacewave_launcher.PushDistConfigRequest) (*spacewave_launcher.PushDistConfigResponse, error) {
	return nil, errors.New("fixture does not accept DistConfig")
}

// RecheckDistConfig is outside the fixture's update contract.
func (f *fixtureLauncher) RecheckDistConfig(context.Context, *spacewave_launcher.RecheckDistConfigRequest) (*spacewave_launcher.RecheckDistConfigResponse, error) {
	return nil, errors.New("fixture does not recheck DistConfig")
}

// ApplyUpdate publishes accepted daemon intent independently of its requester.
func (f *fixtureLauncher) ApplyUpdate(_ context.Context, req *desktop_update.ApplyUpdateRequest) (*desktop_update.ApplyUpdateResponse, error) {
	if req.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON {
		return nil, errors.New("fixture accepts only daemon updates")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	info := f.state.GetValue().CloneVT()
	info.DaemonUpdateState.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING
	info.DaemonUpdateState.ErrorMessage = ""
	f.state.SetValue(info)
	if f.corruptOnce {
		f.corruptOnce = false
		if err := os.WriteFile(f.path, []byte("changed after acceptance"), 0o700); err != nil {
			return nil, err
		}
		close(f.corruptReady)
	}
	return &desktop_update.ApplyUpdateResponse{}, nil
}

// ReportDaemonUpdateFailure exposes the old daemon's pre-claim failure.
func (f *fixtureLauncher) ReportDaemonUpdateFailure(_ context.Context, req *spacewave_launcher.ReportDaemonUpdateFailureRequest) (*spacewave_launcher.ReportDaemonUpdateFailureResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := f.state.GetValue().CloneVT()
	current := info.GetDaemonUpdateState()
	selected := req.GetSelection()
	if current.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING ||
		current.GetStagedPath() != selected.GetStagedPath() || current.GetStagedSha256() != selected.GetStagedSha256() {
		return &spacewave_launcher.ReportDaemonUpdateFailureResponse{}, nil
	}
	current.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR
	current.ErrorMessage = req.GetErrorMessage()
	f.state.SetValue(info)
	if err := os.WriteFile(filepath.Join(f.statePath, "update-failed"), nil, 0o600); err != nil {
		return nil, err
	}
	return &spacewave_launcher.ReportDaemonUpdateFailureResponse{Reported: true}, nil
}

// ClaimDaemonUpdate records a completed fixture owner idle claim.
func (f *fixtureLauncher) ClaimDaemonUpdate(_ context.Context, req *spacewave_launcher.ClaimDaemonUpdateRequest) (*spacewave_launcher.ClaimDaemonUpdateResponse, error) {
	current := f.state.GetValue().GetDaemonUpdateState()
	return &spacewave_launcher.ClaimDaemonUpdateResponse{Claimed: current.EqualVT(req.GetSelection())}, nil
}

// ReportDaemonUpdateWait publishes the old daemon's other work.
func (f *fixtureLauncher) ReportDaemonUpdateWait(_ context.Context, req *spacewave_launcher.ReportDaemonUpdateWaitRequest) (*spacewave_launcher.ReportDaemonUpdateWaitResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := f.state.GetValue().CloneVT()
	if !info.GetDaemonUpdateState().EqualVT(req.GetSelection()) {
		return &spacewave_launcher.ReportDaemonUpdateWaitResponse{}, nil
	}
	if info.DaemonUpdateWait == nil {
		info.DaemonUpdateWait = &spacewave_launcher.DaemonUpdateWait{}
	}
	info.DaemonUpdateWait.OtherClients = req.GetOtherClients()
	info.DaemonUpdateWait.OtherServices = req.GetOtherServices()
	f.state.SetValue(info)
	return &spacewave_launcher.ReportDaemonUpdateWaitResponse{Reported: true}, nil
}

// RestartDaemonUpdateNow asks the waiting old daemon to hand off at once.
func (f *fixtureLauncher) RestartDaemonUpdateNow(context.Context, *spacewave_launcher.RestartDaemonUpdateNowRequest) (*spacewave_launcher.RestartDaemonUpdateNowResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := f.state.GetValue().CloneVT()
	if info.GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
		return nil, errors.New("no accepted daemon update is waiting")
	}
	if info.DaemonUpdateWait == nil {
		info.DaemonUpdateWait = &spacewave_launcher.DaemonUpdateWait{}
	}
	info.DaemonUpdateWait.RestartNow = true
	f.state.SetValue(info)
	return &spacewave_launcher.RestartDaemonUpdateNowResponse{}, nil
}

// restartWhenBusy waits for the old daemon to report its other clients, then
// requests Restart now and returns the reported wait.
func (f *fixtureLauncher) restartWhenBusy(ctx context.Context) (*spacewave_launcher.DaemonUpdateWait, error) {
	info, err := f.state.WaitValueWithValidator(ctx, func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		return info.GetDaemonUpdateWait().GetOtherClients() != 0, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	if _, err := f.RestartDaemonUpdateNow(ctx, &spacewave_launcher.RestartDaemonUpdateNowRequest{}); err != nil {
		return nil, err
	}
	return info.GetDaemonUpdateWait(), nil
}

// restore puts the originally verified fixture bytes back for an explicit retry.
func (f *fixtureLauncher) restore() error {
	original, err := os.ReadFile(filepath.Join(f.statePath, "old-spacewave"))
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, original, 0o700)
}

// fixtureUpdateTrigger routes a test-only Resource call to the launcher while
// the production serve path owns the daemon update subscription.
type fixtureUpdateTrigger struct {
	// launcher publishes the accepted state when the Resource call completes.
	launcher *fixtureLauncher
}

// GetServiceID identifies the fixture's isolated acceptance route.
func (f *fixtureUpdateTrigger) GetServiceID() string { return "test.DaemonUpdate" }

// GetMethodIDs lists the fixture's acceptance operation.
func (f *fixtureUpdateTrigger) GetMethodIDs() []string {
	return []string{"Accept", "Restart", "Restore", "Status"}
}

// InvokeMethod accepts the selected daemon update through a Resource stream.
func (f *fixtureUpdateTrigger) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	if serviceID != f.GetServiceID() {
		return false, nil
	}
	if methodID == "Status" {
		if err := stream.MsgRecv(&spacewave_launcher.WatchLauncherInfoRequest{}); err != nil && err != io.EOF {
			return true, err
		}
		if err := stream.MsgSend(f.launcher.state.GetValue()); err != nil {
			return true, err
		}
		return true, stream.CloseSend()
	}
	if methodID == "Restart" {
		if err := stream.MsgRecv(&spacewave_launcher.RestartDaemonUpdateNowRequest{}); err != nil && err != io.EOF {
			return true, err
		}
		wait, err := f.launcher.restartWhenBusy(stream.Context())
		if err != nil {
			return true, err
		}
		if err := stream.MsgSend(wait); err != nil {
			return true, err
		}
		return true, stream.CloseSend()
	}
	if methodID == "Restore" {
		if err := stream.MsgRecv(&spacewave_launcher.WatchLauncherInfoRequest{}); err != nil && err != io.EOF {
			return true, err
		}
		if err := f.launcher.restore(); err != nil {
			return true, err
		}
		if err := stream.MsgSend(&spacewave_launcher.WatchLauncherInfoRequest{}); err != nil {
			return true, err
		}
		return true, stream.CloseSend()
	}
	if methodID != "Accept" {
		return false, nil
	}
	req := &desktop_update.ApplyUpdateRequest{}
	if err := stream.MsgRecv(req); err != nil && err != io.EOF {
		return true, err
	}
	resp, err := f.launcher.ApplyUpdate(stream.Context(), req)
	if err != nil {
		return true, err
	}
	if err := stream.MsgSend(resp); err != nil {
		return true, err
	}
	return true, stream.CloseSend()
}

// _ is a type assertion.
var (
	_ controller.Controller                 = (*fixtureLauncherController)(nil)
	_ spacewave_launcher.SRPCLauncherServer = (*fixtureLauncher)(nil)
	_ srpc.Invoker                          = (*fixtureLauncherInvoker)(nil)
	_ srpc.Handler                          = (*fixtureUpdateTrigger)(nil)
)
