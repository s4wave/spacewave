package plugin_host_scheduler

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_materializer "github.com/s4wave/spacewave/bldr/manifest/materializer"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	block "github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_lz4 "github.com/s4wave/spacewave/db/block/transform/lz4"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestMaterializeManifestThroughPluginHostVolumeProxy exercises browser
// worker demand, copy completion, and release with and without an instance key.
func TestMaterializeManifestThroughPluginHostVolumeProxy(t *testing.T) {
	t.Run("empty instance key", func(t *testing.T) {
		runBrowserCopyFixture(t, "")
	})
	t.Run("copy-instance", func(t *testing.T) {
		runBrowserCopyFixture(t, "copy-instance")
	})
}

// runBrowserCopyFixture copies a manifest from a source testbed to a host
// testbed through the real Materializer service running on a third plugin
// bus, exercising the production wiring end to end: request-scoped source
// BlockStore RPC (plugin-scoped server filter), plugin-host/ prefix routing,
// and the destination ProxyVolume mapping. instanceKey selects the browser
// worker identity the source filter must accept.
func runBrowserCopyFixture(t *testing.T, instanceKey string) {
	t.Helper()

	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Three independent buses: source store, plugin host, plugin worker.
	srcTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcTB.Release)
	hostTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(hostTB.Release)
	pluginTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(pluginTB.Release)

	// Buckets: source on the source bus, destination on the host bus.
	for _, cfg := range []struct {
		tb       *testbed.Testbed
		bucketID string
	}{{srcTB, "mm-src"}, {hostTB, "mm-dest"}} {
		if _, _, _, err := cfg.tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
			Id:  cfg.bucketID,
			Rev: 1,
		}); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Source cursor with gzip transform; destination cursor with lz4.
	srcTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_lz4.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx, srcTB.Bus, le, srcTB.StepFactorySet,
		"mm-src", srcTB.Volume.GetID(), srcTransformConf, nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcCursor.Release)
	destCursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx, hostTB.Bus, le, hostTB.StepFactorySet,
		"mm-dest", hostTB.Volume.GetID(), destTransformConf, nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(destCursor.Release)

	// Small valid manifest: one nested FS directory node referenced by both
	// dist and assets, so the graph is exactly two blocks.
	nodeData, err := (&unixfs_block.FSNode{
		NodeType: unixfs_block.NodeType_NodeType_DIRECTORY,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	dirRef, _, err := srcCursor.PutBlock(ctx, nodeData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcManifest := &bldr_manifest.Manifest{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "mm-test-manifest",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		Entrypoint:  "entrypoint",
		DistFsRef:   dirRef,
		AssetsFsRef: dirRef,
	}
	btx, bcs := srcCursor.BuildTransaction(nil)
	bcs.SetBlock(srcManifest, true)
	manifestRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(manifestRef)
	srcRef := srcCursor.GetRef()
	if srcRef.GetRootRef().GetEmpty() {
		t.Fatal("test setup: source ref is empty")
	}

	// Follow the manifest root to obtain the selected source cursor, as the
	// scheduler does when executing a manifest.
	srcSel, err := bldr_manifest_world.FollowObjectRefReadOnly(ctx, srcCursor, srcRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcSel.Release)

	// Source blocks must be absent from the destination before the call.
	rootRef := srcRef.GetRootRef()
	for _, ref := range []*block.BlockRef{rootRef, dirRef} {
		_, exists, err := destCursor.GetBucket().GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if exists {
			t.Fatal("test setup: destination already holds a source block")
		}
	}

	// Host mux: fallback invoker into the host bus (as buildPluginMux does),
	// plus the host volume ProxyVolume services under the host prefix.
	hostMux := srpc.NewMux(bifrost_rpc.NewInvoker(
		hostTB.Bus,
		bldr_plugin.PluginServerID("bldr-materializer", ""),
		true,
	))
	if err := volume_rpc_server.RegisterProxyVolumeWithPrefix(
		hostMux,
		volume_rpc_server.NewProxyVolume(ctx, hostTB.Volume, false),
		bldr_plugin.HostVolumeServiceIDPrefix,
	); err != nil {
		t.Fatal(err.Error())
	}

	// Expose the host mux on the host bus under the web-worker server identity,
	// exactly as host/web does, and route the plugin's host client through a
	// bus invoker carrying that identity.
	workerServerID := "web-worker/" + bldr_plugin.PluginServerID("bldr-materializer", instanceKey)
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test-web-worker-rpc-host", Version, "rpc host for plugin"),
		func(_ context.Context, _ func()) (srpc.Invoker, func(), error) {
			return hostMux, nil, nil
		},
		nil,
		false,
		nil,
		nil,
		regexp.MustCompile("^"+regexp.QuoteMeta(workerServerID)+"$"),
	)
	relRpcServiceCtrl, err := hostTB.Bus.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relRpcServiceCtrl)
	hostClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(
		bifrost_rpc.NewInvoker(hostTB.Bus, workerServerID, true),
	)))

	// Plugin bus: reach host services through the host pipe.
	pluginHostBridge := bifrost_rpc.NewClientController(
		le,
		pluginTB.Bus,
		controller.NewInfo("test-plugin-host-bridge", controller.MustParseVersion("0.0.1"), "plugin bus bridge to host services"),
		hostClient,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	relBridge, err := pluginTB.Bus.AddController(ctx, pluginHostBridge, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relBridge)

	// Plugin bus: mount the host volume proxy under the plugin-host volume
	// alias, exactly as the plugin entrypoint does.
	hostVolInfo, err := volume.NewVolumeInfo(
		ctx,
		controller.NewInfo("test-host-volume", controller.MustParseVersion("0.0.1"), "test host volume"),
		hostTB.Volume,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	proxyVolCtrl := volume_rpc_client.NewProxyVolumeControllerWithClient(
		pluginTB.Bus,
		le,
		hostVolInfo,
		[]string{bldr_plugin.PluginVolumeID},
		hostClient,
		bldr_plugin.HostVolumeServiceIDPrefix,
	)
	relProxyVol, err := pluginTB.Bus.AddController(ctx, proxyVolCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relProxyVol)

	// Serve the real Materializer on the plugin bus pipe.
	pluginMux := srpc.NewMux()
	if err := bldr_manifest_materializer.SRPCRegisterMaterializer(
		pluginMux,
		bldr_manifest_materializer.NewMaterializer(le, pluginTB.Bus, pluginTB.StepFactorySet),
	); err != nil {
		t.Fatal(err.Error())
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux)))

	// Route the copy's RPC lookup through the scheduler's normal plugin demand.
	// The fixture publishes its already constructed worker only for LoadPlugin.
	ctrl := &Controller{bus: hostTB.Bus, conf: &Config{InstanceKey: instanceKey}}
	demanded := make(chan string, 1)
	released := make(chan struct{})
	relPluginHandler, err := hostTB.Bus.AddHandler(directive.NewFuncHandler(
		func(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			switch dir := inst.GetDirective().(type) {
			case bifrost_rpc.LookupRpcClient:
				return directive.R(bldr_plugin.ResolveLookupRpcClient(ctx, dir, ctrl))
			case bldr_plugin.LoadPlugin:
				if dir.LoadPluginID() != "bldr-materializer" {
					return nil, nil
				}
				return directive.R(directive.NewFuncResolver(
					func(ctx context.Context, handler directive.ResolverHandler) error {
						demanded <- dir.LoadPluginInstanceKey()
						defer close(released)
						_, _ = handler.AddValue(bldr_plugin.NewRunningPlugin(pluginClient))
						handler.MarkIdle(true)
						<-ctx.Done()
						return ctx.Err()
					},
				), nil)
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relPluginHandler)

	// Call the scheduler helper against the host bus. Bound the call so the
	// RPC completes or fails within the deadline instead of hanging.
	copyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(cancel)
	copiedRef, _, err := ctrl.materializeManifest(
		copyCtx,
		"bldr-materializer",
		destCursor,
		srcSel,
		2,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The copy must demand the scheduler's instance and release it on completion.
	select {
	case got := <-demanded:
		if got != instanceKey {
			t.Fatalf("materializer instance key = %q, want %q", got, instanceKey)
		}
	default:
		t.Fatal("copy completed without demanding the materializer plugin")
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("completed copy retained its materializer plugin demand")
	}

	// The copied root must match the source root hash and sit in the
	// destination bucket.
	if !copiedRef.GetRootRef().EqualsRef(rootRef) {
		t.Fatalf("copied root %s != source root %s",
			copiedRef.GetRootRef().MarshalString(), rootRef.MarshalString())
	}
	if copiedRef.GetBucketId() != "mm-dest" {
		t.Fatalf("copied bucket id = %q, want %q", copiedRef.GetBucketId(), "mm-dest")
	}

	// Both the manifest root and the nested directory node must now exist in
	// the host TB with bytes identical to the source bucket's at-rest bytes.
	for _, ref := range []*struct {
		name string
		ref  *block.BlockRef
	}{{"manifest root", rootRef}, {"directory node", dirRef}} {
		wantData, wantExists, err := srcCursor.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !wantExists {
			t.Fatalf("test setup: source bucket missing %s", ref.name)
		}
		gotData, gotExists, err := destCursor.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !gotExists {
			t.Fatalf("copied %s missing from host destination bucket", ref.name)
		}
		if !bytes.Equal(gotData, wantData) {
			t.Fatalf("copied %s raw bytes != source raw bytes", ref.name)
		}
	}

	// Decode the copied manifest and compare with the source manifest.
	gotCursor, err := destCursor.FollowRef(ctx, copiedRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(gotCursor.Release)
	_, gotBcs := gotCursor.BuildTransaction(nil)
	gotManifest, err := bldr_manifest.UnmarshalManifest(ctx, gotBcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !gotManifest.EqualVT(srcManifest) {
		t.Fatalf("decoded manifest = %v, want %v", gotManifest, srcManifest)
	}
}
