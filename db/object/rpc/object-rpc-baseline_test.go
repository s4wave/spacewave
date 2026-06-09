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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	mux := srpc.NewMux()
	if err := object_rpc.SRPCRegisterObjectStore(mux, object_rpc_server.NewObjectStore(ctx, tb.Volume)); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	done := make(chan error, 1)
	go func() {
		done <- server.AcceptMuxedConn(ctx, serverMp)
	}()

	client := object_rpc_client.NewObjectStore(object_rpc.NewSRPCObjectStoreClient(srpc.NewClientWithMuxedConn(clientMp)))
	const objectStoreID = "rpc-baseline"
	remoteStore, releaseRemoteStore, err := client.AccessObjectStore(ctx, objectStoreID, func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseRemoteStore()

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

	localStore, releaseLocalStore, err := tb.Volume.AccessObjectStore(ctx, objectStoreID, func() {})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseLocalStore()
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

	cancel()
	_ = clientPipe.Close()
	_ = serverPipe.Close()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("unexpected object store RPC server exit: %v", err)
	}
}
