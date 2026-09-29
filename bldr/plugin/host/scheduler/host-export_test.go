package plugin_host_scheduler

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_export "github.com/s4wave/spacewave/bldr/plugin/host/export"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestExecPluginOnExportedHost runs a plugin from a Space's manifest world on
// a plugin host that exists only on the distribution's bus, as the
// spacewave-core plugin does in the desktop and browser apps. The host reads
// the plugin files the runtime publishes, and the Space scheduler reaches the
// running plugin through the export.
func TestExecPluginOnExportedHost(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// The distribution bus owns the only plugin host.
	const platformID = "desktop/darwin/arm64"
	distTb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer distTb.Release()
	host := &exportTestPluginHost{
		testPluginHost: testPluginHost{id: platformID},
		b:              distTb.Bus,
		started:        make(chan struct{}),
		stopped:        make(chan struct{}),
	}
	distSched := &Controller{
		pluginHostsCtr: ccontainer.NewCContainer(&pluginHostSet{
			pluginHosts: []bldr_plugin_host.PluginHost{host},
		}),
	}
	exportMux := srpc.NewMux()
	if err := plugin_host_export.SRPCRegisterHostExport(
		exportMux,
		plugin_host_export.NewServer(le, distTb.Bus, distSched),
	); err != nil {
		t.Fatal(err.Error())
	}
	hostClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(exportMux)))

	// The plugin bus reaches its host's services through the plugin-host prefix.
	spaceTb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceTb.Release()
	for _, ctrl := range []controller.Controller{
		bifrost_rpc.NewClientController(
			le,
			spaceTb.Bus,
			controller.NewInfo("test/plugin-host-client", controller.MustParseVersion("0.0.1"), ""),
			hostClient,
			[]string{bldr_plugin.HostServiceIDPrefix},
		),
		plugin_host_export.NewController(le, spaceTb.Bus),
	} {
		rel, err := spaceTb.Bus.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rel()
	}

	// Store the Space plugin's manifest in the Space's world.
	ocs, err := spaceTb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	distFS := memfs.New()
	if err := billy_util.WriteFile(distFS, "viewer.js", []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	manifest := bldr_manifest.NewManifest(
		bldr_manifest.NewManifestMeta("space-viewer", bldr_manifest.BuildType_DEV, platformID, 1),
		"viewer.js",
	)
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifest, distFS, nil, timestamppb.Now())
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// The Space scheduler finds the exported host on its own bus.
	proxy, _, proxyRef, err := bldr_plugin_host.ExLookupPluginHostByPlatform(ctx, spaceTb.Bus, false, platformID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer proxyRef.Release()

	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           spaceTb.Bus,
			conf:          &Config{},
			worldStateCtr: ccontainer.NewCContainer(wsv),
			hostVolumeCtr: ccontainer.NewCContainer(&hostVol{
				vol:  spaceTb.Volume,
				info: &volume.VolumeInfo{VolumeId: spaceTb.Volume.GetID()},
			}),
			pluginStatusCtr: ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
			pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
		},
		le:               le,
		pluginID:         "space-viewer",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(nil, bldr_plugin.InitialCapabilityRegistrationPending),
		),
	}
	execCtx, cancelExec := context.WithCancel(ctx)
	defer cancelExec()
	execErr := make(chan error, 1)
	go func() {
		execErr <- pi.execPlugin(execCtx, &executePluginArgs{
			manifestSnapshot: &bldr_manifest.ManifestSnapshot{
				ManifestRef: manifestRef,
				Manifest:    manifest,
			},
			pluginHost: proxy,
		})
	}()

	// The host read the entrypoint from the files published on its own bus.
	select {
	case <-host.started:
	case err := <-execErr:
		t.Fatalf("execPlugin exited before the host started: %v", err)
	}
	if host.err != nil {
		t.Fatal(host.err.Error())
	}
	if string(host.entrypoint) != "export {}\n" {
		t.Fatalf("entrypoint bytes = %q", host.entrypoint)
	}

	// The Space scheduler reaches the running plugin through the export.
	state := pi.pluginLoadStateCtr.GetValue()
	for state.GetRpcClient() == nil {
		state, err = pi.pluginLoadStateCtr.WaitValueChange(ctx, state, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	resp, err := echo.NewSRPCEchoerClient(state.GetRpcClient()).Echo(ctx, &echo.EchoMsg{Body: "hello"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetBody() != "hello" {
		t.Fatalf("echo body = %q", resp.GetBody())
	}

	// Stopping the Space's execution stops the plugin on the host.
	cancelExec()
	<-execErr
	<-host.stopped
}

// exportTestPluginHost reads the entrypoint through the bus, as the browser
// host's worker does, then serves Echo until its execution stops.
type exportTestPluginHost struct {
	testPluginHost
	// b is the distribution bus where plugin files are published
	b bus.Bus
	// started closes once entrypoint or err is set
	started chan struct{}
	// stopped closes when the execution returns
	stopped chan struct{}

	entrypoint []byte
	err        error
}

func (h *exportTestPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	defer close(h.stopped)
	h.entrypoint, h.err = h.readPublished(ctx, bldr_plugin.PluginArtifactID(pluginID, manifestRoot), entrypoint)
	close(h.started)
	if h.err != nil {
		return h.err
	}

	mux := srpc.NewMux()
	if err := echo.SRPCRegisterEchoer(mux, echo.NewEchoServer(nil)); err != nil {
		return err
	}
	if err := rpcInit(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))); err != nil {
		return err
	}
	<-ctx.Done()
	return context.Canceled
}

// readPublished reads a dist file published for artifactID on the bus.
func (h *exportTestPluginHost) readPublished(ctx context.Context, artifactID, path string) ([]byte, error) {
	access, ref, err := unixfs_access.ExAccessUnixFS(ctx, h.b, bldr_plugin.PluginDistFsId(artifactID), true, nil)
	if err != nil {
		return nil, err
	}
	if access == nil {
		return nil, errors.New("plugin dist is not published")
	}
	defer ref.Release()
	fs, release, err := access(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	file, _, err := fs.LookupPath(ctx, path)
	if err != nil {
		if file != nil {
			file.Release()
		}
		return nil, err
	}
	defer file.Release()
	return unixfs.ReadFile(ctx, file)
}
