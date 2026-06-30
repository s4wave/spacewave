package bifrost_rpc_access

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestAccessRpcService(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	serverBus, _, err := core.NewCoreBus(ctx, le.WithField("test-bus", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}

	serverMux := srpc.NewMux()
	accessServer := NewAccessRpcServiceServer(serverBus, true, nil)
	if err := SRPCRegisterAccessRpcService(serverMux, accessServer); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(serverMux)

	targetMux := srpc.NewMux()
	targetService := echo.NewEchoServer(nil)
	if err := echo.SRPCRegisterEchoer(targetMux, targetService); err != nil {
		t.Fatal(err.Error())
	}
	invokerCtrl := bifrost_rpc.NewInvokerController(
		le,
		serverBus,
		controller.NewInfo("bifrost/rpc/access/invoker", controller.MustParseVersion("0.0.1"), ""),
		targetMux,
		nil,
	)
	invokerRel, err := serverBus.AddController(ctx, invokerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer invokerRel()

	clientBus, _, err := core.NewCoreBus(ctx, le.WithField("test-bus", "client"))
	if err != nil {
		t.Fatal(err)
	}
	openClientStream := srpc.NewServerPipe(server)
	client := srpc.NewClient(openClientStream)
	clientCtrl := NewClientController(
		le,
		controller.NewInfo("bifrost/rpc/access/client", controller.MustParseVersion("0.0.1"), ""),
		NewAccessClientFunc(NewSRPCAccessRpcServiceClient(client)),
		nil,
		nil,
		false,
		nil,
	)
	clientRel, err := clientBus.AddController(ctx, clientCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer clientRel()

	clientServer := srpc.NewServer(bifrost_rpc.NewInvoker(clientBus, "test-server", true))

	clientClient := srpc.NewClient(srpc.NewServerPipe(clientServer))
	echoClient := echo.NewSRPCEchoerClient(clientClient)
	resp, err := echoClient.Echo(ctx, &echo.EchoMsg{Body: "hello world"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(resp.GetBody()) == 0 {
		t.Fatalf("expected response body but got %v", resp)
	}
	le.Infof("successfully round-tripped Echo: %s", resp.GetBody())
	<-time.After(time.Millisecond * 50)
}
