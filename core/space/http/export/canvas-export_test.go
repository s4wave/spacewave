package space_http_export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	space_unixfs "github.com/s4wave/spacewave/core/space/unixfs"
	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	"github.com/sirupsen/logrus"
)

func TestExportSpaceWithCanvasWorldObjectNode(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	unixfsOps := world.NewLookupOpController("test-canvas-export-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOps, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	const imageName = "Screenshot 2026-05-12 at 9.28.36 PM.png"
	if _, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		true,
		time.Time{},
	); err != nil {
		t.Fatal(err)
	}

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	cursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		tx,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		sender,
	)
	files, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	defer files.Release()
	if err := files.Mknod(ctx, true, []string{imageName}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	image, _, err := files.LookupPath(ctx, imageName)
	if err != nil {
		t.Fatal(err)
	}
	if err := image.WriteAt(ctx, 0, []byte("png fixture"), time.Time{}); err != nil {
		image.Release()
		t.Fatal(err)
	}
	image.Release()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	canvas := &s4wave_canvas.CanvasState{Nodes: map[string]*s4wave_canvas.CanvasNode{
		"unixfs-demo": {
			Id:        "unixfs-demo",
			X:         100,
			Y:         100,
			Width:     996.64,
			Height:    677.72,
			ZIndex:    0,
			Type:      s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT,
			ObjectKey: "files",
			Pinned:    true,
			ViewPath:  "/" + imageName,
		},
	}}
	if _, _, err := world.CreateWorldObject(ctx, ws, "canvas", func(bcs *block.Cursor) error {
		bcs.SetBlock(canvas, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, "canvas", s4wave_canvas_world.CanvasTypeID); err != nil {
		t.Fatal(err)
	}

	root, err := space_unixfs.BuildFSHandle(le, world.NewEngineWorldState(wtb.Engine, false), 19, "space-canvas-export")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Release()
	space, _, err := root.LookupPath(ctx, "u/19/so/space-canvas-export/-")
	if err != nil {
		t.Fatal(err)
	}
	defer space.Release()

	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, space); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var foundImage bool
	for _, file := range zr.File {
		if file.Name != "files/-/"+imageName {
			continue
		}
		foundImage = true
		rd, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rd)
		if closeErr := rd.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "png fixture" {
			t.Fatalf("exported image = %q", data)
		}
	}
	if !foundImage {
		t.Fatalf("missing exported image in %#v", zr.File)
	}

	canvasObject, found, err := ws.GetObject(ctx, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("canvas object disappeared after export")
	}
	defer world.ReleaseObjectState(canvasObject)
	var persistedState *s4wave_canvas.CanvasState
	if _, _, err := world.AccessObjectState(ctx, canvasObject, false, func(bcs *block.Cursor) error {
		persistedState, err = s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !canvas.EqualVT(persistedState) {
		t.Fatalf("canvas state changed during export: %#v", persistedState)
	}
}
