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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

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

	clients, _, ref, err := ExLookupRpcClient(ctx, b, "plugin-host/bldr.plugin.PluginHost", "test-client", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(clients) != 1 {
		t.Fatalf("expected 1 matching client, got %d", len(clients))
	}
}
