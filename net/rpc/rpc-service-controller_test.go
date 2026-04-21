package bifrost_rpc

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

const mockServiceID = "/foo/" + echo.SRPCEchoerServiceID

func startMockHandler(t *testing.T, tb *testbed.Testbed) func() {
	// create a rpc handler
	mux := srpc.NewMux()
	_ = mux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID))

	// attach it to the bus
	handlerCtrl := NewRpcServiceController(
		controller.NewInfo("bifrost/rpc/test-handler", semver.MustParse("0.0.1"), "test handler"),
		NewRpcServiceBuilder(mux),
		[]string{"/foo/"},
		true,
		nil,
		nil,
		nil,
	)
	relHandler, err := tb.Bus.AddController(tb.Context, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return relHandler
}

func TestRpcServiceController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{
		NoEcho: true,
		NoPeer: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer startMockHandler(t, tb)()

	invoker := NewInvoker(tb.Bus, "", false)
	server := srpc.NewServer(invoker)
	serverOpenConn := srpc.NewServerPipe(server)
	client := srpc.NewClient(serverOpenConn)
	echoClient := echo.NewSRPCEchoerClientWithServiceID(client, mockServiceID)
	resp, err := echoClient.Echo(ctx, &echo.EchoMsg{
		Body: "testing",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetBody() != "testing" {
		t.FailNow()
	}
}
