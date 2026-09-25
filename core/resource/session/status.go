package resource_session

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/space"
	spacewave_transport "github.com/s4wave/spacewave/core/transport"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

// StatusResource implements the SystemStatusService for a session.
type StatusResource struct {
	b    bus.Bus
	sess session.Session
	// rendererRecoveryCtr stores renderer-published, session-local recovery
	// facts. It is diagnostic status only and is not persisted.
	rendererRecoveryCtr *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest]
}

// NewStatusResource creates a new StatusResource.
func NewStatusResource(
	b bus.Bus,
	rendererRecoveryCtr *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest],
) *StatusResource {
	return NewStatusResourceWithSession(b, nil, rendererRecoveryCtr)
}

// NewStatusResourceWithSession creates a new StatusResource for sess.
func NewStatusResourceWithSession(
	b bus.Bus,
	sess session.Session,
	rendererRecoveryCtr *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest],
) *StatusResource {
	if rendererRecoveryCtr == nil {
		rendererRecoveryCtr = newRendererRecoveryCtr()
	}
	return &StatusResource{
		b:                   b,
		sess:                sess,
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
				ControllerCount: uint32(len(infos)), //nolint:gosec // infos is the bounded response collection.
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
				DirectiveCount: uint32(len(infos)), //nolint:gosec // infos is the bounded response collection.
			}
		},
		func(resp *s4wave_status.WatchDirectivesResponse) error {
			return strm.Send(resp)
		},
	)
}

// WatchPlugins streams the plugin instances of every plugin host scheduler
// reachable from the session bus: the root scheduler, when mounted, and the
// scheduler of each running Space runtime. The first snapshot is sent once the
// scheduler lookup is idle, so the stream starts even when no scheduler exists.
func (r *StatusResource) WatchPlugins(
	_ *s4wave_status.WatchPluginsRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchPluginsStream,
) error {
	var prev *s4wave_status.WatchPluginsResponse
	return watchPluginSchedulers(strm.Context(), r.b, func(
		schedulers []bldr_plugin.PluginScheduler,
		snapshots []*bldr_plugin.PluginStatusSnapshot,
	) error {
		resp := buildPluginsResponse(schedulers, snapshots)
		if prev != nil && resp.EqualVT(prev) {
			return nil
		}
		prev = resp
		return strm.Send(resp)
	})
}

// watchPluginSchedulers calls cb with every plugin host scheduler reachable
// from b and its current status snapshot, then again after each change to the
// scheduler set or a snapshot. snapshots[i] belongs to schedulers[i]. It
// returns when ctx ends, the lookup fails, or cb returns an error.
func watchPluginSchedulers(
	ctx context.Context,
	b bus.Bus,
	cb func([]bldr_plugin.PluginScheduler, []*bldr_plugin.PluginStatusSnapshot) error,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// bcast guards schedulers, collected, and lookupErr.
	var bcast broadcast.Broadcast
	var schedulers []bldr_plugin.PluginScheduler
	var collected bool
	var lookupErr error
	_, release, err := bus.ExecCollectValuesWatch(
		ctx,
		b,
		bldr_plugin.NewLookupPluginScheduler(),
		true,
		func(_ []error, vals []bldr_plugin.LookupPluginSchedulerValue) error {
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				schedulers = slices.Clone(vals)
				collected = true
				broadcast()
			})
			return nil
		},
		func(err error) {
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				lookupErr = err
				broadcast()
			})
		},
	)
	if err != nil {
		return err
	}
	defer release()

	for {
		var current []bldr_plugin.PluginScheduler
		var ready bool
		var setWaitCh <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			current, ready, err, setWaitCh = schedulers, collected, lookupErr, getWaitCh()
		})
		if err != nil {
			return err
		}
		if !ready {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-setWaitCh:
				continue
			}
		}

		snapshots := make([]*bldr_plugin.PluginStatusSnapshot, len(current))
		for i, scheduler := range current {
			snapshots[i] = scheduler.GetPluginStatusCtr().GetValue()
		}
		if err := cb(current, snapshots); err != nil {
			return err
		}

		waitCtx, waitCancel := context.WithCancel(ctx)
		statusCh := waitPluginStatusChange(waitCtx, current, snapshots)
		select {
		case <-ctx.Done():
			waitCancel()
			return ctx.Err()
		case <-setWaitCh:
		case <-statusCh:
		}
		waitCancel()
	}
}

