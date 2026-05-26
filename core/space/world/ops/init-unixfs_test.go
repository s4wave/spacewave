package space_world_ops

import (
	"bytes"
	"context"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestInitUnixFSBatchStarterTree(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer btb.Release()

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	opc := world.NewLookupOpController("test-space-ops", wtb.EngineID, LookupWorldOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	<-time.After(100 * time.Millisecond)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	if _, _, err := InitUnixFS(ctx, ws, wtb.Volume.GetPeerID(), "drive/fs", time.Now()); err != nil {
		t.Fatal(err)
	}

	cursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: "drive/fs"},
		wtb.Volume.GetPeerID(),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()

	handle, err := unixfs_sdk.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	if _, err := bfs.Stat("test"); err == nil {
		t.Fatal("test should not exist")
	}
	if _, err := billy_util.ReadFile(bfs, "hello.txt"); err == nil {
		t.Fatal("hello.txt should not exist")
	}
	if _, err := billy_util.ReadFile(bfs, "world.md"); err == nil {
		t.Fatal("world.md should not exist")
	}
	data, err := billy_util.ReadFile(bfs, "getting-started.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("getting-started.md should not be empty")
	}
	if !bytes.Contains(data, []byte("single guide")) {
		t.Fatalf("getting-started.md missing updated starter text: %q", string(data))
	}
}
