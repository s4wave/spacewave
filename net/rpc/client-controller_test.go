package bifrost_rpc

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

func TestPrefixClientValueSatisfiesSRPCClient(t *testing.T) {
	ctrl := NewClientController(
		logrus.NewEntry(logrus.New()),
		nil,
		controller.NewInfo("test/rpc-client", controller.MustParseVersion("0.0.1"), ""),
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux()))),
		[]string{"plugin-host/"},
	)
	var val directive.Value = ctrl.GetClient()
	if _, ok := val.(srpc.Client); !ok {
		t.Fatal("expected prefixed client value to satisfy srpc.Client")
	}
}

func TestClientControllerResolvesMatchingServicePrefix(t *testing.T) {
	// Bound the test lifetime while resolving the matching service prefix.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create a bus for publishing the prefixed RPC client.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Attach a client controller advertising the plugin service prefix.
	ctrl := NewClientController(
		le,
		b,
		controller.NewInfo("test/rpc-client", controller.MustParseVersion("0.0.1"), ""),
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux()))),
		[]string{"plugin-host/"},
	)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Resolve the advertised RPC client through the real bus.
	clients, _, ref, err := ExLookupRpcClient(ctx, b, "plugin-host/bldr.plugin.PluginHost", "test-client", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// Verify the service prefix resolves exactly one client.
	if len(clients) != 1 {
		t.Fatalf("expected 1 matching client, got %d", len(clients))
	}
}