// waitPluginStatusChange returns a channel closed when any scheduler's status
// snapshot differs from the matching entry in snapshots. The watches end with
// ctx.
func waitPluginStatusChange(
	ctx context.Context,
	schedulers []bldr_plugin.PluginScheduler,
	snapshots []*bldr_plugin.PluginStatusSnapshot,
) <-chan struct{} {
	changed := make(chan struct{})
	var once sync.Once
	for i, scheduler := range schedulers {
		go func() {
			_, err := scheduler.GetPluginStatusCtr().WaitValueChange(ctx, snapshots[i], nil)
			if err == nil {
				once.Do(func() { close(changed) })
			}
		}()
	}
	return changed
}

// WatchNetworkStats streams the session transport's live bifrost link snapshot.
func (r *StatusResource) WatchNetworkStats(
	_ *s4wave_status.WatchNetworkStatsRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchNetworkStatsStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_status.WatchNetworkStatsResponse
	for {
		resp, waitChs := r.buildNetworkStatsResponse()
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp.CloneVT()
		}
		if err := broadcast.WaitAny(ctx, waitChs...); err != nil {
			return err
		}
	}
}

// ReportRecoveryStatus stores renderer-owned runtime recovery facts for status
// composition. The report is volatile diagnostic state; it never mutates
// release, Manifest, package, boot, or asset-serving state.
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

// WatchRecoveryStatus streams the recorded runtime recovery status snapshots.
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
	rootPlugins := ccontainer.NewCContainer[*bldr_plugin.PluginStatusSnapshot](nil)
	launcher := spacewave_launcher.NewInfoWatcher(nil, r.b)
	launcher.SetContext(ctx)
	go r.watchRecoveryOwnerChanges(ctx, notify)
	go r.watchRecoveryRendererChanges(ctx, notify)
	go watchRecoveryPluginChanges(ctx, r.b, rootPlugins, notify)
	go watchRecoveryLauncherChanges(ctx, launcher, notify)

	var last *s4wave_status.RecoveryStatus
	for {
		launcherInfo, _ := launcher.Snapshot()
		status := r.buildRecoveryStatus(launcherInfo, rootPlugins.GetValue().GetManifestRecovery())
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

// watchRecoveryOwnerChanges restarts owner watches when the controller set changes until cancellation.
func (r *StatusResource) watchRecoveryOwnerChanges(ctx context.Context, notify func()) {
	for {
		waitCh := r.controllersWaitCh()
		watchCtx, cancel := context.WithCancel(ctx)
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

// watchRecoveryLauncherChanges notifies on each launcher info change until ctx ends.
func watchRecoveryLauncherChanges(
	ctx context.Context,
	launcher *spacewave_launcher.InfoWatcher,
	notify func(),
) {
	for {
		_, waitCh := launcher.Snapshot()
		notify()
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
		}
	}
}

// watchRecoveryPluginChanges publishes the root plugin host's status snapshot
// to rootPlugins until ctx ends.
func watchRecoveryPluginChanges(
	ctx context.Context,
	b bus.Bus,
	rootPlugins *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot],
	notify func(),
) {
	_ = watchPluginSchedulers(ctx, b, func(
		schedulers []bldr_plugin.PluginScheduler,
		snapshots []*bldr_plugin.PluginStatusSnapshot,
	) error {
		var root *bldr_plugin.PluginStatusSnapshot
		for i, scheduler := range schedulers {
			if scheduler.GetInstanceKey() == "" {
				root = snapshots[i]
				break
			}
		}
		rootPlugins.SetValue(root)
		notify()
		return nil
	})
}

// watchRecoveryRendererChanges watches volatile renderer facts until the status stream ends.
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

// rendererRecoveryStatusEqual compares nullable renderer snapshots by value.
func rendererRecoveryStatusEqual(
	a,
	b *s4wave_status.ReportRecoveryStatusRequest,
) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.EqualVT(b)
}

// controllersWaitCh captures controller-change notification under its owning lock.
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

// buildPluginsResponse merges the scheduler snapshots into plugin status
// records sorted by Space, plugin, and instance. snapshots[i] belongs to
// schedulers[i].
func buildPluginsResponse(
	schedulers []bldr_plugin.PluginScheduler,
	snapshots []*bldr_plugin.PluginStatusSnapshot,
) *s4wave_status.WatchPluginsResponse {
	var infos []*s4wave_status.PluginInfo
	for i, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		schedulerKey := schedulers[i].GetInstanceKey()
		for _, plugin := range snapshot.Plugins {
			infos = append(infos, &s4wave_status.PluginInfo{
				Id:          plugin.GetPluginId(),
				InstanceKey: plugin.GetInstanceKey(),
				State:       pluginStateString(plugin.GetState()),
				SpaceId:     pluginSpaceID(schedulerKey, plugin.GetInstanceKey()),
			})
		}
	}
	slices.SortFunc(infos, func(a, b *s4wave_status.PluginInfo) int {
		return cmp.Or(
			cmp.Compare(a.GetSpaceId(), b.GetSpaceId()),
			cmp.Compare(a.GetId(), b.GetId()),
			cmp.Compare(a.GetInstanceKey(), b.GetInstanceKey()),
		)
	})
	return &s4wave_status.WatchPluginsResponse{
		Plugins:     infos,
		PluginCount: uint32(len(infos)), //nolint:gosec // infos is the bounded response collection.
	}
}

