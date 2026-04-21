package bifrost_rpc_access

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestAccessRpcService(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// construct the server bus
	serverBus, _, err := core.NewCoreBus(ctx, le.WithField("test-bus", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// construct the AccessRpcService server on serverBus
	serverMux := srpc.NewMux()
	accessServer := NewAccessRpcServiceServer(serverBus, true, nil)
	if err := SRPCRegisterAccessRpcService(serverMux, accessServer); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(serverMux)

	// construct the destination service mux + attach to serverBus
	targetMux := srpc.NewMux()
	targetService := echo.NewEchoServer(nil)
	if err := echo.SRPCRegisterEchoer(targetMux, targetService); err != nil {
		t.Fatal(err.Error())
	}
	invokerCtrl := bifrost_rpc.NewInvokerController(
		le,
		serverBus,
		controller.NewInfo("bifrost/rpc/access/invoker", semver.MustParse("0.0.1"), ""),
		targetMux,
		nil,
	)
	invokerRel, err := serverBus.AddController(ctx, invokerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer invokerRel()

	// construct client bus + access client controller
	// this will forward LookupRpcService<echo...> to serverMux
	clientBus, _, err := core.NewCoreBus(ctx, le.WithField("test-bus", "client"))
	if err != nil {
		t.Fatal(err)
	}
	openClientStream := srpc.NewServerPipe(server)
	client := srpc.NewClient(openClientStream)
	clientCtrl := NewClientController(
		le,
		controller.NewInfo("bifrost/rpc/access/client", semver.MustParse("0.0.1"), ""),
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

	// construct the srpc server running on clientBus
	clientServer := srpc.NewServer(bifrost_rpc.NewInvoker(clientBus, "test-server", true))

	// access the echo service via clientServer
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
