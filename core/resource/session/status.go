package resource_session

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	spacewave_launcher_controller "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	"github.com/s4wave/spacewave/core/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

// RecoveryStatusRegistry owns volatile renderer-published recovery facts for
// logical sessions. Multiple mounted SessionResources for the same SessionRef
// share one status container through this registry.
type RecoveryStatusRegistry struct {
	mtx       sync.Mutex
	bySession map[string]*ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest]
}

// NewRecoveryStatusRegistry creates a new recovery status registry.
func NewRecoveryStatusRegistry() *RecoveryStatusRegistry {
	return &RecoveryStatusRegistry{bySession: make(map[string]*ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest])}
}

// GetSessionRecoveryStatusCtr returns the shared volatile recovery status
// container for sess.
func (r *RecoveryStatusRegistry) GetSessionRecoveryStatusCtr(
	sess session.Session,
) *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	if r == nil || sess == nil || sess.GetSessionRef() == nil {
		return newRendererRecoveryCtr()
	}
	return r.getSessionRecoveryStatusCtrForRef(sess.GetSessionRef())
}

func (r *RecoveryStatusRegistry) getSessionRecoveryStatusCtrForRef(
	ref *session.SessionRef,
) *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	if r == nil || ref == nil {
		return newRendererRecoveryCtr()
	}
	key := recoveryStatusSessionKey(ref)
	r.mtx.Lock()
	defer r.mtx.Unlock()
	ctr := r.bySession[key]
	if ctr == nil {
		ctr = newRendererRecoveryCtr()
		r.bySession[key] = ctr
	}
	return ctr
}

func recoveryStatusSessionKey(ref *session.SessionRef) string {
	providerRef := ref.GetProviderResourceRef()
	return providerRef.GetProviderId() + "/" + providerRef.GetProviderAccountId() + "/" + providerRef.GetId()
}

func newRendererRecoveryCtr() *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	return ccontainer.NewCContainerWithEqual(nil, rendererRecoveryStatusEqual)
}

// StatusResource implements the SystemStatusService for a session.
type StatusResource struct {
	b bus.Bus
	// rendererRecoveryCtr stores renderer-published, session-local recovery
	// facts. It is diagnostic status only and is not persisted.
	rendererRecoveryCtr *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest]
}

// NewStatusResource creates a new StatusResource.
func NewStatusResource(
	b bus.Bus,
	rendererRecoveryCtr *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest],
) *StatusResource {
	if rendererRecoveryCtr == nil {
		rendererRecoveryCtr = newRendererRecoveryCtr()
	}
	return &StatusResource{
		b:                   b,
		rendererRecoveryCtr: rendererRecoveryCtr,
	}
}

// WatchControllers streams the list of active controllers on change.
func (r *StatusResource) WatchControllers(
	_ *s4wave_status.WatchControllersRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchControllersStream,
) error {
	bcast := r.b.GetControllersBroadcast()
	return broadcast.WatchBroadcastVT(
		strm.Context(),
		bcast,
		func() *s4wave_status.WatchControllersResponse {
			ctrls := r.b.GetControllers()
			infos := make([]*s4wave_status.ControllerInfo, len(ctrls))
			for i, c := range ctrls {
				ci := c.GetControllerInfo()
				infos[i] = &s4wave_status.ControllerInfo{
					Id:          ci.GetId(),
					Version:     ci.GetVersion(),
					Description: ci.GetDescription(),
				}
			}
			return &s4wave_status.WatchControllersResponse{
				Controllers:     infos,
				ControllerCount: uint32(len(infos)),
			}
		},
		func(resp *s4wave_status.WatchControllersResponse) error {
			return strm.Send(resp)
		},
	)
}

