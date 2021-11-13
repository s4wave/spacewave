package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	hcli "github.com/aperturerobotics/hydra/cli"
	"github.com/aperturerobotics/hydra/daemon/prof"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/hydra/unixfs/fuse"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

type hDaemonArgs = hcli.DaemonArgs

var daemonFlags struct {
	hDaemonArgs
}

var fuseRoot = "./fuseroot"
var verbose bool
var dotOut string
var profListen string

func main() {
	app := cli.NewApp()
	app.Usage = "unixfs filesystem demo"

	dflags := (&daemonFlags.hDaemonArgs).BuildFlags()
	dflags = append(
		dflags,
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "enable verbose logging",
			Destination: &verbose,
		},
		&cli.StringFlag{
			Name:        "viz-dot-out",
			Usage:       "dot visualization output (if set) (e.x. demo.dot)",
			Destination: &dotOut,
			Value:       dotOut,
		},
		cli.StringFlag{
			Name:        "prof-listen",
			Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
			Destination: &profListen,
		},
	)
	app.Flags = dflags
	app.Action = func(c *cli.Context) error {
		ctx := context.Background()
		sctx, sctxStop := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
		defer sctxStop()

		return execute(sctx)
	}
	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
}

func execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	testbed.Verbose = verbose

	if profListen != "" {
		go prof.ListenProf(le, profListen)
	}

	volConfig := daemonFlags.hDaemonArgs.BuildSingleVolume()
	tb, err := testbed.NewTestbed(
		ctx,
		le,
		testbed.WithVolumeConfig(volConfig),
	)
	if err != nil {
		return err
	}
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		return err
	}

	vol := tb.Volume
	engineID := wtb.EngineID
	var sender peer.Peer = vol

	// provide op handlers to bus
	opc := world.NewLookupOpController("test-fs-ops", engineID, unixfs_world.LookupFsOp)
	go tb.Bus.ExecuteController(ctx, opc)
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// initialize filesystem if it doesn't exist

	// NOTE: BusEngine looks up the engine on the bus for every call (slow)
	// use a wrapper around the Engine directly to avoid this slowdown:
	// ws := wtb.WorldState
	eng := wtb.Engine
	ws := world.NewEngineWorldState(ctx, eng, true)

	objKey := "test-filesystem"
	_, exists, err := ws.GetObject(objKey)
	if err != nil {
		return err
	}
	if !exists {
		_, _, err = ws.ApplyWorldOp(
			unixfs_world.NewFsInitOp(objKey, unixfs_world.FSType_FSType_FS_NODE, nil, 0, true, time.Now()),
			sender.GetPeerID(),
		)
		if err != nil {
			return err
		}
	}

	// TODO: access and add some test data
	testFilename := "test-file.txt"
	_, _, err = world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		ftree, err := unixfs_block.NewFSTree(bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			return err
		}
		fnode, _, err := ftree.LookupFollowDirent(testFilename)
		if err != nil {
			return err
		}
		if fnode == nil {
			now := timestamp.Now()
			fnode, err = ftree.Mknod(testFilename, unixfs_block.NodeType_NodeType_FILE, nil, 0, &now)
			if err != nil {
				return err
			}
		}
		fh, err := fnode.BuildFileHandle(ctx)
		if err != nil {
			return err
		}
		fw := file.NewWriter(fh, nil, nil)
		return fw.WriteBytes(0, []byte("Hello world from FUSE!\n"))
	})
	if err != nil {
		return err
	}

	// start the filesystem
	watchChanges := true
	fsType := unixfs_world.FSType_FSType_FS_NODE
	writer := unixfs_world.NewFSWriter(ws, objKey, fsType, sender.GetPeerID())
	rootFSCursor := unixfs_world.NewFSCursor(le, eng, objKey, fsType, writer, watchChanges)
	ufs := unixfs.NewFS(ctx, le, rootFSCursor, nil)

	le.Debug("mounting rootfs fuse")
	rootFS, err := fuse.Mount(ctx, le, fuseRoot, ufs, verbose)
	if err != nil {
		return errors.Wrap(err, "build rootfs fuse")
	}

	go func() {
		err := rootFS.Serve()
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				le.WithError(err).Warn("server exited with error")
			}
		}
		ctxCancel()
	}()

	le.Info("startup complete")
	<-ctx.Done()

	le.Info("shutting down")
	rootFS.Close()
	_ = fuse.Unmount(fuseRoot)
	return nil
}
