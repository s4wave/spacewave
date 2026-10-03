package bifrost_rpc_access

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// newAccessTestBus serves an Echoer through the access service and returns a
// client bus whose access client controller reaches it through svcFn.
func newAccessTestBus(
	ctx context.Context,
	t *testing.T,
	svcFn func(SRPCAccessRpcServiceClient) AccessClientFunc,
) (*logrus.Entry, bus.Bus) {
	// Log both buses at debug level.
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Serve the access service on the server bus.
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

	// Expose the Echoer service on the server bus.
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
	t.Cleanup(invokerRel)

	// Reach the server bus from the client bus through the access client.
	clientBus, _, err := core.NewCoreBus(ctx, le.WithField("test-bus", "client"))
	if err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(server))
	clientCtrl := NewClientController(
		le,
		controller.NewInfo("bifrost/rpc/access/client", controller.MustParseVersion("0.0.1"), ""),
		svcFn(NewSRPCAccessRpcServiceClient(client)),
		nil,
		nil,
		false,
		nil,
	)
	clientRel, err := clientBus.AddController(ctx, clientCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(clientRel)
	return le, clientBus
}

// TestAccessRpcService checks an Echo call through the access service.
func TestAccessRpcService(t *testing.T) {
	// Build the buses with a static access client.
	ctx := context.Background()
	le, clientBus := newAccessTestBus(ctx, t, NewAccessClientFunc)

	// Call the remote Echoer through the client bus invoker.
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
}

// TestAccessRpcServiceClientReplaced checks that a held lookup publishes the
// service again after its access client is released and rebuilt.
func TestAccessRpcServiceClientReplaced(t *testing.T) {
	// Bound the test so a lost lookup fails instead of hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Record the released callback of each access client build.
	var mtx sync.Mutex
	var releaseClient func()
	_, clientBus := newAccessTestBus(ctx, t, func(svc SRPCAccessRpcServiceClient) AccessClientFunc {
		return func(ctx context.Context, released func()) (SRPCAccessRpcServiceClient, func(), error) {
			mtx.Lock()
			releaseClient = released
			mtx.Unlock()
			return svc, nil, nil
		}
	})

	// Hold the lookup and signal each published service invoker.
	added := make(chan struct{}, 8)
	_, dirRef, err := clientBus.AddDirective(
		bifrost_rpc.NewLookupRpcService(echo.SRPCEchoerServiceID, ""),
		directive.NewCallbackHandler(func(directive.AttachedValue) {
			added <- struct{}{}
		}, nil, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dirRef.Release()

	// waitAdded waits for the next published service invoker.
	waitAdded := func(stage string) {
		select {
		case <-added:
		case <-ctx.Done():
			t.Fatalf("%s: service invoker was not published", stage)
		}
	}
	waitAdded("first client")

	// Release the access client and expect the lookup through its replacement.
	mtx.Lock()
	rel := releaseClient
	mtx.Unlock()
	rel()
	waitAdded("replacement client")
}
