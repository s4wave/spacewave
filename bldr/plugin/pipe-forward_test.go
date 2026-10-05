package bldr_plugin

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	yamux "github.com/libp2p/go-yamux/v5"
	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// TestPipeForwardsLargeResponses checks that a plugin call forwarded to the
// host over the plugin pipe returns multi-megabyte responses intact, one at a
// time and concurrently, as a release manifest checkout reads them.
func TestPipeForwardsLargeResponses(t *testing.T) {
	// Listen on the plugin pipe as the process host does.
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	le := logrus.New().WithField("test", t.Name())
	pipeID := "fwd-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	listener, err := bldr_pipesock.Listen(le, t.TempDir(), pipeID)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Serve the echo service on the host end of the muxed pipe.
	hostMux := srpc.NewMux()
	if err := echo.NewEchoServer(hostMux).Register(hostMux); err != nil {
		t.Fatal(err)
	}
	hostErr := make(chan error, 1)
	go func() {
		// Accept the plugin's connection.
		conn, err := listener.Accept()
		if err != nil {
			hostErr <- err
			return
		}

		// Serve host calls over the muxed connection until it ends.
		muxed, err := srpc.NewMuxedConn(conn, true, pluginPipeYamuxConfig())
		if err != nil {
			hostErr <- err
			return
		}
		defer muxed.Close()
		hostErr <- srpc.NewServer(hostMux).AcceptMuxedConn(ctx, muxed)
	}()

	// Dial the host as the plugin entrypoint does.
	conn, err := bldr_pipesock.Dial(ctx, le, listener.GetRootDir(), pipeID)
	if err != nil {
		t.Fatal(err)
	}
	muxed, err := srpc.NewMuxedConn(conn, false, pluginPipeYamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer muxed.Close()

	// Call the host through the plugin-host forwarding invoker.
	hostClient := srpc.NewClientWithMuxedConn(muxed)
	local := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(newClientForwardingInvoker(hostClient, "plugin-host/"))))
	client := echo.NewSRPCEchoerClientWithServiceID(local, "plugin-host/"+echo.SRPCEchoerServiceID)
	roundTrip := func(size int) error {
		// Echo a body of size bytes.
		body := bytes.Repeat([]byte{byte(size)}, size)
		reply, err := client.Echo(ctx, &echo.EchoMsg{Body: string(body)})
		if err != nil {
			return err
		}

		// Compare the forwarded reply with the request.
		if reply.GetBody() != string(body) {
			t.Errorf("%d byte echo returned %d bytes", size, len(reply.GetBody()))
		}
		return nil
	}

	// Echo each size in turn.
	for _, size := range []int{1 << 10, 256 << 10, 1 << 20, 4 << 20, 9 << 20} {
		if err := roundTrip(size); err != nil {
			t.Fatalf("%d byte echo: %v", size, err)
		}
	}

	// Echo a concurrent burst of multi-megabyte responses.
	var burst errgroup.Group
	for i := range 16 {
		burst.Go(func() error { return roundTrip(2<<20 + i) })
	}
	if err := burst.Wait(); err != nil {
		t.Fatalf("concurrent echo: %v", err)
	}

	// The host connection stays up through every call.
	select {
	case err := <-hostErr:
		t.Fatalf("host connection ended: %v", err)
	default:
	}
}

// pluginPipeYamuxConfig returns the yamux settings both plugin pipe ends use.
func pluginPipeYamuxConfig() *yamux.Config {
	conf := srpc.NewYamuxConfig()
	conf.EnableKeepAlive = false
	return conf
}
