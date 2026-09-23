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
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestPluginHostServerResolvesCallerInstance(t *testing.T) {
	s := &PluginHostServer{instanceKey: "space-a"}
	got, err := s.resolveInstanceKey("")
	if err != nil || got != "space-a" {
		t.Fatalf("unqualified instance = %q, %v", got, err)
	}
	if _, err := s.resolveInstanceKey("space-b"); err == nil {
		t.Fatal("foreign instance was accepted")
	}
}

// TestPluginRpcMissingManifestFails keeps an exact RPC from becoming an ordinary
// instance demand that waits forever for the newest version of the plugin.
func TestPluginRpcMissingManifestFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
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
	host := plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	client := rpcstream.NewRpcStreamClient(host.PluginRpc,
		plugin.BuildPluginRpcComponentID("colors", "space-a", root.MarshalString()), true)
	_, err = echo.NewSRPCEchoerClient(client).Echo(ctx, &echo.EchoMsg{Body: "exact"})
	if err == nil || !strings.Contains(err.Error(), "unavailable") || ctx.Err() != nil {
		t.Fatalf("missing executable did not settle: %v (caller: %v)", err, ctx.Err())
	}
}
