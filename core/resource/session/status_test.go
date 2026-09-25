package resource_session

import (
	"context"
	"slices"
	"testing"
	"time"

	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/core/provider"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/peer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	"github.com/sirupsen/logrus"
)

func TestReportRecoveryStatusPublishesRendererFacts(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	statusRes := NewStatusResource(b, nil)

	initial := statusRes.buildRecoveryStatus(nil, nil)
	if initial.GetBoot().GetStatus() != "not-reported" {
		t.Fatalf("initial boot status = %q, want not-reported", initial.GetBoot().GetStatus())
	}
	if initial.GetRuntimeAsset().GetStatus() != "not-reported" {
		t.Fatalf("initial asset status = %q, want not-reported", initial.GetRuntimeAsset().GetStatus())
	}

	_, err := statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
		RuntimeAsset: &s4wave_status.RuntimeAssetRecoveryStatus{
			ScriptPath:     "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs",
			StatusCode:     404,
			Classification: "missing",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := statusRes.buildRecoveryStatus(nil, nil)
	if status.GetBoot().GetCompatibilityVersion() != "1000000" ||
		status.GetBoot().GetLastResetDecision() != "reset-complete" ||
		status.GetBoot().GetStatus() != "reported" {
		t.Fatalf("unexpected boot recovery status: %#v", status.GetBoot())
	}
	if status.GetRuntimeAsset().GetScriptPath() != "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs" ||
		status.GetRuntimeAsset().GetStatusCode() != 404 ||
		status.GetRuntimeAsset().GetClassification() != "missing" ||
		status.GetRuntimeAsset().GetStatus() != "reported" {
		t.Fatalf("unexpected runtime asset recovery status: %#v", status.GetRuntimeAsset())
	}
}

func TestRecoveryStatusRegistrySharesRendererFactsAcrossResources(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	registry := NewRecoveryStatusRegistry()
	ref := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "session-1",
			ProviderId:        "local",
			ProviderAccountId: "account-1",
		},
	}
	publisher := NewStatusResource(b, registry.getSessionRecoveryStatusCtrForRef(ref))
	reader := NewStatusResource(b, registry.getSessionRecoveryStatusCtrForRef(ref))

	_, err := publisher.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := reader.buildRecoveryStatus(nil, nil)
	if status.GetBoot().GetCompatibilityVersion() != "1000000" ||
		status.GetBoot().GetLastResetDecision() != "reset-complete" ||
		status.GetBoot().GetStatus() != "reported" {
		t.Fatalf("reader did not see shared renderer status: %#v", status.GetBoot())
	}
}

