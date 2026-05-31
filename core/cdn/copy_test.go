package cdn

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
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

	content := bytes.Repeat([]byte("copied rootfs payload\n"), 64*1024)
	createFSNodeObjectWithFile(t, ctx, srcTB.WorldState, assetKey, "hello.txt", content)
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

	got := readFSNodeFile(t, ctx, dstTB.WorldState, assetKey, "hello.txt")
	if !bytes.Equal(got, content) {
		t.Fatalf("copied asset file content mismatch: got %q want %q", string(got), string(content))
	}
}

func createFSNodeObjectWithFile(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	name string,
	content []byte,
) {
	t.Helper()
	op := unixfs_world.NewFsInitOp(objKey, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Unix(1, 0))
	if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("create fs-node object %q: %v", objKey, err)
	}
	obj, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatalf("get fs-node object %q: %v", objKey, err)
	}
	if !found {
		t.Fatalf("expected fs-node object %q to exist", objKey)
	}
	if _, _, err := unixfs_world.FsMknodWithContent(
		ctx,
		obj,
		"",
		unixfs_world.FSType_FSType_FS_NODE,
		[]string{name},
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Unix(2, 0),
	); err != nil {
		t.Fatalf("create fs-node file %q: %v", name, err)
	}
}

func readFSNodeFile(t *testing.T, ctx context.Context, ws world.WorldState, objKey, name string) []byte {
	t.Helper()
	fsc := unixfs_world.NewFSCursor(logrus.NewEntry(logrus.New()), ws, objKey, unixfs_world.FSType_FSType_FS_NODE, nil, false)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		fsc.Release()
		t.Fatalf("open fs-node handle %q: %v", objKey, err)
	}
	defer fsh.Release()

	child, err := fsh.Lookup(ctx, name)
	if err != nil {
		t.Fatalf("lookup copied file %q: %v (%s)", name, err, describeFSNodeObject(t, ctx, ws, objKey))
	}
	defer child.Release()

	got, err := unixfs.ReadFile(ctx, child)
	if err != nil {
		t.Fatalf("read copied file %q: %v", name, err)
	}
	return got
}

func describeFSNodeObject(t *testing.T, ctx context.Context, ws world.WorldState, objKey string) string {
	t.Helper()
	obj, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return fmt.Sprintf("get object: %v", err)
	}
	if !found {
		return "object not found"
	}
	var out string
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		store, _ := bcs.GetBlockStore()
		rootRef := bcs.GetRef()
		rootExists := false
		if store != nil && rootRef != nil && !rootRef.GetEmpty() {
			rootExists, _ = store.GetBlockExists(ctx, rootRef)
		}
		root, err := unixfs_block.UnmarshalFSNode(ctx, bcs)
		if err != nil {
			out = fmt.Sprintf("root-ref=%s root-exists=%v root-unmarshal=%v", rootRef.MarshalString(), rootExists, err)
			return nil
		}
		out = fmt.Sprintf("root-ref=%s root-exists=%v dirents=%d", rootRef.MarshalString(), rootExists, len(root.GetDirectoryEntry()))
		for _, dirent := range root.GetDirectoryEntry() {
			childRef := dirent.GetNodeRef()
			childExists := false
			if store != nil && childRef != nil && !childRef.GetEmpty() {
				childExists, _ = store.GetBlockExists(ctx, childRef)
			}
			out += fmt.Sprintf(" %s=%s exists=%v", dirent.GetName(), childRef.MarshalString(), childExists)
		}
		return nil
	})
	if err != nil {
		return fmt.Sprintf("access object: %v", err)
	}
	return out
}
