package volume_rpc_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// TestRPCVolume tests the RPC volume end to end.
func TestRPCVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// tb1 is the server bus
	tb1, err := testbed.NewTestbed(ctx, le.WithField("testbed", "server"))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb1.StaticResolver.AddFactory(volume_rpc_server.NewFactory(tb1.Bus))

	// tb2 is the client bus
	tb2, err := testbed.NewTestbed(ctx, le.WithField("testbed", "client"), testbed.WithVolumeConfig(nil))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb2.StaticResolver.AddFactory(volume_rpc_client.NewFactory(tb2.Bus))

	// construct the rpc server
	volumeServiceID := "rpc.volume.AccessVolumes"
	hostServicePrefix := "remote/"
	proxyVolumeID := tb1.Volume.GetID()
	_, _, proxyVolumeServerRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb1.Bus,
		resolver.NewLoadControllerWithConfig(volume_rpc_server.NewConfig(
			volumeServiceID,
			[]string{proxyVolumeID},
		)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer proxyVolumeServerRef.Release()

	// forward incoming RPCs to directives
	srpcInvoker := bifrost_rpc.NewInvoker(tb1.Bus, "tb2", true)
	srpcServer := srpc.NewServer(srpcInvoker)
	rpcOpenStream := srpc.NewServerPipe(srpcServer)
	rpcClient := srpc.NewClient(rpcOpenStream)

	// add client and forward services with remote/ prefix to tb1
	rpcClientCtrl := bifrost_rpc.NewClientController(
		le,
		tb2.Bus,
		controller.NewInfo("volume/rpc/test/client", semver.MustParse("0.0.1"), "test rpc client"),
		rpcClient,
		[]string{hostServicePrefix},
	)
	rpcClientCtrlRel, err := tb2.Bus.AddController(ctx, rpcClientCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rpcClientCtrlRel()

	// construct the rpc client volume on tb2
	proxyVolumeService := hostServicePrefix + volumeServiceID
	volumeRpcClientConfig := volume_rpc_client.NewConfig(
		proxyVolumeService,
		// allow access to the primary volume only
		regexp.QuoteMeta(proxyVolumeID),
	)
	volumeRpcClientConfig.VolumeAliases = map[string]*volume_rpc_client.VolumeAliases{
		proxyVolumeID: {From: []string{"proxy-volume"}},
	}
	_, _, proxyVolumeClientRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb2.Bus,
		resolver.NewLoadControllerWithConfig(volumeRpcClientConfig),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer proxyVolumeClientRef.Release()

	// lookup the host volume on the client
	_, _, volRef, err := volume.ExLookupVolume(ctx, tb2.Bus, proxyVolumeID, "", false)
	if err == nil && volRef == nil {
		err = errors.New("expected LookupVolume to return the proxy volume but got none")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	volRef.Release()

	// test using the alias as well
	vol, _, volRef, err := volume.ExLookupVolume(ctx, tb2.Bus, "proxy-volume", "", false)
	if err == nil && volRef == nil {
		err = errors.New("expected LookupVolume to return the proxy volume but got none")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	volRef.Release()

	t.Log("testing object store api")
	if err := store_test.TestObjectStore(ctx, vol, store_test.WithVLogger(le)); err != nil {
		t.Fatalf(err.Error())
	}
	t.Log("testing message queue api")
	if err := store_test.TestMqueueAPI(ctx, vol); err != nil {
		t.Fatalf(err.Error())
	}
}
