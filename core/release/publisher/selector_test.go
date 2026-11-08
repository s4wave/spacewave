package publisher_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	launcher_controller "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// selectorDeployment is the Deployment the selector tests sign for.
var selectorDeployment = publisher.Deployment{ProjectID: "demo", ChannelKey: "stable"}

// signSelector signs the selector at rev with key.
func signSelector(t *testing.T, key crypto.PrivKey, rev uint64) string {
	t.Helper()
	conf, err := selectorDeployment.DistConfig(rev, nil)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := publisher.SignDistConfig(key, conf)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// newSigner returns a DistConfig signer's peer ID and key.
func newSigner(t *testing.T) (peer.ID, crypto.PrivKey) {
	t.Helper()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := p.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return p.GetPeerID(), key
}

// startLauncher runs the launcher controller pinning signer on a fresh
// client bus, and returns the bus and a stream of its launcher info.
func startLauncher(t *testing.T, ctx context.Context, signer peer.ID, initDistConfig string, endpoints []*launcher_controller.HttpEndpoint) (bus.Bus, func() *spacewave_launcher.LauncherInfo) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	conf := &launcher_controller.Config{
		ProjectId:            selectorDeployment.ProjectID,
		ChannelKey:           selectorDeployment.ChannelKey,
		VolumeId:             tb.Volume.GetID(),
		InitDistConfig:       initDistConfig,
		DisableEndpointFetch: len(endpoints) == 0,
	}
	ctrl := launcher_controller.NewController(le, tb.Bus, conf, []peer.ID{signer}, endpoints)
	rel, err := tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	// A plugin reaches the launcher plugin through the plugin host; route
	// those calls to the launcher on this bus as the host does.
	invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, tb.Bus, spacewave_launcher.SRPCLauncherServiceID, "", true, nil)
	if err != nil || len(invokers) == 0 {
		t.Fatalf("launcher service: %v", err)
	}
	t.Cleanup(ref.Release)
	host := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/plugin-host-launcher", controller.MustParseVersion("0.0.1"), "routes plugin launcher calls"),
		bifrost_rpc.NewRpcServiceBuilder(hostInvoker{invokers[0]}),
		nil, false, nil,
		[]string{spacewave_launcher.PluginLauncherServiceID},
		nil,
	)
	relHost, err := tb.Bus.AddController(ctx, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relHost)
	return tb.Bus, watchLauncher(t, ctx, tb.Bus)
}

// hostInvoker forwards calls to the launcher plugin as the plugin host does,
// without advertising the services it reaches.
type hostInvoker struct{ srpc.Invoker }

// watchLauncher streams the info of the launcher on b through its own service.
func watchLauncher(t *testing.T, ctx context.Context, b bus.Bus) func() *spacewave_launcher.LauncherInfo {
	t.Helper()
	invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b, spacewave_launcher.SRPCLauncherServiceID, "", true, nil)
	if err != nil || len(invokers) == 0 {
		t.Fatalf("launcher service: %v", err)
	}
	t.Cleanup(ref.Release)
	client := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	strm, err := client.WatchLauncherInfo(ctx, &spacewave_launcher.WatchLauncherInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = strm.Close() })
	return func() *spacewave_launcher.LauncherInfo {
		t.Helper()
		info, err := strm.Recv()
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
}

// waitRev reads launcher info until the selected revision is rev.
func waitRev(t *testing.T, next func() *spacewave_launcher.LauncherInfo, rev uint64) *spacewave_launcher.LauncherInfo {
	t.Helper()
	for {
		info := next()
		if info.GetDistConfig().GetRev() == rev {
			return info
		}
		if info.GetDistConfig().GetRev() > rev {
			t.Fatalf("launcher selected rev %d past %d", info.GetDistConfig().GetRev(), rev)
		}
	}
}

// awaitSent waits for Forward to send the next selector.
func awaitSent(t *testing.T, ctx context.Context, sent <-chan error) {
	t.Helper()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("no selector forwarded")
	}
}

// TestSelectorPushesToConnectedLauncher forwards a new selector to a client
// whose launcher never polls, and checks it adopts the new revision at once
// while an older or badly signed selector changes nothing.
func TestSelectorPushesToConnectedLauncher(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	signer, key := newSigner(t)
	_, otherKey := newSigner(t)

	// The client's launcher starts on rev 1 with endpoint fetch disabled.
	clientBus, next := startLauncher(t, ctx, signer, signSelector(t, key, 1), nil)
	waitRev(t, next, 1)

	// The server holds rev 2 and forwards it over the client connection.
	var sel publisher.Selector
	if !sel.Set(2, signSelector(t, key, 2)) {
		t.Fatal("rev 2 not set")
	}
	sent := make(chan error, 8)
	push := publisher.PushToLauncher(clientBus)
	fwdCtx, fwdCancel := context.WithCancel(ctx)
	defer fwdCancel()
	fwdDone := make(chan error, 1)
	go func() {
		fwdDone <- sel.Forward(fwdCtx, func(ctx context.Context, signed string) error {
			err := push(ctx, signed)
			sent <- err
			return err
		})
	}()
	awaitSent(t, ctx, sent)
	info := waitRev(t, next, 2)
	if src := info.GetFetchStatus().GetSelectedConfigSource(); src != spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_PUSH {
		t.Fatalf("selected source %v, want push", src)
	}

	// A later release reaches the connected client without a reconnect.
	if !sel.Set(3, signSelector(t, key, 3)) {
		t.Fatal("rev 3 not set")
	}
	awaitSent(t, ctx, sent)
	waitRev(t, next, 3)

	// The server refuses an older selector, and the launcher refuses an older
	// or badly signed one pushed to it directly.
	if sel.Set(2, signSelector(t, key, 2)) {
		t.Fatal("selector regressed to rev 2")
	}
	if err := push(ctx, signSelector(t, key, 2)); err != nil {
		t.Fatalf("older selector: %v", err)
	}
	if err := push(ctx, signSelector(t, otherKey, 4)); err == nil {
		t.Fatal("selector signed by an unpinned peer was accepted")
	}
	fwdCancel()
	<-fwdDone
	if rev, _ := sel.Get(); rev != 3 {
		t.Fatalf("server selector rev %d", rev)
	}
	if got := watchLauncher(t, ctx, clientBus)().GetDistConfig().GetRev(); got != 3 {
		t.Fatalf("launcher rev %d after refused selectors, want 3", got)
	}
}

// TestSelectorServesLauncherEndpoint serves the selector with a one-minute
// cache and checks a launcher fetching the endpoint adopts it.
func TestSelectorServesLauncherEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	signer, key := newSigner(t)

	// Before the first release the endpoint has no selector.
	var sel publisher.Selector
	srv := httptest.NewServer(&sel)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("empty selector status %d", resp.StatusCode)
	}

	// The released selector is cached for a minute.
	sel.Set(5, signSelector(t, key, 5))
	resp, err = http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("selector response %d %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}

	// A launcher polling the endpoint verifies and adopts it.
	_, next := startLauncher(t, ctx, signer, "", []*launcher_controller.HttpEndpoint{{Url: srv.URL}})
	waitRev(t, next, 5)
}
