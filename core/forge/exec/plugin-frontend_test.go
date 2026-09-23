//go:build !js && !tinygo

package space_exec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	web_fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	net_testbed "github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// TestPluginFrontendSource runs a real Forge compiler across authenticated peers.
// An accepted World source edit must reach Vite without rebuilding an artifact.
func TestPluginFrontendSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	peers := pluginFrontendPeers(t, ctx)
	author, device, stranger := peers[0], peers[1], peers[2]
	tb, _ := setupIntegrationTest(t, NewDefaultRegistryWithBus(device.Bus))
	nativePeer, err := peer.NewPeer(device.PrivKey)
	if err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, nativePeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePeer()
	sender := device.PeerID
	const sourceKey = "projects/live-colors"
	const config = `{"id":"colors","manifests":{"colors":{"builder":{"id":"bldr/plugin/compiler/js","config":{"modules":[{"kind":"JS_MODULE_KIND_FRONTEND","path":"./Viewer.ts"}]}}}}}`
	createTestFS(t, ctx, tb.WorldState, sender, sourceKey, "bldr.yaml", []byte(config))
	object, err := world.MustGetObject(ctx, tb.WorldState, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(object)
	stamp := time.Unix(1000, 0)
	for name, data := range map[string]string{
		"Viewer.ts":  `import "./viewer.css"; export const label = "colors"`,
		"viewer.css": `p { color: red; }`,
	} {
		_, _, err := unixfs_world.FsMknodWithContent(ctx, object, sender, unixfs_world.FSType_FSType_FS_NODE,
			[]string{name}, unixfs.NewFSCursorNodeType_File(), int64(len(data)), strings.NewReader(data), 0o644, stamp)
		if err != nil {
			t.Fatal(err)
		}
	}
	source, err := forge_value.NewWorldObjectSnapshot(ctx, object, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}

	// Keep the normal execution controller responsible for custody and shutdown.
	id := uuid.NewV4().String()
	buildConfig := &PluginBuildConfig{
		ManifestId: "colors", FrontendId: id, FrontendPeerId: author.PeerID.String(),
		FrontendRoutePrefix: frontend.ServiceRoutePrefix("test/" + frontend.SRPCFrontendServiceID),
	}
	data, err := buildConfig.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	const executionKey = "plugin-builds/live"
	createTestExecutionWithValueSet(t, ctx, tb.WorldState, sender, executionKey, BuildPluginConfigID, data,
		&forge_target.ValueSet{Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot("source", source)}})
	_, executionRef, err := execution_controller.StartControllerWithConfig(ctx, tb.Bus,
		execution_controller.NewConfig(tb.EngineID, executionKey, sender, &forge_target.InputWorld{EngineId: tb.EngineID}))
	if err != nil {
		t.Fatal(err)
	}
	defer executionRef.Release()
	client := frontend.NewSRPCFrontendClient(srpc.NewClient(stream_srpc.NewOpenStreamFunc(author.Bus, PluginFrontendProtocol(id), author.PeerID, device.PeerID, 0)))
	watchCtx, closeWatch := context.WithCancel(ctx)
	defer closeWatch()
	watch, err := client.Watch(watchCtx, &frontend.WatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	snapshot, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	prefix := snapshot.GetSession().GetRoutePrefix()
	if !strings.HasPrefix(prefix, buildConfig.GetFrontendRoutePrefix()) {
		t.Fatalf("compiler lost its attachment route: %q", prefix)
	}
	fetchModule := func(filename string) string {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "http://space.test"+prefix+filename, nil).WithContext(ctx)
		if err := web_fetch.Fetch(ctx, func(ctx context.Context) (web_fetch.SRPCFetchService_FetchClient, error) { return client.Fetch(ctx) }, request, response); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("module fetch returned %d: %s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	if !strings.Contains(fetchModule("Viewer.ts"), "colors") {
		t.Fatal("initial source module was not served")
	}
	if !strings.Contains(fetchModule("viewer.css"), "color: red") {
		t.Fatal("initial stylesheet was not served")
	}

	// Another authenticated Session cannot borrow the author's compiler grant.
	foreign := frontend.NewSRPCFrontendClient(srpc.NewClient(stream_srpc.NewOpenStreamFunc(stranger.Bus,
		PluginFrontendProtocol(id), stranger.PeerID, device.PeerID, 0)))
	_, err = foreign.Send(ctx, &frontend.SendRequest{SessionId: snapshot.GetSession().GetId(), Payload: `{}`})
	if err == nil || !strings.Contains(err.Error(), "another Session") {
		t.Fatalf("unrelated Session accessed the compiler: %v", err)
	}

	// Equal-size content with the original timestamp still produces an update.
	_, _, err = unixfs_world.FsWriteAt(ctx, object, sender, unixfs_world.FSType_FSType_FS_NODE,
		[]string{"viewer.css"}, 0, []byte("p { color: tan; }"), stamp)
	if err != nil {
		t.Fatal(err)
	}
	event, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if event.GetSequence() <= snapshot.GetSequence() || !strings.Contains(event.GetPayload(), "viewer.css") {
		t.Fatalf("accepted source edit did not reach the compiler: %v", event)
	}
	if !strings.Contains(fetchModule("viewer.css"), "color: tan") {
		t.Fatal("compiler served stale source after its update")
	}

	// Closing the authoring stream completes and joins its retained execution.
	// Watch has already closed its send half; Close still releases its transport.
	if err := watch.Close(); err != nil && !errors.Is(err, srpc.ErrCompleted) {
		t.Fatal(err)
	}
	closeWatch()
	completed, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.GetResult().GetSuccess() {
		t.Fatalf("authoring did not close cleanly: %v", completed.GetResult())
	}
}

// pluginFrontendPeers connects the real encrypted transport between two Sessions.
func pluginFrontendPeers(t *testing.T, ctx context.Context) []*net_testbed.Testbed {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	var peers []*net_testbed.Testbed
	var transports []*inproc.Inproc
	for range 3 {
		peer, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(peer.Release)
		peers = append(peers, peer)
	}
	for _, peer := range peers {
		dialers := make(map[string]*dialer.DialerOpts)
		for _, other := range peers {
			if peer != other {
				dialers[other.PeerID.String()] = &dialer.DialerOpts{Address: inproc.NewAddr(other.PeerID).String()}
			}
		}
		ctrl := inproc.BuildInprocController(le, peer.Bus, peer.PeerID, &inproc.Config{
			TransportPeerId: peer.PeerID.String(),
			Dialers:         dialers,
		})
		release, err := peer.Bus.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		transport, err := ctrl.GetTransport(ctx)
		if err != nil {
			t.Fatal(err)
		}
		transports = append(transports, transport.(*inproc.Inproc))
	}
	for _, transport := range transports {
		for _, other := range transports {
			if transport != other {
				transport.ConnectToInproc(ctx, other)
			}
		}
	}
	return peers
}
