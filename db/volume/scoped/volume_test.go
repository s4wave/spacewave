package volume_scoped_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	"github.com/s4wave/spacewave/db/testbed"
	volume_scoped "github.com/s4wave/spacewave/db/volume/scoped"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// viewPrefix is the prefix of the views under test.
const viewPrefix = "space-a/plugin-a/"

// TestRefusesPeerKeyAndLifetime checks the view never reaches the host
// volume's private key or ends its lifetime.
func TestRefusesPeerKeyAndLifetime(t *testing.T) {
	// Build a view over the testbed volume.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	view := volume_scoped.NewVolume(tb.Volume, viewPrefix)

	// Refuse every path to the private key.
	if _, err := view.GetPeer(ctx, true); !errors.Is(err, peer.ErrNoPrivKey) {
		t.Fatalf("GetPeer with private key: %v", err)
	}
	if _, err := view.LoadPeerPriv(ctx); !errors.Is(err, peer.ErrNoPrivKey) {
		t.Fatalf("LoadPeerPriv: %v", err)
	}
	if err := view.StorePeerPriv(ctx, nil); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("StorePeerPriv: %v", err)
	}
	if _, err := view.GetPeer(ctx, false); err != nil {
		t.Fatalf("GetPeer without private key: %v", err)
	}

	// Refuse to delete the host volume and leave it open on close.
	if err := view.Delete(); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("Delete: %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tb.Volume.Sync(ctx); err != nil {
		t.Fatalf("host volume closed with the view: %v", err)
	}
}

// TestBucketsStayInView checks the view lists, reads and applies only the
// buckets under its prefix.
func TestBucketsStayInView(t *testing.T) {
	// Seed the host volume with a bucket outside the view and apply one in it.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{Id: "session", Rev: 1}); err != nil {
		t.Fatal(err.Error())
	}
	view := volume_scoped.NewVolume(tb.Volume, viewPrefix)
	if _, _, curr, err := view.ApplyBucketConfig(ctx, &bucket.Config{Id: "own", Rev: 1}); err != nil || curr.GetId() != "own" {
		t.Fatalf("apply own bucket: %v, %v", curr, err)
	}

	// The host holds the bucket under the prefix.
	hostConf, err := tb.Volume.GetBucketConfig(ctx, viewPrefix+"own")
	if err != nil || hostConf == nil {
		t.Fatalf("host bucket config: %v, %v", hostConf, err)
	}

	// The view lists its own bucket and cannot read the host's.
	infos, err := view.ListBucketInfo(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(infos) != 1 || infos[0].GetConfig().GetId() != "own" {
		t.Fatalf("listed buckets: %v", infos)
	}
	if conf, err := view.GetBucketConfig(ctx, "session"); err != nil || conf != nil {
		t.Fatalf("host bucket reached through the view: %v, %v", conf, err)
	}
}

// TestBlockStoreRefusesDeletion checks the view reads and writes blocks and
// refuses to delete them.
func TestBlockStoreRefusesDeletion(t *testing.T) {
	// Write a block through the view and read it back.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	view := volume_scoped.NewVolume(tb.Volume, viewPrefix)
	ref, _, err := view.PutBlock(ctx, []byte("data"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if data, found, err := view.GetBlock(ctx, ref); err != nil || !found || string(data) != "data" {
		t.Fatalf("read block: %q, %v, %v", data, found, err)
	}

	// Refuse deletion by removal and by tombstone, leaving the block in place.
	if err := view.RmBlock(ctx, ref); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("RmBlock: %v", err)
	}
	err = view.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	if !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("tombstone batch: %v", err)
	}
	if exists, err := tb.Volume.GetBlockExists(ctx, ref); err != nil || !exists {
		t.Fatalf("block deleted through the view: %v, %v", exists, err)
	}
}

