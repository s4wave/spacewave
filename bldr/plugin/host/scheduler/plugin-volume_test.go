package plugin_host_scheduler

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
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

// loadSpy stands in for the app's scheduler: it reports the binding key each
// LoadPlugin directive on the host bus would run under.
type loadSpy struct {
	sched *Controller
	loads chan string
}

func (s *loadSpy) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/load-spy", controller.MustParseVersion("0.0.1"), "")
}

func (s *loadSpy) Execute(context.Context) error {
	return nil
}

func (s *loadSpy) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		select {
		case s.loads <- s.bindingKey(d):
		case <-ctx.Done():
		}
		<-ctx.Done()
		return nil
	}), nil)
}

// bindingKey reports the binding key the scheduler picks for d, or "refused".
func (s *loadSpy) bindingKey(d bldr_plugin.LoadPlugin) string {
	key, err := s.sched.loadBindingKey(d)
	if err != nil {
		return "refused"
	}
	return key
}

func (s *loadSpy) Close() error {
	return nil
}

// loadThroughHost asks host to load pluginID at instanceKey and returns the
// binding key the load reached the scheduler with, or false if refused.
func loadThroughHost(ctx context.Context, host bldr_plugin.SRPCPluginHostClient, loads <-chan string, pluginID, instanceKey string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	strm, err := host.LoadPlugin(ctx, &bldr_plugin.LoadPluginRequest{PluginId: pluginID, InstanceKey: instanceKey})
	if err != nil {
		return "", false
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := strm.Recv()
		errCh <- err
	}()
	select {
	case key := <-loads:
		return key, true
	case <-errCh:
		return "", false
	case <-ctx.Done():
		return "", false
	}
}

// TestKeyedCallerLoadStaysInScope checks a plugin running on a Space's key on
// the app's scheduler cannot load an instance, of itself or another plugin,
// on the unscoped view or another Space's view through the host's LoadPlugin.
func TestKeyedCallerLoadStaysInScope(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	app := &Controller{conf: &Config{HostExportPluginIds: []string{"spacewave-core"}}}
	spy := &loadSpy{sched: app, loads: make(chan string, 1)}
	rel, err := tb.Bus.AddController(ctx, spy, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Serve the host to the attacker as execute-plugin does for its instance.
	srv := bldr_plugin_host.NewPluginHostServer(ctx, tb.Bus, le, "attacker", app.conf.GetInstanceKey(), nil, nil, "", false)
	srv.SetRegistrationInstanceKey(app.bindingKey("space-a"))
	srv.SetHostExportPluginIDs([]string{"spacewave-core"})
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	host := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// want is the binding key the load must reach the scheduler with; empty
	// with refused set means the host refuses it.
	for _, tc := range []struct {
		name, pluginID, key, want string
		refused                   bool
	}{
		{"empty key, itself", "attacker", "", "space-a", false},
		{"empty key, another plugin", "other", "", "space-a", false},
		{"another key, itself", "attacker", "space-b", "", true},
		{"another key, another plugin", "other", "space-b", "", true},
		{"own key", "attacker", "space-a", "space-a", false},
		{"empty key, host-export core", "spacewave-core", "", "", false},
		{"own key, host-export core", "spacewave-core", "space-a", "", false},
	} {
		key, ran := loadThroughHost(ctx, host, spy.loads, tc.pluginID, tc.key)
		if !ran {
			t.Logf("%s: refused", tc.name)
		} else {
			t.Logf("%s: bound to %q", tc.name, key)
		}
		if ran == tc.refused || key != tc.want {
			t.Fatalf("%s: ran=%v bound to %q, want refused=%v bound to %q", tc.name, ran, key, tc.refused, tc.want)
		}
	}
}

// TestHostPluginLoadsBindToSchedulerInstance checks every load of a host
// plugin, whatever key a Space bridge carries, runs the scheduler's one
// instance on the host volume, while a Space plugin keeps its own instance and
// scoped view, and that a Space key cannot pick a host plugin's build.
func TestHostPluginLoadsBindToSchedulerInstance(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	app := &Controller{
		le:              le,
		conf:            &Config{HostExportPluginIds: []string{"spacewave-core"}},
		pluginStatusCtr: ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
	}
	app.pluginInstances = keyed.NewKeyedRefCountWithLogger(app.newPluginInstance, le)

	// Resolve each load as the parent bus does and keep it until the end.
	for _, dir := range []bldr_plugin.LoadPlugin{
		bldr_plugin.NewLoadPlugin("spacewave-core"),
		bldr_plugin.NewLoadPluginInstanced("spacewave-core", "space-a"),
		bldr_plugin.NewLoadPluginInstanced("spacewave-core", "space-b"),
		bldr_plugin.NewLoadPluginInstanced("space-plugin", "space-a"),
	} {
		res, err := app.resolveLoadPlugin(dir)
		if err != nil {
			t.Fatal(err.Error())
		}
		go func() { _ = res.Resolve(ctx, newLoadPluginValuesHandler()) }()
	}

	// One core instance, on the host volume, and the Space plugin's own.
	deadline := time.Now().Add(2 * time.Second)
	for len(app.pluginInstances.GetKeys()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := make(map[string]string)
	for _, ref := range app.pluginInstances.GetKeys() {
		got[ref.pluginID] = ref.instanceKey
		t.Logf("instance %s at %q", ref.pluginID, ref.instanceKey)
	}
	if len(got) != 2 || len(app.pluginInstances.GetKeys()) != 2 {
		t.Fatalf("instances %v, want one core and one Space plugin", app.pluginInstances.GetKeys())
	}
	if key, ok := got["spacewave-core"]; !ok || newPluginVolume(tb.Volume, key, "spacewave-core") != tb.Volume {
		t.Fatalf("core runs at %q, want the scheduler's instance on the host volume", key)
	}
	if key := got["space-plugin"]; key != "space-a" || newPluginVolume(tb.Volume, key, "space-plugin") == tb.Volume {
		t.Fatalf("Space plugin runs at %q, want space-a on a scoped view", key)
	}

	// A Space key cannot select core's build; the scheduler's own key can.
	root := "manifest-root"
	ref := &bldr_manifest.ManifestRef{}
	for _, tc := range []struct {
		name    string
		dir     bldr_plugin.LoadPlugin
		refused bool
	}{
		{"Space key, manifest root", bldr_plugin.NewLoadPluginAtManifest("spacewave-core", "space-a", root), true},
		{"Space key, manifests", bldr_plugin.NewLoadPluginWithManifests("spacewave-core", "space-a", "", ref), true},
		{"own key, manifest root", bldr_plugin.NewLoadPluginAtManifest("spacewave-core", "", root), false},
		{"Space plugin, manifests", bldr_plugin.NewLoadPluginWithManifests("space-plugin", "space-a", "", ref), false},
	} {
		_, err := app.resolveLoadPlugin(tc.dir)
		t.Logf("%s: %v", tc.name, errString(err))
		if (err != nil) != tc.refused {
			t.Errorf("%s: err %v, want refused=%v", tc.name, err, tc.refused)
		}
	}
}
