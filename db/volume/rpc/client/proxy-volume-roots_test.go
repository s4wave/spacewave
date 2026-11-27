package volume_rpc_client

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	rpc_bucket "github.com/s4wave/spacewave/db/bucket/store/rpc"
	rpc_object "github.com/s4wave/spacewave/db/object/rpc"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"

	rpc_block "github.com/s4wave/spacewave/db/block/rpc"
)

// TestProxyVolumeHoldsRoots checks that a proxy volume holds named roots on
// the volume it serves: a root set through the proxy reads back through it
// and from the host, and a move from a stale Previous is refused as moved,
// as a World root's compare and swap needs.
func TestProxyVolumeHoldsRoots(t *testing.T) {
	// Serve the testbed volume and connect a proxy to it.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	mux := srpc.NewMux()
	if err := volume_rpc_server.RegisterProxyVolume(mux, volume_rpc_server.NewProxyVolume(ctx, tb.Volume, false)); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	info, err := volume_rpc.NewSRPCProxyVolumeClient(client).GetVolumeInfo(ctx, &volume_rpc.GetVolumeInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxyVolume(info.GetVolumeInfo(),
		volume_rpc.NewSRPCProxyVolumeClient(client),
		rpc_block.NewSRPCBlockStoreClient(client),
		rpc_bucket.NewSRPCBucketStoreClient(client),
		rpc_object.NewSRPCObjectStoreClient(client))
	if err != nil {
		t.Fatal(err)
	}

	// Hold a block through the proxy and read it back on both sides.
	data := []byte("proxy root")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}
	if err := proxy.SetBucketRoots(ctx, "world-bucket", entries, []block.NamedRoot{{Name: "world:proxy", Ref: ref}}); err != nil {
		t.Fatal(err)
	}
	if got, err := proxy.GetBucketRoot(ctx, "world-bucket", "world:proxy"); err != nil || !got.EqualsRef(ref) {
		t.Fatalf("proxy root = %v, %v", got, err)
	}
	var listed int
	if err := proxy.ListBucketRoots(ctx, "world-bucket", "world:", func(root block.NamedRoot) error {
		listed++
		if root.Name != "world:proxy" || !root.Ref.EqualsRef(ref) {
			t.Fatalf("listed root %+v", root)
		}
		return nil
	}); err != nil || listed != 1 {
		t.Fatalf("listed %d roots: %v", listed, err)
	}

	// A move from a stale Previous is refused as moved.
	err = proxy.SetBucketRoots(ctx, "world-bucket", nil, []block.NamedRoot{{Name: "world:proxy", Previous: nil}})
	if !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("stale move = %v, want ErrRootMoved", err)
	}
}

