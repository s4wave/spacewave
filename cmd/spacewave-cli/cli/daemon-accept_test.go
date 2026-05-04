//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

func TestAcceptDaemonListenerServesConcurrentResourceClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := os.MkdirTemp("/tmp", "sw-daemon-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, socketName)
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	mux := srpc.NewMux()
	resServer := resource_server.NewResourceServer(srpc.NewMux())
	if err := resServer.Register(mux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	srv := srpc.NewServer(mux)
	errCh := make(chan error, 1)
	go func() {
		errCh <- acceptDaemonListener(ctx, lis, srv, nil)
	}()

	first, firstConn := openDaemonResourceClient(t, ctx, sock)
	defer first.Release()
	defer firstConn.Close()

	secondCtx, secondCancel := context.WithTimeout(ctx, time.Second)
	defer secondCancel()
	second, secondConn := openDaemonResourceClient(t, secondCtx, sock)
	defer second.Release()
	defer secondConn.Close()

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
	t.Helper()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}

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