func TestReportRecoveryStatusReplacesRendererSnapshot(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	statusRes := NewStatusResource(b, nil)

	_, err := statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
		RuntimeAsset: &s4wave_status.RuntimeAssetRecoveryStatus{
			ScriptPath:     "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs",
			StatusCode:     404,
			Classification: "missing",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "current",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := statusRes.buildRecoveryStatus(nil, nil)
	if status.GetBoot().GetLastResetDecision() != "current" {
		t.Fatalf("boot decision = %q, want current", status.GetBoot().GetLastResetDecision())
	}
	if status.GetRuntimeAsset().GetStatus() != "not-reported" ||
		status.GetRuntimeAsset().GetScriptPath() != "" {
		t.Fatalf("runtime asset status was not cleared: %#v", status.GetRuntimeAsset())
	}
}

func TestBuildLauncherRecoveryStatusIncludesEntrypointFacts(t *testing.T) {
	status := buildLauncherRecoveryStatus(
		&spacewave_launcher.LauncherInfo{
			DistConfig: &spacewave_launcher.DistConfig{
				ProjectId:  "spacewave",
				Rev:        42,
				ChannelKey: "staging",
			},
			UpdateState: &spacewave_launcher.UpdateState{
				Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
				Version:    "0.2.0",
				StagedPath: "/var/folders/spacewave/Spacewave.app",
			},
			FetchStatus: &spacewave_launcher.FetchStatus{
				SelectedConfigRev:             42,
				SelectedConfigSource:          spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_ENDPOINT,
				FetchedConfigRev:              43,
				FetchedConfigSource:           "https://release.example",
				ReleaseMetadataOutcome:        spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_STAGED,
				ReleaseWorldHeadRef:           "release-world-head",
				SelectedEntrypointManifestId:  "spacewave-dist",
				SelectedEntrypointPlatformId:  "desktop/darwin/arm64",
				SelectedEntrypointManifestRev: 7,
				SelectedEntrypointManifestRef: "manifest-ref",
			},
		},
	)

	if status.GetSelectedChannelKey() != "staging" ||
		status.GetSelectedConfigRev() != 42 ||
		status.GetSelectedConfigSource() != "endpoint" ||
		status.GetReleaseMetadataOutcome() != "staged" ||
		status.GetReleaseWorldHeadRef() != "release-world-head" {
		t.Fatalf("unexpected release status: %#v", status)
	}
	if status.GetSelectedEntrypointManifestId() != "spacewave-dist" ||
		status.GetSelectedEntrypointPlatformId() != "desktop/darwin/arm64" ||
		status.GetSelectedEntrypointManifestRev() != 7 ||
		status.GetSelectedEntrypointManifestRef() != "manifest-ref" {
		t.Fatalf("unexpected selected entrypoint status: %#v", status)
	}
	if status.GetUpdatePhase() != "staged" ||
		status.GetUpdateVersion() != "0.2.0" ||
		status.GetStagedPath() != "/var/folders/spacewave/Spacewave.app" {
		t.Fatalf("unexpected update status: %#v", status)
	}
}

func TestRecoveryStatusKeepsEntrypointAndPluginFactsSeparate(t *testing.T) {
	status := &s4wave_status.RecoveryStatus{
		Launcher: buildLauncherRecoveryStatus(
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        42,
					ChannelKey: "stable",
				},
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
					StagedPath: "/tmp/Spacewave.app",
				},
				FetchStatus: &spacewave_launcher.FetchStatus{
					SelectedEntrypointManifestId:  "spacewave-dist",
					SelectedEntrypointPlatformId:  "desktop/darwin/arm64",
					SelectedEntrypointManifestRef: "entrypoint-ref",
				},
			},
		),
		Plugins: []*bldr_plugin.PluginManifestRecoveryStatus{{
			PluginId:            "spacewave-app",
			ExecuteManifestRef:  "plugin-exec-ref",
			DownloadManifestRef: "plugin-download-ref",
		}},
		NativePackages: []*s4wave_status.NativePackageRecoveryStatus{{
			PluginId:     "spacewave-app",
			DistDir:      "/tmp/p/d/spacewave-app",
			Materialized: true,
			LastAction:   "materialized",
		}},
	}

	if status.GetLauncher().GetSelectedEntrypointManifestRef() != "entrypoint-ref" {
		t.Fatalf("launcher entrypoint ref missing: %#v", status.GetLauncher())
	}
	if status.GetPlugins()[0].GetExecuteManifestRef() != "plugin-exec-ref" ||
		status.GetNativePackages()[0].GetDistDir() != "/tmp/p/d/spacewave-app" {
		t.Fatalf("plugin-owned status missing: %#v", status)
	}
	if status.GetLauncher().GetSelectedEntrypointManifestRef() == status.GetPlugins()[0].GetExecuteManifestRef() {
		t.Fatalf("entrypoint and plugin refs were conflated: %#v", status)
	}
}

func TestBuildNetworkStatsResponseGroupsAndSortsLinksByRemotePeer(t *testing.T) {
	localPeerID := peer.ID("local-peer")
	remotePeerA := peer.ID("remote-peer-a")
	remotePeerB := peer.ID("remote-peer-b")
	links := []transport_controller.LinkSnapshot{
		{
			LinkID:            7,
			TransportID:       700,
			RemoteTransportID: 1700,
			LocalPeerID:       localPeerID,
			RemotePeerID:      remotePeerB,
		},
		{
			LinkID:            11,
			TransportID:       1100,
			RemoteTransportID: 2100,
			LocalPeerID:       localPeerID,
			RemotePeerID:      remotePeerA,
		},
		{
			LinkID:            3,
			TransportID:       300,
			RemoteTransportID: 1300,
			LocalPeerID:       localPeerID,
			RemotePeerID:      remotePeerA,
		},
	}
	slices.SortFunc(links, compareNetworkLinkSnapshots)

	resp := buildNetworkStatsResponse(&s4wave_status.WatchNetworkStatsResponse{}, links)

	if resp.GetPeerCount() != 2 {
		t.Fatalf("peer count = %d, want 2", resp.GetPeerCount())
	}
	if resp.GetLinkCount() != 3 {
		t.Fatalf("link count = %d, want 3", resp.GetLinkCount())
	}
	peerAInfo := requireNetworkPeerInfo(t, resp.GetPeers(), 0, remotePeerA.String(), 2)
	requireNetworkLinkInfo(t, peerAInfo.GetLinks(), 0, 3, 300, 1300, localPeerID.String(), remotePeerA.String())
	requireNetworkLinkInfo(t, peerAInfo.GetLinks(), 1, 11, 1100, 2100, localPeerID.String(), remotePeerA.String())
	peerBInfo := requireNetworkPeerInfo(t, resp.GetPeers(), 1, remotePeerB.String(), 1)
	requireNetworkLinkInfo(t, peerBInfo.GetLinks(), 0, 7, 700, 1700, localPeerID.String(), remotePeerB.String())
}

