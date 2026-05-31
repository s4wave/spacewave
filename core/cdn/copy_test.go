package cdn

import (
	"context"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

func TestCopyV86ImageFromCdnCopiesAssetObjectsBeforeEdges(t *testing.T) {
	ctx := context.Background()
	srcTB, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer srcTB.Release()
	dstTB, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer dstTB.Release()

	const (
		srcImageKey = "v86image/default"
		dstImageKey = "vm-image/default-copy"
		assetKey    = "assets/rootfs"
	)

	createFSNodeObject(t, ctx, srcTB.WorldState, assetKey)
	createdAt := time.Unix(123, 0)
	img := &s4wave_vm.V86Image{
		Name:          "Aperture Linux",
		Platform:      "v86",
		KernelVersion: "7.0.0-rc5",
		CreatedAt:     timestamppb.New(createdAt),
	}
	if _, _, err := srcTB.WorldState.ApplyWorldOp(ctx, s4wave_vm.NewCreateV86ImageOp(srcImageKey, img, createdAt), ""); err != nil {
		t.Fatalf("create source v86 image: %v", err)
	}
	if err := srcTB.WorldState.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(srcImageKey, string(s4wave_vm.PredV86ImageRootfs), assetKey, "")); err != nil {
		t.Fatalf("set source rootfs edge: %v", err)
	}

	if err := CopyV86ImageFromCdn(ctx, srcTB.WorldState, dstTB.WorldState, srcImageKey, dstImageKey); err != nil {
		t.Fatalf("copy v86 image: %v", err)
	}

	if _, found, err := dstTB.WorldState.GetObject(ctx, assetKey); err != nil {
		t.Fatalf("get copied asset: %v", err)
	} else if !found {
		t.Fatalf("expected destination asset object %q to exist", assetKey)
	}
	ft, explicit, err := unixfs_world.LookupFsType(ctx, dstTB.WorldState, assetKey)
	if err != nil {
		t.Fatalf("lookup copied asset type: %v", err)
	}
	if !explicit || ft != unixfs_world.FSType_FSType_FS_NODE {
		t.Fatalf("expected explicit copied FS_NODE type, got type=%s explicit=%v", ft.String(), explicit)
	}
	target, err := lookupV86ImageEdge(ctx, dstTB.WorldState, dstImageKey, string(s4wave_vm.PredV86ImageRootfs))
	if err != nil {
		t.Fatalf("lookup copied rootfs edge: %v", err)
	}
	if target != assetKey {
		t.Fatalf("expected copied rootfs edge target %q, got %q", assetKey, target)
	}
}

func createFSNodeObject(t *testing.T, ctx context.Context, ws world.WorldState, objKey string) {
	t.Helper()
	op := unixfs_world.NewFsInitOp(objKey, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Unix(1, 0))
	if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("create fs-node object %q: %v", objKey, err)
	}
}
