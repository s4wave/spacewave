package volume_rpc_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/coord"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
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
		controller.NewInfo("volume/rpc/test/client", controller.MustParseVersion("0.0.1"), "test rpc client"),
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

	capability, err := vol.Capability(ctx, coord.Scope{
		VolumeID:      vol.GetID(),
		ObjectStoreID: "rpc-volume-test",
		ParticipantID: "rpc-volume-test-client",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !capability.Supported {
		t.Fatal("expected RPC proxy coordinator to report supported remote coordination")
	}
	if capability.Backend != coord.BackendKindRPC {
		t.Fatalf("expected RPC backend kind, got %q", capability.Backend)
	}
	if capability.VolumeID != proxyVolumeID {
		t.Fatalf("expected capability volume id %q, got %q", proxyVolumeID, capability.VolumeID)
	}
	if capability.ObjectStoreID != "rpc-volume-test" {
		t.Fatalf("expected capability object store id %q, got %q", "rpc-volume-test", capability.ObjectStoreID)
	}
	if capability.FallbackReason != coord.FallbackReasonNone {
		t.Fatalf("expected no fallback reason, got %q", capability.FallbackReason)
	}

	scope := coord.Scope{
		VolumeID:      proxyVolumeID,
		ObjectStoreID: "rpc-volume-watch-test",
		ParticipantID: "rpc-volume-test-client",
	}
	watch, err := vol.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer watch.Close()

	publishPrefix := func() error {
		lease, err := tb1.Volume.WaitAcquireWriteLease(ctx, coord.Scope{
			VolumeID:      proxyVolumeID,
			ObjectStoreID: scope.ObjectStoreID,
			ParticipantID: "rpc-volume-test-server",
		})
		if err != nil {
			return err
		}
		_, err = lease.Publish(ctx, coord.Event{
			VolumeID:         proxyVolumeID,
			ObjectStoreID:    scope.ObjectStoreID,
			KeyPrefixChanged: []byte("rpc-prefix"),
		})
		if err != nil {
			_ = lease.Release(ctx)
			return err
		}
		return lease.Release(ctx)
	}
	if err := publishPrefix(); err != nil {
		t.Fatal(err.Error())
	}

	timeout := time.After(5 * time.Second)
	retryPublish := time.NewTicker(50 * time.Millisecond)
	defer retryPublish.Stop()
	foundRemoteWatchEvent := false
	for !foundRemoteWatchEvent {
		select {
		case event, ok := <-watch.Events():
			if !ok {
				t.Fatal("expected remote coordinator key-prefix event before watch closed")
			}
			if event.VolumeID != proxyVolumeID {
				t.Fatalf("expected watch event volume id %q, got %q", proxyVolumeID, event.VolumeID)
			}
			if event.ObjectStoreID != scope.ObjectStoreID {
				t.Fatalf("expected watch event object store id %q, got %q", scope.ObjectStoreID, event.ObjectStoreID)
			}
			if string(event.KeyPrefixChanged) == "rpc-prefix" {
				foundRemoteWatchEvent = true
			}
		case <-retryPublish.C:
			if err := publishPrefix(); err != nil {
				t.Fatal(err.Error())
			}
		case <-timeout:
			t.Fatal("timed out waiting for remote coordinator watch event")
		}
	}
	if err := watch.Close(); err != nil {
		t.Fatal(err.Error())
	}

	snapshot, err := vol.Snapshot(ctx, scope)
	if err != nil {
		t.Fatalf("expected RPC coordinator snapshot, got %v", err)
	}
	if snapshot.VolumeID != proxyVolumeID {
		t.Fatalf("expected snapshot volume id %q, got %q", proxyVolumeID, snapshot.VolumeID)
	}
	lease, ok, err := vol.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatalf("expected RPC coordinator try-lease, got %v", err)
	}
	if !ok {
		t.Fatal("expected RPC coordinator try-lease to acquire")
	}
	if _, err := lease.Refresh(ctx); err != nil {
		_ = lease.Release(ctx)
		t.Fatalf("expected RPC coordinator lease refresh, got %v", err)
	}
	if _, err := lease.Publish(ctx, coord.Event{
		KeyPrefixChanged: []byte("rpc-lease-prefix"),
	}); err != nil {
		_ = lease.Release(ctx)
		t.Fatalf("expected RPC coordinator lease publish, got %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("expected RPC coordinator lease release, got %v", err)
	}

	lease, err = vol.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatalf("expected RPC coordinator wait-lease, got %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("expected RPC coordinator wait-lease release, got %v", err)
	}

	t.Log("testing object store api")
	if err := store_test.TestObjectStore(ctx, vol, store_test.WithVLogger(le)); err != nil {
		t.Fatal(err.Error())
	}
}