// pluginSpaceID returns the engine ID of the Space a plugin instance serves: the
// Space runtime's scheduler key, or on the root host, an instance key that
// names a Space engine. Empty for system plugins.
func pluginSpaceID(schedulerKey, instanceKey string) string {
	if schedulerKey != "" {
		return schedulerKey
	}
	if strings.HasPrefix(instanceKey, space.SpaceBodyType+"/") {
		return instanceKey
	}
	return ""
}

// networkStatusProvider exposes the account-owned transport and its lifecycle.
type networkStatusProvider interface {
	// GetSessionTransport returns the active transport, or nil.
	GetSessionTransport() *spacewave_transport.SessionTransport
	// GetTransportSnapshotWithWait returns running state and its change channel.
	GetTransportSnapshotWithWait() (bool, <-chan struct{})
}

// buildNetworkStatsResponse projects live transport links into network status records.
func (r *StatusResource) buildNetworkStatsResponse() (*s4wave_status.WatchNetworkStatsResponse, []<-chan struct{}) {
	resp := &s4wave_status.WatchNetworkStatsResponse{}
	if r.sess == nil || r.sess.GetProviderAccount() == nil {
		return resp, nil
	}
	provider, ok := r.sess.GetProviderAccount().(networkStatusProvider)
	if !ok {
		return resp, nil
	}
	transportRunning, transportWaitCh := provider.GetTransportSnapshotWithWait()
	resp.TransportRunning = transportRunning
	waitChs := []<-chan struct{}{transportWaitCh}
	st := provider.GetSessionTransport()
	if st == nil {
		return resp, waitChs
	}
	resp.LocalPeerId = st.GetPeerID().String()
	links, linkWaitChs := st.GetLinkSnapshotsWithWait()
	waitChs = append(waitChs, linkWaitChs...)
	slices.SortFunc(links, compareNetworkLinkSnapshots)
	return buildNetworkStatsResponse(resp, links), waitChs
}

// compareNetworkLinkSnapshots orders links by remote identity and link identifier.
func compareNetworkLinkSnapshots(a, b transport_controller.LinkSnapshot) int {
	if n := cmp.Compare(a.RemotePeerID.String(), b.RemotePeerID.String()); n != 0 {
		return n
	}
	return cmp.Compare(a.LinkID, b.LinkID)
}

// buildNetworkStatsResponse projects live transport links into network status records.
func buildNetworkStatsResponse(
	resp *s4wave_status.WatchNetworkStatsResponse,
	links []transport_controller.LinkSnapshot,
) *s4wave_status.WatchNetworkStatsResponse {
	peersByID := make(map[string]*s4wave_status.NetworkPeerInfo)
	for _, link := range links {
		peerID := link.RemotePeerID.String()
		peerInfo := peersByID[peerID]
		if peerInfo == nil {
			peerInfo = &s4wave_status.NetworkPeerInfo{PeerId: peerID}
			peersByID[peerID] = peerInfo
		}
		peerInfo.Links = append(peerInfo.Links, &s4wave_status.NetworkLinkInfo{
			LocalPeerId:       link.LocalPeerID.String(),
			RemotePeerId:      peerID,
			LinkId:            link.LinkID,
			TransportId:       link.TransportID,
			RemoteTransportId: link.RemoteTransportID,
		})
	}
	resp.Peers = make([]*s4wave_status.NetworkPeerInfo, 0, len(peersByID))
	for _, peerInfo := range peersByID {
		peerInfo.LinkCount = uint32(len(peerInfo.Links)) //nolint:gosec // links are the bounded peer response collection.
		resp.Peers = append(resp.Peers, peerInfo)
	}
	slices.SortFunc(resp.Peers, func(a, b *s4wave_status.NetworkPeerInfo) int {
		return cmp.Compare(a.GetPeerId(), b.GetPeerId())
	})
	resp.PeerCount = uint32(len(resp.Peers)) //nolint:gosec // peers is the bounded response collection.
	resp.LinkCount = uint32(len(links))      //nolint:gosec // links is the bounded response collection.
	return resp
}