// TestProxyVolumeHoldsStagesAndPins checks that a proxy carries the holds a
// World needs from the volume it serves, on a host that frees at once: a block
// written under a stage survives collection until the stage is released, a
// pinned root's block survives its root's release until the pin's release,
// and a stage the proxy did not open is refused.
func TestProxyVolumeHoldsStagesAndPins(t *testing.T) {
	// Serve a host volume that frees at once and connect a proxy to it.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.WithVolumeConfig(&volume_s4db.Config{
		Path:         filepath.Join(t.TempDir(), "host.s4wave"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	host := tb.Volume.(*volume_kvtx.Volume)
	proxy := newTestProxy(t, tb.Volume)
	if !proxy.HoldsRoots() {
		t.Fatal("proxy does not report that the host holds roots")
	}
	collect := func() {
		t.Helper()
		for range 16 {
			res, err := host.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				return
			}
		}
	}
	stored := func(ref *block.BlockRef) bool {
		t.Helper()
		found, err := host.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	// A block written under a proxy stage survives collection.
	stage, release, err := proxy.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("staged through the proxy")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proxy.PrepareStagedBlockBatch(ctx, stage, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	collect()
	if !stored(ref) {
		t.Fatal("collection freed a block a proxy stage holds")
	}

	// A stage the proxy did not open is refused.
	if _, err := proxy.PrepareStagedBlockBatch(ctx, stage+"-forged", nil); err == nil {
		t.Fatal("a forged stage name was accepted")
	}

	// A pinned root keeps its block after the stage lets go.
	unpin, err := proxy.PinBucketRoot(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	release()
	collect()
	if !stored(ref) {
		t.Fatal("collection freed a block a proxy pin holds")
	}

	// Without the pin the block goes.
	unpin()
	for range 8 {
		collect()
		if !stored(ref) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a released block survived collection")
}

// TestProxyVolumeRequiresHeld checks that a root move requiring its blocks
// held crosses the proxy: the host refuses a block nothing holds and the
// client reads the refusal as ErrRootNotHeld, and a stage opened through the
// proxy is the evidence that lets the move commit over a tree whose earlier
// release netted and freed it.
func TestProxyVolumeRequiresHeld(t *testing.T) {
	// Serve a host volume that frees at once, so the check refuses rather
	// than observes, and a released tree is freed.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.WithVolumeConfig(&volume_s4db.Config{
		Path:         filepath.Join(t.TempDir(), "host.s4wave"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	host := tb.Volume.(*volume_kvtx.Volume)
	proxy := newTestProxy(t, tb.Volume)

	// Build the tree H -> P -> L, and an admit storing only H.
	entry := func(data string, children ...*block.PutBatchEntry) *block.PutBatchEntry {
		t.Helper()
		e := &block.PutBatchEntry{Data: []byte(data), RefsKnown: true}
		for _, child := range children {
			e.Refs = append(e.Refs, child.Ref)
			e.Data = append(e.Data, child.Ref.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(e.Data, nil)
		if err != nil {
			t.Fatal(err)
		}
		e.Ref = ref
		return e
	}
	l := entry("leaf")
	p := entry("payload", l)
	h := entry("head", p)
	admit := func() error {
		roots := []block.NamedRoot{{Name: "payloads", Ref: h.Ref, RequireHeld: true}}
		return proxy.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{h}, roots)
	}
	stagePut := func(vol volume.Roots) func() {
		t.Helper()
		stage, release, err := vol.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := vol.PrepareStagedBlockBatch(ctx, stage, []*block.PutBatchEntry{l, p}); err != nil {
			t.Fatal(err)
		}
		return release
	}

	// The host has no P: refused.
	if err := admit(); !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("admit over a missing block = %v, want ErrRootNotHeld", err)
	}

	// A host stage stores P -> L and is released. A later write carries
	// the release and collection frees the tree: refused.
	stagePut(host)()
	other := entry("other")
	if err := host.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{other}, []block.NamedRoot{{Name: "other", Ref: other.Ref}}); err != nil {
		t.Fatal(err)
	}
	for range 16 {
		res, err := host.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !res.More {
			break
		}
	}
	if err := admit(); !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("admit over a released tree = %v, want ErrRootNotHeld", err)
	}

	// A live proxy stage that stores the tree again lets the admit commit.
	defer stagePut(proxy)()
	if err := admit(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*block.PutBatchEntry{h, p, l} {
		if found, err := host.GetBlockExists(ctx, e.Ref); err != nil || !found {
			t.Fatalf("block %s stored=%v: %v", e.Data, found, err)
		}
	}
}

// newTestProxy serves vol and returns a proxy volume connected to it.
func newTestProxy(t *testing.T, vol volume.Volume) *ProxyVolume {
	t.Helper()
	ctx := t.Context()
	mux := srpc.NewMux()
	if err := volume_rpc_server.RegisterProxyVolume(mux, volume_rpc_server.NewProxyVolume(ctx, vol, false)); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	info, err := volume_rpc.NewSRPCProxyVolumeClient(client).GetVolumeInfo(ctx, &volume_rpc.GetVolumeInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxyVolume(info.GetVolumeInfo(),
		volume_rpc.NewSRPCProxyVolumeClient(client),
		rpc_block.NewSRPCBlockStoreClient(client),
		rpc_bucket.NewSRPCBucketStoreClient(client),
		rpc_object.NewSRPCObjectStoreClient(client))
	if err != nil {
		t.Fatal(err)
	}
	return proxy
}
