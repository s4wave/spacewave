package plugin_host

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestPluginHostServerResolvesCallerInstance(t *testing.T) {
	s := &PluginHostServer{instanceKey: "space-a"}
	if got := s.resolveInstanceKey(""); got != "space-a" {
		t.Fatalf("unqualified instance = %q, want space-a", got)
	}
	if got := s.resolveInstanceKey("vm-1"); got != "vm-1" {
		t.Fatalf("qualified instance = %q, want vm-1", got)
	}
}

// TestPluginRpcMissingManifestFails keeps an exact RPC from becoming an ordinary
// instance demand that waits forever for the newest version of the plugin.
func TestPluginRpcMissingManifestFails(t *testing.T) {
	// Bound the test with a three second timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start a bus and serve the plugin host over an in-memory pipe.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	root, err := hash.Sum(hash.RecommendedHashType, []byte("missing executable"))
	if err != nil {
		t.Fatal(err)
	}
	mux := srpc.NewMux()
	if err := plugin.SRPCRegisterPluginHost(mux, &PluginHostServer{b: b, le: le, pluginID: "caller"}); err != nil {
		t.Fatal(err)
	}

	// The exact-manifest RPC must settle with an unavailable error.
	host := plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	client := rpcstream.NewRpcStreamClient(host.PluginRpc,
		plugin.BuildPluginRpcComponentID("colors", "space-a", root.MarshalString()), true)
	_, err = echo.NewSRPCEchoerClient(client).Echo(ctx, &echo.EchoMsg{Body: "exact"})
	if err == nil || !strings.Contains(err.Error(), "unavailable") || ctx.Err() != nil {
		t.Fatalf("missing executable did not settle: %v (caller: %v)", err, ctx.Err())
	}
}

// TestLoadPluginRefusesBorrowedPeer keeps a plugin from naming another peer
// whose approvals would admit the roots it asks to load: only a plugin the
// scheduler trusts to run plugins on its hosts may name one.
func TestLoadPluginRefusesBorrowedPeer(t *testing.T) {
	// Bound the test and start a bus with no LoadPlugin resolver, so an
	// accepted load answers not running once its directive goes idle.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	root, err := hash.Sum(hash.RecommendedHashType, []byte("approved root"))
	if err != nil {
		t.Fatal(err)
	}
	ref := &bldr_manifest.ManifestRef{ManifestRef: &bucket.ObjectRef{RootRef: &block.BlockRef{Hash: root}}}

	// load sends one LoadPlugin request from caller and returns the first
	// response or the refusal.
	load := func(caller string, namesPeers bool, peerID string) error {
		srv := &PluginHostServer{b: b, le: le, pluginID: caller}
		srv.SetNamesPeers(namesPeers)
		mux := srpc.NewMux()
		if err := plugin.SRPCRegisterPluginHost(mux, srv); err != nil {
			t.Fatal(err)
		}
		host := plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
		strm, err := host.LoadPlugin(ctx, &plugin.LoadPluginRequest{
			PluginId:  "colors",
			Manifests: []*bldr_manifest.ManifestRef{ref},
			PeerId:    peerID,
		})
		if err != nil {
			return err
		}
		defer strm.Close()
		_, err = strm.Recv()
		return err
	}

	// A plugin outside the trusted set that names a peer is refused.
	err = load("space-plugin", false, "session-peer")
	if err == nil || !strings.Contains(err.Error(), "may not name the approving peer") {
		t.Fatalf("untrusted caller naming a peer: %v, want refused", err)
	}

	// The same plugin naming no peer loads, as does a trusted plugin that
	// forwards a session's load naming the session's peer.
	if err := load("space-plugin", false, ""); err != nil {
		t.Fatalf("untrusted caller naming no peer: %v", err)
	}
	if err := load("spacewave-core", true, "session-peer"); err != nil {
		t.Fatalf("trusted caller naming a peer: %v", err)
	}
}