// TestRefGraphHidesOtherOwners checks the view names its own owners, hides
// the owners of other views and refuses edits it does not own.
func TestRefGraphHidesOtherOwners(t *testing.T) {
	// Build a view and root one block under it and under another owner.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	view := volume_scoped.NewVolume(tb.Volume, viewPrefix)
	ref, _, err := view.PutBlock(ctx, []byte("rooted"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	blockIRI := block_gc.BlockIRI(ref)

	// Root the block under the view's owner and under another view's owner.
	rg := view.GetRefGraph()
	if err := rg.AddObjectRoot(ctx, "key", ref); err != nil {
		t.Fatal(err.Error())
	}
	if err := tb.Volume.GetRefGraph().AddRef(ctx, block_gc.ObjectIRI("space-b/plugin-b/key"), blockIRI); err != nil {
		t.Fatal(err.Error())
	}

	// The host holds the owner under the prefix.
	hostOwners, err := tb.Volume.GetRefGraph().GetIncomingRefs(ctx, blockIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(hostOwners, block_gc.ObjectIRI(viewPrefix+"key")) {
		t.Fatalf("host owners lack the view's root: %v", hostOwners)
	}

	// The view names only its own owner without the prefix.
	owners, err := rg.GetIncomingRefs(ctx, blockIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(owners, []string{block_gc.ObjectIRI("key")}) {
		t.Fatalf("view owners: %v", owners)
	}
	if targets, err := rg.GetOutgoingRefs(ctx, block_gc.ObjectIRI("key")); err != nil || !slices.Equal(targets, []string{blockIRI}) {
		t.Fatalf("view targets: %v, %v", targets, err)
	}

	// Refuse the permanent roots, block removals and the global unreferenced list.
	if err := rg.AddRef(ctx, block_gc.NodeGCRoot, blockIRI); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("gcroot subject: %v", err)
	}
	if err := rg.RemoveRef(ctx, blockIRI, blockIRI); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("block subject removal: %v", err)
	}
	if _, err := rg.GetUnreferencedNodes(ctx); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("unreferenced nodes: %v", err)
	}

	// Refuse a batch whose later edge is outside the view, committing none of it.
	err = rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.ObjectIRI("second"), Object: blockIRI},
		{Subject: block_gc.NodeGCRoot, Object: blockIRI},
	}, nil)
	if !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("batch with a permanent root: %v", err)
	}
	if targets, err := rg.GetOutgoingRefs(ctx, block_gc.ObjectIRI("second")); err != nil || len(targets) != 0 {
		t.Fatalf("refused batch committed: %v, %v", targets, err)
	}
}

// TestRefGraphTracksOwnBuckets checks the view roots its own buckets and marks
// blocks unreferenced, the edges GC tracking of a bucket writes, and cannot
// root anything else.
func TestRefGraphTracksOwnBuckets(t *testing.T) {
	// Build a view and a block it stores.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	view := volume_scoped.NewVolume(tb.Volume, viewPrefix)
	ref, _, err := view.PutBlock(ctx, []byte("tracked"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	blockIRI := block_gc.BlockIRI(ref)

	// Root a bucket of the view, own the block, and stage it, as GC tracking does.
	rg := view.GetRefGraph()
	bucketIRI := block_gc.BucketIRI("own")
	err = rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: bucketIRI},
		{Subject: bucketIRI, Object: blockIRI},
		{Subject: block_gc.NodeUnreferenced, Object: blockIRI},
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The host holds the bucket under the prefix, rooted.
	hostGraph := tb.Volume.GetRefGraph()
	hostBucketIRI := block_gc.BucketIRI(viewPrefix + "own")
	if roots, err := hostGraph.GetOutgoingRefs(ctx, block_gc.NodeGCRoot); err != nil || !slices.Contains(roots, hostBucketIRI) {
		t.Fatalf("host roots lack the view's bucket: %v, %v", roots, err)
	}
	if owned, err := hostGraph.GetOutgoingRefs(ctx, hostBucketIRI); err != nil || !slices.Equal(owned, []string{blockIRI}) {
		t.Fatalf("host bucket edges: %v, %v", owned, err)
	}

	// Unstage the block.
	err = rg.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: blockIRI}})
	if err != nil {
		t.Fatal(err.Error())
	}
	if staged, err := hostGraph.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced); err != nil || slices.Contains(staged, blockIRI) {
		t.Fatalf("host staging after the removal: %v, %v", staged, err)
	}

	// Refuse a root that is not a bucket, an unreferenced mark of an owner, and
	// the removal of a root.
	if err := rg.AddRef(ctx, block_gc.NodeGCRoot, block_gc.ObjectIRI("key")); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("gcroot edge to an object: %v", err)
	}
	if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, bucketIRI); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("unreferenced mark of a bucket: %v", err)
	}
	if err := rg.RemoveRef(ctx, block_gc.NodeGCRoot, bucketIRI); !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("gcroot edge removal: %v", err)
	}
}

// TestCoordinatorConformance runs the coordinator contract over two views of
// one prefix.
func TestCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(t testing.TB) (coord.Coordinator, coord.Coordinator) {
		tb, err := testbed.NewTestbed(t.Context(), logrus.NewEntry(logrus.New()))
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Cleanup(tb.Release)
		return volume_scoped.NewVolume(tb.Volume, viewPrefix), volume_scoped.NewVolume(tb.Volume, viewPrefix)
	})
}