// WatchDirectives streams the list of active directives on change.
func (r *StatusResource) WatchDirectives(
	_ *s4wave_status.WatchDirectivesRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchDirectivesStream,
) error {
	bcast := r.b.GetDirectivesBroadcast()
	return broadcast.WatchBroadcastVT(
		strm.Context(),
		bcast,
		func() *s4wave_status.WatchDirectivesResponse {
			dirs := r.b.GetDirectives()
			infos := make([]*s4wave_status.DirectiveInfo, len(dirs))
			for i, d := range dirs {
				infos[i] = &s4wave_status.DirectiveInfo{
					Name:  d.GetDirective().GetName(),
					Ident: d.GetDirectiveIdent(),
				}
			}
			return &s4wave_status.WatchDirectivesResponse{
				Directives:     infos,
				DirectiveCount: uint32(len(infos)),
			}
		},
		func(resp *s4wave_status.WatchDirectivesResponse) error {
			return strm.Send(resp)
		},
	)
}

// WatchPlugins streams the plugin host scheduler's live plugin instances.
func (r *StatusResource) WatchPlugins(
	_ *s4wave_status.WatchPluginsRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchPluginsStream,
) error {
	ctx := strm.Context()
	for {
		statusCtr, err := r.waitPluginStatusCtr(ctx)
		if err != nil {
			return err
		}
		current := statusCtr.GetValue()
		if err := strm.Send(buildPluginsResponse(current)); err != nil {
			return err
		}
		if err := ccontainer.WatchChanges(
			ctx,
			current,
			statusCtr,
			func(snapshot *plugin_host_scheduler.PluginStatusSnapshot) error {
				return strm.Send(buildPluginsResponse(snapshot))
			},
			nil,
		); err != nil {
			if ctx.Err() != nil {
				return err
			}
		}
	}
}

// ReportRecoveryStatus stores renderer-owned runtime recovery facts for status
// composition. The report is volatile diagnostic state; it never mutates
// release, Manifest, package, boot, or asset-serving owners.
func (r *StatusResource) ReportRecoveryStatus(
	_ context.Context,
	req *s4wave_status.ReportRecoveryStatusRequest,
) (*s4wave_status.ReportRecoveryStatusResponse, error) {
	if req == nil {
		return &s4wave_status.ReportRecoveryStatusResponse{}, nil
	}
	next := &s4wave_status.ReportRecoveryStatusRequest{}
	if boot := req.GetBoot(); boot != nil {
		next.Boot = boot.CloneVT()
		if next.Boot.Status == "" {
			next.Boot.Status = "reported"
		}
	}
	if asset := req.GetRuntimeAsset(); asset != nil {
		next.RuntimeAsset = asset.CloneVT()
		if next.RuntimeAsset.Status == "" {
			next.RuntimeAsset.Status = "reported"
		}
	}
	r.rendererRecoveryCtr.SetValue(next)
	return &s4wave_status.ReportRecoveryStatusResponse{}, nil
}

// WatchRecoveryStatus streams owner-owned runtime recovery status snapshots.
func (r *StatusResource) WatchRecoveryStatus(
	_ *s4wave_status.WatchRecoveryStatusRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchRecoveryStatusStream,
) error {
	ctx := strm.Context()
	changeCh := make(chan struct{}, 1)
	notify := func() {
		select {
		case changeCh <- struct{}{}:
		default:
		}
	}
	go r.watchRecoveryOwnerChanges(ctx, notify)
	go r.watchRecoveryRendererChanges(ctx, notify)

	var last *s4wave_status.RecoveryStatus
	for {
		status := r.buildRecoveryStatus()
		if last == nil || !last.EqualVT(status) {
			if err := strm.Send(&s4wave_status.WatchRecoveryStatusResponse{Status: status}); err != nil {
				return err
			}
			last = status.CloneVT()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changeCh:
		}
	}
}

func (r *StatusResource) watchRecoveryOwnerChanges(ctx context.Context, notify func()) {
	for {
		waitCh := r.controllersWaitCh()
		watchCtx, cancel := context.WithCancel(ctx)
		go r.watchRecoveryLauncherChanges(watchCtx, notify)
		go r.watchRecoveryPluginChanges(watchCtx, notify)
		go r.watchRecoveryPackageChanges(watchCtx, notify)
		select {
		case <-ctx.Done():
			cancel()
			return
		case <-waitCh:
			cancel()
			notify()
		}
	}
}