func TestBuildNetworkStatsResponseHandlesEmptySnapshot(t *testing.T) {
	resp := buildNetworkStatsResponse(&s4wave_status.WatchNetworkStatsResponse{}, nil)

	if resp.GetPeerCount() != 0 {
		t.Fatalf("peer count = %d, want 0", resp.GetPeerCount())
	}
	if resp.GetLinkCount() != 0 {
		t.Fatalf("link count = %d, want 0", resp.GetLinkCount())
	}
	if len(resp.GetPeers()) != 0 {
		t.Fatalf("peers = %#v, want empty", resp.GetPeers())
	}
}

func requireNetworkPeerInfo(
	t *testing.T,
	peers []*s4wave_status.NetworkPeerInfo,
	index int,
	peerID string,
	linkCount uint32,
) *s4wave_status.NetworkPeerInfo {
	t.Helper()
	if len(peers) <= index {
		t.Fatalf("missing peer at index %d: %#v", index, peers)
	}
	peerInfo := peers[index]
	if peerInfo.GetPeerId() != peerID {
		t.Fatalf("peer[%d].peer_id = %q, want %q", index, peerInfo.GetPeerId(), peerID)
	}
	if peerInfo.GetLinkCount() != linkCount {
		t.Fatalf("peer[%d].link_count = %d, want %d", index, peerInfo.GetLinkCount(), linkCount)
	}
	return peerInfo
}

func requireNetworkLinkInfo(
	t *testing.T,
	links []*s4wave_status.NetworkLinkInfo,
	index int,
	linkID uint64,
	transportID uint64,
	remoteTransportID uint64,
	localPeerID string,
	remotePeerID string,
) {
	t.Helper()
	if len(links) <= index {
		t.Fatalf("missing link at index %d: %#v", index, links)
	}
	linkInfo := links[index]
	if linkInfo.GetLinkId() != linkID {
		t.Fatalf("link[%d].link_id = %d, want %d", index, linkInfo.GetLinkId(), linkID)
	}
	if linkInfo.GetTransportId() != transportID {
		t.Fatalf("link[%d].transport_id = %d, want %d", index, linkInfo.GetTransportId(), transportID)
	}
	if linkInfo.GetRemoteTransportId() != remoteTransportID {
		t.Fatalf("link[%d].remote_transport_id = %d, want %d", index, linkInfo.GetRemoteTransportId(), remoteTransportID)
	}
	if linkInfo.GetLocalPeerId() != localPeerID {
		t.Fatalf("link[%d].local_peer_id = %q, want %q", index, linkInfo.GetLocalPeerId(), localPeerID)
	}
	if linkInfo.GetRemotePeerId() != remotePeerID {
		t.Fatalf("link[%d].remote_peer_id = %q, want %q", index, linkInfo.GetRemotePeerId(), remotePeerID)
	}
}

// watchPluginsStream records WatchPlugins responses for a test.
type watchPluginsStream struct {
	srpc.Stream
	ctx  context.Context
	sent chan *s4wave_status.WatchPluginsResponse
}

func (s *watchPluginsStream) Context() context.Context {
	return s.ctx
}

func (s *watchPluginsStream) Send(resp *s4wave_status.WatchPluginsResponse) error {
	s.sent <- resp
	return nil
}

func (s *watchPluginsStream) SendAndClose(resp *s4wave_status.WatchPluginsResponse) error {
	return s.Send(resp)
}

func TestWatchPluginsWithoutSchedulerSendsEmptySnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := inmem.NewBus(cdc.NewController(ctx, logrus.NewEntry(logrus.New())))
	statusRes := NewStatusResource(b, nil)
	strm := &watchPluginsStream{
		ctx:  ctx,
		sent: make(chan *s4wave_status.WatchPluginsResponse, 1),
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- statusRes.WatchPlugins(&s4wave_status.WatchPluginsRequest{}, strm)
	}()

	select {
	case resp := <-strm.sent:
		if resp.GetPluginCount() != 0 || len(resp.GetPlugins()) != 0 {
			t.Fatalf("unexpected plugins without a scheduler: %#v", resp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WatchPlugins sent nothing without a scheduler")
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("WatchPlugins returned %v, want context.Canceled", err)
	}
}

// testPluginScheduler is a PluginScheduler that resolves LookupPluginScheduler
// with itself when added to a bus as a directive handler.
type testPluginScheduler struct {
	instanceKey string
	statusCtr   *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
}

func newTestPluginScheduler(instanceKey string, plugins ...*bldr_plugin.PluginStatus) *testPluginScheduler {
	return &testPluginScheduler{
		instanceKey: instanceKey,
		statusCtr: ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{
			Plugins: plugins,
		}),
	}
}