// buildRecoveryStatus combines the launcher info, current owner facts, the root
// plugin host's manifest recovery rows, and volatile renderer facts.
func (r *StatusResource) buildRecoveryStatus(
	launcher *spacewave_launcher.LauncherInfo,
	plugins []*bldr_plugin.PluginManifestRecoveryStatus,
) *s4wave_status.RecoveryStatus {
	renderer := r.rendererRecoveryCtr.GetValue()
	return &s4wave_status.RecoveryStatus{
		Launcher:       buildLauncherRecoveryStatus(launcher),
		Plugins:        plugins,
		NativePackages: r.buildNativePackageRecoveryStatuses(),
		Boot:           buildBrowserBootRecoveryStatus(renderer),
		RuntimeAsset:   buildRuntimeAssetRecoveryStatus(renderer),
	}
}

// buildLauncherRecoveryStatus projects the launcher metadata and update state,
// or returns nil when no launcher is reachable.
func buildLauncherRecoveryStatus(info *spacewave_launcher.LauncherInfo) *s4wave_status.LauncherRecoveryStatus {
	if info == nil {
		return nil
	}
	fetch := info.GetFetchStatus()
	state := info.GetUpdateState()
	return &s4wave_status.LauncherRecoveryStatus{
		SelectedChannelKey:            info.GetDistConfig().ResolvedChannelKey(),
		SelectedConfigRev:             fetch.GetSelectedConfigRev(),
		SelectedConfigSource:          distConfigSourceString(fetch.GetSelectedConfigSource()),
		FetchedConfigRev:              fetch.GetFetchedConfigRev(),
		FetchedConfigSource:           fetch.GetFetchedConfigSource(),
		ReleaseMetadataOutcome:        releaseMetadataOutcomeString(fetch.GetReleaseMetadataOutcome()),
		ReleaseWorldHeadRef:           fetch.GetReleaseWorldHeadRef(),
		SelectedEntrypointManifestId:  fetch.GetSelectedEntrypointManifestId(),
		SelectedEntrypointPlatformId:  fetch.GetSelectedEntrypointPlatformId(),
		SelectedEntrypointManifestRev: fetch.GetSelectedEntrypointManifestRev(),
		SelectedEntrypointManifestRef: fetch.GetSelectedEntrypointManifestRef(),
		UpdatePhase:                   launcherUpdatePhaseString(state.GetPhase()),
		UpdateVersion:                 state.GetVersion(),
		StagedPath:                    state.GetStagedPath(),
		UpdateError:                   state.GetErrorMessage(),
	}
}

// distConfigSourceString formats the selected DistConfig source for diagnostic status.
func distConfigSourceString(source spacewave_launcher.DistConfigSource) string {
	switch source {
	case spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_NONE:
		return "none"
	case spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_STORED:
		return "stored"
	case spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_PACKAGE:
		return "package"
	case spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_EMBEDDED_DEFAULT:
		return "embedded-default"
	case spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_ENDPOINT:
		return "endpoint"
	default:
		return ""
	}
}

// releaseMetadataOutcomeString formats the release metadata outcome for diagnostic status.
func releaseMetadataOutcomeString(outcome spacewave_launcher.ReleaseMetadataOutcome) string {
	switch outcome {
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_PENDING:
		return "pending"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_IDLE:
		return "idle"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_RESOLVING:
		return "resolving"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_REFRESHING:
		return "refreshing"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_CURRENT:
		return "current"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_DOWNLOADING:
		return "downloading"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_STAGED:
		return "staged"
	case spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_ERROR:
		return "error"
	default:
		return ""
	}
}

// launcherUpdatePhaseString formats the launcher update phase for diagnostic status.
func launcherUpdatePhaseString(phase spacewave_launcher.UpdatePhase) string {
	switch phase {
	case spacewave_launcher.UpdatePhase_UpdatePhase_IDLE:
		return "idle"
	case spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING:
		return "downloading"
	case spacewave_launcher.UpdatePhase_UpdatePhase_STAGED:
		return "staged"
	case spacewave_launcher.UpdatePhase_UpdatePhase_APPLYING:
		return "applying"
	case spacewave_launcher.UpdatePhase_UpdatePhase_ERROR:
		return "error"
	default:
		return "unknown"
	}
}

// buildBrowserBootRecoveryStatus copies the renderer boot report or marks it unreported.
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

// buildRuntimeAssetRecoveryStatus copies the renderer asset report or marks it unreported.
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

// formatRecoveryStatusTime formats a recorded recovery timestamp in UTC.
func formatRecoveryStatusTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339Nano)
}

// pluginStateString formats the plugin lifecycle for diagnostic status.
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

// _ verifies the status service contract.
var _ s4wave_status.SRPCSystemStatusServiceServer = (*StatusResource)(nil)
