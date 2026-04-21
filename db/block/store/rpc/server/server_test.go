package block_store_rpc_server

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreHTTPServer tests the block store rpc server and client.
func TestBlockStoreHTTPServer(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	serverTb, err := testbed.NewTestbed(ctx, le.WithField("testbed", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a block to lookup.
	serverVol := serverTb.Volume
	sampleBlockBody := []byte("testing block store rpc server")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, _, err := serverVol.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("put sample block ref %v", sampleBlockRef.MarshalString())

	// Create the RPC server, handles LookupRpcService
	serviceID := block_rpc.SRPCBlockStoreServiceID
	serverCtrl := NewController(serverTb.Bus, &Config{
		BlockStoreId: serverVol.GetID(),
		ServiceId:    serviceID,
	})
	serverRel, err := serverTb.Bus.AddController(ctx, serverCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer serverRel()

	// create the srpc server
	server := srpc.NewServer(bifrost_rpc.NewInvoker(serverTb.Bus, "test-server", false))
	srpcClient := srpc.NewClient(srpc.NewServerPipe(server))

	// Create the client
	clientTb, err := testbed.NewTestbed(ctx, le.WithField("testbed", "client"))
	if err != nil {
		t.Fatal(err.Error())
	}
	clientTb.StaticResolver.AddFactory(block_store_rpc.NewFactory(clientTb.Bus))

	// Add the client controller
	clientServiceID := "test-server/" + serviceID
	clientCtrl := bifrost_rpc.NewClientController(
		clientTb.Logger,
		clientTb.Bus,
		controller.NewInfo("test/store/client", semver.MustParse("0.0.1"), ""),
		srpcClient,
		[]string{"test-server/"},
	)
	clientRel, err := clientTb.Bus.AddController(ctx, clientCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer clientRel()

	// Create the client store
	clientBlockStoreID := "test-store"
	_, _, lookupCtrlRel, err := loader.WaitExecControllerRunning(
		ctx,
		clientTb.Bus,
		resolver.NewLoadControllerWithConfig(block_store_rpc.NewConfig(clientBlockStoreID, clientServiceID, false, nil)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lookupCtrlRel.Release()

	// Lookup the block store
	st, _, stRef, err := block_store.ExLookupFirstBlockStore(ctx, clientTb.Bus, clientBlockStoreID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer stRef.Release()

	ex, err := st.GetBlockExists(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ex {
		t.Fatal("expected sample block existed")
	}

	err = st.RmBlock(ctx, sampleBlockRef.Clone())
	if err != nil {
		t.Fatal(err.Error())
	}

	err = block_store_test.TestAll(ctx, st, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
}