func (r *StatusResource) watchRecoveryLauncherChanges(ctx context.Context, notify func()) {
	ctr := r.findLauncherFetchStatusCtr()
	if ctr == nil {
		return
	}
	current := ctr.GetValue()
	_ = ccontainer.WatchChanges(
		ctx,
		current,
		ctr,
		func(*spacewave_launcher.FetchStatus) error {
			notify()
			return nil
		},
		nil,
	)
}

func (r *StatusResource) watchRecoveryPluginChanges(ctx context.Context, notify func()) {
	ctr := r.findPluginStatusCtr()
	if ctr == nil {
		return
	}
	current := ctr.GetValue()
	_ = ccontainer.WatchChanges(
		ctx,
		current,
		ctr,
		func(*plugin_host_scheduler.PluginStatusSnapshot) error {
			notify()
			return nil
		},
		nil,
	)
}

func (r *StatusResource) watchRecoveryRendererChanges(ctx context.Context, notify func()) {
	current := r.rendererRecoveryCtr.GetValue()
	_ = ccontainer.WatchChanges(
		ctx,
		current,
		r.rendererRecoveryCtr,
		func(*s4wave_status.ReportRecoveryStatusRequest) error {
			notify()
			return nil
		},
		nil,
	)
}

func (r *StatusResource) findLauncherFetchStatusCtr() ccontainer.Watchable[*spacewave_launcher.FetchStatus] {
	ctrl := spacewave_launcher_controller.FindControllerOnBus(r.b)
	if ctrl == nil {
		return nil
	}
	return ctrl.GetFetchStatusCtr()
}

func rendererRecoveryStatusEqual(
	a,
	b *s4wave_status.ReportRecoveryStatusRequest,
) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.EqualVT(b)
}

