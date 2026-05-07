package plugin_space

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	bldr_core "github.com/s4wave/spacewave/bldr/core"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/net/hash"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// testHostPluginID is the plugin id of the fake host used by the forwarding tests.
const testHostPluginID = "spacewave-core"

func TestRunCloudBlockStoreForwardingExposesHostBucket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	pluginBus, pluginResolver, err := bldr_core.NewCoreBus(ctx, le.WithField("bus", "plugin"))
	if err != nil {
		t.Fatal(err)
	}
	pluginResolver.AddFactory(plugin_host_configset.NewFactory(pluginBus))
	hostBus, _, err := bldr_core.NewCoreBus(ctx, le.WithField("bus", "host"))
	if err != nil {
		t.Fatal(err)
	}
	hostConfigSetCtrl, err := configset_controller.NewController(le.WithField("bus", "host"), hostBus)
	if err != nil {
		t.Fatal(err)
	}
	relHostConfigSet, err := hostBus.AddController(ctx, hostConfigSetCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relHostConfigSet()
	hostNodeCtrl := node_controller.NewController(&node_controller.Config{}, le.WithField("bus", "host"), hostBus)
	relHostNode, err := hostBus.AddController(ctx, hostNodeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relHostNode()

	if rel := addFakePluginHost(t, ctx, le, pluginBus, hostBus); rel != nil {
		defer rel()
	}
	if rel := addPluginClientOnHost(t, ctx, le, pluginBus, hostBus); rel != nil {
		defer rel()
	}

	bucketID := "p/spacewave/acct/blk/space"
	storeCtrl := block_store_inmem.NewController(le, &block_store_inmem.Config{BlockStoreId: bucketID})
	relStore, err := pluginBus.AddController(ctx, storeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStore()

	store, _, storeRef, err := block_store.ExLookupFirstBlockStore(ctx, pluginBus, bucketID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer storeRef.Release()
	body := []byte("forwarded block")
	ref, _, err := store.PutBlock(ctx, body, &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err)
	}

	forwarder := NewCloudBlockStoreForwarder(
		le,
		pluginBus,
		"space/spacewave/acct/space",
		bucketID,
		testHostPluginID,
	)
	forwarderRef, err := pluginBus.AddController(ctx, forwarder, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer forwarderRef()

	hostBucket, _, hostBucketRef, err := bucket.ExBuildBucketAPI(ctx, hostBus, false, bucketID, bucketID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hostBucketRef.Release()
	if hostBucket.GetID() != bucketID {
		t.Fatalf("expected host bucket id %q, got %q", bucketID, hostBucket.GetID())
	}

	got, found, err := hostBucket.GetBucket().GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected block to be readable through host bucket")
	}
	if string(got) != string(body) {
		t.Fatalf("expected block body %q, got %q", string(body), string(got))
	}

	hostLookup, _, hostLookupRef, err := bucket_lookup.ExBuildBucketLookup(ctx, hostBus, false, bucketID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hostLookupRef.Release()

	got, found, err = bucket_lookup.NewBucketFromHandle(hostLookup).GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected block to be readable through host bucket lookup")
	}
	if string(got) != string(body) {
		t.Fatalf("expected lookup block body %q, got %q", string(body), string(got))
	}
}

func addFakePluginHost(t *testing.T, ctx context.Context, le *logrus.Entry, pluginBus bus.Bus, hostBus bus.Bus) func() {
	t.Helper()

	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, &testPluginHost{hostBus: hostBus}); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	ctrl := bifrost_rpc.NewClientController(
		le,
		pluginBus,
		controller.NewInfo("test/plugin-host-client", semver.MustParse("0.0.1"), ""),
		client,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	rel, err := pluginBus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func addPluginClientOnHost(t *testing.T, ctx context.Context, le *logrus.Entry, pluginBus bus.Bus, hostBus bus.Bus) func() {
	t.Helper()

	serverID := bldr_plugin.HostServerID("default")
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(pluginBus, serverID, true))))
	ctrl := bifrost_rpc.NewClientController(
		le,
		hostBus,
		controller.NewInfo("test/spacewave-core-client", semver.MustParse("0.0.1"), ""),
		client,
		[]string{bldr_plugin.PluginServiceID(testHostPluginID, "")},
	)
	rel, err := hostBus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

type testPluginHost struct {
	hostBus bus.Bus
}

func (h *testPluginHost) GetPluginInfo(context.Context, *bldr_plugin.GetPluginInfoRequest) (*bldr_plugin.GetPluginInfoResponse, error) {
	return &bldr_plugin.GetPluginInfoResponse{PluginId: testHostPluginID}, nil
}

func (h *testPluginHost) ExecController(req *controller_exec.ExecControllerRequest, strm bldr_plugin.SRPCPluginHost_ExecControllerStream) error {
	return req.Execute(strm.Context(), h.hostBus, true, strm.Send)
}

func (h *testPluginHost) LoadPlugin(*bldr_plugin.LoadPluginRequest, bldr_plugin.SRPCPluginHost_LoadPluginStream) error {
	return errors.New("LoadPlugin is not used by this test")
}

func (h *testPluginHost) PluginRpc(bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return errors.New("PluginRpc is not used by this test")
}

func (h *testPluginHost) PluginFsRpc(bldr_plugin.SRPCPluginHost_PluginFsRpcStream) error {
	return errors.New("PluginFsRpc is not used by this test")
}

// _ is a type assertion
var _ bldr_plugin.SRPCPluginHostServer = ((*testPluginHost)(nil))
