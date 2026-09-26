package plugin_host_storage

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/core"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestSelectedHostStorage isolates identical volume names over the real plugin
// configset and Volume RPC path, including an unresolved provider selection.
func TestSelectedHostStorage(t *testing.T) {
	// Build the host with two distinct providers and a default provider.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	hostBus, hostResolver, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	hostResolver.AddFactory(volume_rpc_server.NewFactory(hostBus))
	configSet, err := configset_controller.NewController(le, hostBus)
	if err != nil {
		t.Fatal(err)
	}
	release, err := hostBus.AddController(ctx, configSet, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	for _, id := range []string{"default", "left/app", "right/app"} {
		st := storage_inmem.NewInmemStorage(id)
		st.AddFactories(hostBus, hostResolver)
		info := controller.NewInfo(storage_inmem.ControllerID, storage_inmem.Version, "")
		release, err := hostBus.AddController(ctx, storage_controller.BuildStorageController(id, []storage.Storage{st}, info), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}

	// Give each plugin its own bus and the ordinary host RPC transport.
	mount := func(storageID string) (volume.Controller, error) {
		t.Helper()
		pluginBus, pluginResolver, err := core.NewCoreBus(ctx, le)
		if err != nil {
			return nil, err
		}
		pluginResolver.AddFactory(plugin_host_configset.NewFactory(pluginBus))
		selected := NewPluginHostStorage(storageID)
		selected.AddFactories(pluginBus, pluginResolver)
		info := controller.NewInfo("test/plugin-storage", controller.MustParseVersion("0.0.1"), "")
		storageCtrl := storage_controller.BuildStorageController("default", []storage.Storage{selected}, info)
		release, err := pluginBus.AddController(ctx, storageCtrl, nil)
		if err != nil {
			return nil, err
		}
		t.Cleanup(release)
		mux := srpc.NewMux(bifrost_rpc.NewInvoker(hostBus, "", true))
		server := plugin_host.NewPluginHostServer(ctx, hostBus, le, "test-plugin", storageID, nil, nil, storageID, false)
		if err := bldr_plugin.SRPCRegisterPluginHost(mux, server); err != nil {
			return nil, err
		}
		client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
		rpcCtrl := bifrost_rpc.NewClientController(le, pluginBus, info, client, []string{bldr_plugin.HostServiceIDPrefix})
		release, err = pluginBus.AddController(ctx, rpcCtrl, nil)
		if err != nil {
			return nil, err
		}
		t.Cleanup(release)
		vol, ref, err := storage_volume.ExecVolumeController(ctx, pluginBus, &storage_volume.Config{
			StorageId:       "default",
			StorageVolumeId: "same/account",
			VolumeConfig:    &volume_controller.Config{GcIntervalDur: "0"},
		})
		if err != nil {
			return nil, err
		}
		t.Cleanup(ref.Release)
		return vol, nil
	}

	// Equal names in different selected providers have separate identities and data.
	volumes := make([]volume.Volume, 0, 3)
	for _, id := range []string{"", "left/app", "right/app"} {
		ctrl, err := mount(id)
		if err != nil {
			t.Fatal(err)
		}
		vol, err := ctrl.GetVolume(ctx)
		if err != nil {
			t.Fatal(err)
		}
		volumes = append(volumes, vol)
	}
	ref, _, err := volumes[1].PutBlock(ctx, []byte("left-only"), &block.PutOpts{Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	for i, vol := range volumes {
		found, err := vol.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if found != (i == 1) {
			t.Fatalf("provider %d block visibility = %v", i, found)
		}
		if i != 1 && vol.GetPeerID() == volumes[1].GetPeerID() {
			t.Fatalf("provider %d shares the selected provider identity", i)
		}
	}

	// An unknown explicit provider stays pending and observes cancellation.
	missing, err := mount("missing")
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	if vol, err := missing.GetVolume(waitCtx); err == nil || vol != nil {
		t.Fatalf("missing provider returned volume %v, error %v", vol, err)
	}
}
