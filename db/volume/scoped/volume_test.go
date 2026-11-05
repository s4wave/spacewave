package volume_scoped_test

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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
	ref, _, err := view.PutBlock(ctx, []byte("data"), &block.PutOpts{RefsKnown: true})
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
	_, err = view.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	if !errors.Is(err, volume_scoped.ErrRefused) {
		t.Fatalf("tombstone batch: %v", err)
	}
	if exists, err := tb.Volume.GetBlockExists(ctx, ref); err != nil || !exists {
		t.Fatalf("block deleted through the view: %v, %v", exists, err)
	}
}

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
