package bifrost_rpc

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

const mockServiceID = "/foo/" + echo.SRPCEchoerServiceID

func startMockHandler(t *testing.T, tb *testbed.Testbed) func() {
	// Register an echo service in the mock RPC multiplexer.
	mux := srpc.NewMux()
	_ = mux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID))

	// Attach the mock service controller to the test bus.
	handlerCtrl := NewRpcServiceController(
		controller.NewInfo("bifrost/rpc/test-handler", controller.MustParseVersion("0.0.1"), "test handler"),
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
	// Start a bus testbed with the mock echo service.
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

	// Call the mock echo service through a bus-backed RPC pipe.
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

	// Verify the echo response preserves the request body.
	if resp.GetBody() != "testing" {
		t.FailNow()
	}
}

// TestInvokerControllerServiceNamespace checks advertising and calls use the
// same service namespace, including an exact prefix that strips the whole ID.
func TestInvokerControllerServiceNamespace(t *testing.T) {
	// Cover known, missing, prefixed, exact-prefix and opaque service routes.
	for _, tc := range []struct {
		name       string
		prefixes   []string
		service    string
		opaque     bool
		advertised bool
	}{
		{name: "known", service: echo.SRPCEchoerServiceID, advertised: true},
		{name: "missing", service: "missing.Service"},
		{name: "prefixed", prefixes: []string{"host/"}, service: "host/" + echo.SRPCEchoerServiceID, advertised: true},
		{name: "prefixed-missing", prefixes: []string{"host/"}, service: "host/missing.Service"},
		{name: "wrong-prefix", prefixes: []string{"host/"}, service: echo.SRPCEchoerServiceID},
		{name: "exact-prefix", prefixes: []string{echo.SRPCEchoerServiceID}, service: echo.SRPCEchoerServiceID, advertised: true},
		{name: "opaque", service: "forwarded.Service", opaque: true, advertised: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Start an isolated bus and a registered Echo service.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			le := logrus.NewEntry(logrus.New())
			tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{NoEcho: true, NoPeer: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			mux := srpc.NewMux()
			if err := echo.SRPCRegisterEchoer(mux, echo.NewEchoServer(nil)); err != nil {
				t.Fatal(err)
			}

			// Opaque invokers retain their existing configured-route contract.
			var invoker srpc.Invoker = mux
			if tc.opaque {
				invoker = srpc.InvokerFunc(func(string, string, srpc.Stream) (bool, error) {
					return false, nil
				})
			}
			ctrl := NewInvokerController(le, tb.Bus,
				controller.NewInfo("test/service-advertising", controller.MustParseVersion("0.0.1"), "test service"),
				invoker, tc.prefixes)
			release, err := tb.Bus.AddController(ctx, ctrl, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// A controller may advertise only its implemented service namespace.
			invokers, _, ref, err := ExLookupRpcService(ctx, tb.Bus, tc.service, "", false, nil)
			if ref != nil {
				t.Cleanup(ref.Release)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := len(invokers) != 0; got != tc.advertised {
				t.Fatalf("advertised = %t, want %t", got, tc.advertised)
			}
			if !tc.advertised || tc.opaque {
				return
			}

			// Invoke every advertised Echo route through its original service ID.
			client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0])))
			response, err := echo.NewSRPCEchoerClientWithServiceID(client, tc.service).Echo(ctx, &echo.EchoMsg{Body: "route"})
			if err != nil {
				t.Fatal(err)
			}
			if response.GetBody() != "route" {
				t.Fatalf("echo = %q, want route", response.GetBody())
			}
		})
	}
}
