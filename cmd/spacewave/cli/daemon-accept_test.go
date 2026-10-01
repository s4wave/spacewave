//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

func TestAcceptDaemonListenerServesConcurrentResourceClients(t *testing.T) {
	// Listen on a short unix socket for the daemon.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register the resource server on the srpc mux.
	dir := shortSocketDir(t)
	sock := filepath.Join(dir, socketName)
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	// Open two concurrent resource clients against the listener.
	mux := srpc.NewMux()
	resServer := resource_server.NewResourceServer(srpc.NewMux())
	if err := resServer.Register(mux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	srv := srpc.NewServer(mux)
	errCh := make(chan error, 1)
	go func() {
		closeClients, err := acceptDaemonListener(ctx, lis, srv, nil)
		closeClients()
		errCh <- err
	}()

	// Stop the listener and confirm acceptDaemonListener returns.
	first, firstConn := openDaemonResourceClient(t, ctx, sock)
	defer firstConn.Close()
	defer first.Release()

	// Open a second concurrent resource client.
	secondCtx, secondCancel := context.WithTimeout(ctx, time.Second)
	defer secondCancel()
	second, secondConn := openDaemonResourceClient(t, secondCtx, sock)
	defer secondConn.Close()
	defer second.Release()

	// Shut down the listener and confirm acceptDaemonListener returns.
	cancel()
	lis.Close()
	select {
	case <-time.After(time.Second):
		t.Fatal("daemon listener did not stop")
	case <-errCh:
	}
}

func openDaemonResourceClient(
	t *testing.T,
	ctx context.Context,
	sock string,
) (*resource_client.Client, net.Conn) {
	// Dial the daemon socket.
	t.Helper()

	// Build the srpc client and resource service over the connection.
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}

	// Build the srpc client and resource service over the connection.
	srpcClient, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		conn.Close()
		t.Fatalf("create srpc client: %v", err)
	}
	svc := resource.NewSRPCResourceServiceClient(srpcClient)
	client, err := resource_client.NewClient(ctx, svc)
	if err != nil {
		conn.Close()
		t.Fatalf("resource client: %v", err)
	}
	return client, conn
}