func (s *testPluginScheduler) GetInstanceKey() string {
	return s.instanceKey
}

func (s *testPluginScheduler) GetPluginStatusCtr() ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] {
	return s.statusCtr
}

func (s *testPluginScheduler) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(bldr_plugin.LookupPluginScheduler); ok {
		return directive.R(directive.NewValueResolver([]bldr_plugin.LookupPluginSchedulerValue{s}), nil)
	}
	return nil, nil
}

func TestWatchPluginsMergesSpaceRuntimeSchedulers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(cdc.NewController(ctx, le))
	// The root host also runs Space plugin instances keyed by Space engine ID.
	const rootSpaceID = "space/local/account/s0"
	root := newTestPluginScheduler("", &bldr_plugin.PluginStatus{
		PluginId: "spacewave-core",
		State:    bldr_plugin.PluginState_PluginState_RUNNING,
	}, &bldr_plugin.PluginStatus{
		PluginId:    "notes",
		InstanceKey: rootSpaceID,
		State:       bldr_plugin.PluginState_PluginState_RUNNING,
	})
	if _, err := b.AddHandler(root); err != nil {
		t.Fatal(err)
	}

	statusRes := NewStatusResource(b, nil)
	strm := &watchPluginsStream{
		ctx:  ctx,
		sent: make(chan *s4wave_status.WatchPluginsResponse, 1),
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- statusRes.WatchPlugins(&s4wave_status.WatchPluginsRequest{}, strm)
	}()
	requirePlugins(t, strm, "/spacewave-core:running", rootSpaceID+"/notes:running")

	// A Space runtime hosts its scheduler on a child bus reached through a
	// bridge on the session bus.
	const spaceID = "space/local/account/s1"
	child := inmem.NewBus(cdc.NewController(ctx, le))
	spaceScheduler := newTestPluginScheduler(spaceID, &bldr_plugin.PluginStatus{
		PluginId: "notes",
		State:    bldr_plugin.PluginState_PluginState_REQUESTED,
	})
	if _, err := child.AddHandler(spaceScheduler); err != nil {
		t.Fatal(err)
	}
	bridgeRelease, err := b.AddController(ctx, bus_bridge.NewBusBridge(child, func(inst directive.Instance) (bool, error) {
		_, ok := inst.GetDirective().(bldr_plugin.LookupPluginScheduler)
		return ok, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	requirePlugins(t, strm, "/spacewave-core:running", rootSpaceID+"/notes:running", spaceID+"/notes:requested")

	spaceScheduler.statusCtr.SetValue(&bldr_plugin.PluginStatusSnapshot{
		Plugins: []*bldr_plugin.PluginStatus{{
			PluginId: "notes",
			State:    bldr_plugin.PluginState_PluginState_RUNNING,
		}},
	})
	requirePlugins(t, strm, "/spacewave-core:running", rootSpaceID+"/notes:running", spaceID+"/notes:running")

	bridgeRelease()
	requirePlugins(t, strm, "/spacewave-core:running", rootSpaceID+"/notes:running")

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("WatchPlugins returned %v, want context.Canceled", err)
	}
}

// requirePlugins waits for the next WatchPlugins response and checks it lists
// want as "<space_id>/<id>:<state>" in order.
func requirePlugins(t *testing.T, strm *watchPluginsStream, want ...string) {
	t.Helper()
	var resp *s4wave_status.WatchPluginsResponse
	select {
	case resp = <-strm.sent:
	case <-time.After(5 * time.Second):
		t.Fatalf("WatchPlugins sent nothing, want %v", want)
	}
	got := make([]string, 0, len(resp.GetPlugins()))
	for _, plugin := range resp.GetPlugins() {
		got = append(got, plugin.GetSpaceId()+"/"+plugin.GetId()+":"+plugin.GetState())
	}
	if !slices.Equal(got, want) || resp.GetPluginCount() != uint32(len(want)) {
		t.Fatalf("plugins = %v (count %d), want %v", got, resp.GetPluginCount(), want)
	}
}
