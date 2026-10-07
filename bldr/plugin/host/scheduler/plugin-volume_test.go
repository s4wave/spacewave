package plugin_host_scheduler

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	rpc_gc "github.com/s4wave/spacewave/db/block/gc/rpc"
	rpc_block "github.com/s4wave/spacewave/db/block/rpc"
	"github.com/s4wave/spacewave/db/bucket"
	rpc_bucket "github.com/s4wave/spacewave/db/bucket/store/rpc"
	rpc_object "github.com/s4wave/spacewave/db/object/rpc"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	volume_scoped "github.com/s4wave/spacewave/db/volume/scoped"
	"github.com/sirupsen/logrus"
)

// servePluginVolume serves the volume a plugin of the scheduler receives and
// returns the plugin's client for it. Each call serves a new proxy volume, as
// each start of the plugin does.
func servePluginVolume(
	ctx context.Context,
	t *testing.T,
	c *Controller,
	hostVol volume.Volume,
	pluginID string,
) *volume_rpc_client.ProxyVolume {
	// Serve the plugin's volume over RPC, as the plugin's mux does.
	t.Helper()
	proxyVol := volume_rpc_server.NewProxyVolume(ctx, c.newPluginVolume(hostVol, pluginID), false)
	mux := srpc.NewMux()
	if err := volume_rpc_server.RegisterProxyVolume(mux, proxyVol); err != nil {
		t.Fatal(err.Error())
	}
	rpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))

	// Build the plugin's client from the served volume's info.
	volClient := volume_rpc.NewSRPCProxyVolumeClient(rpcClient)
	info, err := volClient.GetVolumeInfo(ctx, &volume_rpc.GetVolumeInfoRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	vol, err := volume_rpc_client.NewProxyVolume(
		info.GetVolumeInfo(),
		volClient,
		rpc_block.NewSRPCBlockStoreClient(rpcClient),
		rpc_bucket.NewSRPCBucketStoreClient(rpcClient),
		rpc_object.NewSRPCObjectStoreClient(rpcClient),
		rpc_gc.NewSRPCRefGraphClient(rpcClient),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return vol
}

// writeKey sets key in the object store of vol.
func writeKey(ctx context.Context, t *testing.T, vol volume.Volume, storeID, key, value string) {
	// Open a write transaction on the object store.
	t.Helper()
	store, release, err := vol.AccessObjectStore(ctx, storeID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()

	// Commit the key.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// readKey returns the value of key in the object store of vol.
func readKey(ctx context.Context, t *testing.T, vol volume.Volume, storeID, key string) ([]byte, bool) {
	// Open a read transaction on the object store.
	t.Helper()
	store, release, err := vol.AccessObjectStore(ctx, storeID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Read the key.
	value, found, err := tx.Get(ctx, []byte(key))
	if err != nil {
		t.Fatal(err.Error())
	}
	return value, found
}

// TestSpacePluginVolumeStaysInScope serves plugins of two Spaces the volumes
// the scheduler serves them and checks each reaches only its own data.
func TestSpacePluginVolumeStaysInScope(t *testing.T) {
	// Build the host volume and seed it with app state outside any plugin.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	hostVol := tb.Volume
	writeKey(ctx, t, hostVol, "session-state", "token", "secret")
	if _, _, _, err := hostVol.ApplyBucketConfig(ctx, &bucket.Config{Id: "session", Rev: 1}); err != nil {
		t.Fatal(err.Error())
	}

	// Serve one plugin of Space a, and the same plugin of Space b.
	spaceA := &Controller{conf: &Config{InstanceKey: "space-a"}}
	spaceB := &Controller{conf: &Config{InstanceKey: "space-b"}}
	plugin := servePluginVolume(ctx, t, spaceA, hostVol, "plugin")
	otherSpace := servePluginVolume(ctx, t, spaceB, hostVol, "plugin")

	// The plugin writes its own bucket and object store through the view.
	if _, _, _, err := plugin.ApplyBucketConfig(ctx, &bucket.Config{Id: "world", Rev: 1}); err != nil {
		t.Fatal(err.Error())
	}
	writeKey(ctx, t, plugin, "world", "head", "a")
	writeKey(ctx, t, otherSpace, "world", "head", "b")

	// The data lives under the plugin's prefix in the host volume.
	prefix := pluginVolumePrefix("space-a", "plugin")
	if conf, err := hostVol.GetBucketConfig(ctx, prefix+"world"); err != nil || conf == nil {
		t.Fatalf("host bucket under the prefix: %v, %v", conf, err)
	}
	if value, found := readKey(ctx, t, hostVol, prefix+"world", "head"); !found || string(value) != "a" {
		t.Fatalf("host object store under the prefix: %q, %v", value, found)
	}

	// The plugin sees its own data, not the host's or another Space's.
	if value, found := readKey(ctx, t, plugin, "world", "head"); !found || !bytes.Equal(value, []byte("a")) {
		t.Fatalf("own object store: %q, %v", value, found)
	}
	if _, found := readKey(ctx, t, plugin, "session-state", "token"); found {
		t.Fatal("plugin read the app's session state")
	}
	if _, found := readKey(ctx, t, plugin, pluginVolumePrefix("space-b", "plugin")+"world", "head"); found {
		t.Fatal("plugin read another Space's plugin data by its host ID")
	}
	if conf, err := plugin.GetBucketConfig(ctx, "session"); err != nil || conf != nil {
		t.Fatalf("plugin read an app bucket: %v, %v", conf, err)
	}
	infos, err := plugin.ListBucketInfo(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(infos) != 1 || infos[0].GetConfig().GetId() != "world" {
		t.Fatalf("listed buckets: %v", infos)
	}

	// The plugin cannot reach the peer key store.
	if _, err := plugin.LoadPeerPriv(ctx); err == nil {
		t.Fatal("plugin loaded the peer private key")
	}

	// A restart serves a new volume over the same host data.
	restarted := servePluginVolume(ctx, t, spaceA, hostVol, "plugin")
	if value, found := readKey(ctx, t, restarted, "world", "head"); !found || string(value) != "a" {
		t.Fatalf("object store after restart: %q, %v", value, found)
	}
	if conf, err := restarted.GetBucketConfig(ctx, "world"); err != nil || conf == nil {
		t.Fatalf("bucket after restart: %v, %v", conf, err)
	}
}

// TestSchedulersServeScopedVolumes checks a Space scheduler scopes its plugins
// and a scheduler without an instance key serves the host volume as it is.
func TestSchedulersServeScopedVolumes(t *testing.T) {
	// Build the host volume and both kinds of scheduler.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	space := &Controller{conf: &Config{InstanceKey: "space-a"}}
	app := &Controller{conf: &Config{}}

	// The app's own plugins use the host volume.
	if got := app.newPluginVolume(tb.Volume, "plugin"); got != tb.Volume {
		t.Fatalf("app plugin volume is scoped: %T", got)
	}

	// The Space's plugins get a view that refuses block deletion.
	view := space.newPluginVolume(tb.Volume, "plugin")
	if _, ok := view.(*volume_scoped.Volume); !ok {
		t.Fatalf("Space plugin volume is not scoped: %T", view)
	}
	ref, _, err := view.PutBlock(ctx, []byte("data"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := view.RmBlock(ctx, ref); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("RmBlock through the Space plugin volume: %v", err)
	}
	if err := view.GetRefGraph().AddRef(ctx, block_gc.NodeGCRoot, block_gc.BlockIRI(ref)); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("gcroot edge through the Space plugin volume: %v", err)
	}
}

// TestPluginVolumePrefixIsUnambiguous checks no two Space and plugin pairs
// share a prefix.
func TestPluginVolumePrefixIsUnambiguous(t *testing.T) {
	pairs := [][2]string{{"a/b", "c"}, {"a", "b/c"}, {"a", "b"}, {"a%2Fb", "c"}}
	var prefixes []string
	for _, pair := range pairs {
		prefix := pluginVolumePrefix(pair[0], pair[1])
		if slices.Contains(prefixes, prefix) {
			t.Fatalf("prefix %q repeats", prefix)
		}
		prefixes = append(prefixes, prefix)
	}
}
