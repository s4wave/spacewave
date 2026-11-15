package plugin_host_scheduler

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	"github.com/s4wave/spacewave/db/block"
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
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/sirupsen/logrus"
)

// servePluginClient serves the volume a plugin of the scheduler receives for a
// load without an instance key and returns the plugin's RPC client for it.
func servePluginClient(
	ctx context.Context,
	t testing.TB,
	c *Controller,
	hostVol volume.Volume,
	pluginID string,
) srpc.Client {
	t.Helper()
	return serveInstanceClient(ctx, t, c, hostVol, "", pluginID)
}

// serveInstanceClient serves the volume a plugin of the scheduler receives for
// a load with loadKey, resolved as resolveLoadPlugin and newPluginInstance do,
// and returns the plugin's RPC client for it. Each call serves a new proxy
// volume, as each start of the plugin does.
func serveInstanceClient(
	ctx context.Context,
	t testing.TB,
	c *Controller,
	hostVol volume.Volume,
	loadKey, pluginID string,
) srpc.Client {
	t.Helper()

	// Serve the plugin's volume on a mux, as the plugin's mux does.
	view := newPluginVolume(hostVol, c.bindingKey(loadKey), pluginID)
	proxyVol := volume_rpc_server.NewProxyVolume(ctx, view, false)
	mux := srpc.NewMux()
	if err := volume_rpc_server.RegisterProxyVolume(mux, proxyVol); err != nil {
		t.Fatal(err.Error())
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
}

// servePluginVolume serves the volume a plugin of the scheduler receives and
// returns the plugin's client for it.
func servePluginVolume(
	ctx context.Context,
	t testing.TB,
	c *Controller,
	hostVol volume.Volume,
	pluginID string,
) *volume_rpc_client.ProxyVolume {
	t.Helper()
	return serveInstanceVolume(ctx, t, c, hostVol, "", pluginID)
}

// serveInstanceVolume serves the volume a plugin of the scheduler receives for
// a load with loadKey and returns the plugin's client for it.
func serveInstanceVolume(
	ctx context.Context,
	t testing.TB,
	c *Controller,
	hostVol volume.Volume,
	loadKey, pluginID string,
) *volume_rpc_client.ProxyVolume {
	// Build the plugin's client from the served volume's info.
	t.Helper()
	rpcClient := serveInstanceClient(ctx, t, c, hostVol, loadKey, pluginID)
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

	// A Space hosted in a plugin loads the same plugin on the app's scheduler
	// with the Space's key; it gets the same view, not the host volume.
	app := &Controller{conf: &Config{}}
	hosted := serveInstanceVolume(ctx, t, app, hostVol, "space-a", "plugin")
	if value, found := readKey(ctx, t, hosted, "world", "head"); !found || string(value) != "a" {
		t.Fatalf("hosted object store: %q, %v", value, found)
	}
	if _, found := readKey(ctx, t, hosted, "session-state", "token"); found {
		t.Fatal("hosted plugin read the app's session state")
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
	if got := newPluginVolume(tb.Volume, app.bindingKey(""), "plugin"); got != tb.Volume {
		t.Fatalf("app plugin volume is scoped: %T", got)
	}

	// The Space's plugins get a view that refuses block deletion.
	view := newPluginVolume(tb.Volume, space.bindingKey(""), "plugin")
	if _, ok := view.(*volume_scoped.Volume); !ok {
		t.Fatalf("Space plugin volume is not scoped: %T", view)
	}
	ref, _, err := view.PutBlock(ctx, []byte("data"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := view.RmBlock(ctx, ref); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("RmBlock through the Space plugin volume: %v", err)
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

// forgeApproval writes, through vol, an ALLOWED decision of peerID for another
// plugin's root into the store the device's approvals live in.
func forgeApproval(ctx context.Context, t *testing.T, vol volume.Volume, storeID, peerID, pluginID, root string) {
	t.Helper()
	store, release, err := vol.AccessObjectStore(ctx, storeID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	err = plugin_approval.PutApproval(ctx, store, peerID, &s4wave_plugin_approval.PluginApproval{
		PluginId:     pluginID,
		ManifestRoot: root,
		State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
}

// TestKeyedInstanceCannotForgeApproval checks a plugin loaded with a Space's
// key on the app's scheduler, as a Space hosted in a plugin loads it, cannot
// write or delete the device's approvals through its volume handle.
func TestKeyedInstanceCannotForgeApproval(t *testing.T) {
	// Build the host volume with the device's approvals on it.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	const storeID, peerID = "platform-account", "peer"
	approvals := plugin_approval.NewApprovals(tb.Bus, tb.Volume.GetID(), storeID)
	admits := func(pluginID, root string) bool {
		t.Helper()
		ok, err := approvals.Admits(ctx, peerID, pluginID, root)
		if err != nil {
			t.Fatal(err.Error())
		}
		return ok
	}
	if err := approvals.Set(ctx, peerID, &s4wave_plugin_approval.PluginApproval{
		PluginId:     "allowed",
		ManifestRoot: "root-allowed",
		State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	}); err != nil {
		t.Fatal(err.Error())
	}

	// The hosted instance writes an ALLOWED decision through its handle.
	app := &Controller{conf: &Config{}}
	hosted := serveInstanceVolume(ctx, t, app, tb.Volume, "space-a", "plugin")
	forgeApproval(ctx, t, hosted, storeID, peerID, "victim", "root-forged")
	if admits("victim", "root-forged") {
		t.Fatal("forged: a keyed instance wrote an approval the device admits")
	}
	t.Log("hosted instance on the app's scheduler: refused")

	// A second route: name the host store through an escaping ID, then delete
	// the store by its plain and escaping IDs. The device's decision survives.
	for _, id := range []string{"../../../" + storeID, "/" + storeID} {
		forgeApproval(ctx, t, hosted, id, peerID, "victim", "root-escaped")
	}
	if admits("victim", "root-escaped") {
		t.Fatal("forged: an escaping store ID reached the device's approvals")
	}
	for _, id := range []string{storeID, "../../../" + storeID} {
		if err := hosted.DeleteObjectStore(ctx, id); err != nil {
			t.Fatal(err.Error())
		}
	}
	if !admits("allowed", "root-allowed") {
		t.Fatal("a keyed instance deleted the device's approvals")
	}

	// The app's own plugins still reach the host volume whole.
	forgeApproval(ctx, t, servePluginVolume(ctx, t, app, tb.Volume, "plugin"), storeID, peerID, "victim", "root-app")
	if !admits("victim", "root-app") {
		t.Fatal("an unkeyed app plugin lost the host volume")
	}
}