func (r *StatusResource) waitPluginStatusCtr(
	ctx context.Context,
) (ccontainer.Watchable[*plugin_host_scheduler.PluginStatusSnapshot], error) {
	for {
		if ctr := r.findPluginStatusCtr(); ctr != nil {
			return ctr, nil
		}
		waitCh := r.controllersWaitCh()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}

func (r *StatusResource) findPluginStatusCtr() ccontainer.Watchable[*plugin_host_scheduler.PluginStatusSnapshot] {
	for _, ctrl := range r.b.GetControllers() {
		scheduler, ok := ctrl.(*plugin_host_scheduler.Controller)
		if ok {
			return scheduler.GetPluginStatusCtr()
		}
	}
	return nil
}

func (r *StatusResource) controllersWaitCh() <-chan struct{} {
	var waitCh <-chan struct{}
	r.b.GetControllersBroadcast().HoldLock(func(
		broadcast func(),
		getWaitCh func() <-chan struct{},
	) {
		waitCh = getWaitCh()
	})
	return waitCh
}

func buildPluginsResponse(snapshot *plugin_host_scheduler.PluginStatusSnapshot) *s4wave_status.WatchPluginsResponse {
	var infos []*s4wave_status.PluginInfo
	if snapshot != nil {
		infos = make([]*s4wave_status.PluginInfo, 0, len(snapshot.Plugins))
		for _, plugin := range snapshot.Plugins {
			infos = append(infos, &s4wave_status.PluginInfo{
				Id:          plugin.GetPluginId(),
				InstanceKey: plugin.GetInstanceKey(),
				State:       pluginStateString(plugin.GetState()),
			})
		}
	}
	return &s4wave_status.WatchPluginsResponse{
		Plugins:     infos,
		PluginCount: uint32(len(infos)),
	}
}

func (r *StatusResource) buildRecoveryStatus() *s4wave_status.RecoveryStatus {
	pluginSnapshot := (*plugin_host_scheduler.PluginStatusSnapshot)(nil)
	if statusCtr := r.findPluginStatusCtr(); statusCtr != nil {
		pluginSnapshot = statusCtr.GetValue()
	}
	renderer := r.rendererRecoveryCtr.GetValue()
	return &s4wave_status.RecoveryStatus{
		Launcher:       r.buildLauncherRecoveryStatus(),
		Plugins:        buildPluginManifestRecoveryStatuses(pluginSnapshot),
		NativePackages: r.buildNativePackageRecoveryStatuses(),
		Boot:           buildBrowserBootRecoveryStatus(renderer),
		RuntimeAsset:   buildRuntimeAssetRecoveryStatus(renderer),
	}
}

func (r *StatusResource) buildLauncherRecoveryStatus() *s4wave_status.LauncherRecoveryStatus {
	ctrl := spacewave_launcher_controller.FindControllerOnBus(r.b)
	if ctrl == nil || ctrl.GetFetchStatusCtr() == nil {
		return nil
	}
	status := ctrl.GetFetchStatusCtr().GetValue()
	if status == nil {
		return nil
	}
	return &s4wave_status.LauncherRecoveryStatus{
		SelectedConfigRev:      status.SelectedConfigRev,
		SelectedConfigSource:   status.SelectedConfigSource,
		FetchedConfigRev:       status.FetchedConfigRev,
		FetchedConfigSource:    status.FetchedConfigSource,
		ReleaseMetadataOutcome: status.ReleaseMetadataOutcome,
	}
}

func buildPluginManifestRecoveryStatuses(
	snapshot *plugin_host_scheduler.PluginStatusSnapshot,
) []*s4wave_status.PluginManifestRecoveryStatus {
	if snapshot == nil || len(snapshot.ManifestRecovery) == 0 {
		return nil
	}
	out := make([]*s4wave_status.PluginManifestRecoveryStatus, 0, len(snapshot.ManifestRecovery))
	for _, row := range snapshot.ManifestRecovery {
		if row == nil {
			continue
		}
		out = append(out, &s4wave_status.PluginManifestRecoveryStatus{
			PluginId:                    row.PluginID,
			InstanceKey:                 row.InstanceKey,
			ExecuteManifestRef:          row.ExecuteManifestRef,
			DownloadManifestRef:         row.DownloadManifestRef,
			SkippedCandidateCount:       uint32(row.SkippedCandidateCount),
			SkippedCandidateSummary:     row.SkippedCandidateSummary,
			IgnoredCandidateCount:       uint32(row.IgnoredCandidateCount),
			IgnoredCandidateSummary:     row.IgnoredCandidateSummary,
			QuarantinedCandidateCount:   uint32(row.QuarantinedCandidateCount),
			QuarantinedCandidateSummary: row.QuarantinedCandidateSummary,
		})
	}
	return out
}

func buildBrowserBootRecoveryStatus(
	renderer *s4wave_status.ReportRecoveryStatusRequest,
) *s4wave_status.BrowserBootRecoveryStatus {
	if renderer == nil || renderer.GetBoot() == nil {
		return &s4wave_status.BrowserBootRecoveryStatus{Status: "not-reported"}
	}
	status := renderer.GetBoot().CloneVT()
	if status.Status == "" {
		status.Status = "reported"
	}
	return status
}

func buildRuntimeAssetRecoveryStatus(
	renderer *s4wave_status.ReportRecoveryStatusRequest,
) *s4wave_status.RuntimeAssetRecoveryStatus {
	if renderer == nil || renderer.GetRuntimeAsset() == nil {
		return &s4wave_status.RuntimeAssetRecoveryStatus{Status: "not-reported"}
	}
	status := renderer.GetRuntimeAsset().CloneVT()
	if status.Status == "" {
		status.Status = "reported"
	}
	return status
}

func formatRecoveryStatusTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339Nano)
}

func pluginStateString(state bldr_plugin.PluginState) string {
	switch state {
	case bldr_plugin.PluginState_PluginState_REQUESTED:
		return "requested"
	case bldr_plugin.PluginState_PluginState_RUNNING:
		return "running"
	default:
		return "unknown"
	}
}

// _ is a type assertion
var _ s4wave_status.SRPCSystemStatusServiceServer = ((*StatusResource)(nil))
