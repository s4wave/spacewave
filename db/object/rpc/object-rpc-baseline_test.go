package object_rpc_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	object_rpc "github.com/s4wave/spacewave/db/object/rpc"
	object_rpc_client "github.com/s4wave/spacewave/db/object/rpc/client"
	object_rpc_server "github.com/s4wave/spacewave/db/object/rpc/server"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestObjectStoreRPCMutationPreservesProxyTransactionPath(t *testing.T) {
	// Start a testbed that hosts the object store RPC service.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build the testbed logger and attach a volume-backed environment.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Connect the RPC client and server with an in-process pipe.
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Create the client and server multiplexed connections.
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the object store service and serve muxed RPC connections.
	mux := srpc.NewMux()
	if err := object_rpc.SRPCRegisterObjectStore(mux, object_rpc_server.NewObjectStore(ctx, tb.Volume)); err != nil {
		t.Fatal(err.Error())
	}

	// Accept the server connection while the test uses the RPC client.
	server := srpc.NewServer(mux)
	done := make(chan error, 1)
	go func() {
		done <- server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Access the object store through its generated RPC client.
	client := object_rpc_client.NewObjectStore(object_rpc.NewSRPCObjectStoreClient(srpc.NewClientWithMuxedConn(clientMp)))
	const objectStoreID = "rpc-baseline"
	remoteStore, releaseRemoteStore, err := client.AccessObjectStore(ctx, objectStoreID, func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseRemoteStore()

	// Commit a mutation through the remote proxy transaction.
	remoteTx, err := remoteStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := remoteTx.Set(ctx, []byte("mutation-path"), []byte("rpc-proxy")); err != nil {
		remoteTx.Discard()
		t.Fatal(err.Error())
	}
	if err := remoteTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the same object store locally to verify the RPC mutation.
	localStore, releaseLocalStore, err := tb.Volume.AccessObjectStore(ctx, objectStoreID, func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseLocalStore()

	// Read the committed mutation from the volume's local store.
	localTx, err := localStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer localTx.Discard()
	value, found, err := localTx.Get(ctx, []byte("mutation-path"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || !bytes.Equal(value, []byte("rpc-proxy")) {
		t.Fatalf("remote RPC mutation was not committed through the proxy transaction path: found=%v value=%q", found, value)
	}

	// Stop both RPC endpoints and verify the server exits cleanly.
	cancel()
	_ = clientPipe.Close()
	_ = serverPipe.Close()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("unexpected object store RPC server exit: %v", err)
	}
}
